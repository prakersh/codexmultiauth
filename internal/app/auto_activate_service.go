package app

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/prakersh/codexmultiauth/internal/domain"
)

var errNoSavedAccounts = errors.New("no saved accounts")

const (
	autoFiveHourWindow = 5 * time.Hour
	autoWeeklyWindow   = 7 * 24 * time.Hour
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
	leftScore := scoreAutoCandidate(left, now)
	rightScore := scoreAutoCandidate(right, now)

	if diff := compareAutoScoreValue(leftScore.total, rightScore.total); diff != 0 {
		return diff
	}
	if diff := compareAutoScoreValue(leftScore.weeklyScore, rightScore.weeklyScore); diff != 0 {
		return diff
	}
	if diff := compareAutoScoreValue(leftScore.fiveHourScore, rightScore.fiveHourScore); diff != 0 {
		return diff
	}
	if diff := compareAutoScoreValue(leftScore.weeklyAvailable, rightScore.weeklyAvailable); diff != 0 {
		return diff
	}
	if diff := compareAutoScoreValue(leftScore.fiveHourAvailable, rightScore.fiveHourAvailable); diff != 0 {
		return diff
	}
	if diff := compareAutoResetPoints(leftScore.weeklyReset, rightScore.weeklyReset); diff != 0 {
		return diff
	}
	if diff := compareAutoResetPoints(leftScore.fiveHourReset, rightScore.fiveHourReset); diff != 0 {
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

type autoCandidateScore struct {
	total             float64
	weeklyScore       float64
	fiveHourScore     float64
	weeklyAvailable   float64
	fiveHourAvailable float64
	knownQuotaCount   float64
	weeklyReset       autoResetPoint
	fiveHourReset     autoResetPoint
}

type autoResetPoint struct {
	known bool
	at    time.Time
}

func scoreAutoCandidate(result UsageResult, now time.Time) autoCandidateScore {
	fiveHourMetric := autoQuotaMetricFor(result.Usage.Quotas, autoQuotaFiveHour)
	weeklyMetric := autoQuotaMetricFor(result.Usage.Quotas, autoQuotaWeekly)

	fiveHourScore, fiveHourAvailable, fiveHourKnown, fiveHourReset := scoreAutoQuota(fiveHourMetric, now, autoFiveHourWindow)
	weeklyScore, weeklyAvailable, weeklyKnown, weeklyReset := scoreAutoQuota(weeklyMetric, now, autoWeeklyWindow)

	return autoCandidateScore{
		total:             weeklyScore + fiveHourScore,
		weeklyScore:       weeklyScore,
		fiveHourScore:     fiveHourScore,
		weeklyAvailable:   weeklyAvailable,
		fiveHourAvailable: fiveHourAvailable,
		knownQuotaCount:   boolToFloat(weeklyKnown) + boolToFloat(fiveHourKnown),
		weeklyReset:       weeklyReset,
		fiveHourReset:     fiveHourReset,
	}
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

	if !metric.hasResetAt {
		return score, available, known, autoResetPoint{}
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

type autoQuotaKind int

const (
	autoQuotaFiveHour autoQuotaKind = iota
	autoQuotaWeekly
)

type autoQuotaMetric struct {
	hasUsedPercent bool
	usedPercent    float64
	hasResetAt     bool
	resetAt        time.Time
}

func autoQuotaMetricFor(quotas []domain.UsageQuota, kind autoQuotaKind) autoQuotaMetric {
	for _, quota := range quotas {
		if !matchesAutoQuotaKind(quota, kind) {
			continue
		}

		metric := autoQuotaMetric{}
		if quota.UsedPercent != nil {
			metric.hasUsedPercent = true
			metric.usedPercent = *quota.UsedPercent
		}
		if quota.ResetsAt != nil && !quota.ResetsAt.IsZero() {
			metric.hasResetAt = true
			metric.resetAt = quota.ResetsAt.UTC()
		}
		return metric
	}
	return autoQuotaMetric{}
}

func matchesAutoQuotaKind(quota domain.UsageQuota, kind autoQuotaKind) bool {
	nameLower := strings.ToLower(quota.Name)
	displayLower := strings.ToLower(quota.DisplayName)

	switch kind {
	case autoQuotaFiveHour:
		return containsFiveHourQuota(nameLower) || containsFiveHourQuota(displayLower)
	case autoQuotaWeekly:
		return strings.Contains(nameLower, "weekly") || strings.Contains(displayLower, "weekly")
	default:
		return false
	}
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

func boolToFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
