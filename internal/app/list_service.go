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

	activeIndex := activeAccountIndex(state, currentAuthFingerprint(ctx, m.authStore))

	listed := make([]ListedAccount, 0, len(state.Accounts))
	for index, account := range state.Accounts {
		listed = append(listed, ListedAccount{
			Account:  account,
			IsActive: index == activeIndex,
		})
	}
	return listed, nil
}
