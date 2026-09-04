package usage

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/prakersh/codexmultiauth/internal/domain"
)

type response struct {
	PlanType             string          `json:"plan_type"`
	RateLimit            rateLimit       `json:"rate_limit"`
	CodeReviewRateLimit  *rateLimit      `json:"code_review_rate_limit,omitempty"`
	AdditionalRateLimits json.RawMessage `json:"additional_rate_limits,omitempty"`
	Credits              *credits        `json:"credits,omitempty"`
	RateLimitReachedType reachedType     `json:"rate_limit_reached_type,omitempty"`
}

// reachedType carries rate_limit_reached_type, which the API sends in more
// than one shape: a bare string on individual plans, and an object of the form
// {"type": "...", "details": ...} on workspace plans. Decoding it as a plain
// string made the whole usage response fail to unmarshal for workspace
// accounts, so a fully populated payload was discarded and the account
// degraded to best-effort with no quota data.
//
// An unrecognized shape is ignored rather than failing the parse. Rejecting
// the whole response over one advisory field is the bug this type exists to
// fix, so a future shape must not reintroduce it.
type reachedType struct {
	Type    string
	Details json.RawMessage
}

func (r *reachedType) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var asString string
	if err := json.Unmarshal(data, &asString); err == nil {
		r.Type = asString
		return nil
	}
	var asObject struct {
		Type    string          `json:"type"`
		Details json.RawMessage `json:"details,omitempty"`
	}
	if err := json.Unmarshal(data, &asObject); err == nil {
		r.Type = asObject.Type
		r.Details = asObject.Details
		return nil
	}
	return nil
}

type rateLimit struct {
	Allowed         *bool   `json:"allowed,omitempty"`
	LimitReached    bool    `json:"limit_reached,omitempty"`
	PrimaryWindow   *window `json:"primary_window"`
	SecondaryWindow *window `json:"secondary_window"`
}

type window struct {
	UsedPercent        float64 `json:"used_percent"`
	ResetAtUnix        int64   `json:"reset_at"`
	ResetAfterSeconds  int64   `json:"reset_after_seconds"`
	LimitWindowSeconds int64   `json:"limit_window_seconds"`
}

type credits struct {
	Balance   json.RawMessage `json:"balance,omitempty"`
	Unlimited bool            `json:"unlimited,omitempty"`
}

// Window length constants for the limit tiers Codex currently issues. Free
// plans report a single monthly window; paid plans report a weekly window.
// The legacy 5-hour window is still recognized so older cached payloads and
// any account still receiving one keep rendering correctly.
// Bands rather than exact lengths: the API has reported calendar-month
// windows as both 30 and 31 days, and matching exactly would split accounts on
// the same tier into separate columns.
var windowTiers = []struct {
	name string
	min  int64
	max  int64
}{
	{"five_hour", 4 * 60 * 60, 6 * 60 * 60},
	{"weekly", 6 * 24 * 60 * 60, 8 * 24 * 60 * 60},
	{"monthly", 28 * 24 * 60 * 60, 31 * 24 * 60 * 60},
}

func ParseResponse(data []byte) (domain.UsageSummary, error) {
	var resp response
	if err := json.Unmarshal(data, &resp); err != nil {
		return domain.UsageSummary{}, fmt.Errorf("parse usage response: %w", err)
	}
	summary := domain.UsageSummary{
		PlanType:     resp.PlanType,
		Confidence:   domain.UsageConfidenceConfirmed,
		FetchedAt:    time.Now().UTC(),
		LimitReached: resp.RateLimit.LimitReached || strings.TrimSpace(resp.RateLimitReachedType.Type) != "",
	}
	if resp.RateLimit.Allowed != nil && !*resp.RateLimit.Allowed {
		summary.LimitReached = true
	}
	if resp.Credits != nil {
		summary.CreditsLeft = parseFlexibleFloat(resp.Credits.Balance)
	}

	// Each window is classified by its own limit_window_seconds rather than by
	// whether it arrived as the primary or secondary slot. Codex now varies
	// which tiers an account gets (free: monthly only, paid: weekly only), so
	// slot position no longer implies window length.
	slots := []struct {
		w *window
		// fallback names the slot when the API omits limit_window_seconds,
		// preserving the historical primary=5h / secondary=weekly reading.
		fallback string
	}{
		{resp.RateLimit.PrimaryWindow, "five_hour"},
		{resp.RateLimit.SecondaryWindow, "weekly"},
	}
	for _, slot := range slots {
		if slot.w == nil {
			continue
		}
		quota := quotaFromWindow(windowName(slot.w, slot.fallback), slot.w)
		quota.Category = domain.QuotaCategoryModel
		summary.Quotas = append(summary.Quotas, quota)
	}
	if resp.CodeReviewRateLimit != nil && resp.CodeReviewRateLimit.PrimaryWindow != nil {
		review := quotaFromWindow("code_review", resp.CodeReviewRateLimit.PrimaryWindow)
		review.Category = domain.QuotaCategoryCodeReview
		summary.Quotas = append(summary.Quotas, review)
	}
	summary.Quotas = append(summary.Quotas, parseAdditionalLimits(resp.AdditionalRateLimits)...)

	summary.Quotas = disambiguateQuotas(summary.Quotas)
	// Shortest window first so renderers show the most immediately binding
	// limit on the left; unknown-length windows sort last by name.
	sort.SliceStable(summary.Quotas, func(i, j int) bool {
		left, right := summary.Quotas[i], summary.Quotas[j]
		if left.WindowSeconds != right.WindowSeconds {
			if left.WindowSeconds == 0 {
				return false
			}
			if right.WindowSeconds == 0 {
				return true
			}
			return left.WindowSeconds < right.WindowSeconds
		}
		return left.Name < right.Name
	})
	return summary, nil
}

// windowName derives a stable slug from the window length. Anything that is
// not one of the known tiers gets a generated slug so unfamiliar windows are
// still surfaced instead of being silently mislabeled.
func windowName(w *window, fallback string) string {
	seconds := w.LimitWindowSeconds
	if seconds <= 0 {
		if fallback != "" {
			return fallback
		}
		return "limit"
	}
	for _, tier := range windowTiers {
		if seconds >= tier.min && seconds <= tier.max {
			return tier.name
		}
	}
	switch {
	case seconds%(24*60*60) == 0:
		return fmt.Sprintf("%d_day", seconds/(24*60*60))
	case seconds%(60*60) == 0:
		return fmt.Sprintf("%d_hour", seconds/(60*60))
	default:
		return fmt.Sprintf("%d_second", seconds)
	}
}

func quotaFromWindow(name string, w *window) domain.UsageQuota {
	used := w.UsedPercent
	quota := domain.UsageQuota{
		Name:          name,
		DisplayName:   displayName(name),
		UsedPercent:   &used,
		Status:        statusFromPercent(used),
		WindowSeconds: w.LimitWindowSeconds,
	}
	if w.ResetAtUnix > 0 {
		reset := time.Unix(w.ResetAtUnix, 0).UTC()
		quota.ResetsAt = &reset
	} else if w.ResetAfterSeconds > 0 {
		reset := time.Now().UTC().Add(time.Duration(w.ResetAfterSeconds) * time.Second)
		quota.ResetsAt = &reset
	}
	return quota
}

func displayName(name string) string {
	switch name {
	case "five_hour":
		return "5-Hour Limit"
	case "weekly":
		return "Weekly Limit"
	case "monthly":
		return "Monthly Limit"
	case "code_review":
		return "Review Requests"
	case "limit":
		return "Limit"
	}
	if label, ok := durationLabelFromSlug(name); ok {
		return label
	}
	return humanizeSlug(name)
}

// durationLabelFromSlug turns generated slugs such as "3_day" or "12_hour"
// into a readable column heading.
func durationLabelFromSlug(name string) (string, bool) {
	parts := strings.Split(name, "_")
	if len(parts) != 2 {
		return "", false
	}
	count, err := strconv.Atoi(parts[0])
	if err != nil || count <= 0 {
		return "", false
	}
	var unit string
	switch parts[1] {
	case "day":
		unit = "Day"
	case "hour":
		unit = "Hour"
	case "second":
		unit = "Second"
	default:
		return "", false
	}
	// "3-Day Limit", not "3-Days Limit": the unit is a compound adjective here.
	return fmt.Sprintf("%d-%s Limit", count, unit), true
}

func humanizeSlug(name string) string {
	fields := strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' })
	if len(fields) == 0 {
		return name
	}
	for i, field := range fields {
		first, size := utf8.DecodeRuneInString(field)
		if first == utf8.RuneError {
			continue
		}
		fields[i] = string(unicode.ToUpper(first)) + field[size:]
	}
	return strings.Join(fields, " ")
}

// disambiguateQuotas makes quota names unique. Two windows of the same length
// can arrive in different slots with different usage; suffixing keeps both
// rather than discarding one and under-reporting the account.
func disambiguateQuotas(quotas []domain.UsageQuota) []domain.UsageQuota {
	seen := make(map[string]int, len(quotas))
	for i, quota := range quotas {
		count := seen[quota.Name]
		seen[quota.Name] = count + 1
		if count == 0 {
			continue
		}
		suffixed := fmt.Sprintf("%s_%d", quota.Name, count+1)
		quotas[i].Name = suffixed
		quotas[i].DisplayName = fmt.Sprintf("%s (%d)", quota.DisplayName, count+1)
	}
	return quotas
}

// parseAdditionalLimits reads the additional_rate_limits field, whose shape
// the API has not committed to. It accepts a keyed object or a list, and skips
// any entry that carries none of the fields a window is made of. Decoding
// permissively would mint a quota reading "0.0% used, healthy" for a limit
// whose real usage simply lives under a key we do not recognize.
func parseAdditionalLimits(raw json.RawMessage) []domain.UsageQuota {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	var keyed map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keyed); err == nil {
		keys := make([]string, 0, len(keyed))
		for key := range keyed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		quotas := make([]domain.UsageQuota, 0, len(keys))
		for _, key := range keys {
			w, ok := decodeWindow(keyed[key])
			if !ok {
				continue
			}
			// The map key is the only human-meaningful label available, so it
			// wins over the length-derived slug; otherwise "cloud_tasks" would
			// surface as an indistinguishable second "Weekly Limit" column.
			quota := quotaFromWindow(key, w)
			quota.Category = domain.QuotaCategoryOther
			quotas = append(quotas, quota)
		}
		return quotas
	}

	var listed []json.RawMessage
	if err := json.Unmarshal(raw, &listed); err == nil {
		quotas := make([]domain.UsageQuota, 0, len(listed))
		for _, entry := range listed {
			w, ok := decodeWindow(entry)
			if !ok {
				continue
			}
			quota := quotaFromWindow(windowName(w, ""), w)
			quota.Category = domain.QuotaCategoryOther
			quotas = append(quotas, quota)
		}
		return quotas
	}

	return nil
}

// decodeWindow accepts a window only when it actually carries window data.
// A struct with no required fields would otherwise absorb any JSON object and
// report zeros as though they were measurements.
func decodeWindow(raw json.RawMessage) (*window, bool) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, false
	}
	_, hasUsed := probe["used_percent"]
	_, hasLength := probe["limit_window_seconds"]
	if !hasUsed && !hasLength {
		return nil, false
	}
	var w window
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, false
	}
	return &w, true
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

func parseFlexibleFloat(raw json.RawMessage) *float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}

	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		return &number
	}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	number, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil
	}
	return &number
}
