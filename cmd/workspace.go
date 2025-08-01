package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattsolo1/grove-notebook/cmd/config"
	"github.com/mattsolo1/grove-notebook/pkg/workspace"
)

func NewWorkspaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Manage workspaces",
		Long:  `Manage workspace registrations and settings.`,
	}

	cmd.AddCommand(
		newWorkspaceAddCmd(),
		newWorkspaceListCmd(),
		newWorkspaceRemoveCmd(),
		newWorkspaceCurrentCmd(),
		doctorCmd,
	)

	return cmd
}

func newWorkspaceAddCmd() *cobra.Command {
	var (
		wsName     string
		wsType     string
		wsNotebook string
	)

	cmd := &cobra.Command{
		Use:   "add [path]",
		Short: "Add a new workspace",
		Long: `Register a new workspace. If no path is provided, uses current directory.
	
Examples:
  nb workspace add                     # Register current directory
  nb workspace add ~/projects/myapp    # Register specific path
  nb workspace add . --name myproject  # Register with custom name`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize config and service
			config.InitConfig()
			svc, err := config.InitService()
			if err != nil {
				return err
			}
			defer svc.Close()

			path := "."
			if len(args) > 0 {
				path = args[0]
			}

			absPath, err := filepath.Abs(path)
			if err != nil {
				return err
			}

			// Determine name
			name := wsName
			if name == "" {
				name = filepath.Base(absPath)
			}

			// Determine notebook directory
			notebook := wsNotebook
			if notebook == "" {
				home, _ := os.UserHomeDir()
				notebook = filepath.Join(home, "Documents", "nb")
			}

			ws := &workspace.Workspace{
				Name:        name,
				Path:        absPath,
				Type:        workspace.Type(wsType),
				NotebookDir: notebook,
				Settings:    map[string]any{},
			}

			if err := svc.Registry.Add(ws); err != nil {
				return err
			}

			fmt.Printf("Added workspace '%s' at %s\n", name, absPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&wsName, "name", "", "Workspace name (defaults to directory name)")
	cmd.Flags().StringVar(&wsType, "type", "git-repo", "Workspace type (git-repo, directory, global)")
	cmd.Flags().StringVar(&wsNotebook, "notebook", "", "Notebook directory (defaults to ~/Documents/nb)")

	// Add global flags
	config.AddGlobalFlags(cmd)

	return cmd
}

func newWorkspaceListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List all workspaces",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize config and service
			config.InitConfig()
			svc, err := config.InitService()
			if err != nil {
				return err
			}
			defer svc.Close()

			workspaces, err := svc.Registry.List()
			if err != nil {
				return err
			}

			if len(workspaces) == 0 {
				fmt.Println("No workspaces registered")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tTYPE\tPATH\tLAST USED")

			for _, ws := range workspaces {
				active := ""
				if ws.IsActive() {
					active = " *"
				}
				fmt.Fprintf(w, "%s%s\t%s\t%s\t%s\n",
					ws.Name, active, ws.Type, ws.Path,
					ws.LastUsed.Format("2006-01-02 15:04"))
			}

			return w.Flush()
		},
	}

	// Add global flags
	config.AddGlobalFlags(cmd)

	return cmd
}

func newWorkspaceRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <name>",
		Short:   "Remove a workspace",
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize config and service
			config.InitConfig()
			svc, err := config.InitService()
			if err != nil {
				return err
			}
			defer svc.Close()

			name := args[0]

			if err := svc.Registry.Remove(name); err != nil {
				return err
			}

			fmt.Printf("Removed workspace '%s'\n", name)
			return nil
		},
	}

	// Add global flags
	config.AddGlobalFlags(cmd)

	return cmd
}

func newWorkspaceCurrentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "current",
		Short: "Show current workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize config and service
			config.InitConfig()
			svc, err := config.InitService()
			if err != nil {
				return err
			}
			defer svc.Close()

			ws, err := svc.Registry.DetectCurrent()
			if err != nil {
				return err
			}

			fmt.Printf("Current workspace: %s\n", ws.Name)
			fmt.Printf("Type: %s\n", ws.Type)
			fmt.Printf("Path: %s\n", ws.Path)
			fmt.Printf("Notebook: %s\n", ws.NotebookDir)

			return nil
		},
	}

	// Add global flags
	config.AddGlobalFlags(cmd)

	return cmd
}