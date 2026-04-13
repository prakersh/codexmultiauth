package cmd

import "github.com/spf13/cobra"

var rootCmdFactory = newRootCmd

func Execute() error {
	return rootCmdFactory().Execute()
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "cma",
		Short:         "Manage multiple Codex accounts safely",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(
		newListCmd(),
		newUsageCmd(),
		newLimitsCmd(),
		newVersionCmd(),
		newSaveCmd(),
		newLoginCmd(),
		newActivateCmd(),
		newDeleteCmd(),
		newRenameCmd(),
		newBackupCmd(),
		newRestoreCmd(),
		newTUICmd(),
	)

	return cmd
}
