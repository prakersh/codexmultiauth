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

func (m *FileLockManager) Acquire(ctx context.Context, path string) (Unlocker, error) {
	if err := EnsureParentDir(path); err != nil {
		return nil, err
	}

	lock := newLockHandle(path)
	locked, err := lock.TryLockContext(ctx, 50)
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
