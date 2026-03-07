package fs_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/stretchr/testify/require"
)

func TestFileLockManager_Acquire(t *testing.T) {
	manager := fs.NewFileLockManager()
	lockPath := filepath.Join(t.TempDir(), "locks", "state.lock")

	lock, err := manager.Acquire(context.Background(), lockPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, lock.Unlock()) })
}

func TestFileLockManager_Contention(t *testing.T) {
	manager := fs.NewFileLockManager()
	lockPath := filepath.Join(t.TempDir(), "locks", "state.lock")

	first, err := manager.Acquire(context.Background(), lockPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, first.Unlock()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	second, err := manager.Acquire(ctx, lockPath)
	require.Nil(t, second)
	require.ErrorIs(t, err, fs.ErrLockUnavailable)
}

func TestFileLockManager_CanceledContext(t *testing.T) {
	manager := fs.NewFileLockManager()
	lockPath := filepath.Join(t.TempDir(), "locks", "state.lock")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	lock, err := manager.Acquire(ctx, lockPath)
	require.Nil(t, lock)
	require.ErrorIs(t, err, fs.ErrLockUnavailable)
}
