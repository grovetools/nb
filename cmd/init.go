package cmd

import (
	"fmt"

	grovelogging "github.com/grovetools/core/logging"
	"github.com/spf13/cobra"

	"github.com/grovetools/nb/pkg/service"
)

var initUlog = grovelogging.NewUnifiedLogger("grove-notebook.cmd.init")

func NewInitCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var initMinimal bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize nb in current directory",
		Long: `Initialize nb in the current directory by registering it as a workspace.
	
This command will:
- Register the current directory as a workspace
- Create necessary directory structure`,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := *svc

			if initMinimal {
				// Verify global workspace context can be accessed
				ctx, err := s.GetWorkspaceContext("global")
				if err != nil {
					return fmt.Errorf("could not access global workspace: %w", err)
				}
				path := workspaceDisplayPath(s, ctx)
				pretty := "Initialized with global workspace"
				if path != "" {
					pretty = fmt.Sprintf("Initialized with global workspace at %s", path)
				}
				initUlog.Success("Initialized with global workspace").
					Field("path", path).
					Pretty(pretty).
					PrettyOnly().
					Emit()
				return nil
			}

			// Verify current directory is recognized as a workspace
			ctx, err := s.GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("could not determine workspace context: %w", err)
			}

			path := workspaceDisplayPath(s, ctx)
			pretty := fmt.Sprintf("Initialized workspace '%s'", ctx.NotebookContextWorkspace.Name)
			if path != "" {
				pretty += fmt.Sprintf(" at %s", path)
			}
			if ctx.NotebookContextWorkspace.Kind != "" {
				pretty += fmt.Sprintf("\nKind: %s", ctx.NotebookContextWorkspace.Kind)
			}
			if ctx.Branch != "" {
				pretty += fmt.Sprintf("\nBranch: %s", ctx.Branch)
			}
			pretty += "\n\nReady to use! Try 'nb new' to create your first note."

			initUlog.Success("Initialized workspace").
				Field("name", ctx.NotebookContextWorkspace.Name).
				Field("path", path).
				Field("kind", ctx.NotebookContextWorkspace.Kind).
				Field("branch", ctx.Branch).
				Pretty(pretty).
				PrettyOnly().
				Emit()

			return nil
		},
	}

	cmd.Flags().BoolVar(&initMinimal, "minimal", false, "Only create global workspace")

	return cmd
}

// workspaceDisplayPath returns the path to show the user for an initialized
// workspace. Virtual workspaces like "global" have no on-disk project path,
// so fall back to the resolved notebook notes root for that context.
func workspaceDisplayPath(s *service.Service, ctx *service.WorkspaceContext) string {
	if p := ctx.NotebookContextWorkspace.Path; p != "" {
		return p
	}
	if notesRoot, err := s.GetNotebookLocator().GetNotesDir(ctx.NotebookContextWorkspace, ""); err == nil {
		return notesRoot
	}
	return ""
}
