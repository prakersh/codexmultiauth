package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/stretchr/testify/require"
)

func TestAutoActivateChoosesBestQuotaCandidate(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	ctx := context.Background()
	fixedNow := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return fixedNow }

	savedSoonWeekly := saveAutoAccount(t, ctx, manager, authStore, "soon-weekly", "acc-soon-weekly")
	_ = saveAutoAccount(t, ctx, manager, authStore, "more-five-hour", "acc-more-five-hour")
	_ = saveAutoAccount(t, ctx, manager, authStore, "low-priority", "acc-low-priority")

	manager.SetUsageFetcher(autoUsageFetcher{
		summaries: map[string]domain.UsageSummary{
			"acc-soon-weekly":    autoUsageSummary(20, fixedNow.Add(4*time.Hour), 50, fixedNow.Add(24*time.Hour)),
			"acc-more-five-hour": autoUsageSummary(10, fixedNow.Add(4*time.Hour), 40, fixedNow.Add(5*24*time.Hour)),
			"acc-low-priority":   autoUsageSummary(35, fixedNow.Add(5*time.Hour), 70, fixedNow.Add(48*time.Hour)),
		},
	})

	account, err := manager.AutoActivate(ctx)
	require.NoError(t, err)
	require.Equal(t, savedSoonWeekly.Account.ID, account.ID)

	listed, err := manager.List(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 3)
	for _, item := range listed {
		if item.Account.ID == savedSoonWeekly.Account.ID {
			require.True(t, item.IsActive)
			continue
		}
		require.False(t, item.IsActive)
	}

	record, err := authStore.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, savedSoonWeekly.Account.Fingerprint, record.Fingerprint)
}

func TestSelectAutoActivationPrefersSoonerWeeklyResetOverSmallFiveHourAdvantage(t *testing.T) {
	fixedNow := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)

	results := []UsageResult{
		{
			Account: domain.Account{ID: "soon", DisplayName: "soon"},
			Usage:   autoUsageSummary(20, fixedNow.Add(4*time.Hour), 50, fixedNow.Add(24*time.Hour)),
		},
		{
			Account: domain.Account{ID: "later", DisplayName: "later"},
			Usage:   autoUsageSummary(10, fixedNow.Add(4*time.Hour), 40, fixedNow.Add(5*24*time.Hour)),
		},
	}

	selected, err := selectAutoActivation(results, fixedNow)
	require.NoError(t, err)
	require.Equal(t, "soon", selected.Account.ID)
}

func TestAutoActivateReturnsHelpfulErrorWithoutAccounts(t *testing.T) {
	manager, _, _ := newTestManager(t)

	_, err := manager.AutoActivate(context.Background())
	require.ErrorIs(t, err, errNoSavedAccounts)
}

type autoUsageFetcher struct {
	summaries map[string]domain.UsageSummary
}

func (f autoUsageFetcher) Fetch(ctx context.Context, auth store.CodexAuth) (domain.UsageSummary, error) {
	if auth.Tokens == nil {
		return domain.UsageSummary{}, errors.New("missing tokens")
	}
	summary, ok := f.summaries[auth.Tokens.AccountID]
	if !ok {
		return domain.UsageSummary{}, errors.New("missing summary")
	}
	return summary, nil
}

func saveAutoAccount(t *testing.T, ctx context.Context, manager *Manager, authStore *memoryAuthStore, displayName, accountID string) SaveResult {
	t.Helper()

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-`+accountID+`","refresh_token":"refresh-`+accountID+`","account_id":"`+accountID+`"}}`), domain.AuthStoreFile)
	saved, err := manager.Save(ctx, SaveInput{DisplayName: displayName})
	require.NoError(t, err)
	return saved
}

func autoUsageSummary(fiveHourUsed float64, fiveHourReset time.Time, weeklyUsed float64, weeklyReset time.Time) domain.UsageSummary {
	return domain.UsageSummary{
		Confidence: domain.UsageConfidenceConfirmed,
		PlanType:   "team",
		Quotas: []domain.UsageQuota{
			{
				DisplayName: "5-Hour Limit",
				UsedPercent: autoFloatPtr(fiveHourUsed),
				ResetsAt:    autoTimePtr(fiveHourReset),
			},
			{
				DisplayName: "Weekly Limit",
				UsedPercent: autoFloatPtr(weeklyUsed),
				ResetsAt:    autoTimePtr(weeklyReset),
			},
		},
	}
}

func autoFloatPtr(value float64) *float64 {
	return &value
}

func autoTimePtr(value time.Time) *time.Time {
	return &value
}
