package cmd

import (
	"context"
	"fmt"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/spf13/cobra"
)

func newLoginCmd() *cobra.Command {
	var name string
	var aliases string
	var deviceAuth bool
	var withAPIKey bool

	cmd := &cobra.Command{
		Use:     "login",
		Aliases: []string{"new"},
		Short:   "Run Codex login and save the resulting account",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newService()
			if err != nil {
				return err
			}
			if name == "" && !withAPIKey {
				name, err = promptText("Account name (optional)", "")
				if err != nil {
					return err
				}
			}
			result, err := manager.New(context.Background(), app.NewInput{
				DisplayName: name,
				Aliases:     splitAliases(aliases),
				DeviceAuth:  deviceAuth,
				WithAPIKey:  withAPIKey,
			})
			if err != nil {
				return err
			}
			if result.Updated {
				fmt.Fprintf(cmd.OutOrStdout(), "Updated %s\n", result.Account.DisplayName)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Saved %s\n", result.Account.DisplayName)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "display name")
	cmd.Flags().StringVar(&aliases, "aliases", "", "comma-separated aliases")
	cmd.Flags().BoolVar(&deviceAuth, "device-auth", false, "use device auth flow")
	cmd.Flags().BoolVar(&withAPIKey, "with-api-key", false, "read the API key from stdin through codex login")
	return cmd
}

func newNewCmd() *cobra.Command {
	return newLoginCmd()
}
