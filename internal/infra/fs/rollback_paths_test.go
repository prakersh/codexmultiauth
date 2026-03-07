package fs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteFileAtomicVerifyRollbackAndNewFileCleanup(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, os.WriteFile(existing, []byte("old"), FileMode))

	err := WriteFileAtomic(existing, []byte("new"), AtomicWriteOptions{
		Mode: FileMode,
		Verify: func(path string) error {
			return errors.New("mismatch")
		},
	})
	require.Error(t, err)

	data, readErr := os.ReadFile(existing)
	require.NoError(t, readErr)
	require.Equal(t, "old", string(data))

	fresh := filepath.Join(t.TempDir(), "fresh.json")
	err = WriteFileAtomic(fresh, []byte("new"), AtomicWriteOptions{
		Mode: FileMode,
		Hooks: AtomicWriteHooks{
			AfterRename: func() error { return errors.New("rename hook failed") },
		},
	})
	require.Error(t, err)
	_, statErr := os.Stat(fresh)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestAcquireTreatsCancelledContextAsUnavailable(t *testing.T) {
	original := newLockHandle
	defer func() { newLockHandle = original }()

	newLockHandle = func(path string) lockHandle {
		return fakeLock{lockErr: context.Canceled, path: path}
	}

	lock, err := NewFileLockManager().Acquire(context.Background(), filepath.Join(t.TempDir(), "cancel.lock"))
	require.Nil(t, lock)
	require.ErrorIs(t, err, ErrLockUnavailable)
}

func TestAcquireFailsWhenLockFilePermissionsCannotBeVerified(t *testing.T) {
	original := newLockHandle
	defer func() { newLockHandle = original }()

	newLockHandle = func(path string) lockHandle {
		return fakeLock{locked: true, path: path, skipTouch: true}
	}

	lock, err := NewFileLockManager().Acquire(context.Background(), filepath.Join(t.TempDir(), "missing.lock"))
	require.Nil(t, lock)
	require.Error(t, err)
}

func TestEnsureDirCanRepairPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.Chmod(dir, 0o755))

	require.NoError(t, EnsureDir(dir))

	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.Equal(t, DirMode, info.Mode().Perm())
}

func TestChmodFailurePathsAndReadExistingError(t *testing.T) {
	original := chmodPath
	defer func() { chmodPath = original }()

	chmodPath = func(path string, mode os.FileMode) error {
		return errors.New("chmod denied")
	}

	err := EnsureDir(filepath.Join(t.TempDir(), "config"))
	require.Error(t, err)

	file := filepath.Join(t.TempDir(), "file.json")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	err = EnsureFileMode(file)
	require.Error(t, err)

	dir := t.TempDir()
	err = WriteFileAtomic(dir, []byte("nope"), AtomicWriteOptions{Mode: FileMode})
	require.Error(t, err)
}
