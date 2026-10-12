package usage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/stretchr/testify/require"
)

func TestClientFetchDoesNotProbeAlternateOnAuthorizationFailure(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var requests int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				require.Equal(t, "/backend-api/wham/usage", r.URL.Path)
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":{"code":"token_expired","message":"secret-access-token"}}`))
			}))
			defer server.Close()
			client := NewClient(server.URL + "/backend-api/wham/usage")
			client.HTTPClient = server.Client()
			_, err := client.Fetch(context.Background(), store.CodexAuth{
				Tokens: &store.CodexTokens{AccessToken: "secret-access-token"},
			})
			var requestErr *RequestError
			require.ErrorAs(t, err, &requestErr)
			require.Equal(t, status, requestErr.StatusCode)
			require.Equal(t, "usage", requestErr.Operation)
			require.Equal(t, 1, requests)
			require.NotContains(t, err.Error(), "secret-access-token")
			require.Equal(t, status == http.StatusUnauthorized, IsUnauthorized(fmt.Errorf("wrapped: %w", err)))
		})
	}
}

func TestRefreshErrorClassifiesSafeMachineCodes(t *testing.T) {
	tests := []struct {
		name          string
		status        int
		body          string
		code          string
		requiresLogin bool
	}{
		{"expired", 401, `{"error":{"code":"refresh_token_expired","message":"secret-token"}}`, "refresh_token_expired", true},
		{"reused", 401, `{"error":{"code":"refresh_token_reused"}}`, "refresh_token_reused", true},
		{"case insensitive code", 401, `{"error":{"code":"REFRESH_TOKEN_REUSED"}}`, "refresh_token_reused", true},
		{"revoked", 400, `{"code":"refresh_token_revoked"}`, "refresh_token_revoked", true},
		{"invalidated", 401, `{"error":{"code":"refresh_token_invalidated"}}`, "refresh_token_invalidated", true},
		{"invalid grant", 400, `{"error":"invalid_grant","error_description":"secret-token"}`, "invalid_grant", true},
		{"unknown unauthorized", 401, `{"error":"secret-token","error_description":"secret-token"}`, "", true},
		{"unknown bad request", 400, `{"code":"secret-token"}`, "", false},
		{"temporary", 503, `{"error":"temporarily_unavailable","error_description":"secret-token"}`, "temporarily_unavailable", false},
		{"malformed", 500, `secret-token`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			refresher := &TokenRefresher{OAuthURL: server.URL, HTTPClient: server.Client()}
			_, changed, err := refresher.Refresh(context.Background(), store.CodexAuth{
				Tokens: &store.CodexTokens{RefreshToken: "secret-token"},
			})
			require.False(t, changed)
			var requestErr *RequestError
			require.ErrorAs(t, err, &requestErr)
			require.Equal(t, "refresh", requestErr.Operation)
			require.Equal(t, tt.status, requestErr.StatusCode)
			require.Equal(t, tt.code, requestErr.Code)
			require.Equal(t, tt.requiresLogin, requestErr.RequiresLogin())
			require.NotContains(t, err.Error(), "secret-token")
			require.False(t, IsUnauthorized(err), "only usage 401 should trigger usage recovery")
		})
	}
}

func TestMaybeRefreshMatchesCodexProactivePolicy(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	recent := now.Add(-time.Hour)
	boundary := now.Add(-8 * 24 * time.Hour)
	old := boundary.Add(-time.Second)
	tests := []struct {
		name        string
		accessToken string
		lastRefresh *time.Time
		wantRefresh bool
	}{
		{"healthy token", jwtWithExp(now.Add(6 * time.Minute)), &old, false},
		{"five minute boundary", jwtWithExp(now.Add(5 * time.Minute)), &recent, true},
		{"expired token", jwtWithExp(now.Add(-time.Minute)), &recent, true},
		{"opaque recent", "opaque-token", &recent, false},
		{"opaque eight day boundary", "opaque-token", &boundary, false},
		{"opaque older than eight days", "opaque-token", &old, true},
		{"opaque no refresh date", "opaque-token", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh"}`))
			}))
			defer server.Close()
			refresher := &TokenRefresher{OAuthURL: server.URL, HTTPClient: server.Client(), Now: func() time.Time { return now }}
			_, changed, err := refresher.MaybeRefresh(context.Background(), store.CodexAuth{
				Tokens: &store.CodexTokens{
					AccessToken: tt.accessToken, RefreshToken: "refresh", IDToken: jwtWithExp(now.Add(-time.Hour)),
				},
				LastRefresh: tt.lastRefresh,
			})
			require.NoError(t, err)
			require.Equal(t, tt.wantRefresh, changed)
			if tt.wantRefresh {
				require.Equal(t, 1, requests)
			} else {
				require.Zero(t, requests)
			}
		})
	}
}

func TestRequestErrorCannotExposeArbitraryFields(t *testing.T) {
	err := &RequestError{Operation: "secret-token", StatusCode: 401, Code: "secret-token"}
	require.NotContains(t, err.Error(), "secret-token")
	require.False(t, IsUnauthorized(err))
	require.False(t, err.RequiresLogin())
}

func TestAuthorizationStatusSurvivesUnreadableErrorBody(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(failingBody{})}, nil
	})}
	refresher := &TokenRefresher{OAuthURL: "https://example.com/token", HTTPClient: client}
	_, _, refreshErr := refresher.Refresh(context.Background(), store.CodexAuth{
		Tokens: &store.CodexTokens{RefreshToken: "refresh"},
	})
	var requestErr *RequestError
	require.ErrorAs(t, refreshErr, &requestErr)
	require.True(t, requestErr.RequiresLogin())

	usageClient := NewClient("https://example.com/backend-api/wham/usage")
	usageClient.HTTPClient = client
	_, usageErr := usageClient.Fetch(context.Background(), store.CodexAuth{
		Tokens: &store.CodexTokens{AccessToken: "access"},
	})
	require.True(t, IsUnauthorized(usageErr))
}

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) {
	return 0, errors.New("body interrupted")
}
