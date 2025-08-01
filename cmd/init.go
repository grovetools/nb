package cmd

import (
	"fmt"
	"os"

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
				// Just ensure global workspace exists
				_, err := svc.Registry.Global()
				if err != nil {
					return err
				}
				fmt.Println("Initialized with global workspace")
				return nil
			}

			// Register current directory
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			// Auto-register current directory
			ws, err := svc.Registry.AutoRegister(cwd)
			if err != nil {
				return err
			}

			fmt.Printf("Initialized workspace '%s' at %s\n", ws.Name, ws.Path)
			fmt.Println("\nReady to use! Try 'nb new' to create your first note.")

			return nil
		},
	}

	cmd.Flags().BoolVar(&initMinimal, "minimal", false, "Only create global workspace")
	
	// Add global flags
	config.AddGlobalFlags(cmd)

	return cmd
}