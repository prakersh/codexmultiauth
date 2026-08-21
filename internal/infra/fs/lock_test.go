package fs_test

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/prakersh/codexmultiauth/internal/infra/fs"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
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

// TestAcquireWaitsWithoutBusySpinning pins the retry delay units. The untyped
// constant 50 was read as 50 nanoseconds, so a contended lock reopened and
// re-flocked the file in a tight loop and pinned a CPU core.
func TestAcquireWaitsWithoutBusySpinning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "busy.lock")
	holder := flock.New(path)
	locked, err := holder.TryLock()
	require.NoError(t, err)
	require.True(t, locked)
	defer func() { _ = holder.Unlock() }()

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	var usageBefore, usageAfter syscall.Rusage
	require.NoError(t, syscall.Getrusage(syscall.RUSAGE_SELF, &usageBefore))
	_, err = cmafs.NewFileLockManager().Acquire(ctx, path)
	require.NoError(t, syscall.Getrusage(syscall.RUSAGE_SELF, &usageAfter))
	require.ErrorIs(t, err, cmafs.ErrLockUnavailable)

	cpu := cpuSeconds(usageAfter) - cpuSeconds(usageBefore)
	// A 50ms retry over 400ms is about 8 attempts and near-zero CPU. The old
	// 50ns delay burned well over a full core-second here.
	require.Less(t, cpu, 0.15, "lock wait burned %.3fs of CPU, indicating a busy spin", cpu)
}

// TestAcquireTimesOutWithoutCallerDeadline covers the context every command
// actually passes: without a default bound, a contended command hung forever.
func TestAcquireTimesOutWithoutCallerDeadline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodeadline.lock")
	holder := flock.New(path)
	locked, err := holder.TryLock()
	require.NoError(t, err)
	require.True(t, locked)
	defer func() { _ = holder.Unlock() }()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err = cmafs.NewFileLockManager().Acquire(ctx, path)
	require.ErrorIs(t, err, cmafs.ErrLockUnavailable)
}

func cpuSeconds(u syscall.Rusage) float64 {
	user := float64(u.Utime.Sec) + float64(u.Utime.Usec)/1e6
	sys := float64(u.Stime.Sec) + float64(u.Stime.Usec)/1e6
	return user + sys
}
