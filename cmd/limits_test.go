package cmd

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
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
func TestPrintLimitsTablePreservesIdentityWhenPiped(t *testing.T) {
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
	require.Contains(t, row, results[0].Account.DisplayName)
	require.Contains(t, row, results[0].Info.UserEmail)
}

func TestLimitsAdaptsToTerminalWidth(t *testing.T) {
	results := limitsLayoutFixture()
	for _, width := range []int{16, 32, 50, 76, 80, 100, 120, 160} {
		for _, dull := range []bool{false, true} {
			var out bytes.Buffer
			command := &cobra.Command{}
			command.SetOut(&out)
			printLimitsTableAtWidth(command, results, dull, width)
			for _, line := range strings.Split(out.String(), "\n") {
				require.LessOrEqual(t, ansi.StringWidth(line), width, "width=%d line=%q", width, line)
			}
			plain := ansi.Strip(out.String())
			require.Contains(t, plain, "Codex Limits")
			if width >= 32 {
				require.Contains(t, plain, "0.0% left")
				require.Contains(t, plain, "57.5% left")
				require.Contains(t, plain, "Sep 20 02:50")
			}
		}
	}
}

func TestLimitsPipedOutputIsPlainAndKeepsEachAccountOnOneLine(t *testing.T) {
	var out bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&out)
	printLimitsTable(command, limitsLayoutFixture(), false)
	require.NotContains(t, out.String(), "\x1b")
	for _, account := range limitsLayoutFixture() {
		row := findLine(strings.Split(out.String(), "\n"), account.Account.DisplayName)
		require.Contains(t, row, account.Info.UserEmail)
		require.Contains(t, row, "42.5%")
		require.Contains(t, row, "100.0%")
		require.Contains(t, row, "Sep 20 02:50")
	}
}

func TestLimitsEmptyAndNoColor(t *testing.T) {
	var out bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&out)
	printLimitsTableAtWidth(command, nil, true, 40)
	require.Contains(t, out.String(), "No saved accounts")
	require.NotContains(t, out.String(), "\x1b")
}

func TestLimitsCellWidthAndTruncation(t *testing.T) {
	require.Equal(t, 4, displayWidth("账户"))
	require.Equal(t, 1, displayWidth("e\u0301"))
	for _, width := range []int{0, 1, 2, 3, 4, 5, 8} {
		value := truncateCell("账户账户账户", width)
		require.LessOrEqual(t, ansi.StringWidth(value), width)
		require.True(t, utf8.ValidString(value))
	}
}

func TestLimitsCommandJSONAndPlain(t *testing.T) {
	original := newService
	t.Cleanup(func() { newService = original })
	results := limitsLayoutFixture()
	results[0].Account.Source = map[string]any{"private_metadata": "must-not-export"}
	svc := &fakeService{usage: results}
	newService = func() (service, error) { return svc, nil }

	output, err := runCommand(newLimitsCmd(), "--json")
	require.NoError(t, err)
	var rows []struct {
		ID      string              `json:"id"`
		Account string              `json:"account"`
		User    string              `json:"user"`
		Active  bool                `json:"active"`
		Usage   domain.UsageSummary `json:"usage"`
	}
	require.NoError(t, json.Unmarshal([]byte(output), &rows))
	require.Len(t, rows, 1)
	require.Equal(t, results[0].Account.ID, rows[0].ID)
	require.Equal(t, results[0].Account.DisplayName, rows[0].Account)
	require.Equal(t, results[0].Info.UserEmail, rows[0].User)
	require.True(t, rows[0].Active)
	require.Len(t, rows[0].Usage.Quotas, 3)
	require.NotContains(t, output, "must-not-export")
	require.NotContains(t, output, "\x1b")
	require.Equal(t, "all", svc.lastUsage)

	output, err = runCommand(newLimitsCmd(), "--plain")
	require.NoError(t, err)
	require.Contains(t, findLine(strings.Split(output, "\n"), results[0].Account.DisplayName), results[0].Info.UserEmail)
	require.NotContains(t, output, "\x1b")
	_, err = runCommand(newLimitsCmd(), "--plain", "--json")
	require.Error(t, err)

	svc.usage = nil
	output, err = runCommand(newLimitsCmd(), "--json")
	require.NoError(t, err)
	require.Equal(t, "[]\n", output)
}

func TestLimitsMetadataCannotInjectRowsOrTerminalControls(t *testing.T) {
	results := limitsLayoutFixture()
	results[0].Account.DisplayName = "acct\n\x1b[31mred"
	results[0].Info.UserEmail = "user\t@example.com"
	results[0].Usage.Quotas[0].DisplayName = "5H\nLimit\x1b[31m"
	var out bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&out)
	printLimitsTable(command, results, false)
	require.NotContains(t, out.String(), "\x1b")
	row := findLine(strings.Split(out.String(), "\n"), "acct red")
	require.Contains(t, row, "user @example.com")
	require.Contains(t, row, "100.0%")
}

func limitsLayoutFixture() []app.UsageResult {
	at := time.Date(2026, 9, 20, 2, 50, 0, 0, time.Local)
	return []app.UsageResult{{
		Account: domain.Account{ID: "fixture-account", DisplayName: "plus-account-账户"},
		Info:    app.UsageAccountInfo{IsActive: true, UserEmail: "long-email-address@example.com"},
		Usage: domain.UsageSummary{PlanType: "plus", Confidence: domain.UsageConfidenceConfirmed,
			Quotas: []domain.UsageQuota{
				{Name: "short", DisplayName: "5H Limit", WindowSeconds: 18000, UsedPercent: floatPtr(100), ResetsAt: &at},
				{Name: "weekly", DisplayName: "Weekly Limit", WindowSeconds: 604800, UsedPercent: floatPtr(42.5), ResetsAt: &at},
				{Name: "review", DisplayName: "Review Requests", UsedPercent: floatPtr(12), ResetsAt: &at},
			},
		},
	}}
}

// TestPrintLimitsTableShowsDataProvenance covers the ambiguity the DATA column
// removes: usage fetch errors are swallowed, so a row of "-" could mean either
// "nothing consumed" or "never reached the API".
func TestPrintLimitsTableShowsDataProvenance(t *testing.T) {
	at := time.Date(2026, 9, 20, 2, 50, 0, 0, time.Local)
	used := 0.0
	results := []app.UsageResult{
		{
			Account: domain.Account{DisplayName: "live-acct"},
			Info:    app.UsageAccountInfo{UserEmail: "live@example.com"},
			Usage: domain.UsageSummary{
				PlanType: "free", Confidence: domain.UsageConfidenceConfirmed,
				Quotas: []domain.UsageQuota{{
					Name: "monthly", DisplayName: "Monthly Limit",
					WindowSeconds: 2592000, UsedPercent: &used, ResetsAt: &at,
				}},
			},
		},
		{
			Account: domain.Account{DisplayName: "unreachable"},
			Info:    app.UsageAccountInfo{UserEmail: "down@example.com"},
			Usage:   domain.UsageSummary{PlanType: "free", Confidence: domain.UsageConfidenceUnknown},
		},
		{
			Account: domain.Account{DisplayName: "blocked"},
			Info:    app.UsageAccountInfo{UserEmail: "blocked@example.com"},
			Usage: domain.UsageSummary{
				PlanType: "pro", Confidence: domain.UsageConfidenceConfirmed, LimitReached: true,
			},
		},
	}

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	printLimitsTable(cmd, results, true)

	output := out.String()
	lines := strings.Split(output, "\n")
	require.Contains(t, findLine(lines, "ACCOUNT"), "DATA")
	require.Contains(t, findLine(lines, "live-acct"), "live")
	require.Contains(t, findLine(lines, "unreachable"), "none")
	require.Contains(t, findLine(lines, "blocked"), "limited")

	// User-facing output must stay on standard keyboard characters.
	require.NotContains(t, output, "─")
	require.NotContains(t, output, "—")
	require.NotContains(t, output, "…")
}
