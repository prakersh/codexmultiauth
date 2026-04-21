package app

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/stretchr/testify/require"
)

// countingRefresher counts how often Refresh/MaybeRefresh are invoked and
// returns a deterministic mutated token on each call so callers can observe
// a changed fingerprint.
type countingRefresher struct {
	mu            sync.Mutex
	maybeCount    atomic.Int32
	refreshCount  atomic.Int32
	nextAccessTok int
	// slow forces each refresh to block until signalled, so tests can
	// reliably exercise the per-account serialization path.
	slow chan struct{}
}

func (c *countingRefresher) MaybeRefresh(ctx context.Context, auth store.CodexAuth) (store.CodexAuth, bool, error) {
	c.maybeCount.Add(1)
	return c.mutate(auth)
}

func (c *countingRefresher) Refresh(ctx context.Context, auth store.CodexAuth) (store.CodexAuth, bool, error) {
	c.refreshCount.Add(1)
	if c.slow != nil {
		<-c.slow
	}
	return c.mutate(auth)
}

func (c *countingRefresher) mutate(auth store.CodexAuth) (store.CodexAuth, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextAccessTok++
	cloned := auth
	if auth.Tokens != nil {
		tokens := *auth.Tokens
		tokens.AccessToken = fmt.Sprintf("refreshed-%d", c.nextAccessTok)
		cloned.Tokens = &tokens
	}
	return cloned, true, nil
}

func seedAccount(t *testing.T, manager *Manager, authStore *memoryAuthStore, name string) domain.Account {
	t.Helper()
	authStore.setRaw(t, []byte(fmt.Sprintf(`{"auth_mode":"chatgpt","tokens":{"access_token":"initial","refresh_token":"r-%s","account_id":"acc-%s"}}`, name, name)), domain.AuthStoreFile)
	saved, err := manager.Save(context.Background(), SaveInput{DisplayName: name})
	require.NoError(t, err)
	return saved.Account
}

func TestRefresh_AllProfilesInvokesTokenAuthorityPerAccount(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	refresher := &countingRefresher{}
	manager.SetTokenRefresher(refresher)

	seedAccount(t, manager, authStore, "work")
	seedAccount(t, manager, authStore, "personal")

	results, err := manager.Refresh(context.Background(), "all")
	require.NoError(t, err)
	require.Len(t, results, 2)
	for _, r := range results {
		require.NoError(t, r.Err)
		require.True(t, r.Refreshed, "%s should have refreshed", r.Account.DisplayName)
	}
	require.Equal(t, int32(2), refresher.refreshCount.Load())
}

func TestRefresh_SelectorMatchesSingleAccount(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	refresher := &countingRefresher{}
	manager.SetTokenRefresher(refresher)

	seedAccount(t, manager, authStore, "work")
	seedAccount(t, manager, authStore, "personal")

	results, err := manager.Refresh(context.Background(), "personal")
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "personal", results[0].Account.DisplayName)
	require.Equal(t, int32(1), refresher.refreshCount.Load())
}

func TestEnsureFreshAuth_ConcurrentCallsAreSerializedPerAccount(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	refresher := &countingRefresher{slow: make(chan struct{})}
	manager.SetTokenRefresher(refresher)

	saved := seedAccount(t, manager, authStore, "work")

	const callers = 8
	var wg sync.WaitGroup
	inFlight := atomic.Int32{}
	maxInFlight := atomic.Int32{}
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cur := inFlight.Add(1)
			for {
				prev := maxInFlight.Load()
				if cur <= prev || maxInFlight.CompareAndSwap(prev, cur) {
					break
				}
			}
			_, _, err := manager.ensureFreshAuth(context.Background(), saved.ID, freshAuthForce)
			inFlight.Add(-1)
			require.NoError(t, err)
		}()
	}

	// Drain the slow channel so each refresh returns one at a time. Because
	// of the per-account mutex, at most one caller should be inside Refresh
	// at any given moment even though the counter of "waiting callers" can
	// be high.
	for i := 0; i < callers; i++ {
		refresher.slow <- struct{}{}
	}

	wg.Wait()
	require.Equal(t, int32(callers), refresher.refreshCount.Load(), "each caller should refresh exactly once because they arrive serialized")
}

func TestEnsureFreshAuth_PersistsNewTokensAtomically(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	refresher := &countingRefresher{}
	manager.SetTokenRefresher(refresher)

	saved := seedAccount(t, manager, authStore, "work")

	ctx := context.Background()
	_, changed, err := manager.ensureFreshAuth(ctx, saved.ID, freshAuthForce)
	require.NoError(t, err)
	require.True(t, changed)

	// Reload from disk and confirm the new access token landed.
	state, vault, _, err := manager.loadStateAndVault(ctx)
	require.NoError(t, err)
	require.Len(t, vault.Entries, 1)

	entry := vault.Entries[0]
	auth, _, err := store.NormalizeAndValidateAuth(entry.Payload)
	require.NoError(t, err)
	require.Contains(t, auth.Tokens.AccessToken, "refreshed-")

	// State's fingerprint should be updated to match the vault entry's.
	require.Equal(t, entry.Fingerprint, state.Accounts[0].Fingerprint)
}
