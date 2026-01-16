package cmd

import (
	"github.com/grovetools/nb/pkg/service"
	"github.com/spf13/cobra"
)

// NewTmuxCmd returns the tmux command with all subcommands configured.
func NewTmuxCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	tmuxCmd := &cobra.Command{
		Use:   "tmux",
		Short: "Tmux window management commands",
		Long:  `Commands for managing nb in dedicated tmux windows.`,
	}

	tmuxCmd.AddCommand(NewTmuxTuiCmd(svc, workspaceOverride))

	return tmuxCmd
}
