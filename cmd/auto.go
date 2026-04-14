package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newAutoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "auto",
		Short: "Activate the best account by remaining quota",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newService()
			if err != nil {
				return err
			}
			account, err := manager.AutoActivate(context.Background())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Activated %s\n", account.DisplayName)
			return nil
		},
	}
}
