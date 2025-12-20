package browser

import (
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-notebook/pkg/service"
	"github.com/mattsolo1/grove-notebook/pkg/tree"
)

type workspacesLoadedMsg struct {
	workspaces []*workspace.WorkspaceNode
}

type itemsLoadedMsg struct {
	items []*tree.Item
}

func fetchFocusedItemsCmd(svc *service.Service, focusedWS *workspace.WorkspaceNode, showArtifacts bool) tea.Cmd {
	return func() tea.Msg {
		var itemsToLoad []*workspace.WorkspaceNode
		itemsToLoad = append(itemsToLoad, focusedWS)

		// If focused on an ecosystem, also load items from its direct children
		if focusedWS.IsEcosystem() {
			allWorkspaces := svc.GetWorkspaceProvider().All()
			for _, ws := range allWorkspaces {
				if ws.IsChildOf(focusedWS.Path) {
					itemsToLoad = append(itemsToLoad, ws)
				}
			}
		}

		var allItems []*tree.Item
		// Use a map to deduplicate items by path
		seenItems := make(map[string]bool)

		for _, wsNode := range itemsToLoad {
			// Get context for the workspace
			wsCtx, err := svc.GetWorkspaceContext(wsNode.Path)
			if err != nil {
				// Log or handle error, for now, we skip
				continue
			}

			// Fetch items for the workspace (including archived)
			items, err := svc.ListAllItems(wsCtx, true, showArtifacts)
			if err == nil {
				for _, item := range items {
					if !seenItems[item.Path] {
						allItems = append(allItems, item)
						seenItems[item.Path] = true
					}
				}
			}
		}

		// Also fetch global items explicitly (including archived)
		globalItems, err := svc.ListAllGlobalItems(true, showArtifacts)
		if err == nil {
			for _, item := range globalItems {
				if !seenItems[item.Path] {
					allItems = append(allItems, item)
					seenItems[item.Path] = true
				}
			}
		}

		// Combine and sort
		sort.Slice(allItems, func(i, j int) bool {
			return allItems[i].ModTime.After(allItems[j].ModTime)
		})
		return itemsLoadedMsg{items: allItems}
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

func fetchAllItemsCmd(svc *service.Service, showArtifacts bool) tea.Cmd {
	return func() tea.Msg {
		// Fetch items from all provider-known workspaces (including archived)
		items, err := svc.ListItemsFromAllWorkspaces(true, showArtifacts)
		if err != nil {
			// In a real app, we'd return an error message.
			// For now, we return an empty list.
			return itemsLoadedMsg{items: []*tree.Item{}}
		}

		// Also fetch global items explicitly and append them (including archived)
		globalItems, err := svc.ListAllGlobalItems(true, showArtifacts)
		if err == nil {
			items = append(items, globalItems...)
		}

		// Sort by modified date descending by default
		sort.Slice(items, func(i, j int) bool {
			return items[i].ModTime.After(items[j].ModTime)
		})
		return itemsLoadedMsg{items: items}
	}
}
