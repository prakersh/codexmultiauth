package cmd

import (
	"context"
	"fmt"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/spf13/cobra"
)

func newNewCmd() *cobra.Command {
	var name string
	var aliases string
	var deviceAuth bool

	cmd := &cobra.Command{
		Use:   "new",
		Short: "Run Codex login and save the resulting account",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newService()
			if err != nil {
				return err
			}
			if name == "" {
				name, err = promptText("Account name (optional)", "")
				if err != nil {
					return err
				}
			}
			if aliases == "" {
				aliases, err = promptText("Aliases (comma-separated, optional)", "")
				if err != nil {
					return err
				}
			}
			result, err := manager.New(context.Background(), app.NewInput{
				DisplayName: name,
				Aliases:     splitAliases(aliases),
				DeviceAuth:  deviceAuth,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Saved %s\n", result.Account.DisplayName)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "display name")
	cmd.Flags().StringVar(&aliases, "aliases", "", "comma-separated aliases")
	cmd.Flags().BoolVar(&deviceAuth, "device-auth", false, "use device auth flow")
	return cmd
}
