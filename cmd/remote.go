package cmd

import (
	"github.com/spf13/cobra"

	"github.com/grovetools/nb/pkg/service"
)

// NewRemoteCmd creates the `remote` command and its subcommands.
func NewRemoteCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Manage forge integrations and mirror issues/PRs into notes",
		Long: `Commands for mirroring remote forges (GitHub today) into notes.

Notebook document sync lives at ` + "`nb sync`" + `, not here.`,
	}

	cmd.AddCommand(NewRemoteSyncCmd(svc, workspaceOverride))

	return cmd
}
