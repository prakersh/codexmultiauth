package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/prakersh/codexmultiauth/internal/infra/backup"
)

type mode int

const (
	modeMain mode = iota
	modeBackupName
	modeBackupPass
	modeRestoreSource
	modeRestorePass
	modeRestoreReview
	modeRestoreConflict
)

type model struct {
	service               Service
	accounts              []app.ListedAccount
	usage                 []app.UsageResult
	selected              int
	message               string
	input                 textinput.Model
	mode                  mode
	pendingName           string
	pendingRestore        string
	pendingPass           []byte
	width                 int
	height                int
	restoreCandidates     []app.RestoreCandidate
	restoreSelected       map[string]bool
	restoreCursor         int
	restoreAll            bool
	restoreConflictPolicy domain.ConflictPolicy
	restoreConflictQueue  []app.RestoreCandidate
	restoreConflictIndex  int
	restoreConflictChoice int
	restoreDecisions      map[string]domain.ConflictPolicy
}

type Service interface {
	List(ctx context.Context) ([]app.ListedAccount, error)
	Usage(ctx context.Context, selector string) ([]app.UsageResult, error)
	Backup(ctx context.Context, input app.BackupInput) (string, error)
	InspectBackup(input app.RestoreInput) (backup.Plaintext, []app.RestoreCandidate, error)
	Restore(ctx context.Context, input app.RestoreInput) (app.RestoreSummary, error)
	Save(ctx context.Context, input app.SaveInput) (app.SaveResult, error)
	Activate(ctx context.Context, selector string) (domain.Account, error)
	Delete(ctx context.Context, input app.DeleteInput) error
}

type accountsMsg struct {
	accounts []app.ListedAccount
	err      error
}

type usageMsg struct {
	usage []app.UsageResult
	err   error
}

type actionMsg struct {
	message      string
	err          error
	clearRestore bool
}

type inspectMsg struct {
	source     string
	passphrase []byte
	artifact   backup.Plaintext
	candidates []app.RestoreCandidate
	err        error
}

type teaProgram interface {
	Run() (tea.Model, error)
}

var newProgram = func(m tea.Model) teaProgram {
	return tea.NewProgram(m)
}

func Run(service Service) error {
	input := textinput.New()
	input.Prompt = "> "
	input.CharLimit = 256
	m := model{
		service: service,
		input:   input,
	}
	_, err := newProgram(m).Run()
	return err
}

func (m model) Init() tea.Cmd {
	return m.loadAccounts()
}

func (m model) loadAccounts() tea.Cmd {
	return func() tea.Msg {
		accounts, err := m.service.List(context.Background())
		return accountsMsg{accounts: accounts, err: err}
	}
}

func (m model) loadUsage(selector string) tea.Cmd {
	return func() tea.Msg {
		usage, err := m.service.Usage(context.Background(), selector)
		return usageMsg{usage: usage, err: err}
	}
}

func (m model) inspectBackupCmd(source, passphrase string) tea.Cmd {
	return func() tea.Msg {
		input := app.RestoreInput{
			Source:     source,
			Passphrase: []byte(passphrase),
		}
		artifact, candidates, err := m.service.InspectBackup(input)
		return inspectMsg{
			source:     source,
			passphrase: append([]byte(nil), input.Passphrase...),
			artifact:   artifact,
			candidates: candidates,
			err:        err,
		}
	}
}

func (m model) selectedSelector() string {
	if len(m.accounts) == 0 || m.selected >= len(m.accounts) {
		return ""
	}
	return m.accounts[m.selected].Account.ID
}

func (m model) backupCmd(target, passphrase string) tea.Cmd {
	return func() tea.Msg {
		_, err := m.service.Backup(context.Background(), app.BackupInput{
			Target:     target,
			Passphrase: []byte(passphrase),
		})
		if err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{message: "Backup complete"}
	}
}

func (m model) restoreCmd(source, passphrase string) tea.Cmd {
	return func() tea.Msg {
		summary, err := m.service.Restore(context.Background(), app.RestoreInput{
			Source:     source,
			Passphrase: []byte(passphrase),
			All:        true,
			Conflict:   domain.ConflictOverwrite,
		})
		if err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{message: fmt.Sprintf("Imported %d account(s)", summary.Imported), clearRestore: true}
	}
}

func (m model) restorePlannedCmd() tea.Cmd {
	return func() tea.Msg {
		input := app.RestoreInput{
			Source:     m.pendingRestore,
			Passphrase: append([]byte(nil), m.pendingPass...),
			All:        m.restoreAll,
			Conflict:   m.restoreConflictPolicy,
			Decisions:  copyDecisions(m.restoreDecisions),
		}
		if !m.restoreAll {
			input.Selected = m.selectedRestoreIDs()
		}
		summary, err := m.service.Restore(context.Background(), input)
		if err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{
			message:      fmt.Sprintf("Imported %d account(s)", summary.Imported),
			clearRestore: true,
		}
	}
}

func (m model) saveCmd() tea.Cmd {
	return func() tea.Msg {
		_, err := m.service.Save(context.Background(), app.SaveInput{})
		if err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{message: "Current account saved"}
	}
}

func (m model) activateCmd(selector string) tea.Cmd {
	return func() tea.Msg {
		account, err := m.service.Activate(context.Background(), selector)
		if err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{message: fmt.Sprintf("Activated %s", account.DisplayName)}
	}
}

func (m model) deleteCmd(selector string) tea.Cmd {
	return func() tea.Msg {
		err := m.service.Delete(context.Background(), app.DeleteInput{
			Selector:          selector,
			AllowActiveDelete: true,
		})
		if err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{message: "Deleted account"}
	}
}

func (m model) selectedRestoreIDs() []string {
	var selected []string
	for _, candidate := range m.restoreCandidates {
		if m.restoreSelected[candidate.Account.ID] {
			selected = append(selected, candidate.Account.ID)
		}
	}
	return selected
}

func (m model) selectedRestoreConflicts() []app.RestoreCandidate {
	var conflicts []app.RestoreCandidate
	for _, candidate := range m.restoreCandidates {
		if !m.restoreAll && !m.restoreSelected[candidate.Account.ID] {
			continue
		}
		if candidate.Conflict != nil {
			conflicts = append(conflicts, candidate)
		}
	}
	return conflicts
}

func (m *model) beginRestoreReview(msg inspectMsg) {
	m.mode = modeRestoreReview
	m.pendingRestore = msg.source
	m.pendingPass = append([]byte(nil), msg.passphrase...)
	m.restoreCandidates = append([]app.RestoreCandidate(nil), msg.candidates...)
	m.restoreSelected = map[string]bool{}
	for _, candidate := range msg.candidates {
		m.restoreSelected[candidate.Account.ID] = true
	}
	m.restoreCursor = 0
	m.restoreAll = false
	m.restoreConflictPolicy = domain.ConflictAsk
	m.restoreConflictQueue = nil
	m.restoreConflictIndex = 0
	m.restoreConflictChoice = 0
	m.restoreDecisions = map[string]domain.ConflictPolicy{}
	m.message = fmt.Sprintf("Loaded %d backup account(s)", len(msg.candidates))
}

func (m *model) clearRestore() {
	m.mode = modeMain
	m.pendingRestore = ""
	m.pendingPass = nil
	m.restoreCandidates = nil
	m.restoreSelected = nil
	m.restoreCursor = 0
	m.restoreAll = false
	m.restoreConflictPolicy = ""
	m.restoreConflictQueue = nil
	m.restoreConflictIndex = 0
	m.restoreConflictChoice = 0
	m.restoreDecisions = nil
}

func conflictChoiceAt(index int) domain.ConflictPolicy {
	options := []domain.ConflictPolicy{
		domain.ConflictOverwrite,
		domain.ConflictSkip,
		domain.ConflictRename,
	}
	if index < 0 {
		return options[0]
	}
	return options[index%len(options)]
}

func cycleConflictPolicy(current domain.ConflictPolicy) domain.ConflictPolicy {
	options := []domain.ConflictPolicy{
		domain.ConflictAsk,
		domain.ConflictOverwrite,
		domain.ConflictSkip,
		domain.ConflictRename,
	}
	for i, option := range options {
		if option == current {
			return options[(i+1)%len(options)]
		}
	}
	return options[0]
}

func copyDecisions(in map[string]domain.ConflictPolicy) map[string]domain.ConflictPolicy {
	if len(in) == 0 {
		return map[string]domain.ConflictPolicy{}
	}
	out := make(map[string]domain.ConflictPolicy, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	activeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	footerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("230"))
)

func backupDefaultName() string {
	return "backup-" + time.Now().UTC().Format("20060102-150405")
}
