package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

var (
	cfgFile           string
	WorkspaceOverride string
)

func InitConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		configDir := filepath.Join(home, ".config", "nb")
		viper.AddConfigPath(configDir)
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("NB")

	// Set defaults
	viper.SetDefault("data_dir", filepath.Join(os.Getenv("HOME"), ".local", "share", "nb"))
	viper.SetDefault("editor", os.Getenv("EDITOR"))
	viper.SetDefault("default_type", "current")

	if err := viper.ReadInConfig(); err == nil {
		// Do not print this in normal operation, it's noisy.
		// fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}

func InitService() (*service.Service, error) {
	dataDir := viper.GetString("data_dir")

	config := &service.Config{
		DataDir:     dataDir,
		Editor:      viper.GetString("editor"),
		DefaultType: models.NoteType(viper.GetString("default_type")),
		Templates:   viper.GetStringMapString("templates"),
	}

	// Discover all workspaces once and create a provider.
	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	logger.SetLevel(logrus.WarnLevel) // Keep it quiet unless there are issues.
	discoveryService := workspace.NewDiscoveryService(logger)
	result, err := discoveryService.DiscoverAll()
	if err != nil {
		return nil, fmt.Errorf("failed to discover workspaces: %w", err)
	}
	provider := workspace.NewProvider(result)

	svc, err := service.New(config, provider)
	if err != nil {
		return nil, err
	}

	return svc, nil
}

func AddGlobalFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/nb/config.yaml)")
	cmd.PersistentFlags().StringVarP(&WorkspaceOverride, "workspace", "W", "", "Override current workspace context by path")
}
