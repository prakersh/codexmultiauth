package usage

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/prakersh/codexmultiauth/internal/domain"
)

type response struct {
	PlanType            string     `json:"plan_type"`
	RateLimit           rateLimit  `json:"rate_limit"`
	CodeReviewRateLimit rateLimit  `json:"code_review_rate_limit,omitempty"`
	Credits             *credits   `json:"credits,omitempty"`
}

type rateLimit struct {
	PrimaryWindow   *window `json:"primary_window"`
	SecondaryWindow *window `json:"secondary_window"`
}

type window struct {
	UsedPercent        float64 `json:"used_percent"`
	ResetAtUnix        int64   `json:"reset_at"`
	LimitWindowSeconds int64   `json:"limit_window_seconds"`
}

type credits struct {
	Balance *float64 `json:"balance,omitempty"`
}

func ParseResponse(data []byte) (domain.UsageSummary, error) {
	var resp response
	if err := json.Unmarshal(data, &resp); err != nil {
		return domain.UsageSummary{}, fmt.Errorf("parse usage response: %w", err)
	}
	summary := domain.UsageSummary{
		PlanType:   resp.PlanType,
		Confidence: domain.UsageConfidenceConfirmed,
		FetchedAt:  time.Now().UTC(),
	}
	if resp.Credits != nil {
		summary.CreditsLeft = resp.Credits.Balance
	}
	if resp.RateLimit.PrimaryWindow != nil {
		summary.Quotas = append(summary.Quotas, quotaFromWindow(primaryName(resp), resp.RateLimit.PrimaryWindow))
	}
	if resp.RateLimit.SecondaryWindow != nil {
		summary.Quotas = append(summary.Quotas, quotaFromWindow("seven_day", resp.RateLimit.SecondaryWindow))
	}
	if resp.CodeReviewRateLimit.PrimaryWindow != nil {
		summary.Quotas = append(summary.Quotas, quotaFromWindow("code_review", resp.CodeReviewRateLimit.PrimaryWindow))
	}
	sort.Slice(summary.Quotas, func(i, j int) bool {
		return summary.Quotas[i].Name < summary.Quotas[j].Name
	})
	return summary, nil
}

func primaryName(resp response) string {
	if resp.RateLimit.SecondaryWindow != nil {
		return "five_hour"
	}
	if resp.RateLimit.PrimaryWindow != nil && resp.RateLimit.PrimaryWindow.LimitWindowSeconds >= 7*24*60*60 {
		return "seven_day"
	}
	return "five_hour"
}

func quotaFromWindow(name string, w *window) domain.UsageQuota {
	used := w.UsedPercent
	quota := domain.UsageQuota{
		Name:        name,
		DisplayName: displayName(name),
		UsedPercent: &used,
		Status:      statusFromPercent(used),
	}
	if w.ResetAtUnix > 0 {
		reset := time.Unix(w.ResetAtUnix, 0).UTC()
		quota.ResetsAt = &reset
	}
	return quota
}

func displayName(name string) string {
	switch name {
	case "five_hour":
		return "5-Hour Limit"
	case "seven_day":
		return "Weekly All-Model"
	case "code_review":
		return "Review Requests"
	default:
		return name
	}
}

func statusFromPercent(percent float64) string {
	switch {
	case percent >= 95:
		return "critical"
	case percent >= 80:
		return "danger"
	case percent >= 50:
		return "warning"
	default:
		return "healthy"
	}
}
