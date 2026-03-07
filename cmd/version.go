package cmd

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

const (
	repositoryURL = "https://github.com/prakersh/codexmultiauth"
	supportURL    = "https://buymeacoffee.com/prakersh"
)

var (
	// Version can be overridden at build time:
	// go build -ldflags "-X github.com/prakersh/codexmultiauth/cmd.Version=vX.Y.Z"
	Version = ""
	Commit  = "none"
	Date    = "unknown"
)

//go:embed VERSION
var versionFile string

func newVersionCmd() *cobra.Command {
	var short bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show cma version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			version := effectiveVersion()
			if short {
				fmt.Fprintln(cmd.OutOrStdout(), version)
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "cma version: %s\n", version)
			fmt.Fprintf(cmd.OutOrStdout(), "repository: %s\n", repositoryURL)
			fmt.Fprintf(cmd.OutOrStdout(), "support: %s\n", supportURL)
			return nil
		},
	}

	cmd.Flags().BoolVar(&short, "short", false, "print version only")
	return cmd
}

func effectiveVersion() string {
	if v := strings.TrimSpace(Version); v != "" {
		return v
	}
	if v := strings.TrimSpace(versionFile); v != "" {
		return v
	}
	return "dev"
}
