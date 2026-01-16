package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	grovelogging "github.com/grovetools/core/logging"
	"github.com/grovetools/nb/pkg/service"
	"github.com/spf13/cobra"
)

var workspaceUlog = grovelogging.NewUnifiedLogger("grove-notebook.cmd.workspace")

func NewWorkspaceCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Manage workspaces (deprecated)",
		Long:  `Manage workspace registrations and settings. Most of this functionality is now handled by 'grove ws'.`,
	}

	// Most subcommands are removed as workspace management is now centralized in grove-core and 'grove ws' command.
	// We keep 'current' for debugging purposes, but point users to 'nb context'.
	cmd.AddCommand(
		newWorkspaceCurrentCmd(svc, workspaceOverride),
	)

	return cmd
}

func newWorkspaceCurrentCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "current",
		Short: "Show current workspace (use 'nb context' instead)",
		Long:  "This command is deprecated. Please use 'nb context' for more detailed information.",
		RunE: func(cmd *cobra.Command, args []string) error {
			s := *svc

			ctx, err := s.GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return err
			}

			workspaceUlog.Info("Deprecated command used").
				Pretty("DEPRECATED: Please use 'nb context' for detailed workspace information.\n---").
				PrettyOnly().
				Emit()

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "PROPERTY\tVALUE")
			fmt.Fprintln(w, "--------\t-----")
			fmt.Fprintf(w, "Current Location\t%s (%s)\n", ctx.CurrentWorkspace.Name, ctx.CurrentWorkspace.Kind)
			fmt.Fprintf(w, "Notebook Scope\t%s (%s)\n", ctx.NotebookContextWorkspace.Name, ctx.NotebookContextWorkspace.Kind)
			fmt.Fprintf(w, "Branch\t%s\n", ctx.Branch)
			return w.Flush()
		},
	}

	return cmd
}
