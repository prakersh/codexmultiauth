package domain

import "time"

type UsageConfidence string

const (
	UsageConfidenceConfirmed  UsageConfidence = "confirmed"
	UsageConfidenceBestEffort UsageConfidence = "best_effort"
	UsageConfidenceUnknown    UsageConfidence = "unknown"
)

type UsageQuota struct {
	Name        string     `json:"name"`
	DisplayName string     `json:"display_name"`
	UsedPercent *float64   `json:"used_percent,omitempty"`
	ResetsAt    *time.Time `json:"resets_at,omitempty"`
	Status      string     `json:"status,omitempty"`
}

type UsageSummary struct {
	PlanType    string          `json:"plan_type,omitempty"`
	Confidence  UsageConfidence `json:"confidence"`
	FetchedAt   time.Time       `json:"fetched_at"`
	CreditsLeft *float64        `json:"credits_left,omitempty"`
	Quotas      []UsageQuota    `json:"quotas,omitempty"`
}
