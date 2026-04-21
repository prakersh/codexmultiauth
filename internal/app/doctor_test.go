package app

import (
	"context"
	"os"
	"testing"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/stretchr/testify/require"
)

func TestDoctor_ClearsTornFileWhenStateConsistent(t *testing.T) {
	manager, authStore, p := newTestManager(t)
	ctx := context.Background()

	authStore.setRaw(t, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"a","refresh_token":"r","account_id":"acc-1"}}`), domain.AuthStoreFile)
	_, err := manager.Save(ctx, SaveInput{DisplayName: "work"})
	require.NoError(t, err)

	// Simulate a prior failed-rollback torn-state marker.
	require.NoError(t, os.WriteFile(p.TornFile, []byte("torn at test"), 0o600))

	// Any mutation must be blocked while marker is present.
	err = manager.Rename(ctx, RenameInput{Selector: "work", NewName: "work2"})
	require.ErrorIs(t, err, ErrTornState)

	// Doctor should verify + clear.
	status, err := manager.Doctor(ctx)
	require.NoError(t, err)
	require.Contains(t, status, "ok")

	_, statErr := os.Stat(p.TornFile)
	require.True(t, os.IsNotExist(statErr), "torn file should be removed, got: %v", statErr)

	// Mutation now succeeds.
	require.NoError(t, manager.Rename(ctx, RenameInput{Selector: "work", NewName: "work2"}))
}

func TestCheckStateVaultInvariants_OrphanStateAccount(t *testing.T) {
	err := checkStateVaultInvariants(
		domain.State{Accounts: []domain.Account{{ID: "orphan", DisplayName: "x"}}},
		store.Vault{},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no vault entry")
}

func TestCheckStateVaultInvariants_OrphanVaultEntry(t *testing.T) {
	err := checkStateVaultInvariants(
		domain.State{},
		store.Vault{Entries: []store.VaultEntry{{AccountID: "ghost"}}},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "orphan")
}

func TestCheckStateVaultInvariants_ActiveAccountMissing(t *testing.T) {
	err := checkStateVaultInvariants(
		domain.State{ActiveAccountID: "missing"},
		store.Vault{},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "active account")
}

func TestCheckStateVaultInvariants_Consistent(t *testing.T) {
	require.NoError(t, checkStateVaultInvariants(domain.State{}, store.Vault{}))
	require.NoError(t, checkStateVaultInvariants(
		domain.State{
			Accounts:        []domain.Account{{ID: "a"}, {ID: "b"}},
			ActiveAccountID: "a",
		},
		store.Vault{Entries: []store.VaultEntry{{AccountID: "a"}, {AccountID: "b"}}},
	))
}
