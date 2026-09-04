package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/prakersh/codexmultiauth/internal/domain"
	cmacrypto "github.com/prakersh/codexmultiauth/internal/infra/crypto"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
)

func (m *Manager) Activate(ctx context.Context, selector string) (domain.Account, error) {
	var activated domain.Account
	err := m.withMutationLock(ctx, func() error {
		state, vault, key, err := m.loadStateAndVaultLocked(ctx)
		if err != nil {
			return err
		}
		// The vault key is only needed for this operation.
		defer cmacrypto.Zero(key)

		originalRecord, originalErr := m.authStore.Load(ctx)
		originalExists := originalErr == nil
		if originalErr != nil && !errors.Is(originalErr, os.ErrNotExist) {
			return originalErr
		}

		// Codex rotates the refresh token in auth.json as it runs and
		// invalidates the one it replaces, so the vault's copy of the account
		// currently in auth.json goes stale the moment Codex refreshes.
		// Overwriting auth.json without capturing it first left the outgoing
		// account holding a spent token, and switching back needed a browser
		// login. Fold the live credentials into the in-memory vault so the
		// single commit below stores them alongside the switch.
		if originalExists {
			state, vault = m.captureOutgoingAuth(state, vault, originalRecord)
		}

		// Resolved after the capture: activating the account that already owns
		// auth.json must pick up the entry the capture just refreshed, not the
		// stale copy that was loaded before it.
		account, err := domain.ResolveAccount(state.Accounts, selector)
		if err != nil {
			return err
		}
		entry, ok := findVaultEntry(vault, account.ID)
		if !ok {
			return fmt.Errorf("vault entry missing for account %s", account.ID)
		}

		if err := m.authStore.Save(ctx, entry.Payload); err != nil {
			return err
		}
		written, err := m.authStore.Load(ctx)
		if err != nil {
			if rollbackErr := rollbackAuth(ctx, m.authStore, originalExists, originalRecord.Canonical); rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
			return err
		}
		if written.Fingerprint != account.Fingerprint {
			rollbackErr := rollbackAuth(ctx, m.authStore, originalExists, originalRecord.Canonical)
			err = errors.New("activated auth fingerprint mismatch")
			if rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
			return err
		}

		now := m.now()
		account.LastUsedAt = &now
		state = upsertAccount(state, account)
		state.ActiveAccountID = account.ID

		if err := m.commitStateAndVault(state, vault, key); err != nil {
			if rollbackErr := rollbackAuth(ctx, m.authStore, originalExists, originalRecord.Canonical); rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
			return err
		}
		activated = account
		return nil
	})
	return activated, err
}

// captureOutgoingAuth folds the live auth.json into the vault entry of the
// account that currently owns it, so a token Codex rotated since the last save
// is not discarded by the switch.
//
// It only acts when the live credentials clearly belong to the active account:
// the codex account id in the live file must match the one in the stored
// payload. If the user logged in manually as somebody else, or the file is an
// API-key auth with no account id, the capture is skipped rather than risk
// writing one account's tokens into another's slot.
func (m *Manager) captureOutgoingAuth(state domain.State, vault store.Vault, live store.AuthRecord) (domain.State, store.Vault) {
	if state.ActiveAccountID == "" {
		return state, vault
	}
	entry, ok := findVaultEntry(vault, state.ActiveAccountID)
	if !ok {
		return state, vault
	}
	if entry.Fingerprint == live.Fingerprint {
		return state, vault // nothing rotated since the last save
	}
	if !sameCodexIdentity(live.Canonical, entry.Payload) {
		return state, vault
	}

	vault = upsertVaultEntry(vault, state.ActiveAccountID, live, m.now())
	for index, account := range state.Accounts {
		if account.ID == state.ActiveAccountID {
			state.Accounts[index].Fingerprint = live.Fingerprint
			break
		}
	}
	return state, vault
}

// sameCodexIdentity reports whether two auth payloads name the same Codex
// account. Fingerprints cannot be used here: a rotated token is exactly the
// case this needs to accept, and rotation changes the fingerprint.
func sameCodexIdentity(liveRaw, storedRaw []byte) bool {
	liveAuth, _, err := store.NormalizeAndValidateAuth(liveRaw)
	if err != nil || liveAuth.Tokens == nil {
		return false
	}
	storedAuth, _, err := store.NormalizeAndValidateAuth(storedRaw)
	if err != nil || storedAuth.Tokens == nil {
		return false
	}
	liveID := strings.TrimSpace(liveAuth.Tokens.AccountID)
	storedID := strings.TrimSpace(storedAuth.Tokens.AccountID)
	return liveID != "" && liveID == storedID
}

func rollbackAuth(ctx context.Context, authStore AuthStore, existed bool, raw []byte) error {
	if !existed {
		return authStore.Delete(ctx)
	}
	return authStore.Save(ctx, raw)
}
