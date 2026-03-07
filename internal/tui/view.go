package tui

import (
	"fmt"
	"strings"
)

func (m model) View() string {
	switch m.mode {
	case modeBackupName, modeBackupPass, modeRestoreSource, modeRestorePass:
		return titleStyle.Render("CMA TUI") + "\n\n" + m.input.View() + "\n\n" + footerStyle.Render("Enter to submit, Esc to cancel")
	case modeRestoreReview:
		return m.restoreReviewView()
	case modeRestoreConflict:
		return m.restoreConflictView()
	}

	var builder strings.Builder
	builder.WriteString(titleStyle.Render("CodexMultiAuth"))
	builder.WriteString("\n\nAccounts\n")
	if len(m.accounts) == 0 {
		builder.WriteString("  No saved accounts\n")
	}
	for index, account := range m.accounts {
		line := fmt.Sprintf("  %d. %s", index+1, account.Account.DisplayName)
		if account.IsActive {
			line += " " + activeStyle.Render("(active)")
		}
		if index == m.selected {
			line = selectedStyle.Render(line)
		}
		builder.WriteString(line + "\n")
	}
	builder.WriteString("\nUsage\n")
	if len(m.usage) == 0 {
		builder.WriteString("  Press u to fetch usage for the selected account\n")
	}
	for _, usage := range m.usage {
		builder.WriteString(fmt.Sprintf("  %s [%s]\n", usage.Account.DisplayName, usage.Usage.Confidence))
		for _, quota := range usage.Usage.Quotas {
			if quota.UsedPercent != nil {
				builder.WriteString(fmt.Sprintf("    %s: %.1f%%\n", quota.DisplayName, *quota.UsedPercent))
			}
		}
	}
	if m.message != "" {
		builder.WriteString("\n" + statusStyle.Render(m.message) + "\n")
	}
	builder.WriteString("\n")
	builder.WriteString(footerStyle.Render("j/k move  u usage  s save  a activate  d delete  b backup  R restore  r refresh  q quit"))
	return builder.String()
}

func (m model) restoreReviewView() string {
	var builder strings.Builder
	builder.WriteString(titleStyle.Render("Restore Review"))
	builder.WriteString("\n\n")
	builder.WriteString(fmt.Sprintf("Source: %s\n", m.pendingRestore))
	builder.WriteString(fmt.Sprintf("Mode: %s\n", restoreModeLabel(m.restoreAll)))
	builder.WriteString(fmt.Sprintf("Conflict policy: %s\n\n", m.restoreConflictPolicy))
	if len(m.restoreCandidates) == 0 {
		builder.WriteString("No backup accounts found\n")
	} else {
		for i, candidate := range m.restoreCandidates {
			marker := "[ ]"
			if m.restoreSelected[candidate.Account.ID] {
				marker = "[x]"
			}
			line := fmt.Sprintf("  %s %s", marker, candidate.Account.DisplayName)
			if candidate.Conflict != nil {
				line += fmt.Sprintf(" [conflict:%s]", candidate.Conflict.Reason)
			}
			if i == m.restoreCursor {
				line = selectedStyle.Render(line)
			}
			builder.WriteString(line + "\n")
		}
	}
	if m.message != "" {
		builder.WriteString("\n" + statusStyle.Render(m.message) + "\n")
	}
	builder.WriteString("\n")
	builder.WriteString(footerStyle.Render("j/k move  space toggle  A all  c cycle policy  Enter continue  Esc cancel"))
	return builder.String()
}

func (m model) restoreConflictView() string {
	var builder strings.Builder
	builder.WriteString(titleStyle.Render("Resolve Conflicts"))
	builder.WriteString("\n\n")
	if len(m.restoreConflictQueue) == 0 {
		builder.WriteString("No conflicts pending\n")
	} else {
		current := m.restoreConflictQueue[m.restoreConflictIndex]
		builder.WriteString(fmt.Sprintf("Account: %s\n", current.Account.DisplayName))
		builder.WriteString(fmt.Sprintf("Conflict: %s\n", current.Conflict.Reason))
		builder.WriteString(fmt.Sprintf("Existing: %s\n\n", current.Conflict.Existing.DisplayName))
		options := []string{"overwrite", "skip", "rename"}
		for index, option := range options {
			line := "  " + option
			if index == m.restoreConflictChoice {
				line = selectedStyle.Render(line)
			}
			builder.WriteString(line + "\n")
		}
	}
	if m.message != "" {
		builder.WriteString("\n" + statusStyle.Render(m.message) + "\n")
	}
	builder.WriteString("\n")
	builder.WriteString(footerStyle.Render("left/right choose  Enter apply  Esc back"))
	return builder.String()
}

func restoreModeLabel(all bool) string {
	if all {
		return "all"
	}
	return "selected"
}
