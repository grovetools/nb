package service

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/util/pathutil"
)

// GetFileStatus runs `git status --porcelain=v1` and returns a map of absolute,
// normalized file paths to their status codes.
func GetFileStatus(repoPath string) (map[string]string, error) {
	cmd := exec.Command("git", "status", "--porcelain=v1")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w\n%s", err, string(output))
	}

	statusMap := make(map[string]string)
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		statusCode := line[:2]
		filePath := strings.TrimSpace(line[3:])
		absPath := filepath.Join(repoPath, filePath)

		normalizedPath, err := pathutil.NormalizeForLookup(absPath)
		if err == nil {
			statusMap[normalizedPath] = statusCode
		}
	}

	return statusMap, nil
}
