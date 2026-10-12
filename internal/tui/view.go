package tui

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/x/ansi"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/domain"
)

func (m model) View() string {
	if m.terminalTooSmall() {
		return m.resizeView()
	}
	switch m.mode {
	case modeBackupName, modeBackupPass, modeRestoreSource, modeRestorePass:
		return m.inputView()
	case modeRestoreReview:
		return m.restoreReviewView()
	case modeRestoreConflict:
		return m.restoreConflictView()
	case modeDeleteConfirm:
		return m.deleteConfirmView()
	default:
		return m.mainView()
	}
}

const (
	minimumWidth  = 20
	minimumHeight = 8
)

func (m model) terminalTooSmall() bool {
	return m.width > 0 && m.height > 0 && (m.width < minimumWidth || m.height < minimumHeight)
}

func (m model) resizeView() string {
	width, _ := m.size()
	head := []string{titleStyle.Render(fitText("Terminal too small", width))}
	body := wrappedText(fmt.Sprintf("Resize to at least %d columns and %d rows.", minimumWidth, minimumHeight), width)
	foot := []string{footerStyle.Render(fitText("q quit  Esc cancel", width))}
	return m.finish(head, body, foot)
}

func (m model) size() (int, int) {
	width, height := m.width, m.height
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 26
	}
	return width, height
}

// frame reserves the footer before allocating content, so resizing never hides
// the available keys behind a long list or a status message.
func (m model) frame(title, subtitle string, help []string) (head, foot []string, width, bodyHeight int) {
	width, height := m.size()
	head = []string{titleStyle.Render(fitText(title, width))}
	if height >= 16 && subtitle != "" {
		head = append(head, footerStyle.Render(fitText(subtitle, width)))
	}
	if height > 14 {
		head = append(head, "")
	}
	foot = helpLines(help, width)
	maxHelp := max(1, min(4, height/4))
	if len(foot) > maxHelp {
		foot = foot[:maxHelp]
	}
	for i, line := range foot {
		foot[i] = footerStyle.Render(line)
	}
	if m.message != "" && height >= 10 {
		foot = append([]string{statusStyle.Render(fitText(m.message, width))}, foot...)
	}
	if height > 14 {
		foot = append([]string{""}, foot...)
	}
	bodyHeight = max(0, height-len(head)-len(foot))
	return head, foot, width, bodyHeight
}

func (m model) finish(head, body, foot []string) string {
	width, height := m.size()
	bodyHeight := max(0, height-len(head)-len(foot))
	body = boundedLines(body, width, bodyHeight)
	for len(body) < bodyHeight {
		body = append(body, "")
	}
	lines := append(head, body...)
	lines = append(lines, foot...)
	return strings.Join(boundedLines(lines, width, height), "\n")
}

func (m model) mainView() string {
	help := []string{
		"j/k move  u usage  a activate  s save  d delete",
		"b backup  R restore  r reload  q quit",
	}
	width, height := m.size()
	if height <= 14 {
		help = []string{"j/k move  u usage  a activate  q quit"}
	} else if width < 32 {
		help = []string{"j/k move  u usage", "a use  s save  d del", "b backup  R restore", "r reload  q quit"}
	}
	head, foot, width, bodyHeight := m.frame("CodexMultiAuth", "Accounts and quota", help)
	var body []string
	if width >= 96 {
		leftWidth := min(38, width/3)
		rightWidth := width - leftWidth - 3
		left := m.accountsPanel(leftWidth, bodyHeight)
		right := m.usagePanel(rightWidth, bodyHeight)
		for row := range bodyHeight {
			body = append(body, padLine(lineAt(left, row), leftWidth)+footerStyle.Render(" | ")+lineAt(right, row))
		}
	} else {
		// The account list gives space back to quota details when it is short.
		accountHeight := min(len(m.accounts)+3, max(2, bodyHeight/2))
		if bodyHeight >= 9 {
			accountHeight = min(len(m.accounts)+3, max(3, bodyHeight/2))
		}
		accountHeight = min(accountHeight, bodyHeight)
		body = m.accountsPanel(width, accountHeight)
		usageHeight := bodyHeight - len(body)
		if usageHeight > 3 {
			body = append(body, "")
			usageHeight--
		}
		body = append(body, m.usagePanel(width, usageHeight)...)
	}
	return m.finish(head, body, foot)
}

func (m model) accountsPanel(width, height int) []string {
	if height <= 0 {
		return nil
	}
	lines := []string{sectionStyle.Render(fitText(fmt.Sprintf("Accounts (%d)", len(m.accounts)), width))}
	if height >= 4 {
		lines = append(lines, footerStyle.Render(strings.Repeat("-", width)))
	}
	if len(m.accounts) == 0 {
		lines = append(lines, wrappedText("No saved accounts. Press s to save the current Codex account.", width)...)
		return boundedLines(lines, width, height)
	}
	slots := height - len(lines)
	showPosition := len(m.accounts) > slots && slots >= 2
	if showPosition {
		slots--
	}
	start, end := visibleRange(len(m.accounts), m.selected, slots)
	for i := start; i < end; i++ {
		account := m.accounts[i]
		marker := "  "
		if i == m.selected {
			marker = "> "
		}
		suffix := ""
		if account.IsActive {
			suffix = " * active"
		}
		nameWidth := max(0, width-ansi.StringWidth(marker)-ansi.StringWidth(suffix))
		line := marker + fitText(account.Account.DisplayName, nameWidth) + suffix
		if i == m.selected {
			line = selectedStyle.Render(padLine(line, width))
		} else if account.IsActive {
			line = activeStyle.Render(line)
		}
		lines = append(lines, line)
	}
	if showPosition {
		lines = append(lines, footerStyle.Render(fitText(fmt.Sprintf("%d-%d of %d", start+1, end, len(m.accounts)), width)))
	}
	return boundedLines(lines, width, height)
}

func (m model) selectedUsageResult() *app.UsageResult {
	if m.selected < 0 || m.selected >= len(m.accounts) {
		return nil
	}
	account := m.accounts[m.selected].Account
	for _, result := range m.usage {
		if (account.ID != "" && result.Account.ID == account.ID) ||
			(result.Account.ID == "" && result.Account.DisplayName == account.DisplayName) {
			return &result
		}
	}
	return nil
}

func (m model) selectedUsage() *domain.UsageSummary {
	if result := m.selectedUsageResult(); result != nil {
		return &result.Usage
	}
	if m.selected >= 0 && m.selected < len(m.accounts) {
		return m.accounts[m.selected].Account.Usage
	}
	return nil
}

func (m model) usagePanel(width, height int) []string {
	if height <= 0 {
		return nil
	}
	if height < 10 && m.selected >= 0 && m.selected < len(m.accounts) {
		return m.compactUsagePanel(width, height)
	}
	lines := []string{sectionStyle.Render(fitText("Usage", width))}
	if height >= 4 {
		lines = append(lines, footerStyle.Render(strings.Repeat("-", width)))
	}
	if m.selected < 0 || m.selected >= len(m.accounts) {
		lines = append(lines, wrappedText("Select a saved account to view its quota.", width)...)
		return boundedLines(lines, width, height)
	}
	account := m.accounts[m.selected]
	name := account.Account.DisplayName
	if account.IsActive {
		name += " (active)"
	}
	lines = append(lines, titleStyle.Render(fitText(name, width)))
	if result := m.selectedUsageResult(); result != nil && height >= 16 && result.Info.UserEmail != "" {
		lines = append(lines, footerStyle.Render(fitText(result.Info.UserEmail, width)))
	}
	usage := m.selectedUsage()
	if usage == nil {
		lines = append(lines, "")
		lines = append(lines, wrappedText("Press u to fetch usage for this account.", width)...)
		return boundedLines(lines, width, height)
	}
	confidence := strings.ReplaceAll(string(usage.Confidence), "_", " ")
	if confidence == "" {
		confidence = "unknown"
	}
	problem := false
	if result := m.selectedUsageResult(); result != nil && result.Problem != nil {
		problem = true
		if result.Problem.RequiresLogin {
			confidence = "login required"
		} else {
			confidence = "usage error"
		}
	}
	metadata := confidence
	if usage.PlanType != "" {
		if problem {
			metadata = confidence + " | plan: " + usage.PlanType
		} else {
			metadata = "plan: " + usage.PlanType + " | " + confidence
		}
	}
	lines = append(lines, footerStyle.Render(fitText(metadata, width)))
	if usage.LimitReached {
		lines = append(lines, warningStyle.Render(fitText("Limit reached", width)))
	}
	if !usage.FetchedAt.IsZero() {
		lines = append(lines, footerStyle.Render(fitText("Updated "+usage.FetchedAt.In(time.Local).Format("Jan 02 15:04 MST"), width)))
	}
	if height >= 16 {
		lines = append(lines, "")
	}
	rendered := 0
	for _, quota := range usage.Quotas {
		if quota.UsedPercent == nil {
			continue
		}
		label := quota.DisplayName
		if label == "" {
			label = quota.Name
		}
		remaining := remainingPercent(*quota.UsedPercent)
		values := fmt.Sprintf("%.1f%% used  %.1f%% remaining", *quota.UsedPercent, remaining)
		if ansi.StringWidth(plainText(label))+2+len(values) <= width {
			lines = append(lines, sectionStyle.Render(plainText(label))+"  "+values)
		} else {
			lines = append(lines, sectionStyle.Render(fitText(label, width)))
			lines = append(lines, wrappedText(values, width)...)
		}
		if width >= 12 {
			barWidth := min(36, width-2)
			filled := int(math.Round(float64(barWidth) * remaining / 100))
			bar := "[" + strings.Repeat("#", filled) + strings.Repeat("-", barWidth-filled) + "]"
			style := activeStyle
			if remaining <= 10 {
				style = warningStyle
			}
			lines = append(lines, style.Render(bar))
		}
		if quota.ResetsAt != nil && !quota.ResetsAt.IsZero() {
			lines = append(lines, footerStyle.Render(quotaResetLine(*quota.ResetsAt, width)))
		}
		if height >= 16 {
			lines = append(lines, "")
		}
		rendered++
	}
	if rendered == 0 {
		lines = append(lines, wrappedText("No limit windows reported", width)...)
	} else if len(lines) > height {
		return m.compactUsagePanel(width, height)
	}
	return boundedLines(lines, width, height)
}

func (m model) compactUsagePanel(width, height int) []string {
	usage := m.selectedUsage()
	caption := "Usage: " + m.selectedDisplayName()
	if usage != nil {
		status := strings.ReplaceAll(string(usage.Confidence), "_", " ")
		if status == "" {
			status = "unknown"
		}
		if width < 32 {
			if usage.Confidence == domain.UsageConfidenceConfirmed {
				status = "live"
			} else if usage.Confidence == domain.UsageConfidenceBestEffort {
				status = "best effort"
			}
		}
		if usage.LimitReached {
			status += " limited"
		}
		if result := m.selectedUsageResult(); result != nil && result.Problem != nil {
			if result.Problem.RequiresLogin {
				status = "login required"
			} else {
				status = "usage error"
			}
		}
		nameWidth := width - len("Usage:  | ") - len(status)
		if nameWidth > 0 {
			caption = "Usage: " + fitText(m.selectedDisplayName(), nameWidth) + " | " + status
		} else {
			caption = "Usage | " + status
			if len(caption) > width {
				// The surrounding account selection identifies this panel;
				// preserve provenance and limit state before its label.
				caption = status
			}
		}
	}
	lines := []string{sectionStyle.Render(fitText(caption, width))}
	if usage == nil {
		lines = append(lines, wrappedText("Press u to fetch usage.", width)...)
		return boundedLines(lines, width, height)
	}
	var quotas []domain.UsageQuota
	for _, quota := range usage.Quotas {
		if quota.UsedPercent != nil {
			quotas = append(quotas, quota)
		}
	}
	quotaRows := len(quotas)
	for _, quota := range quotas {
		if quota.ResetsAt != nil && !quota.ResetsAt.IsZero() {
			quotaRows++
		}
	}
	inlineResets := quotaRows+1 > height
	if inlineResets {
		for _, quota := range quotas {
			value := fmt.Sprintf("%.1f%% remaining", remainingPercent(*quota.UsedPercent))
			if quota.ResetsAt != nil && !quota.ResetsAt.IsZero() {
				value += " | reset " + quota.ResetsAt.In(time.Local).Format("Jan 02 15:04")
			}
			// Keep enough space to identify the window as well as its complete
			// value. Cropping either makes different quotas indistinguishable.
			if len(value)+9 > width {
				inlineResets = false
				break
			}
		}
	}
	if inlineResets {
		quotaRows = len(quotas)
	}
	if height > quotaRows+1 {
		metadata := strings.ReplaceAll(string(usage.Confidence), "_", " ")
		if usage.PlanType != "" {
			metadata = "plan: " + usage.PlanType + " | " + metadata
		}
		if metadata != "" {
			lines = append(lines, footerStyle.Render(fitText(metadata, width)))
		}
	}
	if !usage.FetchedAt.IsZero() && height > quotaRows+len(lines) {
		lines = append(lines, footerStyle.Render(fitText("Updated "+usage.FetchedAt.In(time.Local).Format("Jan 02 15:04 MST"), width)))
	}
	available := height - len(lines)
	visible := len(quotas)
	if quotaRows > available {
		budget := max(0, available-1) // reserve the hidden-window count
		visible = 0
		for _, quota := range quotas {
			rows := 1
			if !inlineResets && quota.ResetsAt != nil && !quota.ResetsAt.IsZero() {
				rows++
			}
			if rows > budget {
				break
			}
			budget -= rows
			visible++
		}
	}
	for _, quota := range quotas[:visible] {
		label := quota.DisplayName
		if label == "" {
			label = quota.Name
		}
		remaining := remainingPercent(*quota.UsedPercent)
		values := fmt.Sprintf("%.1f%% used %.1f%% remaining", *quota.UsedPercent, remaining)
		if width < len(values)+10 || inlineResets {
			values = fmt.Sprintf("%.1f%% remaining", remaining)
		}
		if width < 32 && !inlineResets {
			values = fmt.Sprintf("%.1f%% left", remaining)
		}
		if inlineResets && quota.ResetsAt != nil && !quota.ResetsAt.IsZero() {
			values += " | reset " + quota.ResetsAt.In(time.Local).Format("Jan 02 15:04")
		}
		barWidth := min(12, width-len(values)-10)
		bar := ""
		if barWidth >= 4 && !inlineResets {
			filled := int(math.Round(float64(barWidth) * remaining / 100))
			style := activeStyle
			if remaining <= 10 {
				style = warningStyle
			}
			bar = style.Render("["+strings.Repeat("#", filled)+strings.Repeat("-", barWidth-filled)+"]") + " "
		}
		labelWidth := max(0, width-len(values)-ansi.StringWidth(bar)-1)
		line := fitText(label, labelWidth) + " " + bar + values
		lines = append(lines, ansi.Truncate(line, width, ""))
		if !inlineResets && quota.ResetsAt != nil && !quota.ResetsAt.IsZero() {
			lines = append(lines, footerStyle.Render(quotaResetLine(*quota.ResetsAt, width)))
		}
	}
	if visible < len(quotas) {
		lines = append(lines, footerStyle.Render(fitText(fmt.Sprintf("+%d more: cma usage", len(quotas)-visible), width)))
	}
	if len(quotas) == 0 {
		lines = append(lines, wrappedText("No limit windows reported", width)...)
	}
	return boundedLines(lines, width, height)
}

func quotaResetLine(reset time.Time, width int) string {
	local := reset.In(time.Local)
	line := "Resets " + local.Format("Jan 02 15:04")
	withZone := line + " " + local.Format("MST")
	if ansi.StringWidth(withZone) <= width {
		return withZone
	}
	return line
}

func remainingPercent(used float64) float64 {
	if math.IsNaN(used) || math.IsInf(used, 0) {
		return 0
	}
	return max(0, min(100, 100-used))
}

func (m model) inputView() string {
	title, subtitle := "Backup", "Choose a name for the encrypted backup."
	switch m.mode {
	case modeBackupPass:
		subtitle = "Enter the passphrase for this encrypted backup."
	case modeRestoreSource:
		title, subtitle = "Restore", "Enter a backup name or an absolute path."
	case modeRestorePass:
		title, subtitle = "Restore", "Enter the backup passphrase to review its accounts."
	}
	head, foot, width, bodyHeight := m.frame(title, "", []string{"Enter submit  Esc cancel"})
	body := boundedLines(wrappedText(subtitle, width), width, max(0, bodyHeight-2))
	if bodyHeight >= 2 {
		body = append(body, "")
	}
	input := m.input
	if m.mode == modeBackupPass || m.mode == modeRestorePass {
		input.EchoMode = textinput.EchoPassword
		input.EchoCharacter = '*'
	}
	input.Width = max(1, width-ansi.StringWidth(input.Prompt)-1)
	input.SetCursor(input.Position())
	body = append(body, input.View())
	return m.finish(head, body, foot)
}

func (m model) deleteConfirmView() string {
	head, foot, width, _ := m.frame("Delete Account", "", []string{"y delete  any other key cancel"})
	name := m.pendingDeleteName
	if name == "" {
		name = m.pendingDelete
	}
	_, height := m.size()
	body := []string{titleStyle.Render(fitText("Delete "+name+"?", width))}
	if m.pendingDeleteActive {
		warning := "This is the active account."
		if height <= 14 {
			warning = "Active account."
		}
		body = append(body, warningStyle.Render(fitText(warning, width)))
	}
	consequence := "The stored credentials cannot be recovered after deletion."
	if height <= 14 {
		consequence = "Cannot recover credentials."
	} else {
		body = append(body, "")
	}
	body = append(body, wrappedText(consequence, width)...)
	return m.finish(head, body, foot)
}

func (m model) restoreReviewView() string {
	head, foot, width, bodyHeight := m.frame("Restore Review", "Choose accounts to import", []string{
		"Enter continue  Esc cancel", "j/k move  space toggle  A all  c policy",
	})
	body := []string{fitText("Policy: "+string(m.restoreConflictPolicy), width)}
	if bodyHeight >= 3 {
		body = append(body, fitText("Mode: "+restoreModeLabel(m.restoreAll), width))
	}
	if bodyHeight >= 7 {
		body = append(body, footerStyle.Render(fitText("Source: "+m.pendingRestore, width)), "")
	}
	if len(m.restoreCandidates) == 0 {
		body = append(body, fitText("No backup accounts found", width))
	} else {
		// Always reserve at least one row for the selected account.
		if len(body) >= bodyHeight {
			body = body[:max(0, bodyHeight-1)]
		}
		slots := bodyHeight - len(body)
		showPosition := len(m.restoreCandidates) > slots && slots >= 2
		if showPosition {
			slots--
		}
		start, end := visibleRange(len(m.restoreCandidates), m.restoreCursor, slots)
		for i := start; i < end; i++ {
			candidate := m.restoreCandidates[i]
			marker := "  [ ] "
			if m.restoreSelected[candidate.Account.ID] {
				marker = "  [x] "
			}
			if i == m.restoreCursor {
				marker = ">" + marker[1:]
			}
			suffix := ""
			if candidate.Conflict != nil {
				suffix = " [conflict]"
				if width < 40 {
					suffix = " !"
				}
			}
			line := marker + fitText(candidate.Account.DisplayName, max(0, width-ansi.StringWidth(marker+suffix))) + suffix
			if i == m.restoreCursor {
				line = selectedStyle.Render(padLine(line, width))
			}
			body = append(body, line)
		}
		if showPosition {
			body = append(body, footerStyle.Render(fitText(fmt.Sprintf("%d-%d of %d", start+1, end, len(m.restoreCandidates)), width)))
		}
	}
	return m.finish(head, body, foot)
}

func (m model) restoreConflictView() string {
	head, foot, width, bodyHeight := m.frame("Resolve Conflicts", "Choose how to import each conflicting account", []string{"Enter apply  Esc back  h/l choose"})
	var body []string
	if len(m.restoreConflictQueue) == 0 || m.restoreConflictIndex < 0 || m.restoreConflictIndex >= len(m.restoreConflictQueue) {
		body = []string{"No conflicts pending"}
	} else {
		current := m.restoreConflictQueue[m.restoreConflictIndex]
		body = append(body, fitText("Account: "+current.Account.DisplayName, width))
		if current.Conflict != nil {
			body = append(body, footerStyle.Render(fitText("Conflict: "+current.Conflict.Reason, width)))
			body = append(body, footerStyle.Render(fitText("Existing: "+current.Conflict.Existing.DisplayName, width)))
		}
		if bodyHeight >= 8 {
			body = append(body, "")
		}
		options := []string{"overwrite", "skip", "rename"}
		// On short terminals prioritize the selected option over metadata.
		if bodyHeight-len(body) < len(options) {
			body = body[:max(0, bodyHeight-1)]
			option := options[max(0, min(m.restoreConflictChoice, len(options)-1))]
			body = append(body, selectedStyle.Render(padLine("> "+option, width)))
		} else {
			for index, option := range options {
				line := "  " + option
				if index == m.restoreConflictChoice {
					line = selectedStyle.Render(padLine("> "+option, width))
				}
				body = append(body, line)
			}
		}
	}
	return m.finish(head, body, foot)
}

func restoreModeLabel(all bool) string {
	if all {
		return "all"
	}
	return "selected"
}

func visibleRange(count, selected, slots int) (int, int) {
	if slots <= 0 || count == 0 {
		return 0, 0
	}
	selected = max(0, min(selected, count-1))
	start := max(0, selected-slots/2)
	start = min(start, max(0, count-slots))
	return start, min(count, start+slots)
}

// Dynamic text is one line and cannot inject terminal control sequences.
func plainText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(text))
}

func fitText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	tail := "..."
	if width < len(tail) {
		tail = ""
	}
	return ansi.Truncate(plainText(text), width, tail)
}

// A key and its action stay together even when the footer wraps.
func helpLines(groups []string, width int) []string {
	var lines []string
	for _, group := range groups {
		line := ""
		for _, item := range strings.Split(group, "  ") {
			item = fitText(item, width)
			if line != "" && ansi.StringWidth(line)+2+ansi.StringWidth(item) > width {
				lines = append(lines, line)
				line = ""
			}
			if line != "" {
				line += "  "
			}
			line += item
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func wrappedText(text string, width int) []string {
	return strings.Split(ansi.Wrap(plainText(text), max(1, width), ""), "\n")
}

func boundedLines(lines []string, width, height int) []string {
	lines = lines[:min(len(lines), max(0, height))]
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = ansi.Truncate(line, width, "")
	}
	return out
}

func padLine(line string, width int) string {
	line = ansi.Truncate(line, width, "")
	return line + strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
}

func lineAt(lines []string, index int) string {
	if index >= len(lines) {
		return ""
	}
	return lines[index]
}

// Keep application results keyed to their account rather than their position.
func sameUsageAccount(left, right app.UsageResult) bool {
	if left.Account.ID != "" && right.Account.ID != "" {
		return left.Account.ID == right.Account.ID
	}
	return left.Account.DisplayName == right.Account.DisplayName
}
