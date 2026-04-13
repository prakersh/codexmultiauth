package app

import (
	"context"

	"github.com/prakersh/codexmultiauth/internal/domain"
)

func currentAuthFingerprint(ctx context.Context, authStore AuthStore) string {
	if authStore == nil {
		return ""
	}
	auth, err := authStore.Load(ctx)
	if err != nil {
		return ""
	}
	return auth.Fingerprint
}

func activeAccountIndex(state domain.State, activeFingerprint string) int {
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
	return activeIndex
}

func (m *Manager) activeAccountID(ctx context.Context, state domain.State) string {
	activeIndex := activeAccountIndex(state, currentAuthFingerprint(ctx, m.authStore))
	if activeIndex == -1 {
		return ""
	}
	return state.Accounts[activeIndex].ID
}
