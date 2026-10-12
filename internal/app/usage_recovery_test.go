package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	infrausage "github.com/prakersh/codexmultiauth/internal/infra/usage"
	"github.com/stretchr/testify/require"
)

type recoveryUsageFetcher func(context.Context, store.CodexAuth) (domain.UsageSummary, error)

func (f recoveryUsageFetcher) Fetch(ctx context.Context, auth store.CodexAuth) (domain.UsageSummary, error) {
	return f(ctx, auth)
}

func TestUsageUnauthorizedRefreshesOnceAndPersistsCredentials(t *testing.T) {
	m, authStore, _ := newTestManager(t)
	account := seedAccount(t, m, authStore, "work")
	_, err := m.Activate(context.Background(), account.ID)
	require.NoError(t, err)
	refreshes := 0
	m.SetTokenRefresher(fakeTokenRefresher{refresh: func(ctx context.Context, auth store.CodexAuth) (store.CodexAuth, bool, error) {
		refreshes++
		auth.Tokens.AccessToken = "recovered-access"
		return auth, true, nil
	}})
	var tokens []string
	m.SetUsageFetcher(recoveryUsageFetcher(func(ctx context.Context, auth store.CodexAuth) (domain.UsageSummary, error) {
		tokens = append(tokens, auth.Tokens.AccessToken)
		if auth.Tokens.AccessToken != "recovered-access" {
			return domain.UsageSummary{}, &infrausage.RequestError{Operation: "usage", StatusCode: 401}
		}
		return domain.UsageSummary{Confidence: domain.UsageConfidenceConfirmed}, nil
	}))
	results, err := m.Usage(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, 1, refreshes)
	require.Equal(t, []string{"initial", "recovered-access"}, tokens)
	require.Nil(t, results[0].Problem)
	require.Equal(t, domain.UsageConfidenceConfirmed, results[0].Usage.Confidence)
	live, err := authStore.Load(context.Background())
	require.NoError(t, err)
	require.Equal(t, "recovered-access", live.Parsed.Tokens.AccessToken)
}

func TestUsageUnauthorizedReloadsMatchingLiveCredentialsBeforeRefreshing(t *testing.T) {
	m, authStore, _ := newTestManager(t)
	account := seedAccount(t, m, authStore, "work")
	_, err := m.Activate(context.Background(), account.ID)
	require.NoError(t, err)
	refreshes, fetches := 0, 0
	m.SetTokenRefresher(fakeTokenRefresher{refresh: func(ctx context.Context, auth store.CodexAuth) (store.CodexAuth, bool, error) {
		refreshes++
		return auth, false, errors.New("must reload first")
	}})
	m.SetUsageFetcher(recoveryUsageFetcher(func(ctx context.Context, auth store.CodexAuth) (domain.UsageSummary, error) {
		fetches++
		if fetches == 1 {
			authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"live-reloaded","refresh_token":"live-refresh","account_id":"acc-work"}}`), domain.AuthStoreFile)
			return domain.UsageSummary{}, &infrausage.RequestError{Operation: "usage", StatusCode: 401}
		}
		require.Equal(t, "live-reloaded", auth.Tokens.AccessToken)
		return domain.UsageSummary{Confidence: domain.UsageConfidenceConfirmed}, nil
	}))
	results, err := m.Usage(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, 2, fetches)
	require.Zero(t, refreshes)
	require.Nil(t, results[0].Problem)
}

func TestUsagePermanentRefreshFailureIsVisibleAndNotRepeated(t *testing.T) {
	m, authStore, _ := newTestManager(t)
	account := seedAccount(t, m, authStore, "work")
	refreshes, fetches := 0, 0
	m.SetTokenRefresher(fakeTokenRefresher{
		maybeRefresh: func(ctx context.Context, auth store.CodexAuth) (store.CodexAuth, bool, error) {
			return auth, false, &infrausage.RequestError{Operation: "refresh", StatusCode: 401, Code: "refresh_token_invalidated"}
		},
		refresh: func(ctx context.Context, auth store.CodexAuth) (store.CodexAuth, bool, error) {
			refreshes++
			return auth, false, nil
		},
	})
	m.SetUsageFetcher(recoveryUsageFetcher(func(ctx context.Context, auth store.CodexAuth) (domain.UsageSummary, error) {
		fetches++
		return domain.UsageSummary{}, &infrausage.RequestError{Operation: "usage", StatusCode: 401}
	}))
	results, err := m.Usage(context.Background(), account.ID)
	require.NoError(t, err)
	require.Zero(t, refreshes)
	require.Equal(t, 1, fetches)
	require.NotNil(t, results[0].Problem)
	require.True(t, results[0].Problem.RequiresLogin)
	require.Equal(t, "refresh_token_invalidated", results[0].Problem.Code)
	require.Contains(t, results[0].Problem.Message, "sign in")
}

func TestUsageErrorsDoNotExportArbitraryErrorText(t *testing.T) {
	m, authStore, _ := newTestManager(t)
	account := seedAccount(t, m, authStore, "work")
	m.SetUsageFetcher(recoveryUsageFetcher(func(ctx context.Context, auth store.CodexAuth) (domain.UsageSummary, error) {
		return domain.UsageSummary{}, errors.New("echoed-sensitive-test-material")
	}))
	results, err := m.Usage(context.Background(), account.ID)
	require.NoError(t, err)
	require.NotNil(t, results[0].Problem)
	raw, err := json.Marshal(results[0].Problem)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "echoed-sensitive-test-material")
	var problem *UsageProblem
	problem = newUsageProblem(&infrausage.RequestError{Operation: "usage", StatusCode: 401, Code: "echoed-sensitive-test-material"}, "usage")
	raw, err = json.Marshal(problem)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "echoed-sensitive-test-material")
}

func TestUsageRepeatedUnauthorizedStopsAfterOneRefresh(t *testing.T) {
	m, authStore, _ := newTestManager(t)
	account := seedAccount(t, m, authStore, "work")
	refreshes, fetches := 0, 0
	m.SetTokenRefresher(fakeTokenRefresher{refresh: func(ctx context.Context, auth store.CodexAuth) (store.CodexAuth, bool, error) {
		refreshes++
		tokens := *auth.Tokens
		tokens.AccessToken = "refreshed-but-rejected"
		auth.Tokens = &tokens
		return auth, true, nil
	}})
	m.SetUsageFetcher(recoveryUsageFetcher(func(ctx context.Context, auth store.CodexAuth) (domain.UsageSummary, error) {
		fetches++
		return domain.UsageSummary{}, &infrausage.RequestError{Operation: "usage", StatusCode: 401}
	}))
	results, err := m.Usage(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, 1, refreshes)
	require.Equal(t, 2, fetches)
	require.True(t, results[0].Problem.RequiresLogin)
}
