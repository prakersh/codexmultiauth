package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/prakersh/codexmultiauth/internal/domain"
	cmacrypto "github.com/prakersh/codexmultiauth/internal/infra/crypto"
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
// the refresh decision (expiry thresholds, retry policy, and persistence
// atomicity) lives in one place.
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

	// The mutex above only serializes goroutines inside this process. A
	// refresh token is single-use, so two processes reading the same on-disk
	// token would both POST it: one gets a 400 and reports a spurious failure
	// for a healthy account, or both succeed inside the server's grace window
	// and the later write persists a superseded token. Take a per-account file
	// lock so the load-refresh-persist sequence is serialized across processes
	// too. It is per-account, so different accounts still refresh in parallel,
	// and it is always taken before the main mutation lock, never after.
	fileLock, lockErr := m.lockManager.Acquire(ctx, m.refreshLockPath(accountID))
	if lockErr != nil {
		return store.CodexAuth{}, false, fmt.Errorf("acquire refresh lock for %s: %w", accountID, lockErr)
	}
	defer func() { _ = fileLock.Unlock() }()

	// Re-read the on-disk auth inside the critical section: if another
	// process already refreshed while we were waiting, pick up their result
	// instead of issuing a duplicate refresh.
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

// refreshLockPath names a per-account lock file. The account ID is hashed so
// the name is a fixed, filesystem-safe length regardless of the ID's contents.
func (m *Manager) refreshLockPath(accountID string) string {
	sum := sha256.Sum256([]byte(accountID))
	return filepath.Join(m.paths.LockDir, "refresh-"+hex.EncodeToString(sum[:8])+".lock")
}

// loadAccountAuth returns the credentials to reason about for an account.
//
// For the active account the live auth store wins over the vault copy. Codex
// rotates the token in place as it runs, so the vault snapshot can name a
// refresh token that has already been spent; deciding against it meant
// refreshing a dead token and failing an account whose live credentials were
// perfectly good. The live file is only preferred when it names the same Codex
// account, so a manual login as somebody else is ignored here just as it is on
// activate.
func (m *Manager) loadAccountAuth(ctx context.Context, accountID string) (store.CodexAuth, error) {
	state, vault, _, err := m.loadStateAndVault(ctx)
	if err != nil {
		return store.CodexAuth{}, err
	}
	entry, ok := findVaultEntry(vault, accountID)
	if !ok {
		return store.CodexAuth{}, fmt.Errorf("vault entry missing for account %q", accountID)
	}

	if state.ActiveAccountID == accountID {
		if live, liveErr := m.authStore.Load(ctx); liveErr == nil {
			if live.Fingerprint != entry.Fingerprint && sameCodexIdentity(live.Canonical, entry.Payload) {
				if auth, _, parseErr := store.NormalizeAndValidateAuth(live.Canonical); parseErr == nil {
					return auth, nil
				}
			}
		}
	}

	auth, _, err := store.NormalizeAndValidateAuth(entry.Payload)
	if err != nil {
		return store.CodexAuth{}, err
	}
	return auth, nil
}

func (m *Manager) persistRefreshedAuth(ctx context.Context, accountID string, payload []byte, fingerprint string) error {
	return m.withMutationLock(ctx, func() error {
		state, vault, key, err := m.loadStateAndVaultLocked(ctx)
		if err != nil {
			return err
		}
		// The vault key is only needed for this operation.
		defer cmacrypto.Zero(key)

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
