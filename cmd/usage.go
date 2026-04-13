package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/spf13/cobra"
)

func newUsageCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "usage <selector|all>",
		Short: "Show usage with confidence labels",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newService()
			if err != nil {
				return err
			}
			results, err := manager.Usage(context.Background(), args[0])
			if err != nil {
				return err
			}
			printUsageResults(cmd, results)
			return nil
		},
	}
}

func printUsageResults(cmd *cobra.Command, results []app.UsageResult) {
	out := cmd.OutOrStdout()
	for index, result := range results {
		if index > 0 {
			fmt.Fprintln(out)
		}

		title := result.Account.DisplayName
		if result.Info.IsActive {
			title += " [active]"
		}
		fmt.Fprintf(out, "%s\n", title)
		fmt.Fprintf(out, "  id: %s\n", shortenValue(result.Account.ID, 12))
		if len(result.Account.Aliases) > 0 {
			fmt.Fprintf(out, "  aliases: %s\n", strings.Join(result.Account.Aliases, ", "))
		}
		if result.Account.AuthStoreKind != "" {
			fmt.Fprintf(out, "  auth store: %s\n", result.Account.AuthStoreKind)
		}
		if result.Info.AuthMode != "" {
			fmt.Fprintf(out, "  auth mode: %s\n", result.Info.AuthMode)
		}
		if result.Info.CodexAccountID != "" {
			fmt.Fprintf(out, "  codex account id: %s\n", result.Info.CodexAccountID)
		}
		if user := formatUsageUser(result.Info); user != "" {
			fmt.Fprintf(out, "  user: %s\n", user)
		}
		if !result.Account.CreatedAt.IsZero() {
			fmt.Fprintf(out, "  saved: %s\n", formatUsageTime(result.Account.CreatedAt))
		}
		if result.Account.LastUsedAt != nil && !result.Account.LastUsedAt.IsZero() {
			fmt.Fprintf(out, "  last used: %s\n", formatUsageTime(*result.Account.LastUsedAt))
		}
		fmt.Fprintf(out, "  confidence: %s\n", result.Usage.Confidence)
		if !result.Usage.FetchedAt.IsZero() {
			fmt.Fprintf(out, "  fetched: %s\n", formatUsageTime(result.Usage.FetchedAt))
		}
		if result.Usage.PlanType != "" {
			fmt.Fprintf(out, "  plan: %s\n", result.Usage.PlanType)
		}
		if result.Usage.CreditsLeft != nil {
			fmt.Fprintf(out, "  credits left: %.1f\n", *result.Usage.CreditsLeft)
		}
		for _, quota := range result.Usage.Quotas {
			fmt.Fprintf(out, "  %s: %s\n", quota.DisplayName, formatQuota(quota))
		}
	}
}

func formatQuota(quota domain.UsageQuota) string {
	if quota.UsedPercent == nil {
		return "unknown"
	}

	details := []string{}
	if quota.Status != "" {
		details = append(details, quota.Status)
	}
	if quota.ResetsAt != nil && !quota.ResetsAt.IsZero() {
		details = append(details, "resets "+formatUsageTime(*quota.ResetsAt))
	}

	value := fmt.Sprintf("%.1f%% used", *quota.UsedPercent)
	if len(details) == 0 {
		return value
	}
	return value + " (" + strings.Join(details, ", ") + ")"
}

func formatUsageUser(info app.UsageAccountInfo) string {
	switch {
	case info.UserName != "" && info.UserEmail != "" && info.UserName != info.UserEmail:
		return info.UserName + " <" + info.UserEmail + ">"
	case info.UserEmail != "":
		return info.UserEmail
	default:
		return info.UserName
	}
}

func formatUsageTime(value time.Time) string {
	return value.UTC().Format("2006-01-02 15:04 MST")
}

func shortenValue(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}
