package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/service"
	"github.com/mattsolo1/grove-notebook/pkg/workspace"
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
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
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

	svc, err := service.New(config)
	if err != nil {
		return nil, err
	}

	// If workspace override is specified, change to that workspace
	if WorkspaceOverride != "" {
		workspaces, err := svc.Registry.List()
		if err != nil {
			svc.Close()
			return nil, fmt.Errorf("list workspaces: %w", err)
		}

		var targetWorkspace *workspace.Workspace
		for _, ws := range workspaces {
			if ws.Name == WorkspaceOverride {
				targetWorkspace = ws
				break
			}
		}

		if targetWorkspace == nil {
			svc.Close()
			return nil, fmt.Errorf("workspace not found: %s", WorkspaceOverride)
		}

		// Change to the workspace directory
		if err := os.Chdir(targetWorkspace.Path); err != nil {
			svc.Close()
			return nil, fmt.Errorf("change to workspace directory: %w", err)
		}
	}

	return svc, nil
}

func AddGlobalFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/nb/config.yaml)")
	cmd.PersistentFlags().StringVarP(&WorkspaceOverride, "workspace", "W", "", "Override current workspace context")
}