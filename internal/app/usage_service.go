package app

import (
	"context"
	"errors"
	"sync"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	infrausage "github.com/prakersh/codexmultiauth/internal/infra/usage"
)

type UsageResult struct {
	Account domain.Account
	Usage   domain.UsageSummary
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
	return results, nil
}
