package app

import (
	"context"
	"fmt"

	"github.com/prakersh/codexmultiauth/internal/domain"
)

type DeleteInput struct {
	Selector          string
	AllowActiveDelete bool
}

func (m *Manager) Delete(ctx context.Context, input DeleteInput) error {
	return m.withMutationLock(ctx, func() error {
		state, vault, key, err := m.loadStateAndVault(ctx)
		if err != nil {
			return err
		}

		account, err := domain.ResolveAccount(state.Accounts, input.Selector)
		if err != nil {
			return err
		}
		if state.ActiveAccountID == account.ID && !input.AllowActiveDelete {
			return fmt.Errorf("refusing to delete active account %s", account.DisplayName)
		}

		state = removeAccount(state, account.ID)
		vault = removeVaultEntry(vault, account.ID)
		return m.commitStateAndVault(state, vault, key)
	})
}
