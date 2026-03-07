package app

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/prakersh/codexmultiauth/internal/domain"
)

func (m *Manager) Activate(ctx context.Context, selector string) (domain.Account, error) {
	var activated domain.Account
	err := m.withMutationLock(ctx, func() error {
		state, vault, key, err := m.loadStateAndVault(ctx)
		if err != nil {
			return err
		}
		account, err := domain.ResolveAccount(state.Accounts, selector)
		if err != nil {
			return err
		}
		entry, ok := findVaultEntry(vault, account.ID)
		if !ok {
			return fmt.Errorf("vault entry missing for account %s", account.ID)
		}

		originalRecord, originalErr := m.authStore.Load(ctx)
		originalExists := originalErr == nil
		if originalErr != nil && !errors.Is(originalErr, os.ErrNotExist) {
			return originalErr
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

func rollbackAuth(ctx context.Context, authStore AuthStore, existed bool, raw []byte) error {
	if !existed {
		return authStore.Delete(ctx)
	}
	return authStore.Save(ctx, raw)
}
