package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
)

// freshAuthReason selects how ensureFreshAuth decides whether to call the
// token authority. Callers should use freshAuthIfExpiring for background /
// selection paths (Usage, activate, etc.) and freshAuthForce for an explicit
// user command such as `cma refresh`.
type freshAuthReason int

const (
	freshAuthIfExpiring freshAuthReason = iota
	freshAuthForce
)

// RefreshResult describes a single account's outcome under Manager.Refresh.
type RefreshResult struct {
	Account   domain.Account
	Refreshed bool
	Err       error
}

// refreshMutexFor returns a lazily-created mutex keyed by account ID. It is
// used by ensureFreshAuth to serialize concurrent refreshes for the same
// account within a single process so that two goroutines never POST a
// refresh with the same single-use refresh_token.
//
// Cross-process serialization of the *persist* step is handled separately by
// the file lock inside withMutationLock; a competing process that manages to
// refresh in between will be detected by the fingerprint check below.
func (m *Manager) refreshMutexFor(accountID string) *sync.Mutex {
	m.refreshMuGuard.Lock()
	defer m.refreshMuGuard.Unlock()
	if m.refreshMuMap == nil {
		m.refreshMuMap = map[string]*sync.Mutex{}
	}
	mu, ok := m.refreshMuMap[accountID]
	if !ok {
		mu = &sync.Mutex{}
		m.refreshMuMap[accountID] = mu
	}
	return mu
}

// ensureFreshAuth is the single place in the code that decides whether an
// account's OAuth tokens need refreshing, calls the token authority, and
// persists the result. All token-consumption paths (usage, activate, limits,
// TUI selection, explicit `cma refresh`) should go through this function so
// the refresh decision — including expiry thresholds, retry policy, and
// persistence atomicity — lives in one place.
//
// The semantics mirror upstream codex's refresh_token / ReloadedChanged
// pattern: we serialize per-account refreshes, re-read on-disk auth inside
// the critical section so we don't clobber a newer refresh that a concurrent
// caller already persisted, then either reuse the on-disk auth or call the
// token authority and persist the result atomically via commitStateAndVault.
func (m *Manager) ensureFreshAuth(ctx context.Context, accountID string, reason freshAuthReason) (store.CodexAuth, bool, error) {
	if m.tokenRefresher == nil {
		auth, err := m.loadAccountAuth(ctx, accountID)
		return auth, false, err
	}

	mu := m.refreshMutexFor(accountID)
	mu.Lock()
	defer mu.Unlock()

	// Re-read the on-disk auth inside the per-account critical section: if
	// another process already refreshed while we were waiting, pick up their
	// result instead of issuing a duplicate refresh.
	auth, err := m.loadAccountAuth(ctx, accountID)
	if err != nil {
		return store.CodexAuth{}, false, err
	}

	var refreshed store.CodexAuth
	var changed bool
	var refreshErr error
	switch reason {
	case freshAuthForce:
		refreshed, changed, refreshErr = m.tokenRefresher.Refresh(ctx, auth)
	default:
		refreshed, changed, refreshErr = m.tokenRefresher.MaybeRefresh(ctx, auth)
	}
	if refreshErr != nil {
		return auth, false, refreshErr
	}
	if !changed {
		return auth, false, nil
	}

	payload, fingerprint, err := canonicalizeAuth(refreshed)
	if err != nil {
		return auth, false, err
	}

	if err := m.persistRefreshedAuth(ctx, accountID, payload, fingerprint); err != nil {
		return auth, false, err
	}
	return refreshed, true, nil
}

func (m *Manager) loadAccountAuth(ctx context.Context, accountID string) (store.CodexAuth, error) {
	_, vault, _, err := m.loadStateAndVault(ctx)
	if err != nil {
		return store.CodexAuth{}, err
	}
	entry, ok := findVaultEntry(vault, accountID)
	if !ok {
		return store.CodexAuth{}, fmt.Errorf("vault entry missing for account %q", accountID)
	}
	auth, _, err := store.NormalizeAndValidateAuth(entry.Payload)
	if err != nil {
		return store.CodexAuth{}, err
	}
	return auth, nil
}

func (m *Manager) persistRefreshedAuth(ctx context.Context, accountID string, payload []byte, fingerprint string) error {
	return m.withMutationLock(ctx, func() error {
		state, vault, key, err := m.loadStateAndVault(ctx)
		if err != nil {
			return err
		}

		updated := false
		for i, entry := range vault.Entries {
			if entry.AccountID == accountID {
				// If another caller already landed the same refresh, no-op.
				if entry.Fingerprint == fingerprint {
					return nil
				}
				vault.Entries[i].Payload = payload
				vault.Entries[i].Fingerprint = fingerprint
				vault.Entries[i].SavedAt = m.now()
				updated = true
				break
			}
		}
		if !updated {
			return fmt.Errorf("vault entry for account %q disappeared before refresh persist", accountID)
		}

		for i, account := range state.Accounts {
			if account.ID == accountID {
				state.Accounts[i].Fingerprint = fingerprint
				break
			}
		}

		if state.ActiveAccountID == accountID {
			var originalAuth store.AuthRecord
			originalExists := false
			current, loadErr := m.authStore.Load(ctx)
			if loadErr == nil {
				originalAuth = current
				originalExists = true
			} else if !errors.Is(loadErr, os.ErrNotExist) {
				return loadErr
			}
			if err := m.authStore.Save(ctx, payload); err != nil {
				return err
			}
			if err := m.commitStateAndVault(state, vault, key); err != nil {
				rollbackErr := rollbackAuth(ctx, m.authStore, originalExists, originalAuth.Canonical)
				if rollbackErr != nil {
					return errors.Join(err, rollbackErr)
				}
				return err
			}
			return nil
		}
		return m.commitStateAndVault(state, vault, key)
	})
}

// Refresh forces a token refresh for the selected accounts. Pass "all" (or
// an empty selector) to refresh every account. Returns one RefreshResult per
// attempted account; per-account errors are captured in the result rather
// than short-circuiting so a partial batch still reports what landed.
func (m *Manager) Refresh(ctx context.Context, selector string) ([]RefreshResult, error) {
	state, _, _, err := m.loadStateAndVault(ctx)
	if err != nil {
		return nil, err
	}
	accounts, err := domain.ResolveAccounts(state.Accounts, selector)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, nil
	}

	results := make([]RefreshResult, len(accounts))
	for i, account := range accounts {
		_, changed, refreshErr := m.ensureFreshAuth(ctx, account.ID, freshAuthForce)
		results[i] = RefreshResult{
			Account:   account,
			Refreshed: changed,
			Err:       refreshErr,
		}
	}
	return results, nil
}
