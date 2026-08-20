package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/spf13/cobra"
)

func newLimitsCmd() *cobra.Command {
	var dull bool

	cmd := &cobra.Command{
		Use:   "limits",
		Short: "Show limits for all saved accounts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newService()
			if err != nil {
				return err
			}
			results, err := manager.Usage(context.Background(), "all")
			if err != nil {
				return err
			}
			printLimitsTable(cmd, results, dull)
			return nil
		},
	}

	cmd.Flags().BoolVar(&dull, "dull", false, "Use no colors")

	return cmd
}

// quotaColumn describes one limit window that at least one account reports.
// Columns are discovered from the fetched data rather than hardcoded, because
// Codex now issues different window shapes per plan (free accounts report a
// monthly window, paid accounts a weekly one).
type quotaColumn struct {
	key           string
	valueHeader   string
	resetHeader   string
	windowSeconds int64
	firstSeen     int
}

func quotaKey(quota domain.UsageQuota) string {
	if strings.TrimSpace(quota.Name) != "" {
		return quota.Name
	}
	return quota.DisplayName
}

// columnHeaders turns a quota display name into its two table headings.
// "Monthly Limit" yields "MONTHLY LIMIT" and "MONTHLY RESETS AT"; a label that
// is not itself a limit, such as "Review Requests", is used verbatim so the
// heading does not read "REVIEW REQUESTS LIMIT".
func columnHeaders(quota domain.UsageQuota) (valueHeader, resetHeader string) {
	label := strings.TrimSpace(quota.DisplayName)
	if label == "" {
		label = strings.TrimSpace(quota.Name)
	}
	if label == "" {
		return "LIMIT", "RESETS AT"
	}
	if trimmed := strings.TrimSuffix(label, " Limit"); trimmed != label {
		label = strings.ToUpper(trimmed)
		return label + " LIMIT", label + " RESETS AT"
	}
	label = strings.ToUpper(label)
	return label, label + " RESETS AT"
}

func collectQuotaColumns(results []app.UsageResult) []quotaColumn {
	index := make(map[string]*quotaColumn)
	order := 0
	for _, result := range results {
		for _, quota := range result.Usage.Quotas {
			if quota.UsedPercent == nil {
				continue
			}
			key := quotaKey(quota)
			if key == "" {
				continue
			}
			if existing, ok := index[key]; ok {
				if existing.windowSeconds == 0 {
					existing.windowSeconds = quota.WindowSeconds
				}
				continue
			}
			valueHeader, resetHeader := columnHeaders(quota)
			index[key] = &quotaColumn{
				key:           key,
				valueHeader:   valueHeader,
				resetHeader:   resetHeader,
				windowSeconds: quota.WindowSeconds,
				firstSeen:     order,
			}
			order++
		}
	}

	columns := make([]quotaColumn, 0, len(index))
	for _, column := range index {
		columns = append(columns, *column)
	}
	// Shortest window first so the most immediately binding limit leads;
	// windows of unknown length keep their discovery order at the end.
	sort.Slice(columns, func(i, j int) bool {
		left, right := columns[i], columns[j]
		if left.windowSeconds != right.windowSeconds {
			if left.windowSeconds == 0 {
				return false
			}
			if right.windowSeconds == 0 {
				return true
			}
			return left.windowSeconds < right.windowSeconds
		}
		return left.firstSeen < right.firstSeen
	})
	return columns
}

// Caps for the free-text identity columns. Everything else sizes to content.
// These keep a row with one quota pair inside a normal terminal; a vault
// spanning several distinct windows will still be wider than 80 columns,
// which is inherent to showing every limit an account reports.
const (
	maxAccountWidth = 18
	maxUserWidth    = 26
)

// truncateCell shortens a value to limit runes, marking the cut with "..." so
// the table stays inside a normal terminal width.
func truncateCell(value string, limit int) string {
	if displayWidth(value) <= limit {
		return value
	}
	if limit <= 3 {
		return string([]rune(value)[:limit])
	}
	return string([]rune(value)[:limit-3]) + "..."
}

// cell holds a rendered value plus the color it should carry, so widths can be
// measured on the plain text and color applied only at print time.
type cell struct {
	text  string
	color string
}

func printLimitsTable(cmd *cobra.Command, results []app.UsageResult, dull bool) {
	out := cmd.OutOrStdout()

	reset := "\033[0m"
	red := "\033[31m"
	yellow := "\033[33m"
	green := "\033[32m"
	bold := "\033[1m"
	dim := "\033[2m"

	if dull {
		reset, red, yellow, green, bold, dim = "", "", "", "", "", ""
	}

	columns := collectQuotaColumns(results)

	headers := []string{"ACCOUNT", "USER", "PLAN"}
	for _, column := range columns {
		headers = append(headers, column.valueHeader, column.resetHeader)
	}
	// With no quota data at all, still show a limit pair so the table shape
	// stays recognizable instead of collapsing to three columns.
	if len(columns) == 0 {
		headers = append(headers, "LIMIT", "RESETS AT")
	}

	rows := make([][]cell, 0, len(results))
	for _, result := range results {
		name := result.Account.DisplayName
		if result.Info.IsActive {
			name += " *"
		}
		// Identity columns are capped so a long name or email cannot push the
		// row past the terminal width and undo the alignment below. Quota
		// columns stay uncapped; their values are short and fixed in shape.
		name = truncateCell(name, maxAccountWidth)
		plan := result.Usage.PlanType
		if plan == "" {
			plan = "-"
		}

		row := []cell{
			{text: name},
			{text: truncateCell(formatUserShort(result.Info), maxUserWidth)},
			{text: plan},
		}

		byKey := make(map[string]domain.UsageQuota, len(result.Usage.Quotas))
		for _, quota := range result.Usage.Quotas {
			if quota.UsedPercent != nil {
				byKey[quotaKey(quota)] = quota
			}
		}
		for _, column := range columns {
			quota, ok := byKey[column.key]
			if !ok {
				row = append(row, cell{text: "-", color: dim}, cell{text: "-"})
				continue
			}
			row = append(row,
				cell{text: fmt.Sprintf("%.1f%%", *quota.UsedPercent), color: percentColor(*quota.UsedPercent, red, yellow, green)},
				cell{text: formatResetAt(quota.ResetsAt)},
			)
		}
		if len(columns) == 0 {
			row = append(row, cell{text: "-", color: dim}, cell{text: "-"})
		}
		rows = append(rows, row)
	}

	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = displayWidth(header)
	}
	for _, row := range rows {
		for i, c := range row {
			if i < len(widths) && displayWidth(c.text) > widths[i] {
				widths[i] = displayWidth(c.text)
			}
		}
	}

	now := time.Now().In(time.Local)
	fmt.Fprintf(out, "\n%s[%s] Codex Limits%s\n", bold, now.Format("2006-01-02 15:04 MST"), reset)
	fmt.Fprintln(out)

	headerCells := make([]cell, len(headers))
	for i, header := range headers {
		headerCells[i] = cell{text: header}
	}
	fmt.Fprintf(out, "%s%s%s\n", bold, renderRow(headerCells, widths, reset), reset)

	fmt.Fprintf(out, "%s%s%s\n", dim, strings.Repeat("─", rowWidth(widths)), reset)

	for _, row := range rows {
		fmt.Fprintln(out, renderRow(row, widths, reset))
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s* = active account%s\n", dim, reset)
}

// displayWidth counts runes rather than bytes so account names or emails with
// non-ASCII characters do not skew column padding.
func displayWidth(value string) int {
	return utf8.RuneCountInString(value)
}

func rowWidth(widths []int) int {
	total := 2
	for i, width := range widths {
		total += width
		if i < len(widths)-1 {
			total++
		}
	}
	return total
}

// renderRow pads every cell to its column width using the plain text length,
// then wraps the value in color so escape codes never affect alignment.
func renderRow(cells []cell, widths []int, reset string) string {
	var builder strings.Builder
	builder.WriteString("  ")
	for i, c := range cells {
		width := 0
		if i < len(widths) {
			width = widths[i]
		}
		value := c.text
		if c.color != "" && reset != "" {
			value = c.color + c.text + reset
		}
		builder.WriteString(value)
		if padding := width - displayWidth(c.text); padding > 0 {
			builder.WriteString(strings.Repeat(" ", padding))
		}
		if i < len(cells)-1 {
			builder.WriteString(" ")
		}
	}
	return strings.TrimRight(builder.String(), " ")
}

func percentColor(usedPercent float64, red, yellow, green string) string {
	switch {
	case usedPercent >= 100:
		return red
	case usedPercent >= 80:
		return yellow
	default:
		return green
	}
}

func formatResetAt(resetsAt *time.Time) string {
	if resetsAt == nil || resetsAt.IsZero() {
		return "-"
	}
	return resetsAt.In(time.Local).Format("Jan 02 15:04")
}

func formatUserShort(info app.UsageAccountInfo) string {
	if info.UserEmail != "" {
		return info.UserEmail
	}
	if info.UserName != "" {
		return info.UserName
	}
	return "-"
}
