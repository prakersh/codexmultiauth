package cmd

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

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

	require.Equal(t, strings.Index(header, "5H RESETS AT"), strings.Index(rowZero, "Apr 14 16:51"))
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
