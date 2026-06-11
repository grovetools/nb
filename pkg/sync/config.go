package sync

import (
	"fmt"

	coreconfig "github.com/grovetools/core/config"
)

// SyncConfig holds the configuration for a single sync provider for a notebook.
type SyncConfig struct {
	Provider   string
	IssuesType string
	PRsType    string
}

// GetSyncConfigForNotebook extracts the sync provider configurations for a
// specific notebook from the global grove config. The legacy list shape of
// the `sync` key decodes into SyncConfig.Providers in core.
func GetSyncConfigForNotebook(cfg *coreconfig.Config, notebookName string) ([]SyncConfig, error) {
	if cfg == nil || cfg.Notebooks == nil || cfg.Notebooks.Definitions == nil {
		return []SyncConfig{}, nil // No config, no syncs
	}

	notebookDef, ok := cfg.Notebooks.Definitions[notebookName]
	if !ok || notebookDef == nil || notebookDef.Sync == nil {
		return []SyncConfig{}, nil // Notebook not defined or no sync config
	}

	var syncConfigs []SyncConfig
	for i, p := range notebookDef.Sync.Providers {
		if p.Provider == "" {
			return nil, fmt.Errorf("sync config entry %d missing 'provider' field", i)
		}
		syncConfigs = append(syncConfigs, SyncConfig{
			Provider:   p.Provider,
			IssuesType: p.IssuesType,
			PRsType:    p.PRsType,
		})
	}

	return syncConfigs, nil
}
