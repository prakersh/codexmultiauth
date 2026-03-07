package app

import (
	"fmt"
	"time"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/backup"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
)

type RestoreCandidate struct {
	Account  domain.Account
	Payload  []byte
	Conflict *RestoreConflict
}

type RestoreConflict struct {
	Existing domain.Account
	Reason   string
}

func AnalyzeRestore(state domain.State, artifact backup.Plaintext) []RestoreCandidate {
	candidates := make([]RestoreCandidate, 0, len(artifact.Accounts))
	for _, account := range artifact.Accounts {
		candidate := RestoreCandidate{
			Account: account.Account,
			Payload: account.Payload,
		}
		if existing, reason, ok := detectConflict(state, account.Account); ok {
			candidate.Conflict = &RestoreConflict{
				Existing: existing,
				Reason:   reason,
			}
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func ApplyRestore(state domain.State, vault store.Vault, candidates []RestoreCandidate, policy domain.ConflictPolicy, decisions map[string]domain.ConflictPolicy, now func() time.Time) (domain.State, store.Vault, int, error) {
	imported := 0
	for _, candidate := range candidates {
		account := candidate.Account
		payload := candidate.Payload

		if candidate.Conflict == nil {
			state = upsertAccount(state, account)
			vault = removeVaultEntry(vault, account.ID)
			vault.Entries = append(vault.Entries, store.VaultEntry{
				AccountID:   account.ID,
				Fingerprint: account.Fingerprint,
				Payload:     payload,
				Source:      string(account.AuthStoreKind),
				SavedAt:     now(),
			})
			imported++
			continue
		}

		effectivePolicy := policy
		if decided, ok := decisions[candidate.Account.ID]; ok {
			effectivePolicy = decided
		}

		switch effectivePolicy {
		case domain.ConflictSkip:
			continue
		case domain.ConflictOverwrite:
			account.ID = candidate.Conflict.Existing.ID
			state = removeAccount(state, candidate.Conflict.Existing.ID)
			state = upsertAccount(state, account)
			vault = removeVaultEntry(vault, candidate.Conflict.Existing.ID)
			vault.Entries = append(vault.Entries, store.VaultEntry{
				AccountID:   account.ID,
				Fingerprint: account.Fingerprint,
				Payload:     payload,
				Source:      string(account.AuthStoreKind),
				SavedAt:     now(),
			})
			imported++
		case domain.ConflictRename:
			account = renamedAccount(state, account)
			state = upsertAccount(state, account)
			vault.Entries = append(vault.Entries, store.VaultEntry{
				AccountID:   account.ID,
				Fingerprint: account.Fingerprint,
				Payload:     payload,
				Source:      string(account.AuthStoreKind),
				SavedAt:     now(),
			})
			imported++
		case domain.ConflictAsk:
			return domain.State{}, store.Vault{}, imported, fmt.Errorf("interactive conflict resolution required for %s", account.DisplayName)
		default:
			return domain.State{}, store.Vault{}, imported, fmt.Errorf("unsupported conflict policy %q", effectivePolicy)
		}
	}
	return state, vault, imported, nil
}

func detectConflict(state domain.State, incoming domain.Account) (domain.Account, string, bool) {
	for _, existing := range state.Accounts {
		if existing.Fingerprint == incoming.Fingerprint {
			return existing, "fingerprint", true
		}
	}
	for _, existing := range state.Accounts {
		if existing.ID == incoming.ID {
			return existing, "account_id", true
		}
	}
	for _, existing := range state.Accounts {
		if existing.DisplayName == incoming.DisplayName {
			return existing, "display_name", true
		}
		for _, alias := range incoming.Aliases {
			for _, existingAlias := range existing.Aliases {
				if alias == existingAlias {
					return existing, "alias", true
				}
			}
		}
	}
	return domain.Account{}, "", false
}

func renamedAccount(state domain.State, account domain.Account) domain.Account {
	base := account.DisplayName
	if base == "" {
		base = "restored"
	}
	suffix := 2
	for {
		candidate := fmt.Sprintf("%s-restored-%d", base, suffix)
		conflict := false
		for _, existing := range state.Accounts {
			if existing.DisplayName == candidate {
				conflict = true
				break
			}
		}
		if !conflict {
			account.DisplayName = candidate
			break
		}
		suffix++
	}
	return account
}
