package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newService()
			if err != nil {
				return err
			}
			accounts, err := manager.List(context.Background())
			if err != nil {
				return err
			}
			for i, account := range accounts {
				active := " "
				if account.IsActive {
					active = "*"
				}
				aliases := ""
				if len(account.Account.Aliases) > 0 {
					aliases = " [" + strings.Join(account.Account.Aliases, ", ") + "]"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %d. %s%s (%s)\n", active, i+1, account.Account.DisplayName, aliases, account.Account.AuthStoreKind)
			}
			return nil
		},
	}
}
