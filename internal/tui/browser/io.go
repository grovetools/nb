package browser

import (
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

type workspacesLoadedMsg struct {
	workspaces []*workspace.WorkspaceNode
}

type notesLoadedMsg struct {
	notes []*models.Note
}

func fetchFocusedNotesCmd(svc *service.Service, focusedWS *workspace.WorkspaceNode) tea.Cmd {
	return func() tea.Msg {
		// Get context for the focused workspace
		wsCtx, err := svc.GetWorkspaceContext(focusedWS.Path)
		if err != nil {
			// Return empty list on error
			return notesLoadedMsg{notes: []*models.Note{}}
		}

		// Fetch notes for the focused workspace
		focusedNotes, err := svc.ListAllNotes(wsCtx)
		if err != nil {
			focusedNotes = []*models.Note{} // Continue with empty list on error
		}

		// Also fetch global notes explicitly
		globalNotes, err := svc.ListAllGlobalNotes()
		if err != nil {
			globalNotes = []*models.Note{} // Continue with empty list on error
		}

		// Combine and sort
		notes := append(focusedNotes, globalNotes...)
		sort.Slice(notes, func(i, j int) bool {
			return notes[i].ModifiedAt.After(notes[j].ModifiedAt)
		})
		return notesLoadedMsg{notes: notes}
	}
}

func fetchWorkspacesCmd(provider *workspace.Provider) tea.Cmd {
	return func() tea.Msg {
		// Get the real workspaces from the provider.
		workspaces := provider.All()

		// Create and prepend a synthetic "global" workspace node.
		// This confines the concept of a "global" workspace to the notebook TUI.
		globalNode := &workspace.WorkspaceNode{
			Name:  "global",
			Path:  "::global", // A unique, non-filesystem path
			Kind:  workspace.KindStandaloneProject,
			Depth: 0,
		}
		workspaces = append([]*workspace.WorkspaceNode{globalNode}, workspaces...)

		// We need to build the tree to get depth information for filtering ecosystems.
		workspaces = workspace.BuildWorkspaceTree(workspaces)
		return workspacesLoadedMsg{workspaces: workspaces}
	}
}

func fetchAllNotesCmd(svc *service.Service) tea.Cmd {
	return func() tea.Msg {
		// Fetch notes from all provider-known workspaces
		notes, err := svc.ListNotesFromAllWorkspaces()
		if err != nil {
			// In a real app, we'd return an error message.
			// For now, we return an empty list.
			return notesLoadedMsg{notes: []*models.Note{}}
		}

		// Also fetch global notes explicitly and append them
		globalNotes, err := svc.ListAllGlobalNotes()
		if err == nil {
			notes = append(notes, globalNotes...)
		}

		// Sort by modified date descending by default
		sort.Slice(notes, func(i, j int) bool {
			return notes[i].ModifiedAt.After(notes[j].ModifiedAt)
		})
		return notesLoadedMsg{notes: notes}
	}
}
