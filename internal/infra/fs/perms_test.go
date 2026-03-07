package fs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/stretchr/testify/require"
)

func TestEnsureDir_SetsSecurePermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config", "nested")

	require.NoError(t, fs.EnsureDir(dir))

	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.Equal(t, fs.DirMode, info.Mode().Perm())
}

func TestEnsureFileMode_SetsSecurePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))

	require.NoError(t, fs.EnsureFileMode(path))

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, fs.FileMode, info.Mode().Perm())
}

func TestEnsureParentDir_SetsSecurePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "data.json")
	require.NoError(t, fs.EnsureParentDir(path))

	info, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	require.Equal(t, fs.DirMode, info.Mode().Perm())
}
