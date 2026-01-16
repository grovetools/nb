package service

import (
	"fmt"
	"os"

	"github.com/grovetools/core/util/pathutil"
	"github.com/grovetools/nb/pkg/models"
	"github.com/grovetools/nb/pkg/tree"
)

// ListNotesFromAllWorkspaces returns notes from all registered workspaces
func (s *Service) ListNotesFromAllWorkspaces(includeArchived bool, includeArtifacts bool) ([]*models.Note, error) {
	allNotes := []*models.Note{}
	allWorkspaces := s.workspaceProvider.All()

	// Use a map to avoid processing the same notebook context twice (for worktrees)
	seenContexts := make(map[string]bool)

	// Use a map to deduplicate notes by canonical path across all workspaces
	seenNotePaths := make(map[string]bool)

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

		notes, err := s.ListAllNotes(wsCtx, includeArchived, includeArtifacts)
		if err != nil {
			// don't fail, just log and continue
			fmt.Fprintf(os.Stderr, "Warning: could not list notes for workspace %s: %v\n", ws.Name, err)
			continue
		}

		// Deduplicate notes across workspaces by canonical path
		for _, note := range notes {
			canonicalPath, err := pathutil.NormalizeForLookup(note.Path)
			if err != nil {
				// Skip notes we can't normalize
				continue
			}
			if seenNotePaths[canonicalPath] {
				// Skip duplicate note
				continue
			}
			seenNotePaths[canonicalPath] = true
			allNotes = append(allNotes, note)
		}
	}
	return allNotes, nil
}

// ListItemsFromAllWorkspaces returns generic items from all registered workspaces
func (s *Service) ListItemsFromAllWorkspaces(includeArchived bool, includeArtifacts bool) ([]*tree.Item, error) {
	allItems := []*tree.Item{}
	allWorkspaces := s.workspaceProvider.All()

	// Use a map to avoid processing the same notebook context twice (for worktrees)
	seenContexts := make(map[string]bool)

	// Use a map to deduplicate items by canonical path across all workspaces
	seenNotePaths := make(map[string]bool)

	for _, ws := range allWorkspaces {
		contextNode, err := s.findNotebookContextNode(ws)
		if err != nil {
			continue // skip if we can't find context
		}
		if seenContexts[contextNode.Path] {
			continue
		}
		seenContexts[contextNode.Path] = true

		// create a dummy context for ListAllItems
		wsCtx := &WorkspaceContext{
			CurrentWorkspace:         ws,
			NotebookContextWorkspace: contextNode,
		}

		items, err := s.ListAllItems(wsCtx, includeArchived, includeArtifacts)
		if err != nil {
			// don't fail, just log and continue
			fmt.Fprintf(os.Stderr, "Warning: could not list items for workspace %s: %v\n", ws.Name, err)
			continue
		}

		// Deduplicate items across workspaces by canonical path
		for _, item := range items {
			canonicalPath, err := pathutil.NormalizeForLookup(item.Path)
			if err != nil {
				// Skip items we can't normalize
				continue
			}
			if seenNotePaths[canonicalPath] {
				// Skip duplicate item
				continue
			}
			seenNotePaths[canonicalPath] = true
			allItems = append(allItems, item)
		}
	}
	return allItems, nil
}
