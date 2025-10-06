package cmd

import (
	"fmt"

	"github.com/mattsolo1/grove-notebook/cmd/config"
	"github.com/mattsolo1/grove-notebook/pkg/workspace"
	"github.com/spf13/cobra"
)

func newWorkspaceUpdateNotebookDirCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "update-notebook-dir",
		Short: "Update all workspaces to use the configured notebook directory",
		Long: `Updates the notebook_dir for all registered workspaces to match
the configured value in grove.yml (nb.notebook_dir).

This is useful when you've changed your notebook_dir configuration
and want to update existing workspaces to use the new path.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			config.InitConfig()
			svc, err := config.InitService()
			if err != nil {
				return err
			}
			defer svc.Close()

			// Get the configured notebook directory
			newNotebookDir := workspace.GetDefaultNotebookDir()
			fmt.Printf("Configured notebook directory: %s\n\n", newNotebookDir)

			// Get all workspaces
			workspaces, err := svc.Registry.List()
			if err != nil {
				return err
			}

			updated := 0
			unchanged := 0

			for _, ws := range workspaces {
				if ws.NotebookDir == newNotebookDir {
					unchanged++
					continue
				}

				oldDir := ws.NotebookDir
				if dryRun {
					fmt.Printf("[DRY RUN] Would update '%s': %s -> %s\n", ws.Name, oldDir, newNotebookDir)
					updated++
					continue
				}

				// Update the workspace
				ws.NotebookDir = newNotebookDir
				if err := svc.Registry.Add(ws); err != nil {
					fmt.Printf("✗ Failed to update '%s': %v\n", ws.Name, err)
					continue
				}

				fmt.Printf("✓ Updated '%s': %s -> %s\n", ws.Name, oldDir, newNotebookDir)
				updated++
			}

			fmt.Printf("\nUpdate complete:\n")
			fmt.Printf("  Updated: %d\n", updated)
			fmt.Printf("  Unchanged: %d\n", unchanged)

			if dryRun {
				fmt.Printf("\nRun without --dry-run to apply changes\n")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be updated without making changes")

	// Add global flags
	config.AddGlobalFlags(cmd)

	return cmd
}
