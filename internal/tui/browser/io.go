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

func fetchWorkspacesCmd(provider *workspace.Provider) tea.Cmd {
	return func() tea.Msg {
		// The provider already has the data, so this is synchronous.
		// We use a command/message pattern for consistency and future async loading.
		workspaces := provider.All()
		// We need to build the tree to get depth information for filtering ecosystems.
		workspaces = workspace.BuildWorkspaceTree(workspaces)
		return workspacesLoadedMsg{workspaces: workspaces}
	}
}

func fetchAllNotesCmd(svc *service.Service) tea.Cmd {
	return func() tea.Msg {
		notes, err := svc.ListNotesFromAllWorkspaces()
		if err != nil {
			// In a real app, we'd return an error message.
			// For now, we return an empty list.
			return notesLoadedMsg{notes: []*models.Note{}}
		}
		// Sort by modified date descending by default
		sort.Slice(notes, func(i, j int) bool {
			return notes[i].ModifiedAt.After(notes[j].ModifiedAt)
		})
		return notesLoadedMsg{notes: notes}
	}
}
