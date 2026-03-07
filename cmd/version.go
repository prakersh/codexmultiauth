package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const (
	repositoryURL = "https://github.com/prakersh/codexmultiauth"
	supportURL    = "https://buymeacoffee.com/prakersh"
)

var (
	Version = "0.0.1"
	Commit  = "none"
	Date    = "unknown"
)

func newVersionCmd() *cobra.Command {
	var short bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show cma version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			if short {
				fmt.Fprintln(cmd.OutOrStdout(), Version)
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "cma version: %s\n", Version)
			fmt.Fprintf(cmd.OutOrStdout(), "repository: %s\n", repositoryURL)
			fmt.Fprintf(cmd.OutOrStdout(), "support: %s\n", supportURL)
			return nil
		},
	}

	cmd.Flags().BoolVar(&short, "short", false, "print version only")
	return cmd
}
