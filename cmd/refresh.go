package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newRefreshCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh <all|selector>",
		Short: "Refresh OAuth tokens for one or all saved accounts",
		Long: `Refresh forces a token-authority refresh for the selected accounts and persists
the new access/refresh tokens atomically. Pass "all" to refresh every saved
account, or a specific display name, alias, or ID.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newService()
			if err != nil {
				return err
			}
			results, err := manager.Refresh(context.Background(), args[0])
			if err != nil {
				return err
			}
			if len(results) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No accounts matched.")
				return nil
			}

			var firstErr error
			refreshed, unchanged, failed := 0, 0, 0
			for _, result := range results {
				switch {
				case result.Err != nil:
					failed++
					fmt.Fprintf(cmd.OutOrStdout(), "  %s: error: %v\n", result.Account.DisplayName, result.Err)
					if firstErr == nil {
						firstErr = result.Err
					}
				case result.Refreshed:
					refreshed++
					fmt.Fprintf(cmd.OutOrStdout(), "  %s: refreshed\n", result.Account.DisplayName)
				default:
					unchanged++
					fmt.Fprintf(cmd.OutOrStdout(), "  %s: no change\n", result.Account.DisplayName)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nRefreshed %d, unchanged %d, failed %d of %d account(s).\n", refreshed, unchanged, failed, len(results))
			if firstErr != nil {
				return fmt.Errorf("one or more accounts failed to refresh: %w", firstErr)
			}
			return nil
		},
	}
}
