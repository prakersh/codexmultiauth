package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	infrausage "github.com/prakersh/codexmultiauth/internal/infra/usage"
)

type UsageResult struct {
	Account domain.Account
	Usage   domain.UsageSummary
	Info    UsageAccountInfo
	Problem *UsageProblem
}

// UsageProblem contains public diagnostics, never arbitrary provider error text.
type UsageProblem struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	StatusCode    int    `json:"status_code,omitempty"`
	RequiresLogin bool   `json:"requires_login,omitempty"`
}

var errUsageAuthChanged = errors.New("account credentials changed during usage request")

type UsageAccountInfo struct {
	IsActive       bool
	AuthMode       string
	CodexAccountID string
	UserName       string
	UserEmail      string
}

func (m *Manager) Usage(ctx context.Context, selector string) ([]UsageResult, error) {
	state, vault, _, err := m.loadStateAndVault(ctx)
	if err != nil {
		return nil, err
	}
	accounts, err := domain.ResolveAccounts(state.Accounts, selector)
	if err != nil {
		return nil, err
	}
	activeAccountID := m.activeAccountID(ctx, state)

	results := make([]UsageResult, len(accounts))
	var wg sync.WaitGroup
	var firstErr error
	var mu sync.Mutex

	for index, account := range accounts {
		wg.Add(1)
		go func(i int, account domain.Account) {
			defer wg.Done()

			// Resolve the auth for this account through the single
			// refresh-and-persist path so concurrent usage calls never
			// double-refresh and callers outside Usage share the same logic.
			auth, _, refreshErr := m.ensureFreshAuth(ctx, account.ID, freshAuthIfExpiring)
			if !usableUsageAuth(auth) {
				// A failed refresh can still return newer live credentials.
				// Reload only when no usable auth was returned, and prefer the
				// current authoritative store before the original vault snapshot.
				if current, loadErr := m.loadAccountAuth(ctx, account.ID); loadErr == nil {
					auth = current
				}
			}
			if !usableUsageAuth(auth) {
				entry, ok := findVaultEntry(vault, account.ID)
				if !ok {
					mu.Lock()
					if firstErr == nil {
						firstErr = errors.New("vault entry missing for usage")
					}
					mu.Unlock()
					return
				}
				fallback, _, parseErr := store.NormalizeAndValidateAuth(entry.Payload)
				if parseErr != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = parseErr
					}
					mu.Unlock()
					return
				}
				auth = fallback
			}

			summary := infrausage.BestEffortSummary(auth)
			var problem *UsageProblem
			if refreshErr != nil {
				problem = newUsageProblem(refreshErr, "refresh")
			}
			if m.usage != nil {
				previousAuth := auth
				resolvedAuth, fetched, fetchErr := m.fetchUsageWithRecovery(ctx, account.ID, auth, refreshErr)
				auth = resolvedAuth
				if fetchErr == nil {
					summary = fetched
					if !sameAuthCredentials(previousAuth, auth) {
						problem = nil // a changed credential state has recovered
					}
				} else {
					summary = infrausage.BestEffortSummary(auth)
					// Keep a permanent refresh rejection as the actionable cause
					// rather than replacing it with the subsequent usage denial.
					if problem == nil || !problem.RequiresLogin {
						problem = newUsageProblem(fetchErr, "usage")
					}
				}
			}
			metadata := infrausage.ExtractAccountMetadata(auth)
			results[i] = UsageResult{
				Account: account,
				Usage:   summary,
				Problem: problem,
				Info: UsageAccountInfo{
					IsActive:       account.ID == activeAccountID,
					AuthMode:       metadata.AuthMode,
					CodexAccountID: metadata.CodexAccountID,
					UserName:       metadata.UserName,
					UserEmail:      metadata.UserEmail,
				},
			}
		}(index, account)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}

func usableUsageAuth(auth store.CodexAuth) bool {
	return auth.OpenAIAPIKey != "" || (auth.Tokens != nil && auth.Tokens.AccessToken != "")
}

// Recovery is bounded to a matching-store reload and one OAuth refresh. A
// rejection must not erase another account's credentials or loop indefinitely.
func (m *Manager) fetchUsageWithRecovery(ctx context.Context, accountID string, auth store.CodexAuth, refreshErr error) (store.CodexAuth, domain.UsageSummary, error) {
	summary, err := m.usage.Fetch(ctx, auth)
	if !infrausage.IsUnauthorized(err) {
		return auth, summary, err
	}
	if latest, loadErr := m.loadAccountAuth(ctx, accountID); loadErr == nil && !sameAuthCredentials(auth, latest) {
		if !sameUsageIdentity(auth, latest) {
			return auth, domain.UsageSummary{}, errUsageAuthChanged
		}
		auth = latest
		summary, err = m.usage.Fetch(ctx, auth)
		if !infrausage.IsUnauthorized(err) {
			return auth, summary, err
		}
	}
	var rejectedRefresh *infrausage.RequestError
	if errors.As(refreshErr, &rejectedRefresh) && rejectedRefresh.RequiresLogin() {
		return auth, domain.UsageSummary{}, refreshErr
	}
	if m.tokenRefresher == nil {
		return auth, summary, err
	}
	refreshed, _, forceErr := m.ensureFreshAuth(ctx, accountID, freshAuthForce)
	if forceErr != nil {
		return auth, domain.UsageSummary{}, forceErr
	}
	if !usableUsageAuth(refreshed) || sameAuthCredentials(auth, refreshed) || !sameUsageIdentity(auth, refreshed) {
		return auth, summary, err
	}
	auth = refreshed
	summary, err = m.usage.Fetch(ctx, auth)
	return auth, summary, err
}

func sameAuthCredentials(left, right store.CodexAuth) bool {
	_, leftFingerprint, leftErr := canonicalizeAuth(left)
	_, rightFingerprint, rightErr := canonicalizeAuth(right)
	return leftErr == nil && rightErr == nil && leftFingerprint == rightFingerprint
}

func sameUsageIdentity(left, right store.CodexAuth) bool {
	leftRaw, _, leftErr := canonicalizeAuth(left)
	rightRaw, _, rightErr := canonicalizeAuth(right)
	return leftErr == nil && rightErr == nil && sameCodexIdentity(leftRaw, rightRaw)
}

func newUsageProblem(err error, stage string) *UsageProblem {
	problem := &UsageProblem{Code: stage + "_failed", Message: "Could not fetch live usage; retry later."}
	if errors.Is(err, errUsageAuthChanged) {
		return &UsageProblem{Code: "credentials_changed", Message: "Account credentials changed during the request; retry."}
	}
	if stage == "refresh" {
		problem.Message = "Could not renew credentials; retry later."
	}
	var requestErr *infrausage.RequestError
	if errors.As(err, &requestErr) {
		if requestErr.Operation == "refresh" {
			stage = "refresh"
		}
		problem.Code = requestErr.ErrorCode()
		if problem.Code == "" {
			problem.Code = fmt.Sprintf("%s_http_%d", stage, requestErr.StatusCode)
		}
		problem.StatusCode = requestErr.StatusCode
		problem.RequiresLogin = requestErr.RequiresLogin()
		problem.Message = requestErr.Error()
		if problem.RequiresLogin {
			problem.Message += "; sign in again with cma login."
		} else if requestErr.StatusCode == 403 {
			problem.Message += "; access denied for this account."
		}
		return problem
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		problem.Code = stage + "_timeout"
		problem.Message = "Request timed out; retry later."
	}
	return problem
}

func canonicalizeAuth(auth store.CodexAuth) ([]byte, string, error) {
	raw, err := json.Marshal(auth)
	if err != nil {
		return nil, "", err
	}
	_, canonical, err := store.NormalizeAndValidateAuth(raw)
	if err != nil {
		return nil, "", err
	}
	return canonical, store.FingerprintAuth(canonical), nil
}
