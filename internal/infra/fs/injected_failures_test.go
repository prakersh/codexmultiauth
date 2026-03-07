package fs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeLock struct {
	lockErr   error
	unlockErr error
	locked    bool
	path      string
	skipTouch bool
}

func (f fakeLock) TryLockContext(ctx context.Context, retryDelay time.Duration) (bool, error) {
	if f.locked && !f.skipTouch {
		_ = os.WriteFile(f.path, []byte("lock"), FileMode)
	}
	return f.locked, f.lockErr
}

func (f fakeLock) Unlock() error { return f.unlockErr }
func (f fakeLock) Path() string  { return f.path }

func TestAcquire_ErrorAndUnlockErrorPaths(t *testing.T) {
	original := newLockHandle
	defer func() { newLockHandle = original }()

	newLockHandle = func(path string) lockHandle {
		return fakeLock{lockErr: errors.New("boom"), path: path}
	}

	manager := NewFileLockManager()
	lock, err := manager.Acquire(context.Background(), filepath.Join(t.TempDir(), "state.lock"))
	require.Nil(t, lock)
	require.Error(t, err)

	newLockHandle = func(path string) lockHandle {
		return fakeLock{locked: true, unlockErr: errors.New("unlock fail"), path: path}
	}
	lock, err = manager.Acquire(context.Background(), filepath.Join(t.TempDir(), "state2.lock"))
	require.NoError(t, err)
	require.Error(t, lock.Unlock())

	newLockHandle = func(path string) lockHandle {
		return fakeLock{locked: false, path: path}
	}
	lock, err = manager.Acquire(context.Background(), filepath.Join(t.TempDir(), "state3.lock"))
	require.Nil(t, lock)
	require.ErrorIs(t, err, ErrLockUnavailable)
}

func TestAtomicWrite_InjectedFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), FileMode))

	origCreateTemp := createTempFile
	origRename := renameFile
	origOpenDir := openDirFile
	defer func() {
		createTempFile = origCreateTemp
		renameFile = origRename
		openDirFile = origOpenDir
	}()

	createTempFile = func(dir, pattern string) (*os.File, error) {
		return nil, errors.New("temp fail")
	}
	err := WriteFileAtomic(path, []byte("new"), AtomicWriteOptions{Mode: FileMode})
	require.Error(t, err)

	createTempFile = origCreateTemp
	renameFile = func(oldpath, newpath string) error { return errors.New("rename fail") }
	err = WriteFileAtomic(path, []byte("new"), AtomicWriteOptions{Mode: FileMode})
	require.Error(t, err)

	renameFile = origRename
	openDirFile = func(name string) (*os.File, error) { return nil, errors.New("dir sync fail") }
	err = WriteFileAtomic(path, []byte("new"), AtomicWriteOptions{Mode: FileMode})
	require.Error(t, err)

	openDirFile = origOpenDir
	err = WriteFileAtomic(path, []byte("newer"), AtomicWriteOptions{
		Mode: FileMode,
		Hooks: AtomicWriteHooks{
			AfterDirSync: func() error { return errors.New("after dir sync fail") },
		},
	})
	require.Error(t, err)
}

func TestPerms_ErrorPaths(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	err := EnsureDir(filepath.Join(file, "child"))
	require.Error(t, err)

	err = EnsureFileMode(filepath.Join(root, "missing"))
	require.Error(t, err)
}
