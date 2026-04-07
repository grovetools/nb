package browser

import (
	"os"
	"path/filepath"
	"regexp"
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

// sanitizeForFilename converts a string into a valid, URL-friendly slug.
func sanitizeForFilename(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")

	// Remove invalid characters using a regex
	reg, _ := regexp.Compile("[^a-z0-9-]+")
	s = reg.ReplaceAllString(s, "")

	// Remove leading/trailing hyphens
	s = strings.Trim(s, "-")

	// Limit length to avoid overly long names
	if len(s) > 50 {
		s = s[:50]
	}
	// ensure it's not empty
	if s == "" {
		s = "plan"
	}

	return s
}
