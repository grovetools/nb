package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	grovelogging "github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-notebook/pkg/service"
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
			bgCtx := context.Background()
			s := *svc

			if initMinimal {
				// Verify global workspace context can be accessed
				ctx, err := s.GetWorkspaceContext("global")
				if err != nil {
					return fmt.Errorf("could not access global workspace: %w", err)
				}
				initUlog.Success("Initialized with global workspace").
					Field("path", ctx.NotebookContextWorkspace.Path).
					Pretty(fmt.Sprintf("Initialized with global workspace at %s", ctx.NotebookContextWorkspace.Path)).
					PrettyOnly().
					Log(bgCtx)
				return nil
			}

			// Verify current directory is recognized as a workspace
			ctx, err := s.GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("could not determine workspace context: %w", err)
			}

			initUlog.Success("Initialized workspace").
				Field("name", ctx.NotebookContextWorkspace.Name).
				Field("path", ctx.NotebookContextWorkspace.Path).
				Field("kind", ctx.NotebookContextWorkspace.Kind).
				Field("branch", ctx.Branch).
				Pretty(fmt.Sprintf("Initialized workspace '%s' at %s\nKind: %s%s\n\nReady to use! Try 'nb new' to create your first note.",
					ctx.NotebookContextWorkspace.Name,
					ctx.NotebookContextWorkspace.Path,
					ctx.NotebookContextWorkspace.Kind,
					func() string {
						if ctx.Branch != "" {
							return fmt.Sprintf("\nBranch: %s", ctx.Branch)
						}
						return ""
					}())).
				PrettyOnly().
				Log(bgCtx)

			return nil
		},
	}

	cmd.Flags().BoolVar(&initMinimal, "minimal", false, "Only create global workspace")

	return cmd
}