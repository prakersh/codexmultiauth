package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
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
	require.NotNil(t, cmd, "start the passphrase input cursor")

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

	m.accounts = []app.ListedAccount{
		{Account: domain.Account{DisplayName: "p7free"}},
		{Account: domain.Account{DisplayName: "paid"}},
	}
	view := m.View()
	require.Contains(t, view, "Monthly Limit")
	require.Contains(t, view, "0.0% used")
	require.Contains(t, view, "Resets Sep 20 02:50")
	require.Contains(t, view, "plan: free")
	require.NotContains(t, view, "Weekly Limit")
	require.NotContains(t, view, "5-Hour")
	m.selected = 1
	view = m.View()
	require.Contains(t, view, "Weekly Limit")
	require.Contains(t, view, "42.5% used")
	require.Contains(t, view, "Resets Aug 28 03:19")
	require.NotContains(t, view, "Monthly Limit")
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
	m.accounts = []app.ListedAccount{{Account: domain.Account{DisplayName: "acct"}}}
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
	m.accounts = []app.ListedAccount{{Account: domain.Account{DisplayName: "acct"}}}
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

func TestResponsiveMainPanels(t *testing.T) {
	m := model{input: textInput(), width: 120, height: 24, accounts: []app.ListedAccount{
		{Account: domain.Account{ID: "one", DisplayName: "personal"}, IsActive: true},
		{Account: domain.Account{ID: "two", DisplayName: "work"}},
	}}
	wide := strings.Split(ansi.Strip(m.View()), "\n")
	sharedHeading := false
	for _, line := range wide {
		if strings.Contains(line, "Accounts") && strings.Contains(line, "Usage") {
			sharedHeading = true
		}
	}
	require.True(t, sharedHeading, "wide terminals should place both panels on the same row")
	m.width = 48
	narrow := ansi.Strip(m.View())
	require.Greater(t, strings.Index(narrow, "Usage"), strings.Index(narrow, "personal"), "narrow terminals should stack Usage below Accounts")
}

func TestAllViewsStayWithinTerminalBounds(t *testing.T) {
	longName := strings.Repeat("工程e\u0301", 35)
	for _, size := range [][2]int{{1, 1}, {20, 8}, {40, 16}, {80, 24}, {120, 32}} {
		for _, screen := range []mode{modeMain, modeBackupName, modeBackupPass, modeRestoreSource, modeRestorePass, modeRestoreReview, modeRestoreConflict, modeDeleteConfirm} {
			t.Run(fmt.Sprintf("%dx%d/mode%d", size[0], size[1], screen), func(t *testing.T) {
				candidate := app.RestoreCandidate{
					Account:  domain.Account{ID: "one", DisplayName: longName},
					Conflict: &app.RestoreConflict{Reason: longName, Existing: domain.Account{DisplayName: longName}},
				}
				m := model{
					input: textInput(), width: size[0], height: size[1], mode: screen,
					accounts:          []app.ListedAccount{{Account: candidate.Account, IsActive: true}},
					pendingDeleteName: longName, pendingDeleteActive: true,
					pendingRestore:    strings.Repeat("/very-long-backup-path", 20),
					restoreCandidates: []app.RestoreCandidate{candidate}, restoreSelected: map[string]bool{"one": true},
					restoreConflictQueue: []app.RestoreCandidate{candidate}, message: strings.Repeat("Very long status ", 30),
				}
				m.input.SetValue(longName)
				lines := strings.Split(m.View(), "\n")
				require.LessOrEqual(t, len(lines), m.height)
				for row, line := range lines {
					require.LessOrEqual(t, ansi.StringWidth(line), m.width, "row %d: %q", row, ansi.Strip(line))
				}
			})
		}
	}
}

func TestLongListsKeepSelectionVisible(t *testing.T) {
	m := model{input: textInput(), width: 80, height: 20, selected: 37, restoreCursor: 37, restoreSelected: map[string]bool{}}
	for i := range 50 {
		account := domain.Account{ID: fmt.Sprintf("id%d", i), DisplayName: fmt.Sprintf("account-%02d", i)}
		m.accounts = append(m.accounts, app.ListedAccount{Account: account})
		m.restoreCandidates = append(m.restoreCandidates, app.RestoreCandidate{Account: account})
	}
	require.Contains(t, m.View(), "> account-37")
	require.NotContains(t, m.View(), "account-00")
	m.mode = modeRestoreReview
	require.Contains(t, m.View(), "> [ ] account-37")
	require.NotContains(t, m.View(), "account-00")
}

func TestUsagePanelFollowsSelectedAccount(t *testing.T) {
	used := 27.0
	m := model{input: textInput(), width: 120, height: 24, selected: 1,
		accounts: []app.ListedAccount{
			{Account: domain.Account{ID: "personal", DisplayName: "personal"}},
			{Account: domain.Account{ID: "work", DisplayName: "work"}},
		},
		usage: []app.UsageResult{{
			Account: domain.Account{ID: "personal", DisplayName: "personal"},
			Usage: domain.UsageSummary{PlanType: "personal-plan", Confidence: domain.UsageConfidenceConfirmed,
				Quotas: []domain.UsageQuota{{DisplayName: "Weekly Limit", UsedPercent: &used}}},
		}},
	}
	require.NotContains(t, m.View(), "personal-plan", "changing selection must not show another account's usage")
	require.Contains(t, m.View(), "Press u")
	m.selected = 0
	view := m.View()
	require.Contains(t, view, "27.0% used")
	require.Contains(t, view, "73.0% remaining")
	require.Contains(t, view, "#")
}

func TestAccountRefreshPreservesSelectionAndUsageCache(t *testing.T) {
	personal := domain.Account{ID: "one", DisplayName: "personal"}
	work := domain.Account{ID: "two", DisplayName: "work"}
	m := model{input: textInput(), selected: 1, accounts: []app.ListedAccount{{Account: personal}, {Account: work}}}
	next, _ := m.Update(accountsMsg{accounts: []app.ListedAccount{{Account: work}, {Account: personal}}})
	m = next.(model)
	require.Equal(t, "two", m.selectedSelector(), "reload must preserve the selected account when list order changes")
	for _, account := range []domain.Account{personal, work} {
		next, _ = m.Update(usageMsg{usage: []app.UsageResult{{Account: account, Usage: domain.UsageSummary{PlanType: account.DisplayName}}}})
		m = next.(model)
	}
	require.Len(t, m.usage, 2, "fetching another account should preserve already fetched usage")
	require.Equal(t, "work", m.selectedUsage().PlanType)
	next, _ = m.Update(accountsMsg{})
	m = next.(model)
	require.Equal(t, 0, m.selected)
	require.Empty(t, m.selectedSelector())
}

func TestResizedInputKeepsCursorEndVisibleAndPassphraseHidden(t *testing.T) {
	m := model{input: textInput(), mode: modeBackupName, width: 40, height: 14}
	m.input.SetValue(strings.Repeat("very-long-prefix-", 20) + "VISIBLE_END")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 24, Height: 14})
	m = next.(model)
	require.Contains(t, ansi.Strip(m.View()), "VISIBLE_END")
	m.mode = modeBackupPass
	m.input.EchoMode = textinput.EchoPassword
	require.NotContains(t, m.View(), "VISIBLE_END")
}

func TestUsagePanelsKeepBothQuotaResetsAtNormalNarrowSize(t *testing.T) {
	used := 1.0
	reset := time.Date(2026, 10, 12, 4, 17, 0, 0, time.Local)
	usage := domain.UsageSummary{PlanType: "plus", Confidence: domain.UsageConfidenceConfirmed, FetchedAt: reset,
		Quotas: []domain.UsageQuota{
			{DisplayName: "5-Hour Limit", UsedPercent: &used, ResetsAt: &reset},
			{DisplayName: "Weekly Limit", UsedPercent: &used, ResetsAt: &reset},
		}}
	m := model{input: textInput(), width: 80, height: 24, accounts: []app.ListedAccount{
		{Account: domain.Account{ID: "one", DisplayName: "personal", Usage: &usage}},
		{Account: domain.Account{ID: "two", DisplayName: "work"}},
	}}
	view := ansi.Strip(m.View())
	require.Contains(t, view, "5-Hour Limit")
	require.Contains(t, view, "Weekly Limit")
	require.Equal(t, 2, strings.Count(view, "Resets Oct 12 04:17"))
}

func TestShortViewsPreserveResetTimesAndDeleteTarget(t *testing.T) {
	used := 1.0
	reset := time.Date(2026, 10, 12, 4, 17, 0, 0, time.Local)
	m := model{input: textInput(), width: 76, height: 12, accounts: []app.ListedAccount{
		{Account: domain.Account{ID: "one", DisplayName: "personal"}},
		{Account: domain.Account{ID: "two", DisplayName: "work", Usage: &domain.UsageSummary{Quotas: []domain.UsageQuota{
			{DisplayName: "5-Hour Limit", UsedPercent: &used, ResetsAt: &reset},
			{DisplayName: "Weekly Limit", UsedPercent: &used, ResetsAt: &reset},
		}}}},
	}, selected: 1}
	require.Equal(t, 2, strings.Count(ansi.Strip(m.View()), "Resets Oct 12 04:17"))
	m.width, m.height, m.mode = 20, 8, modeDeleteConfirm
	m.pendingDeleteName, m.pendingDeleteActive = "work", true
	view := ansi.Strip(m.View())
	require.Contains(t, view, "Delete work?")
	require.Contains(t, view, "Active account.")
	require.Contains(t, view, "Cannot recover")
}

func TestCompactQuotaKeepsRemainingValueWithLongLabel(t *testing.T) {
	used := 27.0
	m := model{input: textInput(), width: 76, height: 12, accounts: []app.ListedAccount{
		{Account: domain.Account{ID: "one", DisplayName: "work", Usage: &domain.UsageSummary{
			Quotas: []domain.UsageQuota{{DisplayName: strings.Repeat("Very long quota label ", 20), UsedPercent: &used}},
		}}},
	}}
	require.Contains(t, ansi.Strip(m.View()), "73.0% remaining")
}

func TestMinimumWidthQuotaRowsAreCompleteOrExplicitlyHidden(t *testing.T) {
	used := 1.0
	reset := time.Date(2026, 10, 12, 4, 17, 0, 0, time.Local)
	m := model{accounts: []app.ListedAccount{{Account: domain.Account{ID: "one", DisplayName: "work",
		Usage: &domain.UsageSummary{Confidence: domain.UsageConfidenceConfirmed, Quotas: []domain.UsageQuota{
			{DisplayName: "5-Hour Limit", UsedPercent: &used, ResetsAt: &reset},
			{DisplayName: "Weekly Limit", UsedPercent: &used, ResetsAt: &reset},
		}},
	}}}}
	for _, width := range []int{20, 40} {
		view := ansi.Strip(strings.Join(m.compactUsagePanel(width, 3), "\n"))
		require.Contains(t, view, "+2 more: cma usage", "incomplete quota details must not be cropped into indistinguishable rows")
		require.NotContains(t, view, "remaining |")
	}
	view := ansi.Strip(strings.Join(m.compactUsagePanel(20, 5), "\n"))
	require.Contains(t, view, "5-Hour")
	require.Contains(t, view, "Weekly")
	require.Equal(t, 2, strings.Count(view, "99.0% left"))
	require.Equal(t, 2, strings.Count(view, "Resets Oct 12 04:17"))
	for _, line := range strings.Split(view, "\n") {
		require.LessOrEqual(t, ansi.StringWidth(line), 20)
	}
}

func TestPassphraseViewsAlwaysMaskTheirInput(t *testing.T) {
	for _, screen := range []mode{modeBackupPass, modeRestorePass} {
		m := model{input: textInput(), mode: screen, width: 20, height: 8}
		m.input.SetValue("never-render-this-value")
		view := ansi.Strip(m.View())
		require.NotContains(t, view, "never-render")
		require.Contains(t, view, "*")
	}
}

func TestTinyTerminalBlocksSubmissionsAndAllowsCancel(t *testing.T) {
	for _, screen := range []mode{modeMain, modeBackupPass, modeRestoreReview, modeRestoreConflict, modeDeleteConfirm} {
		for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyRunes, Runes: []rune("y")}, {Type: tea.KeyRunes, Runes: []rune("a")}} {
			m := model{service: &fakeService{}, input: textInput(), mode: screen, width: 40, height: 5,
				pendingDelete: "one", pendingDeleteName: "work", pendingDeleteActive: true,
				accounts: []app.ListedAccount{{Account: domain.Account{ID: "one", DisplayName: "work"}}},
			}
			require.Contains(t, ansi.Strip(m.View()), "Resize to at least")
			next, cmd := m.Update(key)
			require.Nil(t, cmd, "tiny terminal must not submit in mode %d", screen)
			require.Equal(t, screen, next.(model).mode)
			next, _ = next.Update(tea.KeyMsg{Type: tea.KeyEsc})
			cancelMode := modeMain
			if screen == modeRestoreConflict {
				cancelMode = modeRestoreReview
			}
			require.Equal(t, cancelMode, next.(model).mode)
		}
	}
	m := model{input: textInput(), width: 16, height: 4, mode: modeDeleteConfirm, pendingDelete: "one"}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	require.NotNil(t, cmd)
	require.IsType(t, tea.QuitMsg{}, cmd())
	m = next.(model)
	next, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	require.False(t, next.(model).terminalTooSmall())
}

func TestShortInputAndRestoreViewsKeepPrimaryControls(t *testing.T) {
	m := model{input: textInput(), width: 20, height: 8}
	m.input.SetValue("visible-input")
	m.mode = modeBackupName
	require.Contains(t, ansi.Strip(m.View()), "visible-input")
	for _, screen := range []mode{modeBackupName, modeRestoreReview, modeRestoreConflict} {
		m.mode = screen
		view := ansi.Strip(m.View())
		require.Contains(t, view, "Enter")
		require.Contains(t, view, "Esc")
	}
}

func TestUsageDensityFitsReportedWindowsAndShowsOverflow(t *testing.T) {
	for _, fixture := range []struct{ width, quotaCount int }{{80, 3}, {120, 4}, {120, 30}} {
		t.Run(fmt.Sprintf("width%d/quotas%d", fixture.width, fixture.quotaCount), func(t *testing.T) {
			used := 12.0
			reset := time.Date(2026, 10, 12, 4, 17, 0, 0, time.Local)
			usage := domain.UsageSummary{PlanType: "plus", Confidence: domain.UsageConfidenceConfirmed, FetchedAt: reset}
			for i := range fixture.quotaCount {
				usage.Quotas = append(usage.Quotas, domain.UsageQuota{DisplayName: fmt.Sprintf("Quota %d", i+1), UsedPercent: &used, ResetsAt: &reset})
			}
			m := model{input: textInput(), width: fixture.width, height: 24, accounts: []app.ListedAccount{
				{Account: domain.Account{ID: "one", DisplayName: "personal", Usage: &usage}},
				{Account: domain.Account{ID: "two", DisplayName: "work"}},
			}}
			view := ansi.Strip(m.View())
			if fixture.quotaCount <= 4 {
				require.Equal(t, fixture.quotaCount, strings.Count(view, "Resets Oct 12 04:17"))
				require.Contains(t, view, fmt.Sprintf("Quota %d", fixture.quotaCount))
			} else {
				require.Contains(t, view, "more: cma usage")
			}
		})
	}
}

func TestNarrowRestoreReviewShowsPolicyAndTarget(t *testing.T) {
	m := model{input: textInput(), width: 20, height: 8, mode: modeRestoreReview,
		restoreConflictPolicy: domain.ConflictAsk, restoreSelected: map[string]bool{},
		restoreCandidates: []app.RestoreCandidate{{
			Account:  domain.Account{ID: "one", DisplayName: "work-safe"},
			Conflict: &app.RestoreConflict{Reason: "display_name"},
		}},
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	view := ansi.Strip(next.View())
	require.Contains(t, view, "Policy: overwrite")
	require.Contains(t, view, "work-safe")
	require.Contains(t, view, " !")
	require.Contains(t, view, "Enter")
	require.Contains(t, view, "Esc")
}

func TestCompactUsageKeepsConfidenceAndLimitState(t *testing.T) {
	used := 100.0
	for _, confidence := range []domain.UsageConfidence{domain.UsageConfidenceConfirmed, domain.UsageConfidenceBestEffort, domain.UsageConfidenceUnknown} {
		m := model{input: textInput(), width: 76, height: 12, accounts: []app.ListedAccount{
			{Account: domain.Account{ID: "one", DisplayName: strings.Repeat("Long account name ", 10), Usage: &domain.UsageSummary{
				Confidence: confidence, LimitReached: true,
				Quotas: []domain.UsageQuota{{DisplayName: "Weekly", UsedPercent: &used}},
			}}},
		}}
		view := ansi.Strip(m.View())
		require.Contains(t, view, strings.ReplaceAll(string(confidence), "_", " "))
		require.Contains(t, view, "limited")
	}
}

func TestMinimumWidthKeepsCompactConfidenceAndLimitState(t *testing.T) {
	used := 100.0
	for _, fixture := range []struct {
		confidence domain.UsageConfidence
		label      string
	}{{domain.UsageConfidenceConfirmed, "live"}, {domain.UsageConfidenceBestEffort, "best effort"}, {domain.UsageConfidenceUnknown, "unknown"}} {
		m := model{input: textInput(), width: 20, height: 12, accounts: []app.ListedAccount{
			{Account: domain.Account{ID: "one", DisplayName: "work account", Usage: &domain.UsageSummary{
				Confidence: fixture.confidence, LimitReached: true,
				Quotas: []domain.UsageQuota{{DisplayName: "Weekly", UsedPercent: &used}},
			}}},
		}}
		view := ansi.Strip(m.View())
		require.Contains(t, view, fixture.label)
		require.Contains(t, view, "limited")
	}
}
