package integration_test

import (
	"context"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/infra/codexcli"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/paths"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/prakersh/codexmultiauth/test/testenv"
	"github.com/stretchr/testify/require"
)

type fakeLoginCLI struct {
	login func(ctx context.Context, deviceAuth bool, withAPIKey bool) error
}

func (f fakeLoginCLI) Login(ctx context.Context, deviceAuth bool, withAPIKey bool) error {
	return f.login(ctx, deviceAuth, withAPIKey)
}

func (f fakeLoginCLI) Status(ctx context.Context) (string, error) {
	return "", nil
}

func newManager(t *testing.T) (*app.Manager, paths.Paths, *store.CodexAuthStore) {
	t.Helper()
	p := testenv.New(t).Paths

	configRepo := store.NewConfigRepo(p)
	authStore := store.NewCodexAuthStore(p, nil, configRepo)
	manager := app.NewManager(
		p,
		authStore,
		store.NewStateRepo(p),
		store.NewVaultRepo(p),
		store.NewVaultKeyManager(p, configRepo, nil),
		cmafs.NewFileLockManager(),
		codexcli.NewClient("codex"),
	)
	return manager, p, authStore
}

func TestSaveListDeleteWorkflow(t *testing.T) {
	manager, _, authStore := newManager(t)
	ctx := context.Background()

	require.NoError(t, authStore.Save(ctx, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-1","refresh_token":"refresh-1","account_id":"acc-1"}}`)))

	saved, err := manager.Save(ctx, app.SaveInput{DisplayName: "work", Aliases: []string{"main"}})
	require.NoError(t, err)
	require.False(t, saved.Deduplicated)

	listed, err := manager.List(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "work", listed[0].Account.DisplayName)

	require.NoError(t, manager.Delete(ctx, app.DeleteInput{Selector: "work"}))

	listed, err = manager.List(ctx)
	require.NoError(t, err)
	require.Empty(t, listed)
}

func TestNewWorkflow(t *testing.T) {
	_, p, authStore := newManager(t)
	ctx := context.Background()

	require.NoError(t, authStore.Save(ctx, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-old","refresh_token":"refresh-old","account_id":"acc-old"}}`)))
	configRepo := store.NewConfigRepo(p)
	manager := app.NewManager(
		p,
		authStore,
		store.NewStateRepo(p),
		store.NewVaultRepo(p),
		store.NewVaultKeyManager(p, configRepo, nil),
		cmafs.NewFileLockManager(),
		fakeLoginCLI{login: func(ctx context.Context, deviceAuth bool, withAPIKey bool) error {
			require.True(t, deviceAuth)
			require.False(t, withAPIKey)
			return authStore.Save(ctx, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"token-new","refresh_token":"refresh-new","account_id":"acc-new"}}`))
		}},
	)

	saved, err := manager.New(ctx, app.NewInput{DisplayName: "fresh", DeviceAuth: true})
	require.NoError(t, err)
	require.Equal(t, "fresh", saved.Account.DisplayName)

	listed, err := manager.List(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "fresh", listed[0].Account.DisplayName)
}
