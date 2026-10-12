package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func wideLimitsFixture() []app.UsageResult {
	used := 27.0
	reset := time.Date(2026, 10, 12, 4, 17, 0, 0, time.Local)
	return []app.UsageResult{{
		Account: domain.Account{DisplayName: "personal-work"},
		Info:    app.UsageAccountInfo{IsActive: true, UserEmail: "barlowschmidt.with.a.long.alias+work@example.com"},
		Usage: domain.UsageSummary{PlanType: "plus", Confidence: domain.UsageConfidenceConfirmed,
			Quotas: []domain.UsageQuota{
				{Name: "five_hour", DisplayName: "5-Hour Limit", WindowSeconds: 18000, UsedPercent: &used, ResetsAt: &reset},
				{Name: "weekly", DisplayName: "Weekly Limit", WindowSeconds: 604800, UsedPercent: &used, ResetsAt: &reset},
				{Name: "review", DisplayName: "Code Review", Category: domain.QuotaCategoryCodeReview, UsedPercent: &used, ResetsAt: &reset},
			}},
	}}
}

func TestWideLimitsUseAvailableSpaceAndPreserveIdentity(t *testing.T) {
	results := wideLimitsFixture()
	for _, width := range []int{100, 160, 240, 300} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			var out bytes.Buffer
			printResponsiveLimits(&out, results, true, width)
			text := out.String()
			require.Contains(t, text, results[0].Info.UserEmail)
			require.Equal(t, 3, strings.Count(text, "73.0% left"))
			require.Equal(t, 3, strings.Count(text, "Oct 12 04:17"))
			for _, line := range strings.Split(text, "\n") {
				require.LessOrEqual(t, ansi.StringWidth(line), width-1)
			}
			if width >= 160 {
				row := findLine(strings.Split(text, "\n"), "personal-work")
				require.Contains(t, row, results[0].Info.UserEmail)
				require.Equal(t, 3, strings.Count(row, "73.0% left"))
				require.Equal(t, 3, strings.Count(row, "Oct 12 04:17"))
				require.True(t, strings.HasPrefix(row, "  personal-work"), "wide rows should start at the ordinary inset, not be centered")
				if width >= 240 {
					require.Equal(t, 3, strings.Count(row, "27.0% used"))
				}
			}
		})
	}
}

func TestWideLimitsUseOneRowPerAccountAndKeepResetsTogether(t *testing.T) {
	results := wideLimitsFixture()
	results[0].Info.UserEmail = "alex@example.com"
	results[0].Usage.Quotas = results[0].Usage.Quotas[:2]
	second := results[0]
	second.Account.DisplayName = "plus2"
	second.Info = app.UsageAccountInfo{UserEmail: "work@example.com"}
	results = append(results, second)
	for _, width := range []int{120, 160, 240, 300} {
		var out bytes.Buffer
		printResponsiveLimits(&out, results, true, width)
		lines := strings.Split(out.String(), "\n")
		first := findLine(lines, "personal-work")
		require.Contains(t, first, "alex@example.com")
		require.Equal(t, 2, strings.Count(first, "73.0% left"))
		require.Equal(t, 2, strings.Count(first, "Oct 12 04:17"))
		secondRow := findLine(lines, "plus2")
		require.Contains(t, secondRow, "work@example.com")
		require.Equal(t, 2, strings.Count(secondRow, "Oct 12 04:17"))
		for i, line := range lines {
			if line == first {
				require.Equal(t, secondRow, lines[i+1], "no spacer or secondary rows between accounts")
			}
		}
	}
}

func TestLimitsLayoutKeepsNarrowAndPlainOutput(t *testing.T) {
	results := wideLimitsFixture()
	for _, width := range []int{40, 76} {
		var out bytes.Buffer
		printResponsiveLimits(&out, results, true, width)
		require.NotContains(t, out.String(), "ACCOUNT / USER")
		require.Equal(t, 3, strings.Count(out.String(), "Oct 12 04:17"))
		for _, line := range strings.Split(out.String(), "\n") {
			require.LessOrEqual(t, ansi.StringWidth(line), width-1)
		}
	}
	var out bytes.Buffer
	command := &cobra.Command{}
	command.SetOut(&out)
	printLimitsTableAtWidth(command, results, false, 0)
	require.NotContains(t, out.String(), "\x1b")
	row := findLine(strings.Split(out.String(), "\n"), "personal-work")
	require.Contains(t, row, results[0].Info.UserEmail)
	require.Equal(t, 3, strings.Count(row, "27.0%"))
}

func TestWideLimitsMeasureUnicodeIdentityInTerminalCells(t *testing.T) {
	results := wideLimitsFixture()
	results[0].Account.DisplayName = strings.Repeat("工程e\u0301", 12)
	results[0].Info.UserEmail = strings.Repeat("工程", 8) + "@example.com"
	var out bytes.Buffer
	printResponsiveLimits(&out, results, true, 240)
	require.Contains(t, out.String(), results[0].Account.DisplayName)
	require.Contains(t, out.String(), results[0].Info.UserEmail)
	for _, line := range strings.Split(out.String(), "\n") {
		require.LessOrEqual(t, ansi.StringWidth(line), 239)
	}
}

func TestWideLimitsContentCapDoesNotTruncateIdentityThatFits(t *testing.T) {
	results := wideLimitsFixture()
	results[0].Info.UserEmail = strings.Repeat("alias", 35) + "@example.com"
	var out bytes.Buffer
	printResponsiveLimits(&out, results, true, 300)
	require.Contains(t, out.String(), results[0].Info.UserEmail)
	for _, line := range strings.Split(out.String(), "\n") {
		require.LessOrEqual(t, ansi.StringWidth(line), 299)
	}
}
