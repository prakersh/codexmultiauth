package cmd

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestPrintLimitsTableKeepsQuotaColumnsAligned(t *testing.T) {
	results := []app.UsageResult{
		{
			Account: domain.Account{DisplayName: "ecom1"},
			Info:    app.UsageAccountInfo{UserEmail: "ecom1@sansaarbazar.com"},
			Usage: domain.UsageSummary{
				PlanType: "team",
				Quotas: []domain.UsageQuota{
					{
						DisplayName: "5-Hour Limit",
						UsedPercent: floatPtr(0),
						ResetsAt:    timePtr(time.Date(2026, 4, 14, 16, 51, 0, 0, time.Local)),
					},
					{
						DisplayName: "Weekly Limit",
						UsedPercent: floatPtr(16),
						ResetsAt:    timePtr(time.Date(2026, 4, 21, 3, 19, 0, 0, time.Local)),
					},
				},
			},
		},
		{
			Account: domain.Account{DisplayName: "ecom2"},
			Info:    app.UsageAccountInfo{UserEmail: "ecom2@sansaarbazar.com"},
			Usage: domain.UsageSummary{
				PlanType: "team",
				Quotas: []domain.UsageQuota{
					{
						DisplayName: "5-Hour Limit",
						UsedPercent: floatPtr(100),
						ResetsAt:    timePtr(time.Date(2026, 4, 14, 15, 54, 0, 0, time.Local)),
					},
					{
						DisplayName: "Weekly Limit",
						UsedPercent: floatPtr(31),
						ResetsAt:    timePtr(time.Date(2026, 4, 21, 5, 13, 0, 0, time.Local)),
					},
				},
			},
		},
	}

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	printLimitsTable(cmd, results, false)

	output := stripANSI(out.String())
	lines := strings.Split(output, "\n")

	header := findLine(lines, "ACCOUNT")
	rowZero := findLine(lines, "ecom1")
	rowFull := findLine(lines, "ecom2")

	require.NotEmpty(t, header)
	require.NotEmpty(t, rowZero)
	require.NotEmpty(t, rowFull)

	require.Equal(t, strings.Index(header, "5-HOUR RESETS AT"), strings.Index(rowZero, "Apr 14 16:51"))
	require.Equal(t, strings.Index(header, "WEEKLY RESETS AT"), strings.Index(rowZero, "Apr 21 03:19"))
	require.Equal(t, strings.Index(rowZero, "Apr 14 16:51"), strings.Index(rowFull, "Apr 14 15:54"))
	require.Equal(t, strings.Index(rowZero, "Apr 21 03:19"), strings.Index(rowFull, "Apr 21 05:13"))
}

func stripANSI(value string) string {
	return regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(value, "")
}

func findLine(lines []string, needle string) string {
	for _, line := range lines {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

// TestPrintLimitsTableRendersColumnsPerReportedWindow proves the table is built
// from the windows the accounts actually report: a free account contributes a
// monthly column and a paid account a weekly column, and neither invents a
// 5-hour column that Codex no longer issues.
func TestPrintLimitsTableRendersColumnsPerReportedWindow(t *testing.T) {
	results := []app.UsageResult{
		{
			Account: domain.Account{DisplayName: "freeacct"},
			Info:    app.UsageAccountInfo{UserEmail: "free@example.com"},
			Usage: domain.UsageSummary{
				PlanType: "free",
				Quotas: []domain.UsageQuota{{
					Name:          "monthly",
					DisplayName:   "Monthly Limit",
					WindowSeconds: 2592000,
					UsedPercent:   floatPtr(0),
					ResetsAt:      timePtr(time.Date(2026, 9, 19, 21, 18, 0, 0, time.Local)),
				}},
			},
		},
		{
			Account: domain.Account{DisplayName: "plusacct"},
			Info:    app.UsageAccountInfo{UserEmail: "plus@example.com"},
			Usage: domain.UsageSummary{
				PlanType: "plus",
				Quotas: []domain.UsageQuota{{
					Name:          "weekly",
					DisplayName:   "Weekly Limit",
					WindowSeconds: 604800,
					UsedPercent:   floatPtr(42.5),
					ResetsAt:      timePtr(time.Date(2026, 8, 28, 3, 19, 0, 0, time.Local)),
				}},
			},
		},
	}

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	printLimitsTable(cmd, results, false)

	output := stripANSI(out.String())
	lines := strings.Split(output, "\n")
	header := findLine(lines, "ACCOUNT")
	freeRow := findLine(lines, "freeacct")
	plusRow := findLine(lines, "plusacct")

	require.Contains(t, header, "WEEKLY LIMIT")
	require.Contains(t, header, "MONTHLY LIMIT")
	require.NotContains(t, header, "5-HOUR")

	// Weekly is the shorter window so it must lead the monthly columns.
	require.Less(t, strings.Index(header, "WEEKLY LIMIT"), strings.Index(header, "MONTHLY LIMIT"))

	// Each account fills only its own window and shows "-" for the other.
	require.Equal(t, strings.Index(header, "MONTHLY RESETS AT"), strings.Index(freeRow, "Sep 19 21:18"))
	require.Equal(t, strings.Index(header, "WEEKLY RESETS AT"), strings.Index(plusRow, "Aug 28 03:19"))
	require.Contains(t, freeRow, "-")
	require.Contains(t, plusRow, "42.5%")
}

// TestPrintLimitsTableWithoutQuotaData keeps the table readable when no account
// returned usable limit data.
func TestPrintLimitsTableWithoutQuotaData(t *testing.T) {
	results := []app.UsageResult{{
		Account: domain.Account{DisplayName: "acct"},
		Info:    app.UsageAccountInfo{UserEmail: "a@example.com"},
		Usage:   domain.UsageSummary{PlanType: "free"},
	}}

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	printLimitsTable(cmd, results, true)

	output := out.String()
	require.Contains(t, output, "LIMIT")
	require.Contains(t, output, "RESETS AT")
	require.Contains(t, output, "acct")
}

// TestPrintLimitsTableAlignsNonASCIINames guards column padding against
// multi-byte account names, which would skew a byte-length based width.
func TestPrintLimitsTableAlignsNonASCIINames(t *testing.T) {
	at := time.Date(2026, 9, 20, 2, 50, 0, 0, time.Local)
	used := 0.0
	row := func(name, email string) app.UsageResult {
		return app.UsageResult{
			Account: domain.Account{DisplayName: name},
			Info:    app.UsageAccountInfo{UserEmail: email},
			Usage: domain.UsageSummary{PlanType: "free", Quotas: []domain.UsageQuota{{
				Name: "monthly", DisplayName: "Monthly Limit",
				WindowSeconds: 2592000, UsedPercent: &used, ResetsAt: &at,
			}}},
		}
	}

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	printLimitsTable(cmd, []app.UsageResult{row("café-née", "a@example.com"), row("plain", "b@example.com")}, true)

	lines := strings.Split(out.String(), "\n")
	// Rune offsets, not byte offsets: the accented name is longer in bytes but
	// occupies the same number of columns.
	require.Equal(t,
		runeIndex(findLine(lines, "café"), "Sep 20"),
		runeIndex(findLine(lines, "plain"), "Sep 20"))
}

func runeIndex(line, needle string) int {
	at := strings.Index(line, needle)
	if at < 0 {
		return -1
	}
	return utf8.RuneCountInString(line[:at])
}

// TestPrintLimitsTableCapsIdentityColumns keeps a long account name or email
// from pushing the row past a normal terminal width and undoing alignment.
func TestPrintLimitsTableCapsIdentityColumns(t *testing.T) {
	at := time.Date(2026, 9, 20, 2, 50, 0, 0, time.Local)
	used := 0.0
	results := []app.UsageResult{{
		Account: domain.Account{DisplayName: strings.Repeat("n", 60)},
		Info:    app.UsageAccountInfo{UserEmail: strings.Repeat("e", 60) + "@example.com"},
		Usage: domain.UsageSummary{PlanType: "free", Quotas: []domain.UsageQuota{{
			Name: "monthly", DisplayName: "Monthly Limit",
			WindowSeconds: 2592000, UsedPercent: &used, ResetsAt: &at,
		}}},
	}}

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	printLimitsTable(cmd, results, true)

	row := findLine(strings.Split(out.String(), "\n"), "nnn")
	require.Contains(t, row, "...")
	// Uncapped this row would exceed 130 columns from the identity fields
	// alone; capped it stays close to a standard terminal width.
	require.LessOrEqual(t, utf8.RuneCountInString(row), 85)
	require.LessOrEqual(t, utf8.RuneCountInString(findLine(strings.Split(out.String(), "\n"), "ACCOUNT")), 85)
}
