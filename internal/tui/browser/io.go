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

func fetchFocusedNotesCmd(svc *service.Service, focusedWS *workspace.WorkspaceNode, showArtifacts bool) tea.Cmd {
	return func() tea.Msg {
		var notesToLoad []*workspace.WorkspaceNode
		notesToLoad = append(notesToLoad, focusedWS)

		// If focused on an ecosystem, also load notes from its direct children
		if focusedWS.IsEcosystem() {
			allWorkspaces := svc.GetWorkspaceProvider().All()
			for _, ws := range allWorkspaces {
				if ws.IsChildOf(focusedWS.Path) {
					notesToLoad = append(notesToLoad, ws)
				}
			}
		}

		var allNotes []*models.Note
		// Use a map to deduplicate notes by path
		seenNotes := make(map[string]bool)

		for _, wsNode := range notesToLoad {
			// Get context for the workspace
			wsCtx, err := svc.GetWorkspaceContext(wsNode.Path)
			if err != nil {
				// Log or handle error, for now, we skip
				continue
			}

			// Fetch notes for the workspace (including archived)
			notes, err := svc.ListAllNotes(wsCtx, true, showArtifacts)
			if err == nil {
				for _, note := range notes {
					if !seenNotes[note.Path] {
						allNotes = append(allNotes, note)
						seenNotes[note.Path] = true
					}
				}
			}
		}

		// Also fetch global notes explicitly (including archived)
		globalNotes, err := svc.ListAllGlobalNotes(true, showArtifacts)
		if err == nil {
			for _, note := range globalNotes {
				if !seenNotes[note.Path] {
					allNotes = append(allNotes, note)
					seenNotes[note.Path] = true
				}
			}
		}

		// Combine and sort
		sort.Slice(allNotes, func(i, j int) bool {
			return allNotes[i].ModifiedAt.After(allNotes[j].ModifiedAt)
		})
		return notesLoadedMsg{notes: allNotes}
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

func fetchAllNotesCmd(svc *service.Service, showArtifacts bool) tea.Cmd {
	return func() tea.Msg {
		// Fetch notes from all provider-known workspaces (including archived)
		notes, err := svc.ListNotesFromAllWorkspaces(true, showArtifacts)
		if err != nil {
			// In a real app, we'd return an error message.
			// For now, we return an empty list.
			return notesLoadedMsg{notes: []*models.Note{}}
		}

		// Also fetch global notes explicitly and append them (including archived)
		globalNotes, err := svc.ListAllGlobalNotes(true, showArtifacts)
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
