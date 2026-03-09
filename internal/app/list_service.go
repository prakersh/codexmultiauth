package app

import (
	"context"

	"github.com/prakersh/codexmultiauth/internal/domain"
)

type ListedAccount struct {
	Account  domain.Account
	IsActive bool
}

func (m *Manager) List(ctx context.Context) ([]ListedAccount, error) {
	state, err := m.stateRepo.Load()
	if err != nil {
		return nil, err
	}

	activeFingerprint := ""
	if auth, err := m.authStore.Load(ctx); err == nil {
		activeFingerprint = auth.Fingerprint
	}

	activeIndex := -1
	if activeFingerprint != "" {
		for index, account := range state.Accounts {
			if account.Fingerprint == activeFingerprint && account.ID == state.ActiveAccountID {
				activeIndex = index
				break
			}
		}
		if activeIndex == -1 {
			for index, account := range state.Accounts {
				if account.Fingerprint == activeFingerprint {
					activeIndex = index
					break
				}
			}
		}
	}
	if activeIndex == -1 && state.ActiveAccountID != "" {
		for index, account := range state.Accounts {
			if account.ID == state.ActiveAccountID {
				activeIndex = index
				break
			}
		}
	}

	listed := make([]ListedAccount, 0, len(state.Accounts))
	for index, account := range state.Accounts {
		listed = append(listed, ListedAccount{
			Account:  account,
			IsActive: index == activeIndex,
		})
	}
	return listed, nil
}
