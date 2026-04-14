package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/prakersh/codexmultiauth/internal/app"
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

func printLimitsTable(cmd *cobra.Command, results []app.UsageResult, dull bool) {
	out := cmd.OutOrStdout()

	const (
		nameWidth  = 12
		userWidth  = 26
		planWidth  = 7
		quotaWidth = 16
		resetWidth = 18
	)

	// ANSI color codes
	reset := "\033[0m"
	red := "\033[31m"
	yellow := "\033[33m"
	green := "\033[32m"
	bold := "\033[1m"
	dim := "\033[2m"

	if dull {
		reset, red, yellow, green, bold, dim = "", "", "", "", "", ""
	}

	// Header
	now := time.Now().In(time.Local)
	fmt.Fprintf(out, "\n%s[%s] Codex Limits%s\n", bold, now.Format("2006-01-02 15:04 MST"), reset)
	fmt.Fprintln(out)

	// Header row
	fmt.Fprintf(out, "%s  %-*s %-*s %-*s  %-*s %-*s  %-*s %-*s%s\n",
		bold,
		nameWidth, "ACCOUNT",
		userWidth, "USER",
		planWidth, "PLAN",
		quotaWidth, "5H LIMIT",
		resetWidth, "5H RESETS AT",
		quotaWidth, "WEEKLY LIMIT",
		resetWidth, "WEEKLY RESETS AT",
		reset)

	// Separator
	sep := strings.Repeat("─", 2+nameWidth+1+userWidth+1+planWidth+2+quotaWidth+1+resetWidth+2+quotaWidth+1+resetWidth)
	fmt.Fprintf(out, "%s%s%s\n", dim, sep, reset)

	// Data rows
	for _, result := range results {
		name := result.Account.DisplayName
		if result.Info.IsActive {
			name += " *"
		}
		if len(name) > nameWidth-1 {
			name = name[:nameWidth-1] + "…"
		}

		user := formatUserShort(result.Info)
		if len(user) > userWidth-1 {
			user = user[:userWidth-1] + "…"
		}

		plan := result.Usage.PlanType
		if plan == "" {
			plan = "-"
		}

		// Find 5-hour and weekly limit quotas separately
		var limit5h, reset5h string
		var limitW, resetW string

		for _, quota := range result.Usage.Quotas {
			nameLower := strings.ToLower(quota.Name)
			displayLower := strings.ToLower(quota.DisplayName)

			// Check quota type by name pattern
			is5h := false
			isWeekly := false

			// 5-Hour Limit pattern: contains "5" and "hour" (matches "5-Hour Limit", "5 hour", etc.)
			if strings.Contains(nameLower, "5") && strings.Contains(nameLower, "hour") {
				is5h = true
			} else if strings.Contains(displayLower, "5") && strings.Contains(displayLower, "hour") {
				is5h = true
			}

			// Weekly Limit pattern
			if strings.Contains(nameLower, "weekly") {
				isWeekly = true
			} else if strings.Contains(displayLower, "weekly") {
				isWeekly = true
			}

			if is5h && quota.UsedPercent != nil {
				limit5h, reset5h = formatQuotaLine(*quota.UsedPercent, quota.ResetsAt, dull, red, yellow, green, reset, quotaWidth)
			} else if isWeekly && quota.UsedPercent != nil {
				limitW, resetW = formatQuotaLine(*quota.UsedPercent, quota.ResetsAt, dull, red, yellow, green, reset, quotaWidth)
			}
		}

		if limit5h == "" {
			limit5h = formatStyledCell("-", dim, reset, quotaWidth)
			reset5h = "-"
		}
		if limitW == "" {
			limitW = formatStyledCell("-", dim, reset, quotaWidth)
			resetW = "-"
		}

		fmt.Fprintf(out, "  %-*s %-*s %-*s  %s %-*s  %s %-*s%s\n",
			nameWidth, name,
			userWidth, user,
			planWidth, plan,
			limit5h, resetWidth, reset5h,
			limitW, resetWidth, resetW,
			reset)
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s* = active account%s\n", dim, reset)
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

func formatQuotaLine(usedPercent float64, resetsAt *time.Time, dull bool, red, yellow, green, reset string, quotaWidth int) (limit, resetAt string) {
	if dull {
		red, yellow, green = "", "", ""
	}

	var color string
	if usedPercent >= 100 {
		color = red
	} else if usedPercent >= 80 {
		color = yellow
	} else {
		color = green
	}

	limit = formatStyledCell(fmt.Sprintf("%.1f%%", usedPercent), color, reset, quotaWidth)

	if resetsAt != nil && !resetsAt.IsZero() {
		resetAt = resetsAt.In(time.Local).Format("Jan 02 15:04")
	} else {
		resetAt = "-"
	}

	return limit, resetAt
}

func formatStyledCell(value, color, reset string, width int) string {
	padding := width - len(value)
	if padding < 0 {
		padding = 0
	}
	if color == "" || reset == "" {
		return value + strings.Repeat(" ", padding)
	}
	return color + value + reset + strings.Repeat(" ", padding)
}
