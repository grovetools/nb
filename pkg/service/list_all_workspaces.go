package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/workspace"
)

// ListNotesFromAllWorkspaces returns notes from all registered workspaces
func (s *Service) ListNotesFromAllWorkspaces() ([]*models.Note, error) {
	allNotes := []*models.Note{}

	// Get all workspaces
	workspaces, err := s.Registry.List()
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}

	// Iterate through each workspace
	for _, ws := range workspaces {
		var branchContexts []string

		if ws.Type == workspace.TypeGitRepo {
			// For git repos, we need to find all branches that have note directories
			repoNoteDir := filepath.Join(ws.NotebookDir, "repos", ws.Name)
			dirs, err := os.ReadDir(repoNoteDir)
			if err != nil {
				// If the repo dir doesn't exist, just skip
				continue
			}
			for _, dir := range dirs {
				if dir.IsDir() {
					branchContexts = append(branchContexts, dir.Name())
				}
			}
		} else {
			// For non-git or global workspaces, there's only one context (no branches)
			branchContexts = append(branchContexts, "")
		}

		for _, branch := range branchContexts {
			// Create a context for each workspace/branch combo
			ctx := &WorkspaceContext{
				Workspace: ws,
				Branch:    branch,
				Paths:     make(map[string]string),
			}

			// Get all notes for this context
			notes, err := s.ListAllNotes(ctx)
			if err != nil {
				// Log error but continue with other workspaces
				fmt.Fprintf(os.Stderr, "Warning: could not list notes for %s/%s: %v\n", ws.Name, branch, err)
				continue
			}

			allNotes = append(allNotes, notes...)
		}
	}

	return allNotes, nil
}
