package app

import (
	"context"
	"sync"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/domain"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/stretchr/testify/require"
)

// recordingLockManager wraps the real lock behavior while recording which
// paths were taken and whether each was shared.
type recordingLockManager struct {
	mu       sync.Mutex
	inner    LockManager
	acquired []lockRecord
}

type lockRecord struct {
	path   string
	shared bool
}

func (r *recordingLockManager) Acquire(ctx context.Context, path string) (cmafs.Unlocker, error) {
	r.record(path, false)
	return r.inner.Acquire(ctx, path)
}

func (r *recordingLockManager) AcquireShared(ctx context.Context, path string) (cmafs.Unlocker, error) {
	r.record(path, true)
	return r.inner.AcquireShared(ctx, path)
}

func (r *recordingLockManager) record(path string, shared bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.acquired = append(r.acquired, lockRecord{path: path, shared: shared})
}

func (r *recordingLockManager) records() []lockRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]lockRecord(nil), r.acquired...)
}

// TestReadPathsTakeSharedLock covers the torn-read window: reading state.json
// and the vault unlocked let a reader straddle a concurrent commit and produce
// a snapshot referencing an account whose vault entry was already gone.
func TestReadPathsTakeSharedLock(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	ctx := context.Background()

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"a","refresh_token":"r","account_id":"acc-1"}}`), domain.AuthStoreFile)
	_, err := manager.Save(ctx, SaveInput{DisplayName: "work"})
	require.NoError(t, err)

	rec := &recordingLockManager{inner: manager.lockManager}
	manager.lockManager = rec

	_, err = manager.Usage(ctx, "all")
	require.NoError(t, err)

	records := rec.records()
	require.NotEmpty(t, records, "read path took no lock at all")
	var sawShared bool
	for _, record := range records {
		if record.shared && record.path == manager.lockPath() {
			sawShared = true
		}
	}
	require.True(t, sawShared, "Usage must read state and vault under a shared lock")
}

// TestReadLockDoesNotDeadlockAgainstMutation ensures the shared lock taken by
// readers is never taken again inside a mutation that already holds the
// exclusive lock, which would deadlock the process against itself.
func TestReadLockDoesNotDeadlockAgainstMutation(t *testing.T) {
	manager, authStore, _ := newTestManager(t)
	ctx := context.Background()

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"a","refresh_token":"r","account_id":"acc-1"}}`), domain.AuthStoreFile)

	done := make(chan error, 1)
	go func() {
		_, err := manager.Save(ctx, SaveInput{DisplayName: "work"})
		if err != nil {
			done <- err
			return
		}
		done <- manager.Rename(ctx, RenameInput{Selector: "work", NewName: "work2"})
	}()

	require.NoError(t, <-done, "mutation deadlocked or failed while holding the exclusive lock")
}

// TestRefreshLockPathIsPerAccount pins that the refresh lock is scoped to one
// account, so serializing a single account's refresh across processes does not
// serialize every account's.
func TestRefreshLockPathIsPerAccount(t *testing.T) {
	manager, _, p := newTestManager(t)

	first := manager.refreshLockPath("account-one")
	second := manager.refreshLockPath("account-two")

	require.NotEqual(t, first, second)
	require.Equal(t, first, manager.refreshLockPath("account-one"), "path must be stable across calls")
	require.Contains(t, first, p.LockDir)
	require.NotContains(t, first, "account-one", "raw account id must not leak into the filename")
}
