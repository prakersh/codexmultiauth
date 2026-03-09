package testenv

import (
	"os"
	"path/filepath"
	"testing"

	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/stretchr/testify/require"
)

func TestNewOverridesSharedExternalConfigPaths(t *testing.T) {
	sharedRoot := t.TempDir()
	sharedConfig := filepath.Join(sharedRoot, ".config")
	sharedCodex := filepath.Join(sharedRoot, ".codex")
	require.NoError(t, os.MkdirAll(filepath.Join(sharedConfig, "cma"), cmafs.DirMode))
	require.NoError(t, os.MkdirAll(sharedCodex, cmafs.DirMode))
	require.NoError(t, os.WriteFile(filepath.Join(sharedConfig, "cma", "state.json"), []byte("{bad"), cmafs.FileMode))

	t.Setenv("XDG_CONFIG_HOME", sharedConfig)
	t.Setenv("CODEX_HOME", sharedCodex)

	sandbox := New(t)

	require.NotEqual(t, filepath.Join(sharedConfig, "cma"), sandbox.Paths.ConfigDir)
	require.NotEqual(t, filepath.Join(sharedCodex, "auth.json"), sandbox.Paths.CodexAuth)

	state, err := store.NewStateRepo(sandbox.Paths).Load()
	require.NoError(t, err)
	require.Empty(t, state.Accounts)
}
