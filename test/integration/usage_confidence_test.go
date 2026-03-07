package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/domain"
	infrausage "github.com/prakersh/codexmultiauth/internal/infra/usage"
	"github.com/stretchr/testify/require"
)

func TestUsageConfidence_ConfirmedFromAPI(t *testing.T) {
	manager, _, authStore := newManager(t)
	ctx := context.Background()

	require.NoError(t, authStore.Save(ctx, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-1","refresh_token":"refresh-1","account_id":"acc-1"}}`)))
	_, err := manager.Save(ctx, app.SaveInput{DisplayName: "work"})
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"plan_type":"team","rate_limit":{"primary_window":{"used_percent":12.5,"reset_at":1900000000,"limit_window_seconds":18000}}}`))
	}))
	defer server.Close()

	manager.SetUsageFetcher(infrausage.NewClient(server.URL))
	results, err := manager.Usage(ctx, "work")
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, domain.UsageConfidenceConfirmed, results[0].Usage.Confidence)
}

func TestUsageConfidence_UnknownWithoutPlanMetadata(t *testing.T) {
	manager, _, authStore := newManager(t)
	ctx := context.Background()

	require.NoError(t, authStore.Save(ctx, []byte(`{"OPENAI_API_KEY":"sk-example"}`)))
	_, err := manager.Save(ctx, app.SaveInput{DisplayName: "api-key"})
	require.NoError(t, err)

	results, err := manager.Usage(ctx, "api-key")
	require.NoError(t, err)
	require.Equal(t, domain.UsageConfidenceUnknown, results[0].Usage.Confidence)
}
