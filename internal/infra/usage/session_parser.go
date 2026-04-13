package usage

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
)

type AccountMetadata struct {
	AuthMode       string
	CodexAccountID string
	UserName       string
	UserEmail      string
}

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

func ExtractAccountMetadata(auth store.CodexAuth) AccountMetadata {
	metadata := AccountMetadata{
		AuthMode: strings.TrimSpace(auth.AuthMode),
	}
	if metadata.AuthMode == "" {
		switch {
		case strings.TrimSpace(auth.OpenAIAPIKey) != "":
			metadata.AuthMode = "api_key"
		case auth.Tokens != nil:
			metadata.AuthMode = "chatgpt"
		}
	}
	if auth.Tokens != nil {
		metadata.CodexAccountID = strings.TrimSpace(auth.Tokens.AccountID)
	}

	for _, token := range tokensForClaims(auth) {
		claims := parseJWTClaims(token)
		if len(claims) == 0 {
			continue
		}
		if metadata.UserEmail == "" {
			metadata.UserEmail = firstNonEmptyClaim(claims, "email", "preferred_username", "upn")
		}
		if metadata.UserName == "" {
			metadata.UserName = extractUserName(claims)
		}
		if metadata.UserEmail != "" && metadata.UserName != "" {
			break
		}
	}

	return metadata
}

func extractPlanType(auth store.CodexAuth) string {
	for _, token := range tokensForClaims(auth) {
		if plan := parsePlanTypeFromJWT(token); plan != "" {
			return plan
		}
	}
	return ""
}

func parsePlanTypeFromJWT(token string) string {
	claims := parseJWTClaims(token)
	authClaim, _ := claims["https://api.openai.com/auth"].(map[string]any)
	if plan, _ := authClaim["chatgpt_plan_type"].(string); plan != "" {
		return plan
	}
	if plan, _ := claims["chatgpt_plan_type"].(string); plan != "" {
		return plan
	}
	return ""
}

func tokensForClaims(auth store.CodexAuth) []string {
	if auth.Tokens == nil {
		return nil
	}
	return []string{auth.Tokens.IDToken, auth.Tokens.AccessToken}
}

func parseJWTClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
	}
	return claims
}

func firstNonEmptyClaim(claims map[string]any, keys ...string) string {
	for _, key := range keys {
		value, _ := claims[key].(string)
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func extractUserName(claims map[string]any) string {
	if name := firstNonEmptyClaim(claims, "name", "preferred_username", "nickname"); name != "" {
		return name
	}
	given := firstNonEmptyClaim(claims, "given_name")
	family := firstNonEmptyClaim(claims, "family_name")
	return strings.TrimSpace(given + " " + family)
}
