package cmd

import (
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/domain"
)

type limitsStyles struct {
	reset, bold, dim, red, yellow, green string
}

func newLimitsStyles(dull bool) limitsStyles {
	if dull {
		return limitsStyles{}
	}
	return limitsStyles{reset: "\033[0m", bold: "\033[1m", dim: "\033[2m",
		red: "\033[31m", yellow: "\033[33m", green: "\033[32m"}
}

func (s limitsStyles) paint(value, color string) string {
	if color == "" {
		return value
	}
	return color + value + s.reset
}

func printResponsiveLimits(out io.Writer, results []app.UsageResult, dull bool, width int) {
	s := newLimitsStyles(dull)
	// Reserve a cell to avoid an automatic wrap at the terminal's right edge.
	width = max(1, width-1)
	write := func(line string) { fmt.Fprintln(out, ansi.Wrap(line, width, "")) }
	write(s.paint("Codex Limits", s.bold))
	write(s.paint(time.Now().In(time.Local).Format("2006-01-02 15:04 MST"), s.dim))
	write("")
	if len(results) == 0 {
		write("No saved accounts. Run cma save to add one.")
		return
	}
	columns := collectQuotaColumns(results)
	// Every quota gets a readable value and a complete reset timestamp.
	// If that cannot fit, switch the entire layout rather than clipping columns.
	identityWidth := 24
	planWidth, dataWidth, quotaWidth := 6, 7, 25
	for _, result := range results {
		planWidth = max(planWidth, displayWidth(cleanLimitText(result.Usage.PlanType)))
	}
	tableWidth := identityWidth + planWidth + dataWidth + 4 + len(columns)*(quotaWidth+2)
	if len(columns) > 0 && tableWidth <= width {
		wanted := identityWidth
		for _, result := range results {
			wanted = max(wanted, displayWidth(cleanLimitText(formatUserShort(result.Info))),
				displayWidth(cleanLimitText(result.Account.DisplayName))+2)
		}
		identityWidth = min(40, wanted, identityWidth+width-tableWidth)
		printLimitsComparison(out, results, columns, s, identityWidth, planWidth, dataWidth, quotaWidth)
	} else {
		for i, result := range results {
			if i > 0 {
				write("")
			}
			name := cleanLimitText(result.Account.DisplayName)
			if result.Info.IsActive {
				name += " [active]"
			}
			write(s.paint(name, s.bold))
			write(cleanLimitText(formatUserShort(result.Info)))
			plan := cleanLimitText(result.Usage.PlanType)
			if plan == "" {
				plan = "-"
			}
			data := confidenceCell(result, s.red, s.yellow, s.green, s.dim)
			write("Plan: " + plan + "  |  Data: " + s.paint(data.text, data.color))
			rendered := false
			labelWidth := 0
			for _, quota := range result.Usage.Quotas {
				labelWidth = max(labelWidth, displayWidth(cleanLimitText(quota.DisplayName)), displayWidth(cleanLimitText(quota.Name)))
			}
			labelWidth = min(labelWidth, max(1, width-18))
			for _, column := range columns {
				for _, quota := range result.Usage.Quotas {
					if quotaKey(quota) != column.key || quota.UsedPercent == nil {
						continue
					}
					write("")
					label := cleanLimitText(quota.DisplayName)
					if strings.TrimSpace(label) == "" {
						label = cleanLimitText(quota.Name)
					}
					label = truncateCell(label, labelWidth)
					prefix := "  " + label + strings.Repeat(" ", labelWidth-displayWidth(label)+2)
					value := fmt.Sprintf("%.1f%% left", remainingQuota(*quota.UsedPercent))
					if displayWidth(prefix)+14+1+displayWidth(value) <= width {
						value = quotaBar(*quota.UsedPercent, 12) + " " + value
					}
					write(s.paint(prefix, s.bold) + s.paint(value, percentColor(*quota.UsedPercent, s.red, s.yellow, s.green)))
					used := fmt.Sprintf("%.1f%% used", *quota.UsedPercent)
					reset := "resets " + formatResetAt(quota.ResetsAt)
					if displayWidth(used)+3+displayWidth(reset)+4 <= width {
						write(s.paint("    "+used+" | "+reset, s.dim))
					} else {
						write(s.paint("    "+used, s.dim))
						write(s.paint("    "+reset, s.dim))
					}
					rendered = true
				}
			}
			if !rendered {
				write("LIMIT: unavailable")
				write(s.paint("RESETS AT: -", s.dim))
			}
		}
	}
	write("")
	write(s.paint("Percentages show quota left and used. Reset times are local.", s.dim))
}

func printLimitsComparison(out io.Writer, results []app.UsageResult, columns []quotaColumn, s limitsStyles, identityWidth, planWidth, dataWidth, quotaWidth int) {
	widths := []int{identityWidth, planWidth, dataWidth}
	headers := []cell{{text: "ACCOUNT / USER"}, {text: "PLAN"}, {text: "DATA"}}
	for _, column := range columns {
		widths = append(widths, quotaWidth)
		headers = append(headers, cell{text: truncateCell(cleanLimitText(column.valueHeader), quotaWidth)})
	}
	// renderRow uses a two-cell inset and one-cell gaps. Account blocks align
	// below a single header, with secondary details on their own lines.
	fmt.Fprintln(out, s.paint(renderRow(headers, widths, s.reset), s.bold))
	fmt.Fprintln(out, s.paint(strings.Repeat("-", rowWidth(widths)), s.dim))
	for index, result := range results {
		if index > 0 {
			fmt.Fprintln(out)
		}
		name := cleanLimitText(result.Account.DisplayName)
		if result.Info.IsActive {
			name = truncateCell(name, identityWidth-2) + " *"
		}
		data := confidenceCell(result, s.red, s.yellow, s.green, s.dim)
		plan := cleanLimitText(result.Usage.PlanType)
		if plan == "" {
			plan = "-"
		}
		lines := [][]cell{
			{{text: truncateCell(name, identityWidth), color: s.bold}, {text: plan}, data},
			{{text: truncateCell(cleanLimitText(formatUserShort(result.Info)), identityWidth), color: s.dim}, {}, {}},
			{{}, {}, {}},
		}
		for _, column := range columns {
			var quota *domain.UsageQuota
			for i := range result.Usage.Quotas {
				q := &result.Usage.Quotas[i]
				if quotaKey(*q) == column.key && q.UsedPercent != nil {
					quota = q
					break
				}
			}
			if quota == nil {
				lines[0] = append(lines[0], cell{text: "-", color: s.dim})
				lines[1] = append(lines[1], cell{})
				lines[2] = append(lines[2], cell{})
				continue
			}
			used := *quota.UsedPercent
			lines[0] = append(lines[0], cell{text: quotaBar(used, 10) + fmt.Sprintf(" %.1f%% left", remainingQuota(used)),
				color: percentColor(used, s.red, s.yellow, s.green)})
			lines[1] = append(lines[1], cell{text: fmt.Sprintf("%.1f%% used", used), color: s.dim})
			lines[2] = append(lines[2], cell{text: "resets " + formatResetAt(quota.ResetsAt), color: s.dim})
		}
		for _, line := range lines {
			fmt.Fprintln(out, renderRow(line, widths, s.reset))
		}
	}
	fmt.Fprintln(out, s.paint("\n* = active account", s.dim))
}

func remainingQuota(used float64) float64 {
	if math.IsNaN(used) || math.IsInf(used, 0) {
		return 0
	}
	return max(0, min(100, 100-used))
}

func quotaBar(used float64, width int) string {
	filled := int(math.Round(remainingQuota(used) / 100 * float64(width)))
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
}
