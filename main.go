package main

import (
	"os"
	"github.com/mattsolo1/grove-core/cli"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := cli.NewStandardCommand(
		"nb",
		"Notebook and documentation management",
	)

	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		cmd.Println("TODO: Implement nb")
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
