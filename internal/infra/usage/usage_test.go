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
	require.Equal(t, "Weekly All-Model", displayName("seven_day"))
	require.Equal(t, "Review Requests", displayName("code_review"))
	require.Equal(t, "custom", displayName("custom"))
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

	resp := response{RateLimit: rateLimit{PrimaryWindow: &window{LimitWindowSeconds: 7 * 24 * 60 * 60}}}
	require.Equal(t, "seven_day", primaryName(resp))
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func buildJWT(t *testing.T, payload map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]string{"alg": "none", "typ": "JWT"})
	require.NoError(t, err)
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body) + ".sig"
}
