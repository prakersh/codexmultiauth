package cmd

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/AlecAivazis/survey/v2"
	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/backup"
	"github.com/prakersh/codexmultiauth/test/testenv"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

type fakeService struct {
	listed            []app.ListedAccount
	usage             []app.UsageResult
	saveResult        app.SaveResult
	newResult         app.SaveResult
	activateResult    domain.Account
	backupPath        string
	restoreSummary    app.RestoreSummary
	inspectArtifact   backup.Plaintext
	inspectCandidates []app.RestoreCandidate

	lastSaveInput    app.SaveInput
	lastNewInput     app.NewInput
	lastDeleteInput  app.DeleteInput
	lastBackupInput  app.BackupInput
	lastRestoreInput app.RestoreInput
	lastActivate     string
	lastUsage        string
}

func (f *fakeService) List(ctx context.Context) ([]app.ListedAccount, error) { return f.listed, nil }
func (f *fakeService) Usage(ctx context.Context, selector string) ([]app.UsageResult, error) {
	f.lastUsage = selector
	return f.usage, nil
}
func (f *fakeService) Backup(ctx context.Context, input app.BackupInput) (string, error) {
	f.lastBackupInput = input
	return f.backupPath, nil
}
func (f *fakeService) InspectBackup(input app.RestoreInput) (backup.Plaintext, []app.RestoreCandidate, error) {
	f.lastRestoreInput = input
	return f.inspectArtifact, f.inspectCandidates, nil
}
func (f *fakeService) Restore(ctx context.Context, input app.RestoreInput) (app.RestoreSummary, error) {
	f.lastRestoreInput = input
	return f.restoreSummary, nil
}
func (f *fakeService) Save(ctx context.Context, input app.SaveInput) (app.SaveResult, error) {
	f.lastSaveInput = input
	return f.saveResult, nil
}
func (f *fakeService) New(ctx context.Context, input app.NewInput) (app.SaveResult, error) {
	f.lastNewInput = input
	return f.newResult, nil
}
func (f *fakeService) Activate(ctx context.Context, selector string) (domain.Account, error) {
	f.lastActivate = selector
	return f.activateResult, nil
}
func (f *fakeService) Delete(ctx context.Context, input app.DeleteInput) error {
	f.lastDeleteInput = input
	return nil
}

func TestPromptHelpersAndSplitAliases(t *testing.T) {
	originalAskOne := askOne
	defer func() { askOne = originalAskOne }()

	askOne = func(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		switch out := response.(type) {
		case *string:
			switch p := prompt.(type) {
			case *survey.Input:
				*out = p.Default + "-value"
			case *survey.Select:
				*out = p.Options[1]
			}
		case *bool:
			*out = true
		case *[]string:
			*out = []string{"one", "two"}
		}
		return nil
	}

	text, err := promptText("name", "default")
	require.NoError(t, err)
	require.Equal(t, "default-value", text)

	password, err := promptPassword("password")
	require.NoError(t, err)
	require.Empty(t, password)

	confirmed, err := promptConfirm("confirm", false)
	require.NoError(t, err)
	require.True(t, confirmed)

	selected, err := promptMultiSelect("multi", []string{"one", "two"})
	require.NoError(t, err)
	require.Equal(t, []string{"one", "two"}, selected)

	policy, err := promptConflictPolicy("work")
	require.NoError(t, err)
	require.Equal(t, domain.ConflictSkip, policy)

	require.Equal(t, []string{"a", "b", "a"}, splitAliases(" a, b ,a "))
	require.Nil(t, splitAliases("  "))
}

func TestNewManagerAndExecute(t *testing.T) {
	testenv.New(t)

	manager, err := newManager()
	require.NoError(t, err)
	require.NotNil(t, manager)

	originalFactory := rootCmdFactory
	originalService := newService
	defer func() {
		rootCmdFactory = originalFactory
		newService = originalService
	}()

	svc := &fakeService{}
	newService = func() (service, error) { return svc, nil }
	rootCmdFactory = func() *cobra.Command {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"list"})
		return cmd
	}

	require.NoError(t, Execute())
}

func TestCommandWorkflows(t *testing.T) {
	originalService := newService
	originalAskOne := askOne
	originalRunTUI := runTUI
	defer func() {
		newService = originalService
		askOne = originalAskOne
		runTUI = originalRunTUI
	}()

	svc := &fakeService{
		listed: []app.ListedAccount{
			{Account: domain.Account{ID: "1", DisplayName: "work", Aliases: []string{"main"}, AuthStoreKind: domain.AuthStoreFile}, IsActive: true},
			{Account: domain.Account{ID: "2", DisplayName: "personal", AuthStoreKind: domain.AuthStoreKeyring}},
		},
		usage: []app.UsageResult{
			{
				Account: domain.Account{DisplayName: "work"},
				Usage: domain.UsageSummary{
					Confidence: domain.UsageConfidenceConfirmed,
					PlanType:   "team",
					Quotas: []domain.UsageQuota{
						{DisplayName: "5-Hour Limit", UsedPercent: floatPtr(12.5)},
						{DisplayName: "Review Requests"},
					},
				},
			},
		},
		saveResult:     app.SaveResult{Account: domain.Account{DisplayName: "saved"}},
		newResult:      app.SaveResult{Account: domain.Account{DisplayName: "fresh"}},
		activateResult: domain.Account{DisplayName: "work"},
		backupPath:     "/tmp/backup.cma.bak",
		restoreSummary: app.RestoreSummary{Imported: 1},
		inspectCandidates: []app.RestoreCandidate{
			{
				Account: domain.Account{ID: "1", DisplayName: "work"},
				Conflict: &app.RestoreConflict{
					Existing: domain.Account{ID: "existing-1", DisplayName: "work"},
					Reason:   "display_name",
				},
			},
			{Account: domain.Account{ID: "2", DisplayName: "personal"}},
		},
	}
	newService = func() (service, error) { return svc, nil }
	runTUI = func(service service) error { return nil }
	askOne = func(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		switch out := response.(type) {
		case *string:
			switch p := prompt.(type) {
			case *survey.Input:
				switch p.Message {
				case "Account name (optional)":
					*out = "prompted"
				case "Aliases (comma-separated, optional)":
					*out = "a,b"
				}
			case *survey.Select:
				*out = string(domain.ConflictRename)
			}
		case *bool:
			*out = true
		case *[]string:
			*out = []string{"work [conflict:display_name]"}
		}
		return nil
	}

	output, err := runCommand(newListCmd())
	require.NoError(t, err)
	require.Contains(t, output, "* 1. work [main] (file)")

	output, err = runCommand(newUsageCmd(), "all")
	require.NoError(t, err)
	require.Contains(t, output, "confidence: confirmed")
	require.Contains(t, output, "plan: team")
	require.Contains(t, output, "Review Requests: unknown")

	output, err = runCommand(newLimitsCmd())
	require.NoError(t, err)
	require.Contains(t, output, "work")
	require.Equal(t, "all", svc.lastUsage)

	output, err = runCommand(newSaveCmd(), "--name", "named", "--aliases", "one,two")
	require.NoError(t, err)
	require.Contains(t, output, "Saved saved")
	require.Equal(t, []string{"one", "two"}, svc.lastSaveInput.Aliases)

	svc.saveResult = app.SaveResult{Account: domain.Account{DisplayName: "saved"}, Deduplicated: true}
	output, err = runCommand(newSaveCmd())
	require.NoError(t, err)
	require.Contains(t, output, "Already saved as saved")
	require.Equal(t, "prompted", svc.lastSaveInput.DisplayName)

	output, err = runCommand(newNewCmd(), "--device-auth")
	require.NoError(t, err)
	require.Contains(t, output, "Saved fresh")
	require.Equal(t, "prompted", svc.lastNewInput.DisplayName)
	require.True(t, svc.lastNewInput.DeviceAuth)

	output, err = runCommand(newActivateCmd(), "1")
	require.NoError(t, err)
	require.Contains(t, output, "Activated work")
	require.Equal(t, "1", svc.lastActivate)

	output, err = runCommand(newDeleteCmd(), "1")
	require.NoError(t, err)
	require.Contains(t, output, "Deleted work")
	require.True(t, svc.lastDeleteInput.AllowActiveDelete)

	output, err = runCommand(newBackupCmd(), "--allow-plain-pass-arg", "pass:secret", "named")
	require.NoError(t, err)
	require.Contains(t, output, "Backup written to /tmp/backup.cma.bak")
	require.Equal(t, []byte("secret"), svc.lastBackupInput.Passphrase)

	output, err = runCommand(newRestoreCmd(), "--conflict", "ask", "pass:secret", "named")
	require.Error(t, err)
	require.Contains(t, err.Error(), "plain passphrase arguments require --allow-plain-pass-arg")

	output, err = runCommand(newRestoreCmd(), "--allow-plain-pass-arg", "--conflict", "ask", "pass:secret", "named")
	require.NoError(t, err)
	require.Contains(t, output, "Imported 1 account(s)")
	require.Equal(t, []string{"1"}, svc.lastRestoreInput.Selected)
	require.Equal(t, domain.ConflictRename, svc.lastRestoreInput.Decisions["1"])

	output, err = runCommand(newTUICmd())
	require.NoError(t, err)
	require.Empty(t, output)
}

func TestNewCommand_DoesNotPromptForOptionalAliases(t *testing.T) {
	originalService := newService
	originalAskOne := askOne
	defer func() {
		newService = originalService
		askOne = originalAskOne
	}()

	svc := &fakeService{
		newResult: app.SaveResult{Account: domain.Account{DisplayName: "fresh"}},
	}
	newService = func() (service, error) { return svc, nil }
	askOne = func(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		switch p := prompt.(type) {
		case *survey.Input:
			if p.Message == "Aliases (comma-separated, optional)" {
				t.Fatalf("unexpected aliases prompt")
			}
			if p.Message == "Account name (optional)" {
				out := response.(*string)
				*out = "prompted"
			}
		}
		return nil
	}

	output, err := runCommand(newNewCmd())
	require.NoError(t, err)
	require.Contains(t, output, "Saved fresh")
	require.Equal(t, "prompted", svc.lastNewInput.DisplayName)
	require.Empty(t, svc.lastNewInput.Aliases)
}

func TestDeleteActiveCancellationAndContainsHelper(t *testing.T) {
	originalService := newService
	originalAskOne := askOne
	defer func() {
		newService = originalService
		askOne = originalAskOne
	}()

	svc := &fakeService{
		listed: []app.ListedAccount{
			{Account: domain.Account{ID: "1", DisplayName: "work"}, IsActive: true},
		},
	}
	newService = func() (service, error) { return svc, nil }
	askOne = func(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		switch out := response.(type) {
		case *bool:
			*out = false
		}
		return nil
	}

	output, err := runCommand(newDeleteCmd(), "1")
	require.NoError(t, err)
	require.Empty(t, output)
	require.False(t, svc.lastDeleteInput.AllowActiveDelete)

	require.True(t, contains([]string{"a", "b"}, "b"))
	require.False(t, contains([]string{"a", "b"}, "c"))
}

func TestCommandErrorsPropagate(t *testing.T) {
	originalService := newService
	defer func() { newService = originalService }()

	newService = func() (service, error) { return nil, errors.New("boom") }

	_, err := runCommand(newListCmd())
	require.Error(t, err)
	require.Contains(t, err.Error(), "boom")
}

func TestVersionCommand_DefaultOutput(t *testing.T) {
	originalVersion := Version
	defer func() { Version = originalVersion }()
	Version = "v9.9.9"

	output, err := runCommand(newRootCmd(), "version")
	require.NoError(t, err)
	require.Contains(t, output, "cma version: v9.9.9")
	require.Contains(t, output, "repository: https://github.com/prakersh/codexmultiauth")
	require.Contains(t, output, "support: https://buymeacoffee.com/prakersh")
}

func TestVersionCommand_ShortOutput(t *testing.T) {
	originalVersion := Version
	defer func() { Version = originalVersion }()
	Version = "v1.2.3"

	output, err := runCommand(newRootCmd(), "version", "--short")
	require.NoError(t, err)
	require.Equal(t, "v1.2.3\n", output)
}

func TestVersionCommand_UsesVersionFileFallback(t *testing.T) {
	originalVersion := Version
	defer func() { Version = originalVersion }()
	Version = ""

	output, err := runCommand(newRootCmd(), "version", "--short")
	require.NoError(t, err)
	require.Equal(t, "0.0.1\n", output)
}

func runCommand(cmd *cobra.Command, args ...string) (string, error) {
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func floatPtr(value float64) *float64 {
	return &value
}
