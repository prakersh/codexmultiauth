package integration_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/domain"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/prakersh/codexmultiauth/test/testenv"
	"github.com/stretchr/testify/require"
)

type failingStateRepo struct {
	delegate *store.StateRepo
	failSave bool
}

func (r *failingStateRepo) Load() (domain.State, error) {
	return r.delegate.Load()
}

func (r *failingStateRepo) Save(state domain.State) error {
	if r.failSave {
		return errors.New("boom")
	}
	return r.delegate.Save(state)
}

func TestActivate_RollsBackAuthOnStateFailure(t *testing.T) {
	p := testenv.New(t).Paths

	configRepo := store.NewConfigRepo(p)
	authStore := store.NewCodexAuthStore(p, nil, configRepo)
	require.NoError(t, authStore.Save(context.Background(), []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"old","refresh_token":"refresh-old","account_id":"acc-old"}}`)))

	stateRepo := store.NewStateRepo(p)
	vaultRepo := store.NewVaultRepo(p)
	keyManager := store.NewVaultKeyManager(p, configRepo, nil)
	key, _, err := keyManager.LoadOrCreate(context.Background())
	require.NoError(t, err)

	state := domain.State{
		Accounts: []domain.Account{
			{ID: "acc-1", DisplayName: "work", Fingerprint: store.FingerprintAuth(mustCanonical(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"new","refresh_token":"refresh-new","account_id":"acc-new"}}`))), CreatedAt: timeNow()},
		},
	}
	require.NoError(t, stateRepo.Save(state))
	require.NoError(t, vaultRepo.Save(store.Vault{
		Entries: []store.VaultEntry{
			{AccountID: "acc-1", Fingerprint: state.Accounts[0].Fingerprint, Payload: mustCanonical(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"new","refresh_token":"refresh-new","account_id":"acc-new"}}`)), Source: "file", SavedAt: timeNow()},
		},
	}, key))

	manager := app.NewManager(
		p,
		authStore,
		&failingStateRepo{delegate: stateRepo, failSave: true},
		vaultRepo,
		keyManager,
		cmafs.NewFileLockManager(),
		nil,
	)

	_, err = manager.Activate(context.Background(), "work")
	require.Error(t, err)

	record, loadErr := authStore.Load(context.Background())
	require.NoError(t, loadErr)
	require.Contains(t, string(record.Canonical), `"access_token":"old"`)

	info, err := os.Stat(p.CodexAuth)
	require.NoError(t, err)
	require.Equal(t, cmafs.FileMode, info.Mode().Perm())
}

func mustCanonical(t *testing.T, raw []byte) []byte {
	t.Helper()
	_, canonical, err := store.NormalizeAndValidateAuth(raw)
	require.NoError(t, err)
	return canonical
}

func timeNow() time.Time {
	return time.Now().UTC()
}
