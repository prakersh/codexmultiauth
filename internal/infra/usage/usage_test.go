package usage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/stretchr/testify/require"
)

func TestParseResponse(t *testing.T) {
	summary, err := ParseResponse([]byte(`{"plan_type":"team","rate_limit":{"primary_window":{"used_percent":10.5,"reset_at":1900000000,"limit_window_seconds":18000}}}`))
	require.NoError(t, err)
	require.Equal(t, domain.UsageConfidenceConfirmed, summary.Confidence)
	require.Len(t, summary.Quotas, 1)
}

func TestBestEffortSummary(t *testing.T) {
	token := buildJWT(t, map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_plan_type": "pro"}})
	summary := BestEffortSummary(store.CodexAuth{
		Tokens: &store.CodexTokens{AccessToken: token},
	})
	require.Equal(t, domain.UsageConfidenceBestEffort, summary.Confidence)
	require.Equal(t, "pro", summary.PlanType)
}

func TestExtractAccountMetadata(t *testing.T) {
	token := buildJWT(t, map[string]any{
		"name":  "Ecom Account",
		"email": "ecom@example.com",
	})
	metadata := ExtractAccountMetadata(store.CodexAuth{
		Tokens: &store.CodexTokens{
			AccountID: "acc-1",
			IDToken:   token,
		},
	})
	require.Equal(t, "chatgpt", metadata.AuthMode)
	require.Equal(t, "acc-1", metadata.CodexAccountID)
	require.Equal(t, "Ecom Account", metadata.UserName)
	require.Equal(t, "ecom@example.com", metadata.UserEmail)

	apiKeyMetadata := ExtractAccountMetadata(store.CodexAuth{OpenAIAPIKey: "sk-example"})
	require.Equal(t, "api_key", apiKeyMetadata.AuthMode)
}

func TestClientFetchFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/backend-api/wham/usage") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"plan_type":"team","rate_limit":{"primary_window":{"used_percent":10,"reset_at":1900000000,"limit_window_seconds":18000}}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL + "/backend-api/wham/usage")
	client.HTTPClient = server.Client()
	summary, err := client.Fetch(context.Background(), store.CodexAuth{
		Tokens: &store.CodexTokens{AccessToken: "token", AccountID: "acc"},
	})
	require.NoError(t, err)
	require.Equal(t, "team", summary.PlanType)
}

func TestClientFetchMissingAccessToken(t *testing.T) {
	client := NewClient("https://example.com")
	_, err := client.Fetch(context.Background(), store.CodexAuth{})
	require.Error(t, err)
}

func TestStatusHelpers(t *testing.T) {
	require.Equal(t, "5-Hour Limit", displayName("five_hour"))
	require.Equal(t, "Weekly Limit", displayName("weekly"))
	require.Equal(t, "Monthly Limit", displayName("monthly"))
	require.Equal(t, "3-Day Limit", displayName("3_day"))
	require.Equal(t, "Review Requests", displayName("code_review"))
	require.Equal(t, "Custom", displayName("custom"))
	require.Equal(t, "healthy", statusFromPercent(10))
	require.Equal(t, "warning", statusFromPercent(60))
	require.Equal(t, "danger", statusFromPercent(85))
	require.Equal(t, "critical", statusFromPercent(99))
}

func TestNewClientDefaults(t *testing.T) {
	client := NewClient("")
	require.Equal(t, "https://chatgpt.com/backend-api/wham/usage", client.BaseURL)
	require.Equal(t, 10*time.Second, client.HTTPClient.Timeout)
}

func TestClientFetchNon200AndMalformedResponse(t *testing.T) {
	t.Run("non-200 on both endpoints", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "denied", http.StatusForbidden)
		}))
		defer server.Close()

		client := NewClient(server.URL + "/backend-api/wham/usage")
		client.HTTPClient = server.Client()

		_, err := client.Fetch(context.Background(), store.CodexAuth{
			Tokens: &store.CodexTokens{AccessToken: "token", AccountID: "acc"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "returned 403")
	})

	t.Run("malformed json after fallback", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/backend-api/wham/usage") {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte("{"))
		}))
		defer server.Close()

		client := NewClient(server.URL + "/backend-api/wham/usage")
		client.HTTPClient = server.Client()

		_, err := client.Fetch(context.Background(), store.CodexAuth{
			Tokens: &store.CodexTokens{AccessToken: "token"},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "parse usage response")
	})
}

func TestClientFetchTransportFailure(t *testing.T) {
	client := NewClient("http://example.com/backend-api/wham/usage")
	client.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("network down")
		}),
	}

	_, err := client.Fetch(context.Background(), store.CodexAuth{
		Tokens: &store.CodexTokens{AccessToken: "token"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "network down")
}

func TestBestEffortSummaryFallbackBranches(t *testing.T) {
	idToken := buildJWT(t, map[string]any{"chatgpt_plan_type": "enterprise"})
	summary := BestEffortSummary(store.CodexAuth{
		Tokens: &store.CodexTokens{IDToken: idToken},
	})
	require.Equal(t, domain.UsageConfidenceBestEffort, summary.Confidence)
	require.Equal(t, "enterprise", summary.PlanType)

	unknown := BestEffortSummary(store.CodexAuth{})
	require.Equal(t, domain.UsageConfidenceUnknown, unknown.Confidence)
}

func TestJWTParsingEdgeCases(t *testing.T) {
	require.Equal(t, "", parsePlanTypeFromJWT("not-a-jwt"))
	require.Equal(t, "", parsePlanTypeFromJWT("a.bad!.sig"))

	body, err := json.Marshal(map[string]any{"chatgpt_plan_type": "plus"})
	require.NoError(t, err)
	token := "header." + base64.RawURLEncoding.EncodeToString(body) + ".sig"
	require.Equal(t, "plus", parsePlanTypeFromJWT(token))
}

func TestParseResponseAdditionalBranches(t *testing.T) {
	summary, err := ParseResponse([]byte(`{
		"plan_type":"enterprise",
		"credits":{"balance":12.5},
		"rate_limit":{
			"primary_window":{"used_percent":12,"reset_at":1900000000,"limit_window_seconds":604800},
			"secondary_window":{"used_percent":40,"reset_at":1900000500,"limit_window_seconds":604800}
		},
		"code_review_rate_limit":{
			"primary_window":{"used_percent":50,"reset_at":1900001000,"limit_window_seconds":86400}
		}
	}`))
	require.NoError(t, err)
	require.Equal(t, "enterprise", summary.PlanType)
	require.Len(t, summary.Quotas, 3)
	require.NotNil(t, summary.CreditsLeft)
	require.Equal(t, 12.5, *summary.CreditsLeft)

	// Both rate_limit windows are 7 days here, so the second is suffixed
	// instead of dropped; the 1-day code review window sorts ahead of them.
	require.Equal(t, "code_review", summary.Quotas[0].Name)
	require.Equal(t, "weekly", summary.Quotas[1].Name)
	require.Equal(t, "weekly_2", summary.Quotas[2].Name)

	require.Equal(t, "weekly", windowName(&window{LimitWindowSeconds: 7 * 24 * 60 * 60}, ""))
	require.Equal(t, "monthly", windowName(&window{LimitWindowSeconds: 30 * 24 * 60 * 60}, ""))
	require.Equal(t, "five_hour", windowName(&window{LimitWindowSeconds: 5 * 60 * 60}, ""))
	require.Equal(t, "five_hour", windowName(&window{}, "five_hour"))
}

func TestParseResponseProliteStringCreditsBalance(t *testing.T) {
	summary, err := ParseResponse([]byte(`{
		"plan_type":"prolite",
		"credits":{"balance":"0"},
		"rate_limit":{
			"primary_window":{"used_percent":14,"reset_at":1900000000,"limit_window_seconds":18000},
			"secondary_window":{"used_percent":2,"reset_at":1900000500,"limit_window_seconds":604800}
		}
	}`))
	require.NoError(t, err)
	require.Equal(t, "prolite", summary.PlanType)
	require.Len(t, summary.Quotas, 2)
	require.NotNil(t, summary.CreditsLeft)
	require.Equal(t, 0.0, *summary.CreditsLeft)
	require.Equal(t, "five_hour", summary.Quotas[0].Name)
	require.Equal(t, "weekly", summary.Quotas[1].Name)
	require.NotNil(t, summary.Quotas[0].ResetsAt)
	require.NotNil(t, summary.Quotas[1].ResetsAt)
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTokenRefresherMaybeRefreshBranches(t *testing.T) {
	now := time.Date(2026, 3, 14, 15, 0, 0, 0, time.UTC)
	refresher := NewTokenRefresher()
	refresher.Now = func() time.Time { return now }

	t.Run("no refresh token returns unchanged", func(t *testing.T) {
		auth := store.CodexAuth{Tokens: &store.CodexTokens{AccessToken: "a", IDToken: buildIDTokenWithExp(t, now.Add(time.Hour))}}
		updated, changed, err := refresher.MaybeRefresh(context.Background(), auth)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, auth, updated)
	})

	t.Run("not expiring soon skips network refresh", func(t *testing.T) {
		called := false
		refresher.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			called = true
			return nil, errors.New("should not call refresh endpoint")
		})}
		auth := store.CodexAuth{Tokens: &store.CodexTokens{AccessToken: "a", RefreshToken: "r", IDToken: buildIDTokenWithExp(t, now.Add(48*time.Hour))}}
		updated, changed, err := refresher.MaybeRefresh(context.Background(), auth)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, auth, updated)
		require.False(t, called)
	})
}

func TestTokenRefresherRefreshSuccess(t *testing.T) {
	now := time.Date(2026, 3, 14, 15, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.NoError(t, r.ParseForm())
		require.Equal(t, "refresh_token", r.Form.Get("grant_type"))
		require.Equal(t, "refresh-old", r.Form.Get("refresh_token"))
		require.Equal(t, codexOAuthClientID, r.Form.Get("client_id"))
		require.Equal(t, codexOAuthScope, r.Form.Get("scope"))
		_, _ = w.Write([]byte(`{"access_token":"access-new","refresh_token":"refresh-new","id_token":"id-new","expires_in":3600}`))
	}))
	defer server.Close()

	refresher := NewTokenRefresher()
	refresher.Now = func() time.Time { return now }
	refresher.OAuthURL = server.URL
	refresher.HTTPClient = server.Client()

	auth := store.CodexAuth{Tokens: &store.CodexTokens{
		AccessToken:  "access-old",
		RefreshToken: "refresh-old",
		IDToken:      buildIDTokenWithExp(t, now.Add(time.Hour)),
		AccountID:    "acc-1",
	}}
	updated, changed, err := refresher.MaybeRefresh(context.Background(), auth)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "access-new", updated.Tokens.AccessToken)
	require.Equal(t, "refresh-new", updated.Tokens.RefreshToken)
	require.Equal(t, "id-new", updated.Tokens.IDToken)
	require.Equal(t, "acc-1", updated.Tokens.AccountID)
	require.NotNil(t, updated.LastRefresh)
	require.Equal(t, now.UTC(), *updated.LastRefresh)
}

func TestTokenRefresherRefreshErrorPaths(t *testing.T) {
	now := time.Date(2026, 3, 14, 15, 0, 0, 0, time.UTC)
	auth := store.CodexAuth{Tokens: &store.CodexTokens{
		AccessToken:  "access-old",
		RefreshToken: "refresh-old",
		IDToken:      buildIDTokenWithExp(t, now.Add(time.Hour)),
	}}

	t.Run("non-200 response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "denied", http.StatusUnauthorized)
		}))
		defer server.Close()

		refresher := NewTokenRefresher()
		refresher.Now = func() time.Time { return now }
		refresher.OAuthURL = server.URL
		refresher.HTTPClient = server.Client()

		updated, changed, err := refresher.Refresh(context.Background(), auth)
		require.Error(t, err)
		require.False(t, changed)
		require.Equal(t, auth, updated)
		require.Contains(t, err.Error(), "status 401")
	})

	t.Run("malformed success body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{"))
		}))
		defer server.Close()

		refresher := NewTokenRefresher()
		refresher.Now = func() time.Time { return now }
		refresher.OAuthURL = server.URL
		refresher.HTTPClient = server.Client()

		updated, changed, err := refresher.Refresh(context.Background(), auth)
		require.Error(t, err)
		require.False(t, changed)
		require.Equal(t, auth, updated)
		require.Contains(t, err.Error(), "parse refresh response")
	})
}

func buildIDTokenWithExp(t *testing.T, exp time.Time) string {
	t.Helper()
	return buildJWT(t, map[string]any{"exp": exp.Unix()})
}

func buildJWT(t *testing.T, payload map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]string{"alg": "none", "typ": "JWT"})
	require.NoError(t, err)
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body) + ".sig"
}

// TestParseResponseFreePlanMonthlyWindow covers the current free-plan shape:
// a single 30-day primary window and no secondary window.
func TestParseResponseFreePlanMonthlyWindow(t *testing.T) {
	summary, err := ParseResponse([]byte(`{
		"plan_type":"free",
		"rate_limit":{
			"allowed":true,
			"limit_reached":false,
			"primary_window":{"used_percent":0,"limit_window_seconds":2592000,"reset_after_seconds":2592000,"reset_at":1789852672},
			"secondary_window":null
		},
		"code_review_rate_limit":null,
		"additional_rate_limits":null,
		"credits":{"has_credits":false,"unlimited":false,"balance":null}
	}`))
	require.NoError(t, err)
	require.Equal(t, "free", summary.PlanType)
	require.False(t, summary.LimitReached)
	require.Len(t, summary.Quotas, 1)
	require.Equal(t, "monthly", summary.Quotas[0].Name)
	require.Equal(t, "Monthly Limit", summary.Quotas[0].DisplayName)
	require.Equal(t, int64(2592000), summary.Quotas[0].WindowSeconds)
	require.NotNil(t, summary.Quotas[0].ResetsAt)
	require.Nil(t, summary.CreditsLeft)
}

// TestParseResponsePaidPlanWeeklyOnly covers the current paid-plan shape after
// the 5-hour window was removed: a single 7-day primary window.
func TestParseResponsePaidPlanWeeklyOnly(t *testing.T) {
	summary, err := ParseResponse([]byte(`{
		"plan_type":"plus",
		"rate_limit":{
			"allowed":true,
			"primary_window":{"used_percent":42.5,"limit_window_seconds":604800,"reset_at":1900000000},
			"secondary_window":null
		}
	}`))
	require.NoError(t, err)
	require.Len(t, summary.Quotas, 1)
	require.Equal(t, "weekly", summary.Quotas[0].Name)
	require.Equal(t, "Weekly Limit", summary.Quotas[0].DisplayName)
	require.Equal(t, int64(604800), summary.Quotas[0].WindowSeconds)
}

// TestParseResponseLimitReached checks the blocked-account signals the API
// exposes alongside the windows.
func TestParseResponseLimitReached(t *testing.T) {
	summary, err := ParseResponse([]byte(`{
		"plan_type":"free",
		"rate_limit":{"allowed":false,"limit_reached":true,"primary_window":{"used_percent":100,"limit_window_seconds":2592000,"reset_at":1900000000}},
		"rate_limit_reached_type":"monthly"
	}`))
	require.NoError(t, err)
	require.True(t, summary.LimitReached)
	require.Equal(t, "critical", summary.Quotas[0].Status)
}

// TestParseResponseSortsShortestWindowFirst keeps the most immediately binding
// limit leftmost for renderers.
func TestParseResponseSortsShortestWindowFirst(t *testing.T) {
	summary, err := ParseResponse([]byte(`{
		"rate_limit":{
			"primary_window":{"used_percent":1,"limit_window_seconds":2592000,"reset_at":1900000000},
			"secondary_window":{"used_percent":2,"limit_window_seconds":18000,"reset_at":1900000000}
		}
	}`))
	require.NoError(t, err)
	require.Len(t, summary.Quotas, 2)
	require.Equal(t, "five_hour", summary.Quotas[0].Name)
	require.Equal(t, "monthly", summary.Quotas[1].Name)
}

// TestParseResponseAdditionalRateLimitsShapes ensures an unfamiliar
// additional_rate_limits payload never fails the whole parse.
func TestParseResponseAdditionalRateLimitsShapes(t *testing.T) {
	keyed, err := ParseResponse([]byte(`{
		"rate_limit":{"primary_window":{"used_percent":5,"limit_window_seconds":604800,"reset_at":1900000000}},
		"additional_rate_limits":{"cloud_tasks":{"used_percent":30,"limit_window_seconds":86400,"reset_at":1900000000}}
	}`))
	require.NoError(t, err)
	require.Len(t, keyed.Quotas, 2)
	// The map key names the quota so it stays distinguishable from the
	// windows the main rate_limit block reports.
	require.Equal(t, "cloud_tasks", keyed.Quotas[0].Name)
	require.Equal(t, domain.QuotaCategoryOther, keyed.Quotas[0].Category)

	listed, err := ParseResponse([]byte(`{
		"rate_limit":{"primary_window":{"used_percent":5,"limit_window_seconds":604800,"reset_at":1900000000}},
		"additional_rate_limits":[{"used_percent":30,"limit_window_seconds":86400,"reset_at":1900000000}]
	}`))
	require.NoError(t, err)
	require.Len(t, listed.Quotas, 2)

	junk, err := ParseResponse([]byte(`{
		"rate_limit":{"primary_window":{"used_percent":5,"limit_window_seconds":604800,"reset_at":1900000000}},
		"additional_rate_limits":"unexpected"
	}`))
	require.NoError(t, err)
	require.Len(t, junk.Quotas, 1)
}

// TestParseResponseLegacyWindowsWithoutLength falls back to slot position when
// the API omits limit_window_seconds, as older payloads did.
func TestParseResponseLegacyWindowsWithoutLength(t *testing.T) {
	summary, err := ParseResponse([]byte(`{
		"rate_limit":{
			"primary_window":{"used_percent":10,"reset_at":1900000000},
			"secondary_window":{"used_percent":20,"reset_at":1900000500}
		}
	}`))
	require.NoError(t, err)
	require.Len(t, summary.Quotas, 2)
	require.Equal(t, "five_hour", summary.Quotas[0].Name)
	require.Equal(t, "weekly", summary.Quotas[1].Name)
}

// TestParseResponseRejectsWindowlessAdditionalLimits guards against minting a
// quota from a JSON object that carries no window fields. Decoding
// permissively reported "0.0% used, healthy" for a limit whose real usage sat
// under an unrecognized key.
func TestParseResponseRejectsWindowlessAdditionalLimits(t *testing.T) {
	summary, err := ParseResponse([]byte(`{
		"rate_limit":{"primary_window":{"used_percent":5,"limit_window_seconds":604800,"reset_at":1900000000}},
		"additional_rate_limits":{"gpt5_codex":{"primary_window":{"used_percent":95,"limit_window_seconds":604800}}}
	}`))
	require.NoError(t, err)
	require.Len(t, summary.Quotas, 1)
	require.Equal(t, "weekly", summary.Quotas[0].Name)
}

// TestParseResponseTagsQuotaCategories keeps review limits distinguishable
// from the limits that gate model usage.
func TestParseResponseTagsQuotaCategories(t *testing.T) {
	summary, err := ParseResponse([]byte(`{
		"rate_limit":{"primary_window":{"used_percent":5,"limit_window_seconds":604800,"reset_at":1900000000}},
		"code_review_rate_limit":{"primary_window":{"used_percent":80,"limit_window_seconds":86400,"reset_at":1900000000}}
	}`))
	require.NoError(t, err)
	require.Len(t, summary.Quotas, 2)
	byName := map[string]domain.UsageQuota{}
	for _, quota := range summary.Quotas {
		byName[quota.Name] = quota
	}
	require.Equal(t, domain.QuotaCategoryModel, byName["weekly"].Category)
	require.True(t, byName["weekly"].IsModelLimit())
	require.Equal(t, domain.QuotaCategoryCodeReview, byName["code_review"].Category)
	require.False(t, byName["code_review"].IsModelLimit())
}

// TestWindowNameToleratesTierDrift keeps accounts on the same tier in one
// column when the API reports a 31-day calendar month or a slightly off window.
func TestWindowNameToleratesTierDrift(t *testing.T) {
	require.Equal(t, "monthly", windowName(&window{LimitWindowSeconds: 31 * 24 * 60 * 60}, ""))
	require.Equal(t, "monthly", windowName(&window{LimitWindowSeconds: 28 * 24 * 60 * 60}, ""))
	require.Equal(t, "weekly", windowName(&window{LimitWindowSeconds: 6 * 24 * 60 * 60}, ""))
	require.Equal(t, "five_hour", windowName(&window{LimitWindowSeconds: 6 * 60 * 60}, ""))
	require.Equal(t, "90_day", windowName(&window{LimitWindowSeconds: 90 * 24 * 60 * 60}, ""))
}

// TestDisplayNameHandlesNonASCIISlug covers labels derived from arbitrary
// additional_rate_limits keys.
func TestDisplayNameHandlesNonASCIISlug(t *testing.T) {
	require.Equal(t, "Über", displayName("über"))
	require.Equal(t, "Cloud Tasks", displayName("cloud_tasks"))
}

// TestFetchSendsAccountScopeHeader pins the account-scoping header name. It was
// misspelled "ChatClaude-Account-Id", so the API ignored it and returned usage
// for whichever account the token defaults to instead of the one requested.
func TestFetchSendsAccountScopeHeader(t *testing.T) {
	var got http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"plan_type":"free","rate_limit":{"primary_window":{"used_percent":0,"limit_window_seconds":2592000,"reset_at":1900000000}}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	_, err := client.Fetch(context.Background(), store.CodexAuth{
		Tokens: &store.CodexTokens{AccessToken: "token", AccountID: "acct-123"},
	})
	require.NoError(t, err)
	require.Equal(t, "acct-123", got.Get("ChatGPT-Account-Id"))
	require.Equal(t, "acct-123", got.Get("X-Account-Id"))
	require.Empty(t, got.Get("ChatClaude-Account-Id"))
}

func jwtWithExp(exp time.Time) string {
	claims, _ := json.Marshal(map[string]any{"exp": exp.Unix()})
	return "h." + base64.RawURLEncoding.EncodeToString(claims) + ".s"
}

// TestMaybeRefreshMeasuresAccessTokenNotIDToken covers the token burn bug.
// Codex issues the id_token with a one hour life and the access token with a
// ten day life. Measuring the id_token meant every command run more than an
// hour after login attempted a refresh, and each attempt spends a single-use
// refresh token.
func TestMaybeRefreshMeasuresAccessTokenNotIDToken(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		_, _ = w.Write([]byte(`{"access_token":"new","refresh_token":"rt2"}`))
	}))
	defer server.Close()
	refresher := &TokenRefresher{OAuthURL: server.URL, Now: func() time.Time { return now }}

	// The realistic steady state: id_token long expired, access token healthy.
	healthy := store.CodexAuth{Tokens: &store.CodexTokens{
		IDToken:      jwtWithExp(now.Add(-11 * time.Hour)),
		AccessToken:  jwtWithExp(now.Add(9 * 24 * time.Hour)),
		RefreshToken: "rt",
	}}
	_, changed, err := refresher.MaybeRefresh(context.Background(), healthy)
	require.NoError(t, err)
	require.False(t, changed)
	require.Zero(t, posts, "refreshed while the access token was valid for nine more days")

	// An access token inside the refresh-ahead window must still refresh.
	expiring := store.CodexAuth{Tokens: &store.CodexTokens{
		IDToken:      jwtWithExp(now.Add(-11 * time.Hour)),
		AccessToken:  jwtWithExp(now.Add(time.Hour)),
		RefreshToken: "rt",
	}}
	_, changed, err = refresher.MaybeRefresh(context.Background(), expiring)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, posts)

	// With no readable access token expiry, fall back to the id_token.
	posts = 0
	legacy := store.CodexAuth{Tokens: &store.CodexTokens{
		IDToken:      jwtWithExp(now.Add(-time.Hour)),
		AccessToken:  "not-a-jwt",
		RefreshToken: "rt",
	}}
	_, _, err = refresher.MaybeRefresh(context.Background(), legacy)
	require.NoError(t, err)
	require.Equal(t, 1, posts, "id_token fallback must still drive a refresh")
}

// TestRefreshSurfacesOAuthError checks that a failure says why. It previously
// reported only "status 401", which did not distinguish a spent token from a
// wrong client id.
func TestRefreshSurfacesOAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh token is expired or already used"}`))
	}))
	defer server.Close()

	refresher := &TokenRefresher{OAuthURL: server.URL}
	_, _, err := refresher.Refresh(context.Background(), store.CodexAuth{
		Tokens: &store.CodexTokens{RefreshToken: "spent"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid_grant")
	require.Contains(t, err.Error(), "already used")
	require.NotContains(t, err.Error(), "spent", "the refresh token must never appear in the error")
}
