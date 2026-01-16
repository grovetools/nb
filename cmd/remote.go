package cmd

import (
	"github.com/grovetools/nb/pkg/service"
	"github.com/spf13/cobra"
)

// NewRemoteCmd creates the `remote` command and its subcommands.
func NewRemoteCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Manage remote integrations and sync notes",
		Long:  `Commands for syncing notes with remote services like GitHub.`,
	}

	cmd.AddCommand(NewSyncCmd(svc, workspaceOverride))

	return cmd
}
