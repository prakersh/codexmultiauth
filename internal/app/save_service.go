package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
)

type SaveInput struct {
	DisplayName string
	Aliases     []string
}

type SaveResult struct {
	Account      domain.Account
	Deduplicated bool
	Updated      bool
}

func (m *Manager) Save(ctx context.Context, input SaveInput) (SaveResult, error) {
	var result SaveResult
	err := m.withMutationLock(ctx, func() error {
		record, err := m.authStore.Load(ctx)
		if err != nil {
			return fmt.Errorf("load current codex auth: %w", err)
		}

		state, vault, key, err := m.loadStateAndVault(ctx)
		if err != nil {
			return err
		}

		aliases := uniqueStrings(input.Aliases)
		for index, account := range state.Accounts {
			if account.Fingerprint == record.Fingerprint {
				updated := applySaveMetadata(account, record, input.DisplayName, aliases)
				if accountsEqual(updated, account) {
					result = SaveResult{Account: account, Deduplicated: true}
					return nil
				}
				state.Accounts[index] = updated
				vault = upsertVaultEntry(vault, updated.ID, record, m.now())
				if err := m.commitStateAndVault(state, vault, key); err != nil {
					return err
				}
				result = SaveResult{Account: updated, Updated: true}
				return nil
			}
		}


		displayName := strings.TrimSpace(input.DisplayName)
		if displayName == "" {
			displayName = defaultDisplayName(record, len(state.Accounts)+1)
		}

		account := domain.Account{
			ID:            m.newID(),
			DisplayName:   displayName,
			Aliases:       aliases,
			Fingerprint:   record.Fingerprint,
			AuthStoreKind: record.StoreKind,
			CreatedAt:     m.now(),
		}
		state = upsertAccount(state, account)
		vault = upsertVaultEntry(vault, account.ID, record, m.now())

		if err := m.commitStateAndVault(state, vault, key); err != nil {
			return err
		}
		result = SaveResult{Account: account}
		return nil
	})
	return result, err
}

func defaultDisplayName(record store.AuthRecord, sequence int) string {
	if record.Parsed.Tokens != nil && record.Parsed.Tokens.AccountID != "" {
		accountID := record.Parsed.Tokens.AccountID
		if len(accountID) > 12 {
			accountID = accountID[:12]
		}
		return "account-" + accountID
	}
	return fmt.Sprintf("account-%d", sequence)
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func upsertVaultEntry(vault store.Vault, accountID string, record store.AuthRecord, savedAt time.Time) store.Vault {
	if vault.Version == "" {
		vault.Version = store.VaultVersionV1
	}
	for index, entry := range vault.Entries {
		if entry.AccountID != accountID {
			continue
		}
		vault.Entries[index].Fingerprint = record.Fingerprint
		vault.Entries[index].Payload = record.Canonical
		vault.Entries[index].Source = string(record.StoreKind)
		vault.Entries[index].SavedAt = savedAt
		return vault
	}
	vault.Entries = append(vault.Entries, store.VaultEntry{
		AccountID:   accountID,
		Fingerprint: record.Fingerprint,
		Payload:     record.Canonical,
		Source:      string(record.StoreKind),
		SavedAt:     savedAt,
	})
	return vault
}

func applySaveMetadata(account domain.Account, record store.AuthRecord, displayName string, aliases []string) domain.Account {
	displayName = strings.TrimSpace(displayName)
	if displayName != "" {
		account.DisplayName = displayName
	}
	if len(aliases) > 0 {
		account.Aliases = aliases
	}
	account.Fingerprint = record.Fingerprint
	account.AuthStoreKind = record.StoreKind
	return account
}

func accountsEqual(left, right domain.Account) bool {
	if left.ID != right.ID || left.DisplayName != right.DisplayName || left.Fingerprint != right.Fingerprint || left.AuthStoreKind != right.AuthStoreKind || left.CreatedAt != right.CreatedAt {
		return false
	}
	if (left.LastUsedAt == nil) != (right.LastUsedAt == nil) {
		return false
	}
	if left.LastUsedAt != nil && right.LastUsedAt != nil && !left.LastUsedAt.Equal(*right.LastUsedAt) {
		return false
	}
	if len(left.Aliases) != len(right.Aliases) {
		return false
	}
	for index := range left.Aliases {
		if left.Aliases[index] != right.Aliases[index] {
			return false
		}
	}
	return true
}
