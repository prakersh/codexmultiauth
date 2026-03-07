package usage

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
)

func BestEffortSummary(auth store.CodexAuth) domain.UsageSummary {
	planType := extractPlanType(auth)
	if planType == "" {
		return domain.UsageSummary{
			Confidence: domain.UsageConfidenceUnknown,
			FetchedAt:  time.Now().UTC(),
		}
	}
	return domain.UsageSummary{
		PlanType:   planType,
		Confidence: domain.UsageConfidenceBestEffort,
		FetchedAt:  time.Now().UTC(),
	}
}

func extractPlanType(auth store.CodexAuth) string {
	if auth.Tokens == nil {
		return ""
	}
	for _, token := range []string{auth.Tokens.AccessToken, auth.Tokens.IDToken} {
		if token == "" {
			continue
		}
		if plan := parsePlanTypeFromJWT(token); plan != "" {
			return plan
		}
	}
	return ""
}

func parsePlanTypeFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	authClaim, _ := claims["https://api.openai.com/auth"].(map[string]any)
	if plan, _ := authClaim["chatgpt_plan_type"].(string); plan != "" {
		return plan
	}
	if plan, _ := claims["chatgpt_plan_type"].(string); plan != "" {
		return plan
	}
	return ""
}
