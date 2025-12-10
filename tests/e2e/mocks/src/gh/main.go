package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// main is a mock of the 'gh' CLI for testing 'nb sync'.
// It reads the GH_MOCK_STATE_DIR environment variable to locate JSON data files.
// Based on the command-line arguments (e.g., "issue list"), it prints the
// content of the corresponding JSON file (issues.json or prs.json) to stdout.
func main() {
	stateDir := os.Getenv("GH_MOCK_STATE_DIR")
	if stateDir == "" {
		fmt.Fprintln(os.Stderr, "mock gh: GH_MOCK_STATE_DIR not set")
		os.Exit(1)
	}

	args := os.Args[1:]
	var jsonFile string

	if len(args) > 1 && args[0] == "issue" && args[1] == "list" {
		jsonFile = "issues.json"
	} else if len(args) > 1 && (args[0] == "pr" || args[0] == "pull-request") && args[1] == "list" {
		jsonFile = "prs.json"
	} else {
		fmt.Fprintf(os.Stderr, "mock gh: unhandled command %v\n", args)
		os.Exit(1)
	}

	jsonPath := filepath.Join(stateDir, jsonFile)
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mock gh: failed to read %s: %v\n", jsonPath, err)
		os.Exit(1)
	}

	fmt.Println(string(data))
}
