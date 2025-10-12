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

	notebookDir := workspace.GetDefaultNotebookDir()

	// Scan global notes
	globalPath := filepath.Join(notebookDir, "global")
	if err := s.scanDirectoryForNotes(globalPath, &allNotes); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not scan global notes: %v\n", err)
	}

	// Scan repo notes
	reposPath := filepath.Join(notebookDir, "repos")
	if err := s.scanDirectoryForNotes(reposPath, &allNotes); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not scan repo notes: %v\n", err)
	}

	return allNotes, nil
}

func (s *Service) scanDirectoryForNotes(rootPath string, notes *[]*models.Note) error {
	return filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() && filepath.Ext(path) == ".md" {
			note, parseErr := ParseNote(path)
			if parseErr == nil {
				*notes = append(*notes, note)
			}
		}
		return nil
	})
}
