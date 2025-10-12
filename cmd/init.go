package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/mattsolo1/grove-notebook/cmd/config"
)

func NewInitCmd() *cobra.Command {
	var initMinimal bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize nb in current directory",
		Long: `Initialize nb in the current directory by registering it as a workspace.
	
This command will:
- Register the current directory as a workspace
- Create necessary directory structure`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize config and service
			config.InitConfig()
			svc, err := config.InitService()
			if err != nil {
				return err
			}
			defer svc.Close()

			if initMinimal {
				// Verify global workspace context can be accessed
				ctx, err := svc.GetWorkspaceContext("global")
				if err != nil {
					return fmt.Errorf("could not access global workspace: %w", err)
				}
				fmt.Printf("Initialized with global workspace at %s\n", ctx.NotebookContextWorkspace.Path)
				return nil
			}

			// Verify current directory is recognized as a workspace
			ctx, err := svc.GetWorkspaceContext("")
			if err != nil {
				return fmt.Errorf("could not determine workspace context: %w", err)
			}

			fmt.Printf("Initialized workspace '%s' at %s\n", ctx.NotebookContextWorkspace.Name, ctx.NotebookContextWorkspace.Path)
			fmt.Printf("Kind: %s\n", ctx.NotebookContextWorkspace.Kind)
			if ctx.Branch != "" {
				fmt.Printf("Branch: %s\n", ctx.Branch)
			}
			fmt.Println("\nReady to use! Try 'nb new' to create your first note.")

			return nil
		},
	}

	cmd.Flags().BoolVar(&initMinimal, "minimal", false, "Only create global workspace")
	
	// Add global flags
	config.AddGlobalFlags(cmd)

	return cmd
}