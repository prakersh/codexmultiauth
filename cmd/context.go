package cmd

import (
	"context"
	"strings"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/backup"
	"github.com/prakersh/codexmultiauth/internal/infra/codexcli"
	cmafs "github.com/prakersh/codexmultiauth/internal/infra/fs"
	"github.com/prakersh/codexmultiauth/internal/infra/paths"
	"github.com/prakersh/codexmultiauth/internal/infra/store"
	infrausage "github.com/prakersh/codexmultiauth/internal/infra/usage"
)

type service interface {
	List(ctx context.Context) ([]app.ListedAccount, error)
	Usage(ctx context.Context, selector string) ([]app.UsageResult, error)
	Backup(ctx context.Context, input app.BackupInput) (string, error)
	InspectBackup(input app.RestoreInput) (backup.Plaintext, []app.RestoreCandidate, error)
	Restore(ctx context.Context, input app.RestoreInput) (app.RestoreSummary, error)
	Save(ctx context.Context, input app.SaveInput) (app.SaveResult, error)
	New(ctx context.Context, input app.NewInput) (app.SaveResult, error)
	Activate(ctx context.Context, selector string) (domain.Account, error)
	Delete(ctx context.Context, input app.DeleteInput) error
}

func newManager() (*app.Manager, error) {
	p, err := paths.Resolve()
	if err != nil {
		return nil, err
	}
	configRepo := store.NewConfigRepo(p)
	manager := app.NewManager(
		p,
		store.NewCodexAuthStore(p, store.OSKeyringClient{}, configRepo),
		store.NewStateRepo(p),
		store.NewVaultRepo(p),
		store.NewVaultKeyManager(p, configRepo, store.OSKeyringClient{}),
		cmafs.NewFileLockManager(),
		codexcli.NewClient("codex"),
	)
	manager.SetUsageFetcher(infrausage.NewClient(""))
	manager.SetTokenRefresher(infrausage.NewTokenRefresher())
	return manager, nil
}

var newService = func() (service, error) {
	return newManager()
}

func splitAliases(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
