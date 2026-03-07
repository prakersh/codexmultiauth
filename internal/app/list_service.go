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

	listed := make([]ListedAccount, 0, len(state.Accounts))
	for _, account := range state.Accounts {
		active := account.ID == state.ActiveAccountID
		if activeFingerprint != "" && account.Fingerprint == activeFingerprint {
			active = true
		}
		listed = append(listed, ListedAccount{
			Account:  account,
			IsActive: active,
		})
	}
	return listed, nil
}
