package app

import (
	"context"
	"fmt"

	"github.com/prakersh/codexmultiauth/internal/domain"
)

type RenameInput struct {
	Selector string
	NewName  string
}

func (m *Manager) Rename(ctx context.Context, input RenameInput) error {
	if input.Selector == "" {
		return fmt.Errorf("selector is required")
	}
	if input.NewName == "" {
		return fmt.Errorf("new name is required")
	}

	return m.withMutationLock(ctx, func() error {
		state, vault, key, err := m.loadStateAndVault(ctx)
		if err != nil {
			return err
		}

		account, err := domain.ResolveAccount(state.Accounts, input.Selector)
		if err != nil {
			return err
		}

		if account.DisplayName == input.NewName {
			return nil
		}

		for _, other := range state.Accounts {
			if other.ID == account.ID {
				continue
			}
			if other.DisplayName == input.NewName {
				return fmt.Errorf("account %q already exists", input.NewName)
			}
			for _, alias := range other.Aliases {
				if alias == input.NewName {
					return fmt.Errorf("account %q is already used as an alias of %q", input.NewName, other.DisplayName)
				}
			}
		}

		account.DisplayName = input.NewName

		for i, a := range state.Accounts {
			if a.ID == account.ID {
				state.Accounts[i] = account
				break
			}
		}

		return m.commitStateAndVault(state, vault, key)
	})
}
