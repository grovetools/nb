package sync

import (
	"fmt"

	coreconfig "github.com/mattsolo1/grove-core/config"
	"github.com/mitchellh/mapstructure"
)

// SyncConfig holds the configuration for a single sync provider for a notebook.
type SyncConfig struct {
	Provider   string `mapstructure:"provider"`
	IssuesType string `mapstructure:"issues_type"`
	PRsType    string `mapstructure:"prs_type"`
}

// GetSyncConfigForNotebook extracts the sync configurations for a specific notebook
// from the global grove.yml configuration.
func GetSyncConfigForNotebook(cfg *coreconfig.Config, notebookName string) ([]SyncConfig, error) {
	if cfg == nil || cfg.Notebooks == nil || cfg.Notebooks.Definitions == nil {
		return []SyncConfig{}, nil // No config, no syncs
	}

	notebookDef, ok := cfg.Notebooks.Definitions[notebookName]
	if !ok || notebookDef == nil || notebookDef.Sync == nil {
		return []SyncConfig{}, nil // Notebook not defined or no sync config
	}

	var syncConfigs []SyncConfig
	syncs, ok := notebookDef.Sync.([]interface{})
	if !ok {
		return nil, fmt.Errorf("sync config for notebook '%s' is not a list", notebookName)
	}

	for i, s := range syncs {
		syncMap, ok := s.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("sync config entry %d for notebook '%s' is not a map", i, notebookName)
		}

		var syncConf SyncConfig
		if err := mapstructure.Decode(syncMap, &syncConf); err != nil {
			return nil, fmt.Errorf("failed to decode sync config entry %d: %w", i, err)
		}

		if syncConf.Provider == "" {
			return nil, fmt.Errorf("sync config entry %d missing 'provider' field", i)
		}
		syncConfigs = append(syncConfigs, syncConf)
	}

	return syncConfigs, nil
}
