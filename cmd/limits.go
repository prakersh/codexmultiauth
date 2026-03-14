package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

func newLimitsCmd() *cobra.Command {
	return &cobra.Command{
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
			printUsageResults(cmd, results)
			return nil
		},
	}
}
