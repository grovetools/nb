package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/grovetools/nb/pkg/service"
)

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
				// Return specific path to stdout
				if path, ok := ctx.Paths[contextPath]; ok {
					fmt.Fprintln(cmd.OutOrStdout(), path)
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

			// Human-readable output to stdout
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "Current Location:")
			fmt.Fprintf(out, "  Name: %s\n", ctx.CurrentWorkspace.Name)
			fmt.Fprintf(out, "  Path: %s\n", ctx.CurrentWorkspace.Path)
			fmt.Fprintf(out, "  Kind: %s\n", ctx.CurrentWorkspace.Kind)
			if ctx.Branch != "" {
				fmt.Fprintf(out, "  Branch: %s\n", ctx.Branch)
			}

			fmt.Fprintln(out, "\nNotebook Scope:")
			fmt.Fprintf(out, "  Name: %s\n", ctx.NotebookContextWorkspace.Name)
			fmt.Fprintf(out, "  Identifier: %s\n", ctx.NotebookContextWorkspace.Identifier())
			fmt.Fprintf(out, "  Path: %s\n", ctx.NotebookContextWorkspace.Path)
			fmt.Fprintf(out, "  Kind: %s\n", ctx.NotebookContextWorkspace.Kind)

			fmt.Fprintln(out, "\nPaths:")
			for key, path := range ctx.Paths {
				fmt.Fprintf(out, "  %s: %s\n", key, path)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&contextJSON, "json", false, "Output as JSON")
	cmd.Flags().StringVar(&contextPath, "path", "", "Get specific path (current, llm, learn)")

	return cmd
}
