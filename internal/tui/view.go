package tui

import (
	"fmt"
	"strings"
	"time"
)

func (m model) View() string {
	switch m.mode {
	case modeBackupName, modeBackupPass, modeRestoreSource, modeRestorePass:
		return titleStyle.Render("CMA TUI") + "\n\n" + m.input.View() + "\n\n" + footerStyle.Render("Enter to submit, Esc to cancel")
	case modeRestoreReview:
		return m.restoreReviewView()
	case modeRestoreConflict:
		return m.restoreConflictView()
	case modeDeleteConfirm:
		return m.deleteConfirmView()
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
		header := fmt.Sprintf("  %s [%s]", usage.Account.DisplayName, usage.Usage.Confidence)
		if usage.Usage.PlanType != "" {
			header += " plan: " + usage.Usage.PlanType
		}
		if usage.Usage.LimitReached {
			header += " limit reached"
		}
		builder.WriteString(header + "\n")
		// Quota rows come straight from whatever windows the API reported, so
		// plans with only a monthly or only a weekly limit render correctly
		// without the view assuming a fixed set of windows.
		rendered := 0
		for _, quota := range usage.Usage.Quotas {
			if quota.UsedPercent == nil {
				continue
			}
			line := fmt.Sprintf("    %s: %.1f%%", quota.DisplayName, *quota.UsedPercent)
			if quota.ResetsAt != nil && !quota.ResetsAt.IsZero() {
				line += " resets " + quota.ResetsAt.In(time.Local).Format("Jan 02 15:04")
			}
			builder.WriteString(line + "\n")
			rendered++
		}
		// Counts rows actually written, not quotas received: windows can
		// arrive without a percentage and would otherwise leave a bare header.
		if rendered == 0 {
			builder.WriteString("    No limit windows reported\n")
		}
	}
	if m.message != "" {
		builder.WriteString("\n" + statusStyle.Render(m.message) + "\n")
	}
	builder.WriteString("\n")
	builder.WriteString(footerStyle.Render("j/k move  u usage  s save  a activate  d delete (asks first)  b backup  R restore  r refresh  q quit"))
	return builder.String()
}

func (m model) deleteConfirmView() string {
	name := m.pendingDeleteName
	if name == "" {
		name = m.pendingDelete
	}
	var builder strings.Builder
	builder.WriteString(titleStyle.Render("Delete Account"))
	builder.WriteString("\n\n")
	builder.WriteString(fmt.Sprintf("Delete %s?\n", name))
	if m.pendingDeleteActive {
		builder.WriteString("This is the active account.\n")
	}
	builder.WriteString("\nThe stored credentials are removed from the vault and cannot be recovered.\n\n")
	builder.WriteString(footerStyle.Render("y to delete, any other key to cancel"))
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
