package cmd

import (
	"github.com/prakersh/codexmultiauth/internal/tui"
	"github.com/spf13/cobra"
)

var runTUI = func(svc service) error {
	return tui.Run(svc)
}

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newService()
			if err != nil {
				return err
			}
			return runTUI(manager)
		},
	}
}
