package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newActivateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "activate <selector>",
		Short: "Activate a saved account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newService()
			if err != nil {
				return err
			}
			account, err := manager.Activate(context.Background(), args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Activated %s\n", account.DisplayName)
			return nil
		},
	}
}
