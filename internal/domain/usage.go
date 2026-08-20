package domain

import "time"

type UsageConfidence string

const (
	UsageConfidenceConfirmed  UsageConfidence = "confirmed"
	UsageConfidenceBestEffort UsageConfidence = "best_effort"
	UsageConfidenceUnknown    UsageConfidence = "unknown"
)

// Quota categories distinguish limits that gate model usage from limits on
// other features. Auto activation scores only model limits, so an exhausted
// review-request quota never makes an account look busier than it is.
const (
	QuotaCategoryModel      = "model"
	QuotaCategoryCodeReview = "code_review"
	QuotaCategoryOther      = "other"
)

type UsageQuota struct {
	Name        string     `json:"name"`
	DisplayName string     `json:"display_name"`
	UsedPercent *float64   `json:"used_percent,omitempty"`
	ResetsAt    *time.Time `json:"resets_at,omitempty"`
	Status      string     `json:"status,omitempty"`
	// WindowSeconds is the length of the rolling limit window reported by the
	// API. Codex plans no longer share a fixed window shape, so renderers and
	// scoring derive everything from this value instead of assuming 5h/weekly.
	WindowSeconds int64 `json:"window_seconds,omitempty"`
	// Category is one of the QuotaCategory constants. An empty value is
	// treated as a model limit so summaries produced before this field
	// existed keep their original scoring behavior.
	Category string `json:"category,omitempty"`
}

// IsModelLimit reports whether this quota gates model usage.
func (q UsageQuota) IsModelLimit() bool {
	return q.Category == "" || q.Category == QuotaCategoryModel
}

type UsageSummary struct {
	PlanType     string          `json:"plan_type,omitempty"`
	Confidence   UsageConfidence `json:"confidence"`
	FetchedAt    time.Time       `json:"fetched_at"`
	CreditsLeft  *float64        `json:"credits_left,omitempty"`
	Quotas       []UsageQuota    `json:"quotas,omitempty"`
	LimitReached bool            `json:"limit_reached,omitempty"`
}
