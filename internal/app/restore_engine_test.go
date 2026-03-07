package app_test

import (
	"testing"
	"time"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/backup"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	"github.com/stretchr/testify/require"
)

func TestApplyRestore_SkipConflict(t *testing.T) {
	now := time.Now().UTC()
	state := domain.State{
		Accounts: []domain.Account{
			{ID: "acc-1", DisplayName: "work", Fingerprint: "fp-1", CreatedAt: now},
		},
	}
	candidates := []app.RestoreCandidate{
		{
			Account:  domain.Account{ID: "acc-2", DisplayName: "work", Fingerprint: "fp-2", CreatedAt: now},
			Payload:  []byte(`{}`),
			Conflict: &app.RestoreConflict{Existing: state.Accounts[0], Reason: "display_name"},
		},
	}

	nextState, _, imported, err := app.ApplyRestore(state, store.Vault{}, candidates, domain.ConflictSkip, nil, func() time.Time { return now })
	require.NoError(t, err)
	require.Len(t, nextState.Accounts, 1)
	require.Equal(t, 0, imported)
}

func TestApplyRestore_RenameConflict(t *testing.T) {
	now := time.Now().UTC()
	state := domain.State{
		Accounts: []domain.Account{
			{ID: "acc-1", DisplayName: "work", Fingerprint: "fp-1", CreatedAt: now},
		},
	}
	candidates := []app.RestoreCandidate{
		{
			Account:  domain.Account{ID: "acc-2", DisplayName: "work", Fingerprint: "fp-2", CreatedAt: now},
			Payload:  []byte(`{}`),
			Conflict: &app.RestoreConflict{Existing: state.Accounts[0], Reason: "display_name"},
		},
	}

	nextState, _, imported, err := app.ApplyRestore(state, store.Vault{}, candidates, domain.ConflictRename, nil, func() time.Time { return now })
	require.NoError(t, err)
	require.Len(t, nextState.Accounts, 2)
	require.Equal(t, 1, imported)
	require.Equal(t, "work-restored-2", nextState.Accounts[1].DisplayName)
}

func TestApplyRestore_OverwriteAndAskError(t *testing.T) {
	now := time.Now().UTC()
	state := domain.State{
		Accounts: []domain.Account{
			{ID: "acc-1", DisplayName: "work", Fingerprint: "fp-1", CreatedAt: now},
		},
	}
	candidates := []app.RestoreCandidate{
		{
			Account:  domain.Account{ID: "acc-2", DisplayName: "work", Fingerprint: "fp-2", CreatedAt: now},
			Payload:  []byte(`{}`),
			Conflict: &app.RestoreConflict{Existing: state.Accounts[0], Reason: "display_name"},
		},
	}

	nextState, nextVault, imported, err := app.ApplyRestore(state, store.Vault{}, candidates, domain.ConflictOverwrite, nil, func() time.Time { return now })
	require.NoError(t, err)
	require.Len(t, nextState.Accounts, 1)
	require.Len(t, nextVault.Entries, 1)
	require.Equal(t, 1, imported)

	_, _, _, err = app.ApplyRestore(state, store.Vault{}, candidates, domain.ConflictAsk, nil, func() time.Time { return now })
	require.Error(t, err)
}

func TestApplyRestore_DecisionOverridesAndUnsupportedPolicy(t *testing.T) {
	now := time.Now().UTC()
	state := domain.State{
		Accounts: []domain.Account{
			{ID: "acc-1", DisplayName: "work", Fingerprint: "fp-1", CreatedAt: now},
		},
	}
	candidates := []app.RestoreCandidate{
		{
			Account:  domain.Account{ID: "acc-2", DisplayName: "work", Fingerprint: "fp-2", CreatedAt: now},
			Payload:  []byte(`{"tokens":{}}`),
			Conflict: &app.RestoreConflict{Existing: state.Accounts[0], Reason: "display_name"},
		},
		{
			Account: domain.Account{ID: "acc-3", DisplayName: "fresh", Fingerprint: "fp-3", CreatedAt: now},
			Payload: []byte(`{"tokens":{}}`),
		},
	}

	nextState, nextVault, imported, err := app.ApplyRestore(state, store.Vault{}, candidates, domain.ConflictSkip, map[string]domain.ConflictPolicy{
		"acc-2": domain.ConflictOverwrite,
	}, func() time.Time { return now })
	require.NoError(t, err)
	require.Equal(t, 2, imported)
	require.Len(t, nextState.Accounts, 2)
	require.Len(t, nextVault.Entries, 2)

	_, _, _, err = app.ApplyRestore(state, store.Vault{}, candidates[:1], domain.ConflictPolicy("broken"), nil, func() time.Time { return now })
	require.Error(t, err)
}

func TestAnalyzeRestoreDetectsConflictReasons(t *testing.T) {
	now := time.Now().UTC()
	state := domain.State{
		Accounts: []domain.Account{
			{ID: "acc-1", DisplayName: "fingerprint", Fingerprint: "fp-1", Aliases: []string{"alias-1"}, CreatedAt: now},
			{ID: "acc-2", DisplayName: "by-id", Fingerprint: "fp-2", Aliases: []string{"alias-2"}, CreatedAt: now},
			{ID: "acc-3", DisplayName: "same-name", Fingerprint: "fp-3", Aliases: []string{"alias-3"}, CreatedAt: now},
			{ID: "acc-4", DisplayName: "alias-owner", Fingerprint: "fp-4", Aliases: []string{"shared"}, CreatedAt: now},
		},
	}

	artifact := storeBackup(now,
		domain.Account{ID: "new-1", DisplayName: "fresh", Fingerprint: "fp-1"},
		domain.Account{ID: "acc-2", DisplayName: "fresh-id", Fingerprint: "fp-new"},
		domain.Account{ID: "new-3", DisplayName: "same-name", Fingerprint: "fp-new-3"},
		domain.Account{ID: "new-4", DisplayName: "fresh-alias", Fingerprint: "fp-new-4", Aliases: []string{"shared"}},
		domain.Account{ID: "new-5", DisplayName: "no-conflict", Fingerprint: "fp-new-5"},
	)

	candidates := app.AnalyzeRestore(state, artifact)
	require.Equal(t, "fingerprint", candidates[0].Conflict.Reason)
	require.Equal(t, "account_id", candidates[1].Conflict.Reason)
	require.Equal(t, "display_name", candidates[2].Conflict.Reason)
	require.Equal(t, "alias", candidates[3].Conflict.Reason)
	require.Nil(t, candidates[4].Conflict)
}

func TestRenameConflictGeneratesUniqueSuffix(t *testing.T) {
	now := time.Now().UTC()
	state := domain.State{
		Accounts: []domain.Account{
			{ID: "acc-1", DisplayName: "work", Fingerprint: "fp-1", CreatedAt: now},
			{ID: "acc-2", DisplayName: "work-restored-2", Fingerprint: "fp-2", CreatedAt: now},
		},
	}
	candidates := []app.RestoreCandidate{
		{
			Account:  domain.Account{ID: "acc-3", DisplayName: "work", Fingerprint: "fp-3", CreatedAt: now},
			Payload:  []byte(`{}`),
			Conflict: &app.RestoreConflict{Existing: state.Accounts[0], Reason: "display_name"},
		},
	}

	nextState, _, imported, err := app.ApplyRestore(state, store.Vault{}, candidates, domain.ConflictRename, nil, func() time.Time { return now })
	require.NoError(t, err)
	require.Equal(t, 1, imported)
	require.Equal(t, "work-restored-3", nextState.Accounts[2].DisplayName)
}

func storeBackup(now time.Time, accounts ...domain.Account) backup.Plaintext {
	artifact := backup.Plaintext{}
	for _, account := range accounts {
		account.CreatedAt = now
		artifact.Accounts = append(artifact.Accounts, backup.Account{
			Account: account,
			Payload: []byte(`{}`),
		})
	}
	return artifact
}
