package main

import (
	"os"
	"github.com/mattsolo1/grove-core/cli"
	"github.com/mattsolo1/grove-notebook/cmd"
)

func main() {
	rootCmd := cli.NewStandardCommand(
		"nb",
		"A workspace-based note-taking system",
	)
	
	// Add subcommands
	rootCmd.AddCommand(cmd.NewNewCmd())
	rootCmd.AddCommand(cmd.NewQuickCmd())
	rootCmd.AddCommand(cmd.NewWorkspaceCmd())
	rootCmd.AddCommand(cmd.NewSearchCmd())
	rootCmd.AddCommand(cmd.NewListCmd())
	rootCmd.AddCommand(cmd.NewArchiveCmd())
	rootCmd.AddCommand(cmd.NewContextCmd())
	rootCmd.AddCommand(cmd.NewInitCmd())
	rootCmd.AddCommand(cmd.NewMigrateCmd())
	rootCmd.AddCommand(cmd.NewMoveCmd())
	rootCmd.AddCommand(cmd.NewObsidianCmd())
	rootCmd.AddCommand(cmd.NewVersionCmd())
	rootCmd.AddCommand(cmd.NewTuiCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}