package cmd

import (
	"context"
	"fmt"

	"github.com/prakersh/codexmultiauth/internal/app"
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
	for _, result := range results {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n", result.Account.DisplayName)
		fmt.Fprintf(cmd.OutOrStdout(), "  confidence: %s\n", result.Usage.Confidence)
		if result.Usage.PlanType != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  plan: %s\n", result.Usage.PlanType)
		}
		for _, quota := range result.Usage.Quotas {
			if quota.UsedPercent != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s: %.1f%%\n", quota.DisplayName, *quota.UsedPercent)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s: unknown\n", quota.DisplayName)
			}
		}
	}
}
