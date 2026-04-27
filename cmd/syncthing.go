package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/grovetools/nb/pkg/service"
	"github.com/grovetools/nb/pkg/syncthing"
)

// NewSyncthingCmd creates the `nb syncthing` command and its subcommands.
func NewSyncthingCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "syncthing",
		Short: "Manage Syncthing integration",
		Long:  `Provides commands for automatically configuring Syncthing for notebook directories.`,
	}

	cmd.AddCommand(newSyncthingSetupCmd(svc, workspaceOverride))
	return cmd
}

func newSyncthingSetupCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var notebookLevel bool

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Setup Syncthing for the current workspace or entire notebook",
		Long: `Registers the current workspace (or entire notebook) with the local Syncthing daemon
and shares it with the devices defined in your grove.yml [notebooks.definitions.main.syncthing] configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := *svc

			client := syncthing.NewClient()
			if !client.IsInstalled() {
				return fmt.Errorf("syncthing cli not found in PATH; please install syncthing first")
			}

			// 1. Resolve Target Directory (mimics nb git init logic)
			wsCtx, err := s.GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("getting workspace context: %w", err)
			}

			locator := s.GetNotebookLocator()
			samplePath, err := locator.GetNotesDir(wsCtx.NotebookContextWorkspace, "inbox")
			if err != nil {
				return fmt.Errorf("could not resolve notebook path: %w", err)
			}

			// Target the workspace directory (one level up from inbox)
			targetDir := filepath.Dir(samplePath)

			if notebookLevel {
				// Go up to the notebook root. Structure: <root>/workspaces/<name>/
				parent := filepath.Dir(targetDir)
				if filepath.Base(parent) == "workspaces" {
					targetDir = filepath.Dir(parent)
				} else {
					targetDir = parent
				}
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Configuring Syncthing for directory: %s\n", targetDir)

			// 2. Fetch configured devices from grove config
			// Use the notebook associated with this workspace, not the default
			var devices []string
			nbName := wsCtx.NotebookContextWorkspace.NotebookName
			if nbName == "" {
				// Fall back to default if not set
				if s.CoreConfig.Notebooks.Rules != nil && s.CoreConfig.Notebooks.Rules.Default != "" {
					nbName = s.CoreConfig.Notebooks.Rules.Default
				} else {
					nbName = "main"
				}
			}

			nbDef, ok := s.CoreConfig.Notebooks.Definitions[nbName]
			if !ok || nbDef.Syncthing == nil {
				return fmt.Errorf("no syncthing config found in grove.yml under notebooks.definitions.%s.syncthing", nbName)
			}

			devices = nbDef.Syncthing.Devices
			if len(devices) == 0 {
				return fmt.Errorf("no syncthing devices configured in grove.yml under notebooks.definitions.%s.syncthing.devices", nbName)
			}

			// 3. Generate folder ID - use custom title if provided, otherwise default
			var folderID string
			if nbDef.Syncthing.FolderTitle != "" {
				if notebookLevel {
					folderID = nbDef.Syncthing.FolderTitle
				} else {
					folderID = fmt.Sprintf("%s-%s", nbDef.Syncthing.FolderTitle, wsCtx.NotebookContextWorkspace.Name)
				}
			} else {
				if notebookLevel {
					folderID = fmt.Sprintf("grove-%s", nbName)
				} else {
					folderID = fmt.Sprintf("grove-%s-%s", nbName, wsCtx.NotebookContextWorkspace.Name)
				}
			}

			// 4. Register Folder
			if err := client.AddFolder(folderID, targetDir); err != nil {
				return fmt.Errorf("failed to add folder to syncthing: %w", err)
			}
			fmt.Fprintf(out, "* Registered Syncthing folder: %s\n", folderID)

			// 5. Share with Devices
			for _, device := range devices {
				if err := client.ShareFolderWithDevice(folderID, device); err != nil {
					fmt.Fprintf(out, "Warning: failed to share with device %s: %v\n", device, err)
				} else {
					fmt.Fprintf(out, "* Shared folder with device: %s\n", device)
				}
			}

			fmt.Fprintln(out, "\nSuccess! Syncthing is now configured for this location.")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&notebookLevel, "notebook", "n", false, "Setup Syncthing for the entire notebook root rather than just the current workspace")
	return cmd
}
