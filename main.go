package main

import (
	"fmt"
	"os"

	"github.com/mattsolo1/grove-core/cli"
	coreconfig "github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-notebook/cmd"
	"github.com/mattsolo1/grove-notebook/pkg/service"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	svc               *service.Service
	workspaceOverride string
)

func main() {
	rootCmd := cli.NewStandardCommand(
		"nb",
		"A workspace-based note-taking system",
	)
	rootCmd.PersistentFlags().StringVarP(&workspaceOverride, "workspace", "W", "", "Override current workspace context by path")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// This runs once before any subcommand
		logger := logging.NewLogger("nb")

		// 1. Load configuration using grove-core
		cfg, err := coreconfig.LoadDefault()
		if err != nil {
			// Non-fatal, proceed with an empty config for local mode.
			cfg = &coreconfig.Config{}
			logger.Debugf("could not load grove config, proceeding in local mode: %v", err)
		}

		// 2. Initialize workspace provider
		// Use a muted logger for discovery to avoid noise
		discoveryLogger := logrus.New()
		discoveryLogger.SetOutput(os.Stderr)
		discoveryLogger.SetLevel(logrus.WarnLevel)
		discoveryService := workspace.NewDiscoveryService(discoveryLogger)
		result, err := discoveryService.DiscoverAll()
		if err != nil {
			return fmt.Errorf("failed to discover workspaces: %w", err)
		}
		provider := workspace.NewProvider(result)

		// 3. Initialize the main service
		// In the future, service.Config can be derived from the core config extensions.
		serviceCfg := &service.Config{
			Editor: os.Getenv("EDITOR"), // A common way to get editor
		}
		svc, err = service.New(serviceCfg, provider, cfg, logger)
		if err != nil {
			return fmt.Errorf("failed to initialize service: %w", err)
		}
		return nil
	}

	// Add subcommands
	rootCmd.AddCommand(cmd.NewNewCmd(&svc, &workspaceOverride))
	rootCmd.AddCommand(cmd.NewQuickCmd(&svc, &workspaceOverride))
	rootCmd.AddCommand(cmd.NewWorkspaceCmd(&svc, &workspaceOverride))
	rootCmd.AddCommand(cmd.NewSearchCmd(&svc, &workspaceOverride))
	rootCmd.AddCommand(cmd.NewListCmd(&svc, &workspaceOverride))
	rootCmd.AddCommand(cmd.NewArchiveCmd(&svc, &workspaceOverride))
	rootCmd.AddCommand(cmd.NewContextCmd(&svc, &workspaceOverride))
	rootCmd.AddCommand(cmd.NewInitCmd(&svc, &workspaceOverride))
	rootCmd.AddCommand(cmd.NewMigrateCmd(&svc, &workspaceOverride))
	rootCmd.AddCommand(cmd.NewMoveCmd(&svc, &workspaceOverride))
	rootCmd.AddCommand(cmd.NewObsidianCmd(&svc, &workspaceOverride))
	rootCmd.AddCommand(cmd.NewVersionCmd())
	rootCmd.AddCommand(cmd.NewTuiCmd(&svc, &workspaceOverride))
	rootCmd.AddCommand(cmd.NewInternalCmd(&svc))
	rootCmd.AddCommand(cmd.NewTmuxCmd(&svc, &workspaceOverride))
	rootCmd.AddCommand(cmd.NewRemoteCmd(&svc, &workspaceOverride))
	rootCmd.AddCommand(cmd.NewGitCmd(&svc, &workspaceOverride))
	rootCmd.AddCommand(cmd.NewConceptCmd(&svc, &workspaceOverride))

	if err := cli.Execute(rootCmd); err != nil {
		os.Exit(1)
	}
}