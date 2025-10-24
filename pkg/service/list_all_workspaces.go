package service

import (
	"fmt"
	"os"

	"github.com/mattsolo1/grove-notebook/pkg/models"
)

// ListNotesFromAllWorkspaces returns notes from all registered workspaces
func (s *Service) ListNotesFromAllWorkspaces() ([]*models.Note, error) {
	allNotes := []*models.Note{}
	allWorkspaces := s.workspaceProvider.All()

	// Use a map to avoid processing the same notebook context twice (for worktrees)
	seenContexts := make(map[string]bool)

	for _, ws := range allWorkspaces {
		contextNode, err := s.findNotebookContextNode(ws)
		if err != nil {
			continue // skip if we can't find context
		}
		if seenContexts[contextNode.Path] {
			continue
		}
		seenContexts[contextNode.Path] = true

		// create a dummy context for ListAllNotes
		wsCtx := &WorkspaceContext{
			CurrentWorkspace:         ws,
			NotebookContextWorkspace: contextNode,
		}

		notes, err := s.ListAllNotes(wsCtx)
		if err != nil {
			// don't fail, just log and continue
			fmt.Fprintf(os.Stderr, "Warning: could not list notes for workspace %s: %v\n", ws.Name, err)
			continue
		}
		allNotes = append(allNotes, notes...)
	}
	return allNotes, nil
}
