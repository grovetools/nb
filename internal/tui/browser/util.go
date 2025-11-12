package browser

import (
	"os"
	"path/filepath"
	"strings"
)

// shortenPath replaces the home directory prefix with a tilde (~).
func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path // Fallback to original path on error
	}

	if strings.HasPrefix(path, home) {
		return filepath.Join("~", strings.TrimPrefix(path, home))
	}

	return path
}
