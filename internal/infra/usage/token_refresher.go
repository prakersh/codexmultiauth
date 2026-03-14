package usage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/prakersh/codexmultiauth/internal/infra/store"
)

const (
	defaultOAuthURL     = "https://auth.openai.com/oauth/token"
	codexOAuthClientID  = "app_EMoamEEZ73f0CkXaXp7hrann"
	codexOAuthScope     = "openid profile email offline_access"
	defaultRefreshAhead = 6 * time.Hour
)

type TokenRefresher struct {
	OAuthURL   string
	HTTPClient *http.Client
	Now        func() time.Time
}

type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func NewTokenRefresher() *TokenRefresher {
	return &TokenRefresher{
		OAuthURL: defaultOAuthURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		Now: time.Now,
	}
}

func (r *TokenRefresher) MaybeRefresh(ctx context.Context, auth store.CodexAuth) (store.CodexAuth, bool, error) {
	if !canRefresh(auth) {
		return auth, false, nil
	}
	if !tokenExpiringSoon(auth, r.now().Add(defaultRefreshAhead)) {
		return auth, false, nil
	}
	return r.Refresh(ctx, auth)
}

func (r *TokenRefresher) Refresh(ctx context.Context, auth store.CodexAuth) (store.CodexAuth, bool, error) {
	if !canRefresh(auth) {
		return auth, false, nil
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", auth.Tokens.RefreshToken)
	form.Set("client_id", codexOAuthClientID)
	form.Set("scope", codexOAuthScope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.oauthURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return auth, false, fmt.Errorf("build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "codex-cli/1.0.0")

	resp, err := r.client().Do(req)
	if err != nil {
		return auth, false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return auth, false, fmt.Errorf("read refresh response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return auth, false, fmt.Errorf("refresh token request failed: status %d", resp.StatusCode)
	}

	var payload refreshResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return auth, false, fmt.Errorf("parse refresh response: %w", err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return auth, false, fmt.Errorf("refresh response missing access_token")
	}

	updated := cloneAuth(auth)
	updated.Tokens.AccessToken = strings.TrimSpace(payload.AccessToken)
	if token := strings.TrimSpace(payload.RefreshToken); token != "" {
		updated.Tokens.RefreshToken = token
	}
	if idToken := strings.TrimSpace(payload.IDToken); idToken != "" {
		updated.Tokens.IDToken = idToken
	}
	now := r.now().UTC()
	updated.LastRefresh = &now

	changed, err := authChanged(auth, updated)
	if err != nil {
		return auth, false, err
	}
	if !changed {
		return auth, false, nil
	}
	return updated, true, nil
}

func (r *TokenRefresher) client() *http.Client {
	if r.HTTPClient != nil {
		return r.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (r *TokenRefresher) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *TokenRefresher) oauthURL() string {
	if strings.TrimSpace(r.OAuthURL) != "" {
		return r.OAuthURL
	}
	return defaultOAuthURL
}

func tokenExpiringSoon(auth store.CodexAuth, threshold time.Time) bool {
	if auth.Tokens == nil {
		return false
	}
	expiry, ok := jwtExpiry(auth.Tokens.IDToken)
	if !ok {
		return false
	}
	return !expiry.After(threshold)
}

func jwtExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	payload, err := decodeJWTPart(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, false
	}
	if claims.Exp <= 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0).UTC(), true
}

func decodeJWTPart(part string) ([]byte, error) {
	if payload, err := base64.RawURLEncoding.DecodeString(part); err == nil {
		return payload, nil
	}
	if payload, err := base64.URLEncoding.DecodeString(part); err == nil {
		return payload, nil
	}
	if payload, err := base64.RawStdEncoding.DecodeString(part); err == nil {
		return payload, nil
	}
	return base64.StdEncoding.DecodeString(part)
}

func canRefresh(auth store.CodexAuth) bool {
	return auth.Tokens != nil && strings.TrimSpace(auth.Tokens.RefreshToken) != ""
}

func cloneAuth(auth store.CodexAuth) store.CodexAuth {
	cloned := auth
	if auth.Tokens != nil {
		tokens := *auth.Tokens
		cloned.Tokens = &tokens
	}
	if auth.LastRefresh != nil {
		last := *auth.LastRefresh
		cloned.LastRefresh = &last
	}
	return cloned
}

func authChanged(before, after store.CodexAuth) (bool, error) {
	beforeCanonical, err := canonicalAuthJSON(before)
	if err != nil {
		return false, err
	}
	afterCanonical, err := canonicalAuthJSON(after)
	if err != nil {
		return false, err
	}
	return !bytes.Equal(beforeCanonical, afterCanonical), nil
}

func canonicalAuthJSON(auth store.CodexAuth) ([]byte, error) {
	raw, err := json.Marshal(auth)
	if err != nil {
		return nil, err
	}
	_, canonical, err := store.NormalizeAndValidateAuth(raw)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}
