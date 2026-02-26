package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
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
	cmd.AddCommand(newObsidianSetupCmd(svc, workspaceOverride))

	return cmd
}

func newObsidianSetupCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var notebookLevel bool

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Initialize an Obsidian vault",
		Long: `Initializes the current workspace (or entire notebook root) as an Obsidian vault.
It creates the .obsidian directory and sets up default preferences (like strict line breaks).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := *svc

			// 1. Resolve Target Directory
			wsCtx, err := s.GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("getting workspace context: %w", err)
			}

			locator := s.GetNotebookLocator()
			samplePath, err := locator.GetNotesDir(wsCtx.NotebookContextWorkspace, "inbox")
			if err != nil {
				return fmt.Errorf("could not resolve notebook path: %w", err)
			}

			// Target the workspace directory
			targetDir := filepath.Dir(samplePath)

			if notebookLevel {
				parent := filepath.Dir(targetDir)
				if filepath.Base(parent) == "workspaces" {
					targetDir = filepath.Dir(parent)
				} else {
					targetDir = parent
				}
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Initializing Obsidian vault at: %s\n", targetDir)

			// 2. Get notebook config
			nbName := wsCtx.NotebookContextWorkspace.NotebookName
			if nbName == "" {
				if s.CoreConfig.Notebooks.Rules != nil && s.CoreConfig.Notebooks.Rules.Default != "" {
					nbName = s.CoreConfig.Notebooks.Rules.Default
				} else {
					nbName = "main"
				}
			}

			obsidianDir := filepath.Join(targetDir, ".obsidian")

			// 3. Check if template_repo is configured
			var templateRepo string
			if nbDef, ok := s.CoreConfig.Notebooks.Definitions[nbName]; ok && nbDef.Obsidian != nil {
				templateRepo = nbDef.Obsidian.TemplateRepo
			}

			if templateRepo != "" {
				// Clone template repo and copy .obsidian contents
				fmt.Fprintf(out, "* Using template from: %s\n", templateRepo)

				// Create temp directory
				tempDir, err := os.MkdirTemp("", "obsidian-template-")
				if err != nil {
					return fmt.Errorf("failed to create temp directory: %w", err)
				}
				defer os.RemoveAll(tempDir)

				// Clone the repo
				cloneCmd := exec.Command("git", "clone", "--depth", "1", templateRepo, tempDir)
				cloneCmd.Stdout = io.Discard
				cloneCmd.Stderr = out
				if err := cloneCmd.Run(); err != nil {
					return fmt.Errorf("failed to clone template repo: %w", err)
				}

				// Copy .obsidian contents (or root if no .obsidian dir)
				sourceDir := filepath.Join(tempDir, ".obsidian")
				if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
					// No .obsidian subdir, use repo root (excluding .git)
					sourceDir = tempDir
				}

				// Create target .obsidian directory
				if err := os.MkdirAll(obsidianDir, 0755); err != nil {
					return fmt.Errorf("failed to create .obsidian directory: %w", err)
				}

				// Copy files from template
				err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}

					// Skip .git directory
					if info.IsDir() && info.Name() == ".git" {
						return filepath.SkipDir
					}

					relPath, err := filepath.Rel(sourceDir, path)
					if err != nil {
						return err
					}

					destPath := filepath.Join(obsidianDir, relPath)

					if info.IsDir() {
						return os.MkdirAll(destPath, info.Mode())
					}

					// Copy file
					srcFile, err := os.Open(path)
					if err != nil {
						return err
					}
					defer srcFile.Close()

					destFile, err := os.Create(destPath)
					if err != nil {
						return err
					}
					defer destFile.Close()

					if _, err := io.Copy(destFile, srcFile); err != nil {
						return err
					}

					return os.Chmod(destPath, info.Mode())
				})
				if err != nil {
					return fmt.Errorf("failed to copy template files: %w", err)
				}

				fmt.Fprintln(out, "* Copied template configuration")
			} else {
				// No template - create basic .obsidian with default app.json
				if err := os.MkdirAll(obsidianDir, 0755); err != nil {
					return fmt.Errorf("failed to create .obsidian directory: %w", err)
				}

				appJsonPath := filepath.Join(obsidianDir, "app.json")
				appJsonContent := `{
  "strictLineBreaks": true,
  "attachmentFolderPath": ".attachments",
  "newFileLocation": "current"
}`
				// Only write if it doesn't exist so we don't overwrite user settings
				if _, err := os.Stat(appJsonPath); os.IsNotExist(err) {
					if err := os.WriteFile(appJsonPath, []byte(appJsonContent), 0644); err != nil {
						return fmt.Errorf("failed to write app.json: %w", err)
					}
					fmt.Fprintln(out, "* Created default app.json")
				}
			}

			// 4. Handle AutoLinkPlugin if configured
			if nbDef, ok := s.CoreConfig.Notebooks.Definitions[nbName]; ok && nbDef.Obsidian != nil {
				if nbDef.Obsidian.AutoLinkPlugin {
					fmt.Fprintln(out, "Note: auto_link_plugin is enabled in config, but plugin linking must currently be done manually using 'nb obsidian install-dev'.")
				}
			}

			fmt.Fprintln(out, "\nSuccess! You can now open this folder as a vault in Obsidian.")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&notebookLevel, "notebook", "n", false, "Initialize vault at the entire notebook root rather than just the current workspace")
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