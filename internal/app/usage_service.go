package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	infrausage "github.com/prakersh/codexmultiauth/internal/infra/usage"
)

type UsageResult struct {
	Account domain.Account
	Usage   domain.UsageSummary
	Info    UsageAccountInfo
}

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
			if refreshErr != nil {
				// Fall back to on-disk auth on refresh failure so usage can
				// still report best-effort data; only surface a hard error
				// if we couldn't even read the on-disk entry.
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
			if m.usage != nil {
				if fetched, err := m.usage.Fetch(ctx, auth); err == nil {
					summary = fetched
				}
			}
			metadata := infrausage.ExtractAccountMetadata(auth)
			results[i] = UsageResult{
				Account: account,
				Usage:   summary,
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
