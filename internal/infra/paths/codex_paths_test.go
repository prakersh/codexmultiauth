package paths_test

import (
	"path/filepath"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/infra/paths"
	"github.com/stretchr/testify/require"
)

func TestResolve_UsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "/tmp/cma-config")
	t.Setenv("CODEX_HOME", "/tmp/codex-home")

	got, err := paths.Resolve()
	require.NoError(t, err)
	require.Equal(t, "/tmp/cma-config/cma", got.ConfigDir)
	require.Equal(t, "/tmp/cma-config/cma/state.json", got.StateFile)
	require.Equal(t, "/tmp/codex-home", got.CodexHome)
	require.Equal(t, "/tmp/codex-home/auth.json", got.CodexAuth)
}

func TestResolve_UsesDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CODEX_HOME", "")

	got, err := paths.Resolve()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".config", "cma"), got.ConfigDir)
	require.Equal(t, filepath.Join(home, ".codex"), got.CodexHome)
}
