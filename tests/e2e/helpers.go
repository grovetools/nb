package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// findProjectBinary locates the nb binary in the bin directory.
// This is needed for TUI tests which require the binary path directly.
func findProjectBinary() (string, error) {
	// The binary should be at ../../bin/nb relative to the test directory
	binPath := filepath.Join("..", "..", "bin", "nb")
	absPath, err := filepath.Abs(binPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	if _, err := os.Stat(absPath); err != nil {
		return "", fmt.Errorf("nb binary not found at %s: %w", absPath, err)
	}

	return absPath, nil
}
