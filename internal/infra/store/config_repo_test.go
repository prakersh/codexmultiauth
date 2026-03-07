package store_test

import (
	"os"
	"testing"

	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/paths"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/stretchr/testify/require"
)

func TestConfigRepo_Load_Defaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p, err := paths.Resolve()
	require.NoError(t, err)

	cfg, err := store.NewConfigRepo(p).Load()
	require.NoError(t, err)
	require.False(t, cfg.DisableKeyring)
}

func TestConfigRepo_Load_EnvDisablesKeyring(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CMA_DISABLE_KEYRING", "1")
	p, err := paths.Resolve()
	require.NoError(t, err)

	cfg, err := store.NewConfigRepo(p).Load()
	require.NoError(t, err)
	require.True(t, cfg.DisableKeyring)
}

func TestConfigRepo_SaveLoadAndCorruptJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p, err := paths.Resolve()
	require.NoError(t, err)

	repo := store.NewConfigRepo(p)
	require.NoError(t, repo.Save(store.Config{DisableKeyring: true}))

	info, err := os.Stat(p.ConfigFile)
	require.NoError(t, err)
	require.Equal(t, cmafs.FileMode, info.Mode().Perm())

	cfg, err := repo.Load()
	require.NoError(t, err)
	require.Equal(t, store.ConfigVersionV1, cfg.Version)
	require.True(t, cfg.DisableKeyring)

	require.NoError(t, os.WriteFile(p.ConfigFile, []byte("{bad"), cmafs.FileMode))
	_, err = repo.Load()
	require.Error(t, err)
}
