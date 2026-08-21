package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/backup"
	"github.com/stretchr/testify/require"
)

type fakeService struct {
	accounts          []app.ListedAccount
	usage             []app.UsageResult
	inspectArtifact   backup.Plaintext
	inspectCandidates []app.RestoreCandidate
	inspectErr        error
	restoreSummary    app.RestoreSummary
	restoreErr        error
	saveResult        app.SaveResult
	saveErr           error
	activateResult    domain.Account
	activateErr       error
	deleteErr         error
	backupPath        string
	backupErr         error

	lastUsageSelector string
	lastBackupInput   app.BackupInput
	lastRestoreInput  app.RestoreInput
	lastSaveInput     app.SaveInput
	lastActivate      string
	lastDeleteInput   app.DeleteInput
	restoreCalls      int
}

func (f *fakeService) List(ctx context.Context) ([]app.ListedAccount, error) { return f.accounts, nil }
func (f *fakeService) Usage(ctx context.Context, selector string) ([]app.UsageResult, error) {
	f.lastUsageSelector = selector
	return f.usage, nil
}
func (f *fakeService) Backup(ctx context.Context, input app.BackupInput) (string, error) {
	f.lastBackupInput = input
	return f.backupPath, f.backupErr
}
func (f *fakeService) InspectBackup(input app.RestoreInput) (backup.Plaintext, []app.RestoreCandidate, error) {
	f.lastRestoreInput = input
	return f.inspectArtifact, f.inspectCandidates, f.inspectErr
}
func (f *fakeService) Restore(ctx context.Context, input app.RestoreInput) (app.RestoreSummary, error) {
	f.lastRestoreInput = input
	f.restoreCalls++
	return f.restoreSummary, f.restoreErr
}
func (f *fakeService) Save(ctx context.Context, input app.SaveInput) (app.SaveResult, error) {
	f.lastSaveInput = input
	return f.saveResult, f.saveErr
}
func (f *fakeService) Activate(ctx context.Context, selector string) (domain.Account, error) {
	f.lastActivate = selector
	return f.activateResult, f.activateErr
}
func (f *fakeService) Delete(ctx context.Context, input app.DeleteInput) error {
	f.lastDeleteInput = input
	return f.deleteErr
}

func TestRestoreWorkflowSupportsInteractiveConflictDecision(t *testing.T) {
	svc := &fakeService{
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
		restoreSummary: app.RestoreSummary{Imported: 1},
	}
	m := model{service: svc, input: textInput()}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	m = next.(model)
	require.Equal(t, modeRestoreSource, m.mode)

	m.input.SetValue("unit-backup")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	require.Equal(t, modeRestorePass, m.mode)
	require.Nil(t, cmd)

	m.input.SetValue("secret")
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	require.Equal(t, modeRestorePass, m.mode)
	require.NotNil(t, cmd)

	msg := cmd()
	next, _ = m.Update(msg)
	m = next.(model)
	require.Equal(t, modeRestoreReview, m.mode)
	require.Len(t, m.restoreCandidates, 2)
	require.True(t, m.restoreSelected["1"])
	require.True(t, m.restoreSelected["2"])

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	require.Equal(t, modeRestoreConflict, m.mode)
	require.Equal(t, 0, m.restoreConflictChoice)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(model)
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	require.Equal(t, modeRestoreConflict, m.mode)
	require.NotNil(t, cmd)

	msg = cmd()
	next, cmd = m.Update(msg)
	m = next.(model)
	require.Equal(t, modeMain, m.mode)
	require.NotNil(t, cmd)
	require.Equal(t, 1, svc.restoreCalls)
	require.Equal(t, []string{"1", "2"}, svc.lastRestoreInput.Selected)
	require.Equal(t, domain.ConflictAsk, svc.lastRestoreInput.Conflict)
	require.Equal(t, app.RestoreDecision{
		Policy:             domain.ConflictSkip,
		ExpectedReason:     "display_name",
		ExpectedExistingID: "existing-1",
	}, svc.lastRestoreInput.Decisions["1"])
	require.Equal(t, "Imported 1 account(s)", m.message)
}

func TestRestoreWorkflowSupportsSelectiveAllAndPolicyCycle(t *testing.T) {
	svc := &fakeService{
		inspectCandidates: []app.RestoreCandidate{
			{Account: domain.Account{ID: "1", DisplayName: "work"}},
			{Account: domain.Account{ID: "2", DisplayName: "personal"}},
		},
		restoreSummary: app.RestoreSummary{Imported: 2},
	}
	m := model{service: svc, input: textInput()}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	m = next.(model)
	m.input.SetValue("backup")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	m.input.SetValue("secret")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	next, _ = m.Update(cmd())
	m = next.(model)
	require.Equal(t, modeRestoreReview, m.mode)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = next.(model)
	require.False(t, m.restoreSelected["2"])

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(model)
	require.Equal(t, domain.ConflictOverwrite, m.restoreConflictPolicy)

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	require.NotNil(t, cmd)
	next, _ = m.Update(cmd())
	m = next.(model)
	require.Equal(t, modeMain, m.mode)
	require.False(t, svc.lastRestoreInput.All)
	require.Equal(t, []string{"1"}, svc.lastRestoreInput.Selected)
	require.Equal(t, domain.ConflictOverwrite, svc.lastRestoreInput.Conflict)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	m = next.(model)
	m.input.SetValue("backup")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	m.input.SetValue("secret")
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	next, _ = m.Update(cmd())
	m = next.(model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")})
	m = next.(model)
	require.True(t, m.restoreAll)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(model)
	require.Equal(t, domain.ConflictSkip, m.restoreConflictPolicy)

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	next, _ = m.Update(cmd())
	m = next.(model)
	require.True(t, svc.lastRestoreInput.All)
	require.Equal(t, domain.ConflictSkip, svc.lastRestoreInput.Conflict)
}

func TestMainActionsAndAsyncMessages(t *testing.T) {
	svc := &fakeService{
		accounts: []app.ListedAccount{
			{Account: domain.Account{ID: "1", DisplayName: "work"}, IsActive: true},
			{Account: domain.Account{ID: "2", DisplayName: "personal"}},
		},
		usage: []app.UsageResult{
			{Account: domain.Account{DisplayName: "work"}, Usage: domain.UsageSummary{Confidence: domain.UsageConfidenceConfirmed}},
		},
		backupPath:     "/tmp/backup.cma.bak",
		saveResult:     app.SaveResult{Account: domain.Account{DisplayName: "saved"}},
		activateResult: domain.Account{DisplayName: "work"},
	}

	m := model{service: svc, input: textInput()}
	initCmd := m.Init()
	require.NotNil(t, initCmd)
	next, _ := m.Update(initCmd())
	m = next.(model)
	require.Len(t, m.accounts, 2)

	require.Equal(t, "1", m.selectedSelector())

	cmd := m.loadUsage("1")
	next, _ = m.Update(cmd())
	m = next.(model)
	require.Equal(t, "Usage refreshed", m.message)
	require.Equal(t, "1", svc.lastUsageSelector)

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = next.(model)
	next, _ = m.Update(cmd())
	m = next.(model)
	require.Equal(t, "Current account saved", m.message)

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = next.(model)
	next, _ = m.Update(cmd())
	m = next.(model)
	require.Equal(t, "Activated work", m.message)
	require.Equal(t, "1", svc.lastActivate)

	// Deleting now takes two keys: d opens the confirmation, y performs it.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(model)
	require.Nil(t, cmd, "d must not delete on its own")
	require.Equal(t, modeDeleteConfirm, m.mode)

	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = next.(model)
	require.Equal(t, modeMain, m.mode)
	next, _ = m.Update(cmd())
	m = next.(model)
	require.Equal(t, "Deleted account", m.message)
	require.Equal(t, "1", svc.lastDeleteInput.Selector)
	require.True(t, svc.lastDeleteInput.AllowActiveDelete)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = next.(model)
	require.Equal(t, modeBackupName, m.mode)
	m.input.SetValue("backup-name")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	require.Equal(t, modeBackupPass, m.mode)
	m.input.SetValue("secret")
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	next, _ = m.Update(cmd())
	m = next.(model)
	require.Equal(t, "Backup complete", m.message)
	require.Equal(t, "backup-name", svc.lastBackupInput.Target)
	require.Equal(t, []byte("secret"), svc.lastBackupInput.Passphrase)
}

func TestViewAndErrorStates(t *testing.T) {
	svc := &fakeService{
		accounts: []app.ListedAccount{
			{Account: domain.Account{ID: "1", DisplayName: "work"}, IsActive: true},
		},
	}
	m := model{service: svc, input: textInput(), accounts: svc.accounts}
	require.Contains(t, m.View(), "CodexMultiAuth")

	next, _ := m.Update(accountsMsg{err: errors.New("list failed")})
	m = next.(model)
	require.Contains(t, m.View(), "list failed")

	next, _ = m.Update(usageMsg{err: errors.New("usage failed")})
	m = next.(model)
	require.Contains(t, m.View(), "usage failed")

	next, _ = m.Update(actionMsg{err: errors.New("save failed")})
	m = next.(model)
	require.Contains(t, m.View(), "save failed")

	m.mode = modeRestoreReview
	m.pendingRestore = "backup.cma.bak"
	m.restoreConflictPolicy = domain.ConflictOverwrite
	m.restoreCandidates = []app.RestoreCandidate{
		{
			Account: domain.Account{ID: "1", DisplayName: "work"},
			Conflict: &app.RestoreConflict{
				Existing: domain.Account{DisplayName: "work"},
				Reason:   "display_name",
			},
		},
	}
	m.restoreSelected = map[string]bool{"1": true}
	require.Contains(t, m.View(), "Restore Review")

	m.mode = modeRestoreConflict
	m.restoreConflictQueue = m.restoreCandidates
	require.Contains(t, m.View(), "Resolve Conflicts")
	require.Equal(t, "all", restoreModeLabel(true))
	require.Equal(t, "selected", restoreModeLabel(false))
}

func TestRunAndLegacyRestoreCmd(t *testing.T) {
	originalProgram := newProgram
	defer func() { newProgram = originalProgram }()

	called := false
	newProgram = func(m tea.Model) teaProgram {
		return fakeProgram{run: func() (tea.Model, error) {
			called = true
			return m, nil
		}}
	}

	require.NoError(t, Run(&fakeService{}))
	require.True(t, called)

	svc := &fakeService{restoreSummary: app.RestoreSummary{Imported: 3}}
	m := model{service: svc}
	msg := m.restoreCmd("backup", "secret")()
	action, ok := msg.(actionMsg)
	require.True(t, ok)
	require.Equal(t, "Imported 3 account(s)", action.message)
	require.True(t, action.clearRestore)
}

func textInput() textinput.Model {
	input := textinput.New()
	return input
}

type fakeProgram struct {
	run func() (tea.Model, error)
}

func (f fakeProgram) Run() (tea.Model, error) {
	return f.run()
}

// TestUsageViewRendersReportedWindowsOnly checks the TUI lists whatever limit
// windows an account reports rather than assuming a fixed 5-hour/weekly pair.
// A free account reports only a monthly window and a paid account only a
// weekly one.
func TestUsageViewRendersReportedWindowsOnly(t *testing.T) {
	monthlyReset := time.Date(2026, 9, 20, 2, 50, 0, 0, time.Local)
	weeklyReset := time.Date(2026, 8, 28, 3, 19, 0, 0, time.Local)
	zero, used := 0.0, 42.5

	m := model{
		input: textInput(),
		usage: []app.UsageResult{
			{
				Account: domain.Account{DisplayName: "p7free"},
				Usage: domain.UsageSummary{
					Confidence: domain.UsageConfidenceConfirmed,
					PlanType:   "free",
					Quotas: []domain.UsageQuota{{
						Name: "monthly", DisplayName: "Monthly Limit",
						WindowSeconds: 2592000, UsedPercent: &zero, ResetsAt: &monthlyReset,
					}},
				},
			},
			{
				Account: domain.Account{DisplayName: "paid"},
				Usage: domain.UsageSummary{
					Confidence: domain.UsageConfidenceConfirmed,
					PlanType:   "plus",
					Quotas: []domain.UsageQuota{{
						Name: "weekly", DisplayName: "Weekly Limit",
						WindowSeconds: 604800, UsedPercent: &used, ResetsAt: &weeklyReset,
					}},
				},
			},
		},
	}

	view := m.View()
	require.Contains(t, view, "Monthly Limit: 0.0% resets Sep 20 02:50")
	require.Contains(t, view, "Weekly Limit: 42.5% resets Aug 28 03:19")
	require.Contains(t, view, "plan: free")
	require.NotContains(t, view, "5-Hour")
}

// TestUsageViewWithoutQuotas keeps the panel informative when the API returned
// no limit windows for an account.
func TestUsageViewWithoutQuotas(t *testing.T) {
	m := model{
		input: textInput(),
		usage: []app.UsageResult{{
			Account: domain.Account{DisplayName: "acct"},
			Usage:   domain.UsageSummary{Confidence: domain.UsageConfidenceBestEffort, PlanType: "free"},
		}},
	}
	require.Contains(t, m.View(), "No limit windows reported")
}

// TestUsageViewEmptyStateWhenNoPercentages covers windows that arrive without a
// usage percentage: the panel must say so rather than show a bare header.
func TestUsageViewEmptyStateWhenNoPercentages(t *testing.T) {
	m := model{
		input: textInput(),
		usage: []app.UsageResult{{
			Account: domain.Account{DisplayName: "acct"},
			Usage: domain.UsageSummary{
				Confidence: domain.UsageConfidenceConfirmed,
				PlanType:   "free",
				Quotas:     []domain.UsageQuota{{Name: "monthly", DisplayName: "Monthly Limit"}},
			},
		}},
	}
	require.Contains(t, m.View(), "No limit windows reported")
}

// TestDeleteRequiresConfirmation covers the destructive path: the vault entry
// is the only copy of an account's credentials, so a single stray keypress
// must not remove it.
func TestDeleteRequiresConfirmation(t *testing.T) {
	svc := &fakeService{accounts: []app.ListedAccount{
		{Account: domain.Account{ID: "1", DisplayName: "work"}, IsActive: true},
		{Account: domain.Account{ID: "2", DisplayName: "personal"}},
	}}
	m := model{service: svc, input: textInput(), accounts: svc.accounts}

	// Any key other than y cancels and deletes nothing.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(model)
	require.Equal(t, modeDeleteConfirm, m.mode)
	require.Contains(t, m.View(), "cannot be recovered")
	require.Contains(t, m.View(), "work")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m = next.(model)
	require.Nil(t, cmd)
	require.Equal(t, modeMain, m.mode)
	require.Equal(t, "Delete cancelled", m.message)
	require.Empty(t, svc.lastDeleteInput.Selector, "cancel must not call Delete")

	// A non-active account must not be force-deleted past the service guard.
	m.selected = 1
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(model)
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = next.(model)
	require.NotNil(t, cmd)
	_ = cmd()
	require.Equal(t, "2", svc.lastDeleteInput.Selector)
	require.False(t, svc.lastDeleteInput.AllowActiveDelete, "only the active account may set AllowActiveDelete")
}
