package app

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/prakersh/codexmultiauth/internal/domain"
)

var errNoSavedAccounts = errors.New("no saved accounts")

const (
	autoFiveHourWindow = 5 * time.Hour
	autoWeeklyWindow   = 7 * 24 * time.Hour
	autoMonthlyWindow  = 30 * 24 * time.Hour
	autoMinRemaining   = time.Minute
	autoScoreTolerance = 1e-9
)

func (m *Manager) AutoActivate(ctx context.Context) (domain.Account, error) {
	results, err := m.Usage(ctx, "all")
	if err != nil {
		return domain.Account{}, err
	}

	selected, err := selectAutoActivation(results, m.now().UTC())
	if err != nil {
		return domain.Account{}, err
	}

	return m.Activate(ctx, selected.Account.ID)
}

func selectAutoActivation(results []UsageResult, now time.Time) (UsageResult, error) {
	if len(results) == 0 {
		return UsageResult{}, errNoSavedAccounts
	}

	best := results[0]
	for _, candidate := range results[1:] {
		if compareAutoCandidates(candidate, best, now) < 0 {
			best = candidate
		}
	}
	return best, nil
}

func compareAutoCandidates(left, right UsageResult, now time.Time) int {
	// An account the API reports as blocked is unusable now, whatever headroom
	// its reported windows show; the limit that tripped may be one it does not
	// report back to us.
	if left.Usage.LimitReached != right.Usage.LimitReached {
		if right.Usage.LimitReached {
			return -1
		}
		return 1
	}

	leftScore := scoreAutoCandidate(left, now)
	rightScore := scoreAutoCandidate(right, now)

	if diff := compareAutoScoreValue(leftScore.total, rightScore.total); diff != 0 {
		return diff
	}
	if diff := compareAutoQuotaScores(leftScore, rightScore); diff != 0 {
		return diff
	}
	if diff := compareAutoScoreValue(leftScore.knownQuotaCount, rightScore.knownQuotaCount); diff != 0 {
		return diff
	}
	if diff := strings.Compare(left.Account.DisplayName, right.Account.DisplayName); diff != 0 {
		return diff
	}
	return strings.Compare(left.Account.ID, right.Account.ID)
}

// autoCandidateScore summarizes an account's remaining headroom. Codex no
// longer gives every plan the same window shape (free accounts report a single
// monthly window, paid accounts a weekly one), so scoring walks whatever
// windows an account actually reports instead of looking for a fixed 5h/weekly
// pair.
type autoCandidateScore struct {
	total           float64
	knownQuotaCount float64
	// quotas are ordered longest window first, so tie-breaking compares the
	// scarcest, slowest-refilling limit before shorter ones.
	quotas []autoQuotaScore
}

type autoQuotaScore struct {
	windowSeconds int64
	score         float64
	available     float64
	known         bool
	reset         autoResetPoint
}

type autoResetPoint struct {
	known bool
	at    time.Time
}

func scoreAutoCandidate(result UsageResult, now time.Time) autoCandidateScore {
	metrics := autoQuotaMetrics(result.Usage.Quotas)

	score := autoCandidateScore{quotas: make([]autoQuotaScore, 0, len(metrics))}
	var sum float64
	for _, metric := range metrics {
		quotaScore, available, known, reset := scoreAutoQuota(metric, now, metric.window)
		score.quotas = append(score.quotas, autoQuotaScore{
			windowSeconds: int64(metric.window / time.Second),
			score:         quotaScore,
			available:     available,
			known:         known,
			reset:         reset,
		})
		if known {
			sum += quotaScore
			score.knownQuotaCount++
		}
	}

	// Averaging rather than summing keeps accounts comparable when they report
	// different numbers of windows; a plan with two windows should not outrank
	// an equally healthy plan that only reports one.
	if score.knownQuotaCount > 0 {
		score.total = sum / score.knownQuotaCount
	}

	sort.SliceStable(score.quotas, func(i, j int) bool {
		return score.quotas[i].windowSeconds > score.quotas[j].windowSeconds
	})
	return score
}

func compareAutoQuotaScores(left, right autoCandidateScore) int {
	count := len(left.quotas)
	if len(right.quotas) < count {
		count = len(right.quotas)
	}
	for i := 0; i < count; i++ {
		if diff := compareAutoScoreValue(left.quotas[i].score, right.quotas[i].score); diff != 0 {
			return diff
		}
	}
	for i := 0; i < count; i++ {
		if diff := compareAutoScoreValue(left.quotas[i].available, right.quotas[i].available); diff != 0 {
			return diff
		}
	}
	for i := 0; i < count; i++ {
		if diff := compareAutoResetPoints(left.quotas[i].reset, right.quotas[i].reset); diff != 0 {
			return diff
		}
	}
	return 0
}

func scoreAutoQuota(metric autoQuotaMetric, now time.Time, window time.Duration) (score, available float64, known bool, reset autoResetPoint) {
	if !metric.hasUsedPercent {
		if metric.hasResetAt {
			reset = autoResetPoint{known: true, at: metric.resetAt}
		}
		return 0, 0, false, reset
	}

	available = clampPercent(100 - metric.usedPercent)
	score = available
	known = true

	if !metric.hasResetAt || window <= 0 {
		if metric.hasResetAt {
			reset = autoResetPoint{known: true, at: metric.resetAt}
		}
		return score, available, known, reset
	}

	reset = autoResetPoint{known: true, at: metric.resetAt}

	remaining := metric.resetAt.Sub(now)
	if remaining < autoMinRemaining {
		remaining = autoMinRemaining
	}

	urgencyRatio := window.Hours() / remaining.Hours()
	if urgencyRatio < 1 {
		urgencyRatio = 1
	}

	score = available * (1 + math.Log(urgencyRatio))
	return score, available, known, reset
}

type autoQuotaMetric struct {
	hasUsedPercent bool
	usedPercent    float64
	hasResetAt     bool
	resetAt        time.Time
	window         time.Duration
}

// autoQuotaMetrics keeps only the limits that gate model usage. A spent
// review-request or other feature quota must not make an account look busier
// than it is for the purpose of choosing where to run next.
func autoQuotaMetrics(quotas []domain.UsageQuota) []autoQuotaMetric {
	metrics := make([]autoQuotaMetric, 0, len(quotas))
	for _, quota := range quotas {
		if !quota.IsModelLimit() || isNonModelQuotaName(quota) {
			continue
		}
		metric := autoQuotaMetric{window: autoQuotaWindow(quota)}
		if quota.UsedPercent != nil {
			metric.hasUsedPercent = true
			metric.usedPercent = *quota.UsedPercent
		}
		if quota.ResetsAt != nil && !quota.ResetsAt.IsZero() {
			metric.hasResetAt = true
			metric.resetAt = quota.ResetsAt.UTC()
		}
		metrics = append(metrics, metric)
	}
	return metrics
}

// autoQuotaWindow prefers the window length the API reported and falls back to
// reading it off the quota label, so summaries produced before WindowSeconds
// existed still score with the right urgency weighting.
func autoQuotaWindow(quota domain.UsageQuota) time.Duration {
	if quota.WindowSeconds > 0 {
		return time.Duration(quota.WindowSeconds) * time.Second
	}
	for _, value := range []string{strings.ToLower(quota.Name), strings.ToLower(quota.DisplayName)} {
		switch {
		case containsFiveHourQuota(value):
			return autoFiveHourWindow
		case strings.Contains(value, "weekly") || strings.Contains(value, "seven_day") || strings.Contains(value, "7_day"):
			return autoWeeklyWindow
		case strings.Contains(value, "monthly") || strings.Contains(value, "30_day"):
			return autoMonthlyWindow
		}
	}
	return 0
}

// isNonModelQuotaName catches review quotas in summaries built before the
// Category field existed, matching the quota kinds the previous scorer
// deliberately ignored.
func isNonModelQuotaName(quota domain.UsageQuota) bool {
	for _, value := range []string{strings.ToLower(quota.Name), strings.ToLower(quota.DisplayName)} {
		if strings.Contains(value, "code_review") || strings.Contains(value, "review request") {
			return true
		}
	}
	return false
}

func containsFiveHourQuota(value string) bool {
	return strings.Contains(value, "5") && strings.Contains(value, "hour")
}

func compareAutoScoreValue(left, right float64) int {
	switch {
	case math.Abs(left-right) <= autoScoreTolerance:
		return 0
	case left > right:
		return -1
	default:
		return 1
	}
}

func compareAutoResetPoints(left, right autoResetPoint) int {
	switch {
	case left.known && !right.known:
		return -1
	case !left.known && right.known:
		return 1
	case !left.known && !right.known:
		return 0
	case left.at.Before(right.at):
		return -1
	case right.at.Before(left.at):
		return 1
	default:
		return 0
	}
}

func clampPercent(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 100:
		return 100
	default:
		return value
	}
}
