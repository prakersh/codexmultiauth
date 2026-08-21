package app

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/backup"
	cmacrypto "github.com/prakersh/codexmultiauth/internal/infra/crypto"
)

type RestoreInput struct {
	Passphrase []byte
	Source     string
	All        bool
	Selected   []string
	Conflict   domain.ConflictPolicy
	Decisions  map[string]RestoreDecision
}

type RestoreSummary struct {
	Imported int
}

func (m *Manager) InspectBackup(input RestoreInput) (backup.Plaintext, []RestoreCandidate, error) {
	artifact, err := backup.Read(m.resolveRestorePath(input.Source), input.Passphrase)
	if err != nil {
		return backup.Plaintext{}, nil, err
	}
	state, err := m.stateRepo.Load()
	if err != nil {
		return backup.Plaintext{}, nil, err
	}
	return artifact, AnalyzeRestore(state, artifact), nil
}

func (m *Manager) Restore(ctx context.Context, input RestoreInput) (RestoreSummary, error) {
	var summary RestoreSummary
	err := m.withMutationLock(ctx, func() error {
		state, vault, key, err := m.loadStateAndVaultLocked(ctx)
		if err != nil {
			return err
		}
		// The vault key is only needed for this operation.
		defer cmacrypto.Zero(key)
		artifact, err := backup.Read(m.resolveRestorePath(input.Source), input.Passphrase)
		if err != nil {
			return err
		}
		candidates := AnalyzeRestore(state, artifact)
		if !input.All {
			candidates = filterCandidates(candidates, input.Selected)
		}
		nextState, nextVault, imported, err := ApplyRestore(state, vault, candidates, input.Conflict, input.Decisions, m.now)
		if err != nil {
			return err
		}
		if err := m.commitStateAndVault(nextState, nextVault, key); err != nil {
			return err
		}
		summary.Imported = imported
		return nil
	})
	return summary, err
}

func filterCandidates(candidates []RestoreCandidate, selected []string) []RestoreCandidate {
	if len(selected) == 0 {
		return nil
	}
	selectedSet := map[string]struct{}{}
	for _, id := range selected {
		selectedSet[id] = struct{}{}
	}
	var filtered []RestoreCandidate
	for _, candidate := range candidates {
		if _, ok := selectedSet[candidate.Account.ID]; ok {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func (m *Manager) resolveRestorePath(source string) string {
	if filepath.IsAbs(source) {
		return source
	}
	name := source
	if !strings.HasSuffix(name, ".cma.bak") {
		name += ".cma.bak"
	}
	return filepath.Join(m.paths.BackupDir, name)
}
