package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mattsolo1/grove-notebook/cmd/config"
)

func NewContextCmd() *cobra.Command {
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
			// Initialize config and service
			config.InitConfig()
			svc, err := config.InitService()
			if err != nil {
				return err
			}
			defer svc.Close()

			ctx, err := svc.GetWorkspaceContext()
			if err != nil {
				return err
			}

			if contextPath != "" {
				// Return specific path
				if path, ok := ctx.Paths[contextPath]; ok {
					fmt.Println(path)
				} else {
					return fmt.Errorf("unknown path type: %s", contextPath)
				}
				return nil
			}

			if contextJSON {
				// JSON output
				output := map[string]any{
					"workspace": ctx.Workspace.Name,
					"type":      ctx.Workspace.Type,
					"branch":    ctx.Branch,
					"paths":     ctx.Paths,
				}

				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(output)
			}

			// Human-readable output
			fmt.Printf("Workspace: %s\n", ctx.Workspace.Name)
			fmt.Printf("Type: %s\n", ctx.Workspace.Type)
			if ctx.Branch != "" {
				fmt.Printf("Branch: %s\n", ctx.Branch)
			}
			fmt.Println("\nPaths:")
			for key, path := range ctx.Paths {
				fmt.Printf("  %s: %s\n", key, path)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&contextJSON, "json", false, "Output as JSON")
	cmd.Flags().StringVar(&contextPath, "path", "", "Get specific path (current, llm, learn)")

	// Add global flags
	config.AddGlobalFlags(cmd)

	return cmd
}