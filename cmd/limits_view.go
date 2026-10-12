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
	columns := collectQuotaColumns(results)
	layout, comparison := limitsComparisonLayoutFor(results, columns, width)
	write := func(line string) {
		fmt.Fprintln(out, ansi.Wrap(line, width, ""))
	}
	title := s.paint("Codex Limits", s.bold)
	stamp := s.paint(time.Now().In(time.Local).Format("2006-01-02 15:04 MST"), s.dim)
	if displayWidth(title+"  "+stamp) <= width {
		write(title + "  " + stamp)
	} else {
		write(title)
		write(stamp)
	}
	write("")
	if len(results) == 0 {
		write("No saved accounts. Run cma save to add one.")
		return
	}
	if comparison {
		printLimitsComparison(out, results, columns, s, layout)
	} else {
		for i, result := range results {
			if i > 0 {
				write("")
			}
			name := cleanLimitText(result.Account.DisplayName)
			if result.Info.IsActive {
				name += " [active]"
			}
			user := cleanLimitText(formatUserShort(result.Info))
			plan := cleanLimitText(result.Usage.PlanType)
			if plan == "" {
				plan = "-"
			}
			data := confidenceCell(result, s.red, s.yellow, s.green, s.dim)
			metadata := "Plan: " + plan + "  |  Data: " + s.paint(data.text, data.color)
			identity := s.paint(name, s.bold) + "  " + user + "  " + metadata
			if displayWidth(identity) <= width {
				write(identity)
			} else {
				if displayWidth(name+"  "+metadata) <= width {
					write(s.paint(name, s.bold) + "  " + metadata)
				} else {
					write(s.paint(name, s.bold))
					write(metadata)
				}
				write(user)
			}
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
					label := cleanLimitText(quota.DisplayName)
					if strings.TrimSpace(label) == "" {
						label = cleanLimitText(quota.Name)
					}
					label = truncateCell(label, labelWidth)
					prefix := "  " + label + strings.Repeat(" ", labelWidth-displayWidth(label)+2)
					value := fmt.Sprintf("%.1f%% left", remainingQuota(*quota.UsedPercent))
					used := fmt.Sprintf("%.1f%% used", *quota.UsedPercent)
					reset := "resets " + formatResetAt(quota.ResetsAt)
					if displayWidth(prefix+value+"  "+used+"  "+reset) <= width {
						value += "  " + used
					}
					if displayWidth(prefix+value+"  "+reset) <= width {
						if displayWidth(prefix)+15+displayWidth(value+"  "+reset) <= width {
							value = quotaBar(*quota.UsedPercent, 12) + " " + value
						}
						write(s.paint(prefix, s.bold) + s.paint(value, percentColor(*quota.UsedPercent, s.red, s.yellow, s.green)) + s.paint("  "+reset, s.dim))
					} else {
						write(s.paint(prefix, s.bold) + s.paint(value, percentColor(*quota.UsedPercent, s.red, s.yellow, s.green)))
						write(s.paint("    "+reset, s.dim))
					}
					rendered = true
				}
			}
			if !rendered {
				write("LIMIT: unavailable; RESETS AT: -")
			}
		}
	}
	write("")
	write(s.paint("* = active account. Reset times are local.", s.dim))
	for _, result := range results {
		if result.Problem != nil {
			color := s.yellow
			if result.Problem.RequiresLogin {
				color = s.red
			}
			write(s.paint(cleanLimitText(result.Account.DisplayName+": "+result.Problem.Message), color))
		}
	}
}

// Fit complete account rows before adding redundant percentages or bars.
// Available width controls density; there is no centered content-width cap.
type limitsComparisonLayout struct {
	headers       []string
	widths        []int
	showUsed      bool
	barWidth, gap int
}

func (l limitsComparisonLayout) width() int {
	total := 2 + (len(l.widths)-1)*l.gap
	for _, width := range l.widths {
		total += width
	}
	return total
}

func limitsComparisonLayoutFor(results []app.UsageResult, columns []quotaColumn, width int) (limitsComparisonLayout, bool) {
	measure := func(showUsed bool, barWidth int) limitsComparisonLayout {
		l := limitsComparisonLayout{headers: []string{"ACCOUNT", "USER", "PLAN", "DATA"}, showUsed: showUsed, barWidth: barWidth, gap: 1}
		for _, column := range columns {
			label := strings.TrimSuffix(cleanLimitText(column.valueHeader), " LIMIT") + " LEFT"
			if showUsed {
				label += " / USED"
			}
			l.headers = append(l.headers, label, "RESETS AT")
		}
		l.widths = make([]int, len(l.headers))
		for i, header := range l.headers {
			l.widths[i] = displayWidth(header)
		}
		for _, result := range results {
			row := limitsComparisonCells(result, columns, l, limitsStyles{})
			for i, c := range row {
				l.widths[i] = max(l.widths[i], displayWidth(c.text))
			}
		}
		return l
	}
	layout := measure(false, 0)
	if layout.width() > width {
		return layout, false
	}
	if full := measure(true, 0); full.width() <= width {
		layout = full
	}
	if len(columns) > 0 {
		barWidth := min(32, (width-layout.width())/len(columns)-3)
		if barWidth >= 6 {
			withBars := measure(layout.showUsed, barWidth)
			if withBars.width() <= width {
				layout = withBars
			}
		}
	}
	if width-layout.width() >= len(layout.widths)-1 {
		layout.gap = 2
	}
	if len(columns) > 0 {
		spare := width - layout.width()
		for i := range columns {
			add := spare / (len(columns) - i)
			layout.widths[4+2*i] += add
			spare -= add
		}
	}
	return layout, true
}

func limitsComparisonCells(result app.UsageResult, columns []quotaColumn, layout limitsComparisonLayout, s limitsStyles) []cell {
	name := cleanLimitText(result.Account.DisplayName)
	if result.Info.IsActive {
		name += " *"
	}
	plan := cleanLimitText(result.Usage.PlanType)
	if plan == "" {
		plan = "-"
	}
	row := []cell{{text: name, color: s.bold}, {text: cleanLimitText(formatUserShort(result.Info))}, {text: plan}, confidenceCell(result, s.red, s.yellow, s.green, s.dim)}
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
			row = append(row, cell{text: "-", color: s.dim}, cell{text: "-", color: s.dim})
			continue
		}
		used := *quota.UsedPercent
		value := fmt.Sprintf("%.1f%% left", remainingQuota(used))
		if layout.showUsed {
			value += fmt.Sprintf(" %.1f%% used", used)
		}
		if layout.barWidth > 0 {
			value = quotaBar(used, layout.barWidth) + " " + value
		}
		row = append(row, cell{text: value, color: percentColor(used, s.red, s.yellow, s.green)}, cell{text: formatResetAt(quota.ResetsAt), color: s.dim})
	}
	return row
}

func renderLimitsComparisonRow(cells []cell, widths []int, gap int, s limitsStyles) string {
	var row strings.Builder
	row.WriteString("  ")
	for i, c := range cells {
		row.WriteString(s.paint(c.text, c.color))
		padding := max(0, widths[i]-displayWidth(c.text))
		if i < len(cells)-1 {
			padding += gap
		}
		row.WriteString(strings.Repeat(" ", padding))
	}
	return strings.TrimRight(row.String(), " ")
}

func printLimitsComparison(out io.Writer, results []app.UsageResult, columns []quotaColumn, s limitsStyles, layout limitsComparisonLayout) {
	headers := make([]cell, len(layout.headers))
	for i, header := range layout.headers {
		headers[i] = cell{text: header}
	}
	fmt.Fprintln(out, s.paint(renderLimitsComparisonRow(headers, layout.widths, layout.gap, s), s.bold))
	fmt.Fprintln(out, s.paint(strings.Repeat("-", layout.width()), s.dim))
	for _, result := range results {
		fmt.Fprintln(out, renderLimitsComparisonRow(limitsComparisonCells(result, columns, layout, s), layout.widths, layout.gap, s))
	}
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
