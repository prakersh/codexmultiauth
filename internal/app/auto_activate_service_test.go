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

// TestSelectAutoActivationScoresSingleWindowPlans covers the current Codex
// shapes: a free account reports only a monthly window and a paid account only
// a weekly one. The account with more headroom must win even though the two
// report different window kinds.
func TestSelectAutoActivationScoresSingleWindowPlans(t *testing.T) {
	fixedNow := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

	results := []UsageResult{
		{
			Account: domain.Account{ID: "freeacct", DisplayName: "freeacct"},
			Usage:   singleWindowSummary("monthly", "Monthly Limit", 30*24*60*60, 90, fixedNow.AddDate(0, 0, 20)),
		},
		{
			Account: domain.Account{ID: "paidacct", DisplayName: "paidacct"},
			Usage:   singleWindowSummary("weekly", "Weekly Limit", 7*24*60*60, 5, fixedNow.AddDate(0, 0, 5)),
		},
	}

	selected, err := selectAutoActivation(results, fixedNow)
	require.NoError(t, err)
	require.Equal(t, "paidacct", selected.Account.ID)
}

// TestSelectAutoActivationSingleWindowNotPenalizedAgainstTwoWindows ensures a
// one-window plan is not outranked purely because another account reports more
// windows at the same headroom.
func TestSelectAutoActivationSingleWindowNotPenalizedAgainstTwoWindows(t *testing.T) {
	fixedNow := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

	single := singleWindowSummary("weekly", "Weekly Limit", 7*24*60*60, 10, fixedNow.AddDate(0, 0, 5))
	legacy := autoUsageSummary(60, fixedNow.Add(4*time.Hour), 60, fixedNow.AddDate(0, 0, 5))

	results := []UsageResult{
		{Account: domain.Account{ID: "legacy", DisplayName: "legacy"}, Usage: legacy},
		{Account: domain.Account{ID: "single", DisplayName: "single"}, Usage: single},
	}

	selected, err := selectAutoActivation(results, fixedNow)
	require.NoError(t, err)
	require.Equal(t, "single", selected.Account.ID)
}

func singleWindowSummary(name, display string, windowSeconds int64, used float64, resetsAt time.Time) domain.UsageSummary {
	return domain.UsageSummary{
		Confidence: domain.UsageConfidenceConfirmed,
		Quotas: []domain.UsageQuota{{
			Name:          name,
			DisplayName:   display,
			WindowSeconds: windowSeconds,
			UsedPercent:   autoFloatPtr(used),
			ResetsAt:      autoTimePtr(resetsAt),
		}},
	}
}

// TestSelectAutoActivationIgnoresCodeReviewQuota reproduces the regression the
// review caught: an exhausted review-request quota must not make an account
// with more model headroom lose to one with less.
func TestSelectAutoActivationIgnoresCodeReviewQuota(t *testing.T) {
	fixedNow := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

	roomy := autoUsageSummary(0, fixedNow.Add(4*time.Hour), 0, fixedNow.AddDate(0, 0, 5))
	roomy.Quotas[0].Category = domain.QuotaCategoryModel
	roomy.Quotas[1].Category = domain.QuotaCategoryModel
	roomy.Quotas = append(roomy.Quotas, domain.UsageQuota{
		Name: "code_review", DisplayName: "Review Requests",
		Category: domain.QuotaCategoryCodeReview, WindowSeconds: 86400,
		UsedPercent: autoFloatPtr(100), ResetsAt: autoTimePtr(fixedNow.AddDate(0, 0, 1)),
	})

	busier := autoUsageSummary(30, fixedNow.Add(4*time.Hour), 30, fixedNow.AddDate(0, 0, 5))

	results := []UsageResult{
		{Account: domain.Account{ID: "busier", DisplayName: "busier"}, Usage: busier},
		{Account: domain.Account{ID: "roomy", DisplayName: "roomy"}, Usage: roomy},
	}

	selected, err := selectAutoActivation(results, fixedNow)
	require.NoError(t, err)
	require.Equal(t, "roomy", selected.Account.ID)
}

// TestSelectAutoActivationSkipsBlockedAccount keeps auto off an account the API
// reports as rate limited even when its reported windows still show headroom.
func TestSelectAutoActivationSkipsBlockedAccount(t *testing.T) {
	fixedNow := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

	blocked := autoUsageSummary(0, fixedNow.Add(4*time.Hour), 0, fixedNow.AddDate(0, 0, 5))
	blocked.LimitReached = true

	results := []UsageResult{
		{Account: domain.Account{ID: "blocked", DisplayName: "blocked"}, Usage: blocked},
		{Account: domain.Account{ID: "usable", DisplayName: "usable"},
			Usage: autoUsageSummary(70, fixedNow.Add(4*time.Hour), 70, fixedNow.AddDate(0, 0, 5))},
	}

	selected, err := selectAutoActivation(results, fixedNow)
	require.NoError(t, err)
	require.Equal(t, "usable", selected.Account.ID)
}

// TestSelectAutoActivationRefusesWithoutUsageData covers the offline case. With
// no quota data anywhere every candidate ties at zero and the winner used to be
// whichever display name sorted first, silently moving the user off a working
// account.
func TestSelectAutoActivationRefusesWithoutUsageData(t *testing.T) {
	fixedNow := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)

	results := []UsageResult{
		{Account: domain.Account{ID: "aardvark", DisplayName: "aardvark"},
			Usage: domain.UsageSummary{Confidence: domain.UsageConfidenceUnknown}},
		{Account: domain.Account{ID: "zebra", DisplayName: "zebra"},
			Usage: domain.UsageSummary{Confidence: domain.UsageConfidenceBestEffort, PlanType: "pro"}},
	}

	_, err := selectAutoActivation(results, fixedNow)
	require.ErrorIs(t, err, errNoUsageData)

	// One account with real data is enough to choose again.
	results[1].Usage = singleWindowSummary("weekly", "Weekly Limit", 7*24*60*60, 10, fixedNow.AddDate(0, 0, 3))
	selected, err := selectAutoActivation(results, fixedNow)
	require.NoError(t, err)
	require.Equal(t, "zebra", selected.Account.ID)
}
