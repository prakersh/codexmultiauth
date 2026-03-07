package cmd

import (
	"context"
	"fmt"

	"github.com/prakersh/codexmultiauth/internal/app"
	"github.com/prakersh/codexmultiauth/internal/domain"
	"github.com/spf13/cobra"
)

func newRestoreCmd() *cobra.Command {
	var all bool
	var allowPlain bool
	var conflict string

	cmd := &cobra.Command{
		Use:   "restore <encrypthash/pass> <pathtobackup|name>",
		Short: "Restore accounts from an encrypted backup",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newService()
			if err != nil {
				return err
			}
			passphrase, err := app.ResolvePassphrase(args[0], allowPlain, func(prompt string) ([]byte, error) {
				return promptPassword(prompt)
			})
			if err != nil {
				return err
			}
			policy := domain.ConflictPolicy(conflict)
			input := app.RestoreInput{
				Passphrase: passphrase,
				Source:     args[1],
				All:        all,
				Conflict:   policy,
				Decisions:  map[string]domain.ConflictPolicy{},
			}
			artifact, candidates, err := manager.InspectBackup(input)
			if err != nil {
				return err
			}
			_ = artifact

			if !all {
				options := make([]string, 0, len(candidates))
				optionToID := map[string]string{}
				for _, candidate := range candidates {
					label := candidate.Account.DisplayName
					if candidate.Conflict != nil {
						label += " [conflict:" + candidate.Conflict.Reason + "]"
					}
					options = append(options, label)
					optionToID[label] = candidate.Account.ID
				}
				selected, err := promptMultiSelect("Select accounts to restore", options)
				if err != nil {
					return err
				}
				for _, label := range selected {
					input.Selected = append(input.Selected, optionToID[label])
				}
			}

			if policy == domain.ConflictAsk {
				for _, candidate := range candidates {
					if candidate.Conflict == nil {
						continue
					}
					if !all && !contains(input.Selected, candidate.Account.ID) {
						continue
					}
					decision, err := promptConflictPolicy(candidate.Account.DisplayName)
					if err != nil {
						return err
					}
					input.Decisions[candidate.Account.ID] = decision
				}
			}

			summary, err := manager.Restore(context.Background(), input)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Imported %d account(s)\n", summary.Imported)
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "restore all accounts atomically")
	cmd.Flags().BoolVar(&allowPlain, "allow-plain-pass-arg", false, "allow pass:<literal> sources")
	cmd.Flags().StringVar(&conflict, "conflict", string(domain.ConflictAsk), "conflict policy: ask|overwrite|skip|rename")
	return cmd
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
