package store_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prakersh/codexmultiauth/internal/domain"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/prakersh/codexmultiauth/test/testenv"
	"github.com/stretchr/testify/require"
)

func TestStateRepo_SaveLoadRoundTrip(t *testing.T) {
	p := testenv.New(t).Paths

	now := time.Now().UTC()
	state := domain.State{
		Accounts: []domain.Account{
			{
				ID:            "acc-1",
				DisplayName:   "work",
				Fingerprint:   "fp-1",
				AuthStoreKind: domain.AuthStoreFile,
				CreatedAt:     now,
			},
		},
		ActiveAccountID: "acc-1",
	}

	repo := store.NewStateRepo(p)
	require.NoError(t, repo.Save(state))

	loaded, err := repo.Load()
	require.NoError(t, err)
	require.Equal(t, "acc-1", loaded.ActiveAccountID)
	require.Len(t, loaded.Accounts, 1)

	info, err := os.Stat(p.StateFile)
	require.NoError(t, err)
	require.Equal(t, cmafs.FileMode, info.Mode().Perm())
}

func TestStateRepo_LoadCorruptJSON(t *testing.T) {
	p := testenv.New(t).Paths

	require.NoError(t, os.MkdirAll(filepath.Dir(p.StateFile), cmafs.DirMode))
	require.NoError(t, os.WriteFile(p.StateFile, []byte("{bad"), cmafs.FileMode))

	_, err := store.NewStateRepo(p).Load()
	require.Error(t, err)
}
