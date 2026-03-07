package fs_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/stretchr/testify/require"
)

func TestWriteFileAtomic_CreatesSecureFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	err := fs.WriteFileAtomic(path, []byte("payload"), fs.AtomicWriteOptions{
		Mode: fs.FileMode,
		Verify: func(path string) error {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, "payload", string(data))
			return nil
		},
	})
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, fs.FileMode, info.Mode().Perm())
}

func TestWriteFileAtomic_RollsBackOnVerifyFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), fs.FileMode))

	err := fs.WriteFileAtomic(path, []byte("new"), fs.AtomicWriteOptions{
		Mode: fs.FileMode,
		Verify: func(path string) error {
			return errors.New("verification failed")
		},
	})
	require.Error(t, err)

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, "old", string(data))
}

func TestWriteFileAtomic_RollsBackOnPostRenameHookFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), fs.FileMode))

	err := fs.WriteFileAtomic(path, []byte("new"), fs.AtomicWriteOptions{
		Mode: fs.FileMode,
		Hooks: fs.AtomicWriteHooks{
			AfterRename: func() error {
				return errors.New("boom")
			},
		},
	})
	require.Error(t, err)

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, "old", string(data))
}

func TestWriteFileAtomic_RollsBackNewFileOnVerifyFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.json")

	err := fs.WriteFileAtomic(path, []byte("new"), fs.AtomicWriteOptions{
		Mode: fs.FileMode,
		Verify: func(path string) error {
			return errors.New("verify")
		},
	})
	require.Error(t, err)
	_, statErr := os.Stat(path)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestWriteFileAtomic_FailsBeforeRenameWithoutTouchingOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), fs.FileMode))

	err := fs.WriteFileAtomic(path, []byte("new"), fs.AtomicWriteOptions{
		Mode: fs.FileMode,
		Hooks: fs.AtomicWriteHooks{
			AfterFileSync: func() error { return errors.New("stop") },
		},
	})
	require.Error(t, err)

	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, "old", string(data))
}
