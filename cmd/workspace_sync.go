package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mattsolo1/grove-notebook/cmd/config"
	"github.com/mattsolo1/grove-notebook/pkg/workspace"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newWorkspaceSyncCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync workspaces from grove.yml groves configuration",
		Long: `Discovers and registers all git repositories found in the groves
defined in your grove.yml configuration file.

This command will:
- Read groves from ~/.config/grove/grove.yml
- Scan each grove path for git repositories
- Auto-register any unregistered repositories as workspaces`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load grove configuration
			groveConfig := viper.New()
			groveConfig.SetConfigName("grove")
			groveConfig.SetConfigType("yaml")
			groveConfig.AddConfigPath("$HOME/.config/grove")

			if err := groveConfig.ReadInConfig(); err != nil {
				return fmt.Errorf("failed to read grove.yml: %w", err)
			}

			// Initialize nb config and service
			config.InitConfig()
			svc, err := config.InitService()
			if err != nil {
				return err
			}
			defer svc.Close()

			// Get groves configuration
			grovesConfig := groveConfig.GetStringMap("groves")
			if len(grovesConfig) == 0 {
				fmt.Println("No groves configured in grove.yml")
				return nil
			}

			notebookDir := workspace.GetDefaultNotebookDir()
			home, _ := os.UserHomeDir()

			registered := 0
			skipped := 0
			errors := 0

			// Iterate through each grove
			for groveName, groveData := range grovesConfig {
				groveMap, ok := groveData.(map[string]interface{})
				if !ok {
					continue
				}

				// Check if enabled
				enabled, ok := groveMap["enabled"].(bool)
				if !enabled {
					fmt.Printf("Skipping disabled grove: %s\n", groveName)
					continue
				}

				// Get path
				grovePath, ok := groveMap["path"].(string)
				if !ok {
					continue
				}

				// Expand ~ in path
				if grovePath[:2] == "~/" {
					grovePath = filepath.Join(home, grovePath[2:])
				}

				fmt.Printf("\nScanning grove '%s' at %s...\n", groveName, grovePath)

				// Walk the grove directory
				err := filepath.Walk(grovePath, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return nil // Skip errors
					}

					// Check if this is a git repository
					if info.IsDir() && info.Name() == ".git" {
						repoPath := filepath.Dir(path)
						repoName := filepath.Base(repoPath)

						// Check if already registered
						if _, err := svc.Registry.Get(repoName); err == nil {
							skipped++
							return filepath.SkipDir
						}

						if dryRun {
							fmt.Printf("  [DRY RUN] Would register: %s\n", repoName)
							registered++
							return filepath.SkipDir
						}

						// Register the workspace
						ws := &workspace.Workspace{
							Name:        repoName,
							Path:        repoPath,
							Type:        workspace.TypeGitRepo,
							NotebookDir: notebookDir,
							Settings: map[string]interface{}{
								"grove":      groveName,
								"discovered": true,
							},
						}

						if err := svc.Registry.Add(ws); err != nil {
							fmt.Printf("  ✗ Failed to register %s: %v\n", repoName, err)
							errors++
						} else {
							fmt.Printf("  ✓ Registered: %s\n", repoName)
							registered++
						}

						return filepath.SkipDir
					}

					return nil
				})

				if err != nil {
					fmt.Printf("Error scanning grove %s: %v\n", groveName, err)
				}
			}

			fmt.Printf("\nSync complete:\n")
			fmt.Printf("  Registered: %d\n", registered)
			fmt.Printf("  Skipped (already registered): %d\n", skipped)
			if errors > 0 {
				fmt.Printf("  Errors: %d\n", errors)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be registered without making changes")

	// Add global flags
	config.AddGlobalFlags(cmd)

	return cmd
}
