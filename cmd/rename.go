package cmd

import (
	"context"
	"fmt"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/spf13/cobra"
)

func newRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <selector> <new-name>",
		Short: "Rename a saved account",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newService()
			if err != nil {
				return err
			}
			if err := manager.Rename(context.Background(), app.RenameInput{
				Selector: args[0],
				NewName:  args[1],
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Renamed %s to %s\n", args[0], args[1])
			return nil
		},
	}
}
