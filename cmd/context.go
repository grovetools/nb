package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	grovelogging "github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

var contextUlog = grovelogging.NewUnifiedLogger("grove-notebook.cmd.context")

func NewContextCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var (
		contextJSON bool
		contextPath string
	)

	cmd := &cobra.Command{
		Use:   "context",
		Short: "Show current workspace context",
		Long: `Display information about the current workspace context.

This is useful for integration with other tools like Neovim.`,
		RunE: func(cmd *cobra.Command, args []string) error {
	s := *svc

			ctx, err := s.GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return err
			}

			if contextPath != "" {
				// Return specific path
				if path, ok := ctx.Paths[contextPath]; ok {
					contextUlog.Info("Context path").
						Field("path_type", contextPath).
						Field("path", path).
						Pretty(path).
						PrettyOnly().
						Emit()
				} else {
					return fmt.Errorf("unknown path type: %s", contextPath)
				}
				return nil
			}

			if contextJSON {
				// JSON output
				output := map[string]any{
					"current_workspace":          ctx.CurrentWorkspace,
					"notebook_context_workspace": ctx.NotebookContextWorkspace,
					"branch":                     ctx.Branch,
					"paths":                      ctx.Paths,
				}

				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(output)
			}

			// Human-readable output
			var prettyOutput strings.Builder
			prettyOutput.WriteString("Current Location:\n")
			prettyOutput.WriteString(fmt.Sprintf("  Name: %s\n", ctx.CurrentWorkspace.Name))
			prettyOutput.WriteString(fmt.Sprintf("  Path: %s\n", ctx.CurrentWorkspace.Path))
			prettyOutput.WriteString(fmt.Sprintf("  Kind: %s\n", ctx.CurrentWorkspace.Kind))
			if ctx.Branch != "" {
				prettyOutput.WriteString(fmt.Sprintf("  Branch: %s\n", ctx.Branch))
			}

			prettyOutput.WriteString("\nNotebook Scope:\n")
			prettyOutput.WriteString(fmt.Sprintf("  Name: %s\n", ctx.NotebookContextWorkspace.Name))
			prettyOutput.WriteString(fmt.Sprintf("  Identifier: %s\n", ctx.NotebookContextWorkspace.Identifier()))
			prettyOutput.WriteString(fmt.Sprintf("  Path: %s\n", ctx.NotebookContextWorkspace.Path))
			prettyOutput.WriteString(fmt.Sprintf("  Kind: %s\n", ctx.NotebookContextWorkspace.Kind))

			prettyOutput.WriteString("\nPaths:\n")
			for key, path := range ctx.Paths {
				prettyOutput.WriteString(fmt.Sprintf("  %s: %s\n", key, path))
			}

	contextUlog.Info("Workspace context").
				Field("current_workspace", ctx.CurrentWorkspace.Name).
				Field("notebook_workspace", ctx.NotebookContextWorkspace.Name).
				Field("branch", ctx.Branch).
				Field("paths", ctx.Paths).
				Pretty(prettyOutput.String()).
				PrettyOnly().
				Emit()

			return nil
		},
	}

	cmd.Flags().BoolVar(&contextJSON, "json", false, "Output as JSON")
	cmd.Flags().StringVar(&contextPath, "path", "", "Get specific path (current, llm, learn)")

	return cmd
}
