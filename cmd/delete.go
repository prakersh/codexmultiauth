package cmd

import (
	"context"
	"fmt"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <selector>",
		Short: "Delete a saved account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newService()
			if err != nil {
				return err
			}
			listed, err := manager.List(context.Background())
			if err != nil {
				return err
			}
			accounts := make([]domain.Account, 0, len(listed))
			for _, item := range listed {
				accounts = append(accounts, item.Account)
			}
			account, err := domain.ResolveAccount(accounts, args[0])
			if err != nil {
				return err
			}
			allowActive := false
			for _, item := range listed {
				if item.Account.ID == account.ID && item.IsActive {
					confirmed, err := promptConfirm("Delete currently active account?", false)
					if err != nil {
						return err
					}
					if !confirmed {
						return nil
					}
					allowActive = true
				}
			}
			if err := manager.Delete(context.Background(), app.DeleteInput{
				Selector:          args[0],
				AllowActiveDelete: allowActive,
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted %s\n", account.DisplayName)
			return nil
		},
	}
}
