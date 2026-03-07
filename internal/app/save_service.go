package app

import (
	"context"
	"fmt"
	"strings"

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

		for _, account := range state.Accounts {
			if account.Fingerprint == record.Fingerprint {
				result = SaveResult{Account: account, Deduplicated: true}
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
			Aliases:       uniqueStrings(input.Aliases),
			Fingerprint:   record.Fingerprint,
			AuthStoreKind: record.StoreKind,
			CreatedAt:     m.now(),
		}
		state = upsertAccount(state, account)
		if vault.Version == "" {
			vault.Version = store.VaultVersionV1
		}
		vault.Entries = append(vault.Entries, store.VaultEntry{
			AccountID:   account.ID,
			Fingerprint: account.Fingerprint,
			Payload:     record.Canonical,
			Source:      string(record.StoreKind),
			SavedAt:     m.now(),
		})

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
