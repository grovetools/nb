package workspace

import (
	"os"
	"path/filepath"

	"github.com/mattsolo1/grove-core/config"
)

// NbConfig represents the 'nb' section in grove.yml
type NbConfig struct {
	NotebookDir string `yaml:"notebook_dir"`
}

// GetDefaultNotebookDir returns the default notebook directory
// Reads from grove.yml nb.notebook_dir, otherwise falls back to ~/.local/share/nb
func GetDefaultNotebookDir() string {
	// Load grove config using grove-core
	cfg, err := config.LoadDefault()
	if err != nil {
		// Fallback if no config found
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "nb")
	}

	// Use UnmarshalExtension to properly decode the nb config
	var nbConfig NbConfig
	err = cfg.UnmarshalExtension("nb", &nbConfig)
	if err == nil && nbConfig.NotebookDir != "" {
		nbDir := nbConfig.NotebookDir
		// Expand ~ in path
		if len(nbDir) >= 2 && nbDir[:2] == "~/" {
			home, _ := os.UserHomeDir()
			return filepath.Join(home, nbDir[2:])
		}
		return nbDir
	}

	// Fallback to XDG default
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "nb")
}
