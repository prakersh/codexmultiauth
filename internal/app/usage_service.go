package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	infrausage "github.com/prakersh/codexmultiauth/internal/infra/usage"
)

type UsageResult struct {
	Account domain.Account
	Usage   domain.UsageSummary
}

type usageAuthUpdate struct {
	Payload     []byte
	Fingerprint string
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

	results := make([]UsageResult, len(accounts))
	updates := make(map[string]usageAuthUpdate)
	var wg sync.WaitGroup
	var firstErr error
	var mu sync.Mutex

	for index, account := range accounts {
		wg.Add(1)
		go func(i int, account domain.Account) {
			defer wg.Done()
			entry, ok := findVaultEntry(vault, account.ID)
			if !ok {
				mu.Lock()
				if firstErr == nil {
					firstErr = errors.New("vault entry missing for usage")
				}
				mu.Unlock()
				return
			}
			auth, _, err := store.NormalizeAndValidateAuth(entry.Payload)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}

			if m.tokenRefresher != nil {
				refreshed, changed, refreshErr := m.tokenRefresher.MaybeRefresh(ctx, auth)
				if refreshErr == nil && changed {
					payload, fingerprint, canonicalErr := canonicalizeAuth(refreshed)
					if canonicalErr != nil {
						mu.Lock()
						if firstErr == nil {
							firstErr = canonicalErr
						}
						mu.Unlock()
						return
					}
					auth = refreshed
					mu.Lock()
					updates[account.ID] = usageAuthUpdate{Payload: payload, Fingerprint: fingerprint}
					mu.Unlock()
				}
			}

			summary := infrausage.BestEffortSummary(auth)
			if m.usage != nil {
				if fetched, err := m.usage.Fetch(ctx, auth); err == nil {
					summary = fetched
				}
			}
			results[i] = UsageResult{Account: account, Usage: summary}
		}(index, account)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := m.persistUsageAuthUpdates(ctx, updates); err != nil {
		return nil, err
	}
	return results, nil
}

func (m *Manager) persistUsageAuthUpdates(ctx context.Context, updates map[string]usageAuthUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	return m.withMutationLock(ctx, func() error {
		state, vault, key, err := m.loadStateAndVault(ctx)
		if err != nil {
			return err
		}

		applicable := make(map[string]usageAuthUpdate, len(updates))
		for i, entry := range vault.Entries {
			update, ok := updates[entry.AccountID]
			if !ok {
				continue
			}
			vault.Entries[i].Payload = update.Payload
			vault.Entries[i].Fingerprint = update.Fingerprint
			vault.Entries[i].SavedAt = m.now()
			applicable[entry.AccountID] = update
		}
		if len(applicable) == 0 {
			return nil
		}

		for i, account := range state.Accounts {
			if update, ok := applicable[account.ID]; ok {
				state.Accounts[i].Fingerprint = update.Fingerprint
			}
		}

		activeUpdate, hasActiveUpdate := applicable[state.ActiveAccountID]
		var originalAuth store.AuthRecord
		originalAuthExists := false
		if hasActiveUpdate {
			currentAuth, loadErr := m.authStore.Load(ctx)
			if loadErr == nil {
				originalAuth = currentAuth
				originalAuthExists = true
			} else if !errors.Is(loadErr, os.ErrNotExist) {
				return loadErr
			}

			if err := m.authStore.Save(ctx, activeUpdate.Payload); err != nil {
				return err
			}
		}

		if err := m.commitStateAndVault(state, vault, key); err != nil {
			if hasActiveUpdate {
				rollbackErr := rollbackAuth(ctx, m.authStore, originalAuthExists, originalAuth.Canonical)
				if rollbackErr != nil {
					return errors.Join(err, rollbackErr)
				}
			}
			return err
		}
		return nil
	})
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
