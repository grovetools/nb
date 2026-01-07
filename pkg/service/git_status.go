package service

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/util/pathutil"
)

// GitStatusResult contains the parsed git status information
type GitStatusResult struct {
	FileStatus   map[string]string // path -> status code
	DeletedFiles []string          // paths of deleted files (don't exist on disk)
}

// GetFileStatus runs `git status --porcelain=v1` and returns a map of absolute,
// normalized file paths to their status codes.
func GetFileStatus(repoPath string) (map[string]string, error) {
	result, err := GetFileStatusExtended(repoPath)
	if err != nil {
		return nil, err
	}
	return result.FileStatus, nil
}

// GetFileStatusExtended runs `git status --porcelain=v1` and returns extended info
// including deleted files that don't exist on disk.
func GetFileStatusExtended(repoPath string) (*GitStatusResult, error) {
	cmd := exec.Command("git", "status", "--porcelain=v1")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w\n%s", err, string(output))
	}

	result := &GitStatusResult{
		FileStatus:   make(map[string]string),
		DeletedFiles: []string{},
	}
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		statusCode := line[:2]
		filePath := strings.TrimSpace(line[3:])

		// Handle renamed files: "R  old -> new" or "RM old -> new"
		if statusCode[0] == 'R' {
			if idx := strings.Index(filePath, " -> "); idx != -1 {
				// Use the NEW path for renamed files
				filePath = filePath[idx+4:]
			}
		}

		absPath := filepath.Join(repoPath, filePath)
		normalizedPath, err := pathutil.NormalizeForLookup(absPath)
		if err != nil {
			continue
		}

		result.FileStatus[normalizedPath] = statusCode

		// Track deleted files separately (staged or unstaged delete)
		if statusCode[0] == 'D' || statusCode[1] == 'D' {
			result.DeletedFiles = append(result.DeletedFiles, normalizedPath)
		}
	}

	return result, nil
}
