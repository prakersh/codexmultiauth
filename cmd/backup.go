package cmd

import (
	"context"
	"fmt"

	"github.com/prakersh/codexmultiauth/internal/app"
	cmacrypto "github.com/prakersh/codexmultiauth/internal/infra/crypto"
	"github.com/spf13/cobra"
)

func newBackupCmd() *cobra.Command {
	var allowPlain bool
	cmd := &cobra.Command{
		Use:   "backup <passphrase-source> <name|abspath>",
		Short: "Write an encrypted backup artifact",
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
			// Wipe the passphrase once the command is done with it, so it does
			// not linger on the heap for the life of the process.
			defer cmacrypto.Zero(passphrase)
			path, err := manager.Backup(context.Background(), app.BackupInput{
				Passphrase: passphrase,
				Target:     args[1],
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Backup written to %s\n", path)
			return nil
		},
	}
	cmd.Flags().BoolVar(&allowPlain, "allow-plain-pass-arg", false, "allow passphrase arguments that place the passphrase in argv: hash:<hex>, pass:<literal>, and bare literals")
	return cmd
}
