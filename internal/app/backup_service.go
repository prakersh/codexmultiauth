package app

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/backup"
)

type BackupInput struct {
	Passphrase []byte
	Target     string
}

func (m *Manager) Backup(ctx context.Context, input BackupInput) (string, error) {
	var outputPath string
	err := m.withMutationLock(ctx, func() error {
		state, vault, _, err := m.loadStateAndVault(ctx)
		if err != nil {
			return err
		}
		artifactAccounts := make([]backup.Account, 0, len(vault.Entries))
		for _, entry := range vault.Entries {
			account, err := domain.ResolveAccount(state.Accounts, entry.AccountID)
			if err != nil {
				continue
			}
			artifactAccounts = append(artifactAccounts, backup.Account{
				Account: account,
				Payload: entry.Payload,
			})
		}
		outputPath = m.resolveBackupPath(input.Target)
		return backup.Write(outputPath, artifactAccounts, input.Passphrase)
	})
	return outputPath, err
}

func (m *Manager) resolveBackupPath(target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	name := target
	if !strings.HasSuffix(name, ".cma.bak") {
		name += ".cma.bak"
	}
	return filepath.Join(m.paths.BackupDir, name)
}
