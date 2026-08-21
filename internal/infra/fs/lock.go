package fs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofrs/flock"
)

var ErrLockUnavailable = errors.New("lock unavailable")

type Unlocker interface {
	Unlock() error
}

type lockHandle interface {
	TryLockContext(ctx context.Context, retryDelay time.Duration) (bool, error)
	TryRLockContext(ctx context.Context, retryDelay time.Duration) (bool, error)
	Unlock() error
	Path() string
}

var newLockHandle = func(path string) lockHandle {
	return flock.New(path)
}

type FileLockManager struct{}

func NewFileLockManager() *FileLockManager {
	return &FileLockManager{}
}

const (
	// lockRetryDelay is how long to sleep between attempts. It must be a typed
	// Duration: the untyped constant 50 was taken as 50 nanoseconds, turning
	// the wait into a hot spin that reopened and re-flocked the file millions
	// of times a second and pinned a CPU core.
	lockRetryDelay = 50 * time.Millisecond
	// defaultLockTimeout bounds the wait when the caller supplies a context
	// with no deadline, which every command does. Without it ErrLockUnavailable
	// is unreachable and a contended command hangs forever with no output.
	defaultLockTimeout = 30 * time.Second
)

// AcquireShared takes a read lock. Several readers hold it at once, but it
// excludes the exclusive lock a mutation takes, so a reader can never observe
// state.json and the vault from opposite sides of a commit.
func (m *FileLockManager) AcquireShared(ctx context.Context, path string) (Unlocker, error) {
	return m.acquire(ctx, path, true)
}

func (m *FileLockManager) Acquire(ctx context.Context, path string) (Unlocker, error) {
	return m.acquire(ctx, path, false)
}

func (m *FileLockManager) acquire(ctx context.Context, path string, shared bool) (Unlocker, error) {
	if err := EnsureParentDir(path); err != nil {
		return nil, err
	}

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultLockTimeout)
		defer cancel()
	}

	lock := newLockHandle(path)
	tryLock := lock.TryLockContext
	if shared {
		tryLock = lock.TryRLockContext
	}
	locked, err := tryLock(ctx, lockRetryDelay)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, ErrLockUnavailable
		}
		return nil, fmt.Errorf("acquire lock %s: %w", path, err)
	}
	if !locked {
		return nil, ErrLockUnavailable
	}
	if err := EnsureFileMode(path); err != nil {
		_ = lock.Unlock()
		return nil, err
	}
	return fileUnlocker{lock: lock}, nil
}

type fileUnlocker struct {
	lock lockHandle
}

func (u fileUnlocker) Unlock() error {
	if err := u.lock.Unlock(); err != nil {
		return fmt.Errorf("unlock %s: %w", u.lock.Path(), err)
	}
	return nil
}
