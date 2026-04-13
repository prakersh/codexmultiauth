package cmd

import (
	"context"
	"fmt"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/spf13/cobra"
)

func newSaveCmd() *cobra.Command {
	var name string
	var aliases string

	cmd := &cobra.Command{
		Use:   "save",
		Short: "Save the current Codex auth into the encrypted vault",
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
			result, err := manager.Save(context.Background(), app.SaveInput{
				DisplayName: name,
				Aliases:     splitAliases(aliases),
			})
			if err != nil {
				return err
			}
			if result.Deduplicated {
				fmt.Fprintf(cmd.OutOrStdout(), "Already saved as %s\n", result.Account.DisplayName)
				return nil
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
	return cmd
}
