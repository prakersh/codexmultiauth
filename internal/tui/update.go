package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/prakersh/codexmultiauth/internal/domain"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case accountsMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
		} else {
			m.accounts = msg.accounts
			if m.selected >= len(m.accounts) && len(m.accounts) > 0 {
				m.selected = len(m.accounts) - 1
			}
		}
		return m, nil
	case usageMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
		} else {
			m.usage = msg.usage
			m.message = "Usage refreshed"
		}
		return m, nil
	case inspectMsg:
		if msg.err != nil {
			m.clearRestore()
			m.message = msg.err.Error()
			return m, nil
		}
		m.beginRestoreReview(msg)
		return m, nil
	case actionMsg:
		if msg.err != nil {
			m.message = msg.err.Error()
		} else {
			m.message = msg.message
		}
		if msg.clearRestore {
			m.clearRestore()
			m.message = msg.message
		}
		return m, m.loadAccounts()
	case tea.KeyMsg:
		switch m.mode {
		case modeMain:
			return m.updateMain(msg)
		case modeRestoreReview:
			return m.updateRestoreReview(msg)
		case modeRestoreConflict:
			return m.updateRestoreConflict(msg)
		default:
			return m.updateInput(msg)
		}
	}
	return m, nil
}

func (m model) updateMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "j", "down":
		if m.selected < len(m.accounts)-1 {
			m.selected++
		}
	case "k", "up":
		if m.selected > 0 {
			m.selected--
		}
	case "r":
		return m, m.loadAccounts()
	case "u":
		if selector := m.selectedSelector(); selector != "" {
			return m, m.loadUsage(selector)
		}
	case "s":
		return m, m.saveCmd()
	case "a":
		if selector := m.selectedSelector(); selector != "" {
			return m, m.activateCmd(selector)
		}
	case "d":
		if selector := m.selectedSelector(); selector != "" {
			return m, m.deleteCmd(selector)
		}
	case "b":
		m.mode = modeBackupName
		m.input.SetValue(backupDefaultName())
		m.input.Placeholder = "backup name"
		m.input.Focus()
	case "R":
		m.mode = modeRestoreSource
		m.input.SetValue("")
		m.input.Placeholder = "backup name or absolute path"
		m.input.Focus()
	}
	return m, nil
}

func (m model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.clearRestore()
		m.input.Blur()
		m.input.EchoMode = textinput.EchoNormal
		m.mode = modeMain
		return m, nil
	case "enter":
		value := m.input.Value()
		m.input.SetValue("")
		m.input.Blur()
		switch m.mode {
		case modeBackupName:
			m.pendingName = value
			m.mode = modeBackupPass
			m.input.Placeholder = "backup passphrase"
			m.input.EchoMode = textinput.EchoPassword
			m.input.Focus()
			return m, nil
		case modeBackupPass:
			m.mode = modeMain
			m.input.EchoMode = textinput.EchoNormal
			return m, m.backupCmd(m.pendingName, value)
		case modeRestoreSource:
			m.pendingRestore = value
			m.mode = modeRestorePass
			m.input.Placeholder = "restore passphrase"
			m.input.EchoMode = textinput.EchoPassword
			m.input.Focus()
			return m, nil
		case modeRestorePass:
			m.input.EchoMode = textinput.EchoNormal
			m.message = fmt.Sprintf("Inspecting %s", m.pendingRestore)
			return m, m.inspectBackupCmd(m.pendingRestore, value)
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateRestoreReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.clearRestore()
		return m, nil
	case "j", "down":
		if m.restoreCursor < len(m.restoreCandidates)-1 {
			m.restoreCursor++
		}
	case "k", "up":
		if m.restoreCursor > 0 {
			m.restoreCursor--
		}
	case " ":
		if len(m.restoreCandidates) == 0 {
			return m, nil
		}
		if m.restoreAll {
			m.restoreAll = false
		}
		accountID := m.restoreCandidates[m.restoreCursor].Account.ID
		m.restoreSelected[accountID] = !m.restoreSelected[accountID]
	case "A":
		m.restoreAll = !m.restoreAll
		if m.restoreAll {
			for _, candidate := range m.restoreCandidates {
				m.restoreSelected[candidate.Account.ID] = true
			}
		}
	case "c":
		m.restoreConflictPolicy = cycleConflictPolicy(m.restoreConflictPolicy)
	case "enter":
		if !m.restoreAll && len(m.selectedRestoreIDs()) == 0 {
			m.message = "Select at least one account to restore"
			return m, nil
		}
		if m.restoreConflictPolicy == domain.ConflictAsk {
			m.restoreConflictQueue = m.selectedRestoreConflicts()
			m.restoreConflictIndex = 0
			m.restoreConflictChoice = 0
			m.restoreDecisions = map[string]domain.ConflictPolicy{}
			if len(m.restoreConflictQueue) > 0 {
				m.mode = modeRestoreConflict
				return m, nil
			}
		}
		return m, m.restorePlannedCmd()
	}
	return m, nil
}

func (m model) updateRestoreConflict(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeRestoreReview
		m.restoreConflictQueue = nil
		m.restoreConflictIndex = 0
		m.restoreConflictChoice = 0
		return m, nil
	case "left", "h", "up", "k":
		if m.restoreConflictChoice > 0 {
			m.restoreConflictChoice--
		}
	case "right", "l", "down", "j":
		if m.restoreConflictChoice < 2 {
			m.restoreConflictChoice++
		}
	case "enter":
		if len(m.restoreConflictQueue) == 0 {
			m.mode = modeRestoreReview
			return m, nil
		}
		current := m.restoreConflictQueue[m.restoreConflictIndex]
		if m.restoreDecisions == nil {
			m.restoreDecisions = map[string]domain.ConflictPolicy{}
		}
		m.restoreDecisions[current.Account.ID] = conflictChoiceAt(m.restoreConflictChoice)
		if m.restoreConflictIndex < len(m.restoreConflictQueue)-1 {
			m.restoreConflictIndex++
			m.restoreConflictChoice = 0
			return m, nil
		}
		return m, m.restorePlannedCmd()
	}
	return m, nil
}
