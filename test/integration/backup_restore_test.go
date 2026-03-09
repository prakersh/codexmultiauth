package integration_test

import (
	"context"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/app"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/prakersh/codexmultiauth/test/testenv"
	"github.com/stretchr/testify/require"
)

func TestBackupRestoreSelectiveImport(t *testing.T) {
	manager, p, authStore := newManager(t)
	ctx := context.Background()

	require.NoError(t, authStore.Save(ctx, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-1","refresh_token":"refresh-1","account_id":"acc-1"}}`)))
	first, err := manager.Save(ctx, app.SaveInput{DisplayName: "work"})
	require.NoError(t, err)
	require.NoError(t, authStore.Save(ctx, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-2","refresh_token":"refresh-2","account_id":"acc-2"}}`)))
	second, err := manager.Save(ctx, app.SaveInput{DisplayName: "personal"})
	require.NoError(t, err)

	backupPath, err := manager.Backup(ctx, app.BackupInput{Passphrase: []byte("secret"), Target: "test-backup"})
	require.NoError(t, err)
	require.Contains(t, backupPath, p.BackupDir)

	manager2, _, _ := newManager(t)
	summary, err := manager2.Restore(ctx, app.RestoreInput{
		Passphrase: []byte("secret"),
		Source:     backupPath,
		Selected:   []string{first.Account.ID},
		Conflict:   "overwrite",
	})
	require.NoError(t, err)
	require.Equal(t, 1, summary.Imported)

	listed, err := manager2.List(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "work", listed[0].Account.DisplayName)
	require.NotEqual(t, second.Account.ID, listed[0].Account.ID)
}

func TestBackupRestoreAllAtomicImport(t *testing.T) {
	manager, _, authStore := newManager(t)
	ctx := context.Background()

	require.NoError(t, authStore.Save(ctx, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-1","refresh_token":"refresh-1","account_id":"acc-1"}}`)))
	_, err := manager.Save(ctx, app.SaveInput{DisplayName: "work"})
	require.NoError(t, err)

	backupPath, err := manager.Backup(ctx, app.BackupInput{Passphrase: []byte("secret"), Target: "all-backup"})
	require.NoError(t, err)

	p := testenv.New(t).Paths
	configRepo := store.NewConfigRepo(p)
	authStore2 := store.NewCodexAuthStore(p, nil, configRepo)
	stateRepo := store.NewStateRepo(p)
	manager2 := app.NewManager(
		p,
		authStore2,
		&failingStateRepo{delegate: stateRepo, failSave: true},
		store.NewVaultRepo(p),
		store.NewVaultKeyManager(p, configRepo, nil),
		cmafs.NewFileLockManager(),
		nil,
	)

	_, err = manager2.Restore(ctx, app.RestoreInput{
		Passphrase: []byte("secret"),
		Source:     backupPath,
		All:        true,
		Conflict:   "overwrite",
	})
	require.Error(t, err)

	listed, err := manager2.List(ctx)
	require.NoError(t, err)
	require.Empty(t, listed)
}
