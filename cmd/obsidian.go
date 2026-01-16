package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/grovetools/nb/pkg/service"
)

func NewObsidianCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "obsidian",
		Short: "Manage Obsidian integration",
		Long:  `Commands for managing nb's Obsidian plugin integration.`,
	}

	cmd.AddCommand(newObsidianInstallCmd())

	return cmd
}

func newObsidianInstallCmd() *cobra.Command {
	var vaultPath string

	cmd := &cobra.Command{
		Use:   "install-dev",
		Short: "Install the nb Obsidian plugin for development",
		Long: `Create a symlink to the nb Obsidian plugin source for development.

This command is intended for developers working on the nb Obsidian plugin.
It creates a symbolic link from the plugin source code to your Obsidian
vault's plugins directory, allowing you to test changes without copying files.

For regular users: Download the plugin from Obsidian's community plugins
or from the releases page instead.

By default, installs to ~/Documents/nb/.obsidian/plugins/nb-integration.
You can specify a different vault path with the --vault flag.

Examples:
  nb obsidian install-dev                       # Link to default vault
  nb obsidian install-dev --vault ~/my-vault    # Link to custom vault`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Determine vault path
			vault := vaultPath
			if vault == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				vault = filepath.Join(home, "Documents", "nb")
			}

			// Expand ~ if present
			if len(vault) > 0 && vault[0] == '~' {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				vault = filepath.Join(home, vault[1:])
			}

			// Check if vault exists
			if _, err := os.Stat(vault); os.IsNotExist(err) {
				return fmt.Errorf("vault directory does not exist: %s", vault)
			}

			// Create .obsidian/plugins directory if it doesn't exist
			pluginsDir := filepath.Join(vault, ".obsidian", "plugins")
			if err := os.MkdirAll(pluginsDir, 0755); err != nil {
				return fmt.Errorf("failed to create plugins directory: %w", err)
			}

			// Get the source plugin directory
			// First, try to find it relative to current working directory
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}

			// Look for obsidian-plugin in common locations
			possiblePaths := []string{
				filepath.Join(cwd, "obsidian-plugin"),
				filepath.Join(cwd, "nb-prototype", "obsidian-plugin"),
				filepath.Join(filepath.Dir(cwd), "obsidian-plugin"),
				"/Users/msolomon/code/random/note-system/nb-prototype/obsidian-plugin",
			}

			var pluginSource string
			for _, path := range possiblePaths {
				if _, err := os.Stat(path); err == nil {
					pluginSource = path
					break
				}
			}

			// Check if source was found
			if pluginSource == "" {
				return fmt.Errorf("obsidian plugin source not found. Searched in: %v", possiblePaths)
			}

			// Destination path
			pluginDest := filepath.Join(pluginsDir, "nb-integration")

			// Remove existing symlink or directory if it exists
			if _, err := os.Lstat(pluginDest); err == nil {
				if err := os.Remove(pluginDest); err != nil {
					// If remove fails, it might be a directory
					if err := os.RemoveAll(pluginDest); err != nil {
						return fmt.Errorf("failed to remove existing plugin: %w", err)
					}
				}
			}

			// Create symlink
			if err := os.Symlink(pluginSource, pluginDest); err != nil {
				return fmt.Errorf("failed to create symlink: %w", err)
			}

			fmt.Printf("* Created development symlink for nb Obsidian plugin\n")
			fmt.Printf("  Source: %s\n", pluginSource)
			fmt.Printf("  Target: %s\n", pluginDest)
			fmt.Printf("\nDevelopment setup:\n")
			fmt.Printf("1. cd %s\n", pluginSource)
			fmt.Printf("2. npm install (if not done already)\n")
			fmt.Printf("3. npm run dev (for watch mode) or npm run build\n")
			fmt.Printf("4. Reload Obsidian (Cmd+R) to see changes\n")
			fmt.Printf("5. Enable 'NB Integration' in Obsidian Community Plugins settings\n")
			fmt.Printf("\nNote: This is a development setup using symlinks.\n")
			fmt.Printf("Changes to the source will be reflected after reloading Obsidian.\n")

			return nil
		},
	}

	cmd.Flags().StringVar(&vaultPath, "vault", "", "Path to Obsidian vault (defaults to ~/Documents/nb)")

	return cmd
}