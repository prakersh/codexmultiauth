package store_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	cmacrypto "github.com/prakersh/codexmultiauth/internal/infra/crypto"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/prakersh/codexmultiauth/test/testenv"
	"github.com/stretchr/testify/require"
)

func TestVaultRepo_SaveLoadRoundTrip(t *testing.T) {
	p := testenv.New(t).Paths

	key, err := cmacrypto.RandomBytes(cmacrypto.KeyLength)
	require.NoError(t, err)

	repo := store.NewVaultRepo(p)
	vault := store.Vault{
		Entries: []store.VaultEntry{
			{
				AccountID:   "acc-1",
				Fingerprint: "fp-1",
				Payload:     []byte(`{"auth_mode":"chatgpt"}`),
				Source:      "file",
				SavedAt:     time.Now().UTC(),
			},
		},
	}

	require.NoError(t, repo.Save(vault, key))

	loaded, err := repo.Load(key)
	require.NoError(t, err)
	require.Len(t, loaded.Entries, 1)
	require.Equal(t, `{"auth_mode":"chatgpt"}`, string(loaded.Entries[0].Payload))

	info, err := os.Stat(p.VaultFile)
	require.NoError(t, err)
	require.Equal(t, cmafs.FileMode, info.Mode().Perm())
}

func TestVaultRepo_DefaultsAndCorruption(t *testing.T) {
	p := testenv.New(t).Paths
	key, err := cmacrypto.RandomBytes(cmacrypto.KeyLength)
	require.NoError(t, err)

	repo := store.NewVaultRepo(p)
	vault, err := repo.Load(key)
	require.NoError(t, err)
	require.Equal(t, store.VaultVersionV1, vault.Version)

	require.NoError(t, os.MkdirAll(filepath.Dir(p.VaultFile), cmafs.DirMode))
	require.NoError(t, os.WriteFile(p.VaultFile, []byte("{bad"), cmafs.FileMode))
	_, err = repo.Load(key)
	require.Error(t, err)
}
