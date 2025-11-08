package browser

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/util/pathutil"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case editFileAndQuitMsg:
		// Write file path to temp file for Neovim to read
		// Use session ID from environment if available, otherwise fall back to PID
		sessionID := os.Getenv("GROVE_NVIM_SESSION_ID")
		if sessionID == "" {
			sessionID = fmt.Sprintf("%d", os.Getpid())
		}
		tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("grove-nb-edit-%s", sessionID))
		err := os.WriteFile(tempFile, []byte("OPEN:"+msg.filePath+"\n"), 0644)
		if err != nil {
			m.statusMessage = fmt.Sprintf("Error writing temp file: %v", err)
		} else {
			m.statusMessage = ""
		}
		// Don't quit - stay open
		return m, nil

	case previewFileMsg:
		// Write file path to temp file for Neovim to preview
		sessionID := os.Getenv("GROVE_NVIM_SESSION_ID")
		if sessionID == "" {
			sessionID = fmt.Sprintf("%d", os.Getpid())
		}
		tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("grove-nb-edit-%s", sessionID))
		err := os.WriteFile(tempFile, []byte("PREVIEW:"+msg.filePath+"\n"), 0644)
		if err != nil {
			m.statusMessage = fmt.Sprintf("Error writing temp file: %v", err)
		} else {
			m.statusMessage = ""
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetSize(msg.Width, msg.Height)
		m.table.SetHeight(m.getViewportHeight())
		return m, nil

	case workspacesLoadedMsg:
		m.workspaces = msg.workspaces
		// Ensure focused workspace is expanded when initially loaded
		if m.focusedWorkspace != nil {
			wsNodeID := "ws:" + m.focusedWorkspace.Path
			delete(m.collapsedNodes, wsNodeID)
			// Collapse all groups within the focused workspace for a cleaner initial view
			// Users can expand individual groups as needed
			m.collapseFocusedWorkspaceGroups()
		}
		m.buildDisplayTree()
		m.applyFilterAndSort()
		return m, nil

	case notesLoadedMsg:
		m.allNotes = msg.notes
		// Collapse groups if we have a focused workspace
		if m.focusedWorkspace != nil {
			m.collapseFocusedWorkspaceGroups()
		}
		m.buildDisplayTree()
		m.applyFilterAndSort()
		return m, nil

	case notesArchivedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error archiving notes: %v", msg.err)
			return m, nil
		}

		// Create a lookup map of archived paths
		archivedMap := make(map[string]bool)
		for _, path := range msg.archivedPaths {
			archivedMap[path] = true
		}

		// Filter out archived notes from allNotes
		newAllNotes := make([]*models.Note, 0)
		for _, note := range m.allNotes {
			if !archivedMap[note.Path] {
				newAllNotes = append(newAllNotes, note)
			}
		}
		m.allNotes = newAllNotes

		// Clear selections
		m.selected = make(map[string]struct{})
		m.selectedGroups = make(map[string]struct{})

		// Rebuild the display
		m.buildDisplayTree()
		m.applyFilterAndSort()

		if msg.archivedPlans > 0 {
			m.statusMessage = fmt.Sprintf("Archived %d note(s) and %d plan(s)", len(msg.archivedPaths), msg.archivedPlans)
		} else {
			m.statusMessage = fmt.Sprintf("Archived %d note(s)", len(msg.archivedPaths))
		}
		return m, nil

	case tea.KeyMsg:
		if m.help.ShowAll {
			m.help.Toggle()
			return m, nil
		}

		// Handle archive confirmation mode
		if m.confirmingArchive {
			switch msg.String() {
			case "y", "Y":
				m.confirmingArchive = false
				m.statusMessage = ""
				return m, m.archiveSelectedNotesCmd()
			case "n", "N", "esc":
				m.confirmingArchive = false
				m.statusMessage = ""
				return m, nil
			}
			return m, nil
		}

		// Handle filtering mode
		if m.filterInput.Focused() {
			switch {
			case key.Matches(msg, m.keys.Back): // Esc
				m.filterInput.Blur()
				return m, nil
			case key.Matches(msg, m.keys.Confirm): // Enter
				m.filterInput.Blur()
				return m, nil
			default:
				m.filterInput, cmd = m.filterInput.Update(msg)
				m.applyFilterAndSort()
				return m, cmd
			}
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.help.Toggle()
			return m, nil
		case key.Matches(msg, m.keys.Up):
			if m.viewMode == treeView {
				if m.cursor > 0 {
					m.cursor--
					m.adjustScroll()
				}
			} else {
				m.table.MoveUp(1)
			}
		case key.Matches(msg, m.keys.Down):
			if m.viewMode == treeView {
				if m.cursor < len(m.displayNodes)-1 {
					m.cursor++
					m.adjustScroll()
				}
			} else {
				m.table.MoveDown(1)
			}
		case key.Matches(msg, m.keys.PageUp):
			if m.viewMode == treeView {
				pageSize := m.getViewportHeight() / 2
				if pageSize < 1 {
					pageSize = 1
				}
				m.cursor -= pageSize
				if m.cursor < 0 {
					m.cursor = 0
				}
				m.adjustScroll()
			} else {
				pageSize := m.getViewportHeight() / 2
				for i := 0; i < pageSize; i++ {
					m.table.MoveUp(1)
				}
			}
		case key.Matches(msg, m.keys.PageDown):
			if m.viewMode == treeView {
				pageSize := m.getViewportHeight() / 2
				if pageSize < 1 {
					pageSize = 1
				}
				m.cursor += pageSize
				if m.cursor >= len(m.displayNodes) {
					m.cursor = len(m.displayNodes) - 1
				}
				m.adjustScroll()
			} else {
				pageSize := m.getViewportHeight() / 2
				for i := 0; i < pageSize; i++ {
					m.table.MoveDown(1)
				}
			}
		case key.Matches(msg, m.keys.GoToTop):
			// Handle 'gg' - go to top when g is pressed twice
			if m.lastKey == "g" {
				if m.viewMode == treeView {
					m.cursor = 0
					m.adjustScroll()
				} else {
					m.table.SetCursor(0)
				}
				m.lastKey = ""
			} else {
				m.lastKey = "g"
			}
		case key.Matches(msg, m.keys.GoToBottom):
			if m.viewMode == treeView {
				if len(m.displayNodes) > 0 {
					m.cursor = len(m.displayNodes) - 1
					m.adjustScroll()
				}
			} else {
				if len(m.filteredNotes) > 0 {
					m.table.SetCursor(len(m.filteredNotes) - 1)
				}
			}
		case key.Matches(msg, m.keys.FoldPrefix):
			// Handle 'z' prefix for fold commands
			if m.viewMode == treeView {
				m.lastKey = "z"
			}
		case msg.String() == "a":
			// za - toggle fold
			if m.lastKey == "z" && m.viewMode == treeView {
				m.toggleFold()
				m.lastKey = ""
			}
		case msg.String() == "o":
			// zo - open fold
			if m.lastKey == "z" && m.viewMode == treeView {
				m.openFold()
				m.lastKey = ""
			}
		case msg.String() == "c":
			// zc - close fold
			if m.lastKey == "z" && m.viewMode == treeView {
				m.closeFold()
				m.lastKey = ""
			}
		case msg.String() == "M":
			// zM - close all folds
			if m.lastKey == "z" && m.viewMode == treeView {
				m.closeAllFolds()
				m.lastKey = ""
			}
		case msg.String() == "R":
			// zR - open all folds
			if m.lastKey == "z" && m.viewMode == treeView {
				m.openAllFolds()
				m.lastKey = ""
			}
		case key.Matches(msg, m.keys.FocusEcosystem):
			if !m.ecosystemPickerMode {
				m.ecosystemPickerMode = true
				m.buildDisplayTree()
				m.applyFilterAndSort()
				m.cursor = 0
			}
		case key.Matches(msg, m.keys.ClearFocus):
			if m.focusedWorkspace != nil || m.ecosystemPickerMode {
				m.focusedWorkspace = nil
				m.ecosystemPickerMode = false
				m.buildDisplayTree()
				m.applyFilterAndSort()
				m.cursor = 0
			}
		case key.Matches(msg, m.keys.FocusParent):
			if m.focusedWorkspace != nil {
				var parent *workspace.WorkspaceNode
				var parentPath string

				if m.focusedWorkspace.ParentEcosystemPath != "" {
					parentPath = m.focusedWorkspace.ParentEcosystemPath
				} else if m.focusedWorkspace.ParentProjectPath != "" {
					parentPath = m.focusedWorkspace.ParentProjectPath
				} else if m.focusedWorkspace.RootEcosystemPath != "" && m.focusedWorkspace.RootEcosystemPath != m.focusedWorkspace.Path {
					parentPath = m.focusedWorkspace.RootEcosystemPath
				}

				if parentPath != "" {
					for _, ws := range m.workspaces {
						if ws.Path == parentPath {
							parent = ws
							break
						}
					}
				}

				m.focusedWorkspace = parent // This can be nil if no parent is found, effectively clearing focus
				// Ensure new focused workspace is expanded
				if m.focusedWorkspace != nil {
					wsNodeID := "ws:" + m.focusedWorkspace.Path
					delete(m.collapsedNodes, wsNodeID)
				}
				m.buildDisplayTree()
				m.applyFilterAndSort()
				m.cursor = 0
			}
		case key.Matches(msg, m.keys.FocusSelected):
			if m.viewMode == treeView && m.cursor < len(m.displayNodes) {
				node := m.displayNodes[m.cursor]
				if node.isWorkspace {
					m.focusedWorkspace = node.workspace
					m.ecosystemPickerMode = false // Focusing on a workspace exits picker mode
					// Ensure focused workspace is expanded
					wsNodeID := "ws:" + m.focusedWorkspace.Path
					delete(m.collapsedNodes, wsNodeID)
					m.buildDisplayTree()
					m.applyFilterAndSort()
					m.cursor = 0
				}
			}
		case key.Matches(msg, m.keys.ToggleView):
			if m.viewMode == treeView {
				m.viewMode = tableView
			} else {
				m.viewMode = treeView
			}
			m.cursor = 0
			m.table.SetCursor(0)
		case key.Matches(msg, m.keys.Search):
			m.filterInput.Focus()
			return m, textinput.Blink
		case key.Matches(msg, m.keys.Sort):
			if m.viewMode == tableView {
				m.sortColumn = (m.sortColumn + 1) % 5
				m.applyFilterAndSort()
			}
		case key.Matches(msg, m.keys.ToggleArchives):
			if m.viewMode == treeView {
				m.showArchives = !m.showArchives
				m.buildDisplayTree()
			}
		case key.Matches(msg, m.keys.ToggleSelect):
			// Toggle selection for the current note or plan group
			if m.viewMode == treeView {
				if m.cursor < len(m.displayNodes) {
					node := m.displayNodes[m.cursor]
					if node.isNote {
						if _, ok := m.selected[node.note.Path]; ok {
							delete(m.selected, node.note.Path)
						} else {
							m.selected[node.note.Path] = struct{}{}
						}
					} else if node.isPlan() {
						// Allow selection of plan groups
						groupKey := m.getGroupKey(node)
						if _, ok := m.selectedGroups[groupKey]; ok {
							delete(m.selectedGroups, groupKey)
						} else {
							m.selectedGroups[groupKey] = struct{}{}
						}
					}
				}
			} else { // Table view
				if m.table.Cursor() < len(m.filteredNotes) {
					note := m.filteredNotes[m.table.Cursor()]
					if _, ok := m.selected[note.Path]; ok {
						delete(m.selected, note.Path)
					} else {
						m.selected[note.Path] = struct{}{}
					}
					m.buildTableRows()
				}
			}
		case key.Matches(msg, m.keys.SelectAll):
			// Select all visible notes
			if m.viewMode == treeView {
				for _, node := range m.displayNodes {
					if node.isNote {
						m.selected[node.note.Path] = struct{}{}
					}
				}
			} else { // Table view
				for _, note := range m.filteredNotes {
					m.selected[note.Path] = struct{}{}
				}
				m.buildTableRows()
			}
		case key.Matches(msg, m.keys.SelectNone):
			// Clear all selections
			m.selected = make(map[string]struct{})
			m.selectedGroups = make(map[string]struct{})
			if m.viewMode == tableView {
				m.buildTableRows()
			}
		case key.Matches(msg, m.keys.Archive):
			// Archive selected notes and/or plan groups
			totalSelected := len(m.selected) + len(m.selectedGroups)
			if totalSelected > 0 {
				m.confirmingArchive = true
				if len(m.selected) > 0 && len(m.selectedGroups) > 0 {
					m.statusMessage = fmt.Sprintf("Archive %d notes and %d plans? (y/N)", len(m.selected), len(m.selectedGroups))
				} else if len(m.selectedGroups) > 0 {
					m.statusMessage = fmt.Sprintf("Archive %d plans? (y/N)", len(m.selectedGroups))
				} else {
					m.statusMessage = fmt.Sprintf("Archive %d notes? (y/N)", len(m.selected))
				}
			}
		case key.Matches(msg, m.keys.Confirm):
			if m.ecosystemPickerMode {
				if m.viewMode == treeView && m.cursor < len(m.displayNodes) {
					selected := m.displayNodes[m.cursor]
					if selected.isWorkspace && selected.workspace.IsEcosystem() {
						m.focusedWorkspace = selected.workspace
						m.ecosystemPickerMode = false
						m.buildDisplayTree()
						m.applyFilterAndSort()
						m.cursor = 0
					}
				}
			} else {
				var noteToOpen *models.Note
				if m.viewMode == treeView {
					if m.cursor < len(m.displayNodes) {
						node := m.displayNodes[m.cursor]
						if node.isNote {
							noteToOpen = node.note
						} else if node.isFoldable() {
							// Toggle fold on workspaces and groups
							m.toggleFold()
							return m, nil
						}
					}
				} else { // Table view
					if m.table.Cursor() < len(m.filteredNotes) {
						noteToOpen = m.filteredNotes[m.table.Cursor()]
					}
				}
				if noteToOpen != nil {
					if os.Getenv("GROVE_NVIM_PLUGIN") == "true" {
						return m, func() tea.Msg {
							return editFileAndQuitMsg{filePath: noteToOpen.Path}
						}
					}
					return m, m.openInEditor(noteToOpen.Path)
				}
			}
		case key.Matches(msg, m.keys.Preview):
			if !m.ecosystemPickerMode {
				var noteToPreview *models.Note
				if m.viewMode == treeView {
					if m.cursor < len(m.displayNodes) {
						node := m.displayNodes[m.cursor]
						if node.isNote {
							noteToPreview = node.note
						}
					}
				} else { // Table view
					if m.table.Cursor() < len(m.filteredNotes) {
						noteToPreview = m.filteredNotes[m.table.Cursor()]
					}
				}
				if noteToPreview != nil && os.Getenv("GROVE_NVIM_PLUGIN") == "true" {
					return m, func() tea.Msg {
						return previewFileMsg{filePath: noteToPreview.Path}
					}
				}
			}
		case key.Matches(msg, m.keys.Back):
			if m.ecosystemPickerMode {
				m.ecosystemPickerMode = false
				m.buildDisplayTree()
				return m, nil
			}
		default:
			// Reset lastKey for any other key press (for gg and z* detection)
			if !key.Matches(msg, m.keys.GoToTop) && !key.Matches(msg, m.keys.FoldPrefix) {
				m.lastKey = ""
			}
		}
	}
	return m, nil
}

// buildDisplayTree constructs the hierarchical list of nodes for rendering.
func (m *Model) buildDisplayTree() {
	var nodes []*displayNode
	var workspacesToShow []*workspace.WorkspaceNode

	// 1. Filter workspaces based on focus mode
	var showUngroupedSection bool
	var ungroupedWorkspaces []*workspace.WorkspaceNode

	if m.ecosystemPickerMode {
		// In picker mode, show only top-level ecosystems and their eco-worktrees
		for _, ws := range m.workspaces {
			if ws.IsEcosystem() && ws.Depth == 0 {
				workspacesToShow = append(workspacesToShow, ws)
				// Also add eco-worktrees that are children of this ecosystem
				for _, child := range m.workspaces {
					if child.IsWorktree() && child.IsEcosystem() && strings.HasPrefix(child.Path, ws.Path+"/") && child.Depth == ws.Depth+1 {
						workspacesToShow = append(workspacesToShow, child)
					}
				}
			}
		}
	} else if m.focusedWorkspace != nil {
		for _, ws := range m.workspaces {
			// Use case-insensitive path comparison
			isSame, _ := pathutil.ComparePaths(ws.Path, m.focusedWorkspace.Path)
			if isSame {
				workspacesToShow = append(workspacesToShow, ws)
				continue
			}
			// Check if it's a child workspace
			normFocused, _ := pathutil.NormalizeForLookup(m.focusedWorkspace.Path)
			normWs, _ := pathutil.NormalizeForLookup(ws.Path)
			if strings.HasPrefix(normWs, normFocused+string(filepath.Separator)) {
				workspacesToShow = append(workspacesToShow, ws)
			}
		}
	} else {
		// Global view: partition into ecosystem workspaces and standalone workspaces
		for _, ws := range m.workspaces {
			// Check if this is a standalone (non-ecosystem) top-level workspace
			if ws.Depth == 0 && !ws.IsEcosystem() {
				ungroupedWorkspaces = append(ungroupedWorkspaces, ws)
			} else if ws.Depth == 0 || ws.IsEcosystem() {
				// Top-level ecosystems and their children
				workspacesToShow = append(workspacesToShow, ws)
			} else {
				// Check if this workspace belongs to a standalone project
				belongsToStandalone := false
				for _, standalone := range ungroupedWorkspaces {
					if strings.HasPrefix(ws.Path, standalone.Path+"/") {
						ungroupedWorkspaces = append(ungroupedWorkspaces, ws)
						belongsToStandalone = true
						break
					}
				}
				if !belongsToStandalone {
					workspacesToShow = append(workspacesToShow, ws)
				}
			}
		}
		showUngroupedSection = len(ungroupedWorkspaces) > 0
	}

	// 2. Group notes by workspace path, then by group (directory)
	notesByWorkspace := make(map[string]map[string][]*models.Note)
	for _, note := range m.allNotes {
		if _, ok := notesByWorkspace[note.Workspace]; !ok {
			notesByWorkspace[note.Workspace] = make(map[string][]*models.Note)
		}
		notesByWorkspace[note.Workspace][note.Group] = append(notesByWorkspace[note.Workspace][note.Group], note)
	}

	// 3. Build the display node list and jump map
	m.jumpMap = make(map[rune]int)
	jumpCounter := '1'
	needsSeparator := false // Track if we need to add a separator before the next workspace

	for _, ws := range workspacesToShow {
		// Add separator between ecosystem's own notes and child workspaces
		if needsSeparator && m.focusedWorkspace != nil && m.focusedWorkspace.IsEcosystem() {
			isSame, _ := pathutil.ComparePaths(ws.Path, m.focusedWorkspace.Path)
			if !isSame {
				// This is a child workspace, add separator
				nodes = append(nodes, &displayNode{
					isSeparator: true,
					prefix:      "  ",
					depth:       0,
				})
				needsSeparator = false // Only add separator once
			}
		}
		// Skip worktrees - they never have their own notes
		if ws.IsWorktree() {
			continue
		}

		hasNotes := len(notesByWorkspace[ws.Name]) > 0
		// Always show ecosystem nodes at depth 0, even if they have no direct notes
		// (their children may have notes)
		if !hasNotes && m.focusedWorkspace == nil && ws.Depth > 0 {
			// In global view, only skip non-ecosystem workspaces that have no notes
			continue
		}

		// Add workspace node
		node := &displayNode{
			isWorkspace: true,
			workspace:   ws,
			prefix:      ws.TreePrefix,
			depth:       ws.Depth,
		}

		// Assign jump key for workspaces at depth <= 1
		if ws.Depth <= 1 && jumpCounter <= '9' {
			node.jumpKey = jumpCounter
			m.jumpMap[jumpCounter] = len(nodes)
			jumpCounter++
		}
		nodes = append(nodes, node)

		// In ecosystem picker mode, don't show notes - just workspaces
		if m.ecosystemPickerMode {
			continue
		}

		// Skip children if workspace is collapsed
		wsNodeID := node.nodeID()
		wsCollapsed := m.collapsedNodes[wsNodeID]
		if wsCollapsed {
			continue
		}

		if noteGroups, ok := notesByWorkspace[ws.Name]; ok {
			// Separate regular groups, plan groups, and archive groups
			var regularGroups []string
			planGroups := make(map[string][]*models.Note) // plan name -> notes
			archiveGroups := make(map[string][]string)    // parent -> archive path

			for name, notes := range noteGroups {
				if strings.HasSuffix(name, "/.archive") {
					// This is an archive group - associate it with its parent
					parent := strings.TrimSuffix(name, "/.archive")
					archiveGroups[parent] = append(archiveGroups[parent], name)
				} else if strings.Contains(name, "/.archive/") {
					// This is an archived plan or similar - skip if archives are hidden
					if m.showArchives {
						regularGroups = append(regularGroups, name)
					}
				} else if strings.HasPrefix(name, "plans/") {
					// Extract plan name (e.g., "plans/nb-tui" -> "nb-tui")
					planName := strings.TrimPrefix(name, "plans/")
					planGroups[planName] = notes
				} else {
					regularGroups = append(regularGroups, name)
				}
			}
			sort.Strings(regularGroups)

			// Check if we have plans to add a "plans" parent group
			hasPlans := len(planGroups) > 0
			totalGroups := len(regularGroups)
			if hasPlans {
				totalGroups++
			}

			for i, groupName := range regularGroups {
				isLastGroup := i == len(regularGroups)-1 && !hasPlans
				notesInGroup := noteGroups[groupName]

				// Calculate group prefix
				var groupPrefix strings.Builder
				indentPrefix := strings.ReplaceAll(ws.TreePrefix, "├─", "│ ")
				indentPrefix = strings.ReplaceAll(indentPrefix, "└─", "  ")
				groupPrefix.WriteString(indentPrefix)
				if ws.Depth > 0 || ws.TreePrefix != "" {
					groupPrefix.WriteString("  ")
				}
				if isLastGroup {
					groupPrefix.WriteString("└─ ")
				} else {
					groupPrefix.WriteString("├─ ")
				}

				// Add group node
				groupNode := &displayNode{
					isGroup:       true,
					groupName:     groupName,
					workspaceName: ws.Name,
					prefix:        groupPrefix.String(),
					depth:         ws.Depth + 1,
				}
				nodes = append(nodes, groupNode)

				// Skip notes if group is collapsed
				groupNodeID := groupNode.nodeID()
				if m.collapsedNodes[groupNodeID] {
					continue
				}

				// Add note nodes
				hasArchive := len(archiveGroups[groupName]) > 0 && m.showArchives

				for j, note := range notesInGroup {
					isLastNote := j == len(notesInGroup)-1 && !hasArchive
					var notePrefix strings.Builder
					noteIndent := strings.ReplaceAll(groupPrefix.String(), "├─", "│ ")
					noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
					notePrefix.WriteString(noteIndent)
					if isLastNote {
						notePrefix.WriteString("└─ ")
					} else {
						notePrefix.WriteString("├─ ")
					}
					nodes = append(nodes, &displayNode{
						isNote: true,
						note:   note,
						prefix: notePrefix.String(),
						depth:  ws.Depth + 2,
					})
				}

				// Add archive subgroup if it exists and archives are visible
				if hasArchive {
					for _, archiveName := range archiveGroups[groupName] {
						archiveNotes := noteGroups[archiveName]

						// Calculate archive prefix
						var archivePrefix strings.Builder
						archiveIndent := strings.ReplaceAll(groupPrefix.String(), "├─", "│ ")
						archiveIndent = strings.ReplaceAll(archiveIndent, "└─", "  ")
						archivePrefix.WriteString(archiveIndent)
						archivePrefix.WriteString("└─ ")

						archiveNode := &displayNode{
							isGroup:       true,
							groupName:     ".archive",
							workspaceName: ws.Name,
							prefix:        archivePrefix.String(),
							depth:         ws.Depth + 2,
						}
						nodes = append(nodes, archiveNode)

						// Skip archive notes if collapsed
						archiveNodeID := archiveNode.nodeID()
						if m.collapsedNodes[archiveNodeID] {
							continue
						}

						// Add archive note nodes
						for k, note := range archiveNotes {
							isLastArchiveNote := k == len(archiveNotes)-1
							var archiveNotePrefix strings.Builder
							archiveNoteIndent := strings.ReplaceAll(archivePrefix.String(), "├─", "│ ")
							archiveNoteIndent = strings.ReplaceAll(archiveNoteIndent, "└─", "  ")
							archiveNotePrefix.WriteString(archiveNoteIndent)
							if isLastArchiveNote {
								archiveNotePrefix.WriteString("└─ ")
							} else {
								archiveNotePrefix.WriteString("├─ ")
							}
							nodes = append(nodes, &displayNode{
								isNote: true,
								note:   note,
								prefix: archiveNotePrefix.String(),
								depth:  ws.Depth + 3,
							})
						}
					}
				}
			}

			// Add "plans" parent group if there are any plans
			if hasPlans {
				// Calculate plans parent prefix
				var plansPrefix strings.Builder
				indentPrefix := strings.ReplaceAll(ws.TreePrefix, "├─", "│ ")
				indentPrefix = strings.ReplaceAll(indentPrefix, "└─", "  ")
				plansPrefix.WriteString(indentPrefix)
				if ws.Depth > 0 || ws.TreePrefix != "" {
					plansPrefix.WriteString("  ")
				}
				plansPrefix.WriteString("└─ ") // Plans is always last

				// Add "plans" parent node
				plansParentNode := &displayNode{
					isGroup:       true,
					groupName:     "plans",
					workspaceName: ws.Name,
					prefix:        plansPrefix.String(),
					depth:         ws.Depth + 1,
				}
				nodes = append(nodes, plansParentNode)

				// Check if plans parent is collapsed
				plansParentNodeID := plansParentNode.nodeID()
				if !m.collapsedNodes[plansParentNodeID] {
					// Sort plan names
					var planNames []string
					for planName := range planGroups {
						planNames = append(planNames, planName)
					}
					sort.Strings(planNames)

					// Add individual plan nodes
					for pi, planName := range planNames {
						isLastPlan := pi == len(planNames)-1
						planNotes := planGroups[planName]

						// Calculate plan prefix
						var planPrefix strings.Builder
						planIndent := strings.ReplaceAll(plansPrefix.String(), "├─", "│ ")
						planIndent = strings.ReplaceAll(planIndent, "└─", "  ")
						planPrefix.WriteString(planIndent)
						if isLastPlan {
							planPrefix.WriteString("└─ ")
						} else {
							planPrefix.WriteString("├─ ")
						}

						// Add plan node with status icon
						planNode := &displayNode{
							isGroup:       true,
							groupName:     "plans/" + planName, // Keep full path for isPlan() check
							workspaceName: ws.Name,
							prefix:        planPrefix.String(),
							depth:         ws.Depth + 2,
						}
						nodes = append(nodes, planNode)

						// Check if this plan is collapsed
						planNodeID := planNode.nodeID()
						if !m.collapsedNodes[planNodeID] {
							// Add notes in this plan
							for ni, note := range planNotes {
								isLastNote := ni == len(planNotes)-1
								var notePrefix strings.Builder
								noteIndent := strings.ReplaceAll(planPrefix.String(), "├─", "│ ")
								noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
								notePrefix.WriteString(noteIndent)
								if isLastNote {
									notePrefix.WriteString("└─ ")
								} else {
									notePrefix.WriteString("├─ ")
								}
								nodes = append(nodes, &displayNode{
									isNote: true,
									note:   note,
									prefix: notePrefix.String(),
									depth:  ws.Depth + 3,
								})
							}
						}
					}
				}
			}
		}

		// Mark that we need a separator before child workspaces
		// This is set after rendering the focused ecosystem's own note groups
		// Only show separator if the ecosystem is expanded
		if m.focusedWorkspace != nil && m.focusedWorkspace.IsEcosystem() && !wsCollapsed {
			isSame, _ := pathutil.ComparePaths(ws.Path, m.focusedWorkspace.Path)
			if isSame {
				needsSeparator = true
			}
		}
	}

	// 4. Add "ungrouped" section if there are standalone workspaces
	if showUngroupedSection {
		ungroupedNode := &displayNode{
			isGroup:   true,
			groupName: "ungrouped",
			prefix:    "",
			depth:     0,
		}
		nodes = append(nodes, ungroupedNode)

		// Check if ungrouped section is collapsed
		if !m.collapsedNodes[ungroupedNode.nodeID()] {
			// Render each ungrouped workspace
			for i, ws := range ungroupedWorkspaces {
				// Skip worktrees
				if ws.IsWorktree() {
					continue
				}

				hasNotes := len(notesByWorkspace[ws.Name]) > 0
				if !hasNotes && ws.Depth > 0 {
					continue
				}

				// Adjust tree prefix to be indented under "Ungrouped"
				adjustedPrefix := "  " + ws.TreePrefix
				// If this is a depth-0 workspace, give it proper indentation
				if ws.Depth == 0 {
					// Check if this is the last ungrouped workspace
					isLast := i == len(ungroupedWorkspaces)-1
					if isLast {
						adjustedPrefix = "  └─ "
					} else {
						adjustedPrefix = "  ├─ "
					}
				}

				// Add workspace node
				node := &displayNode{
					isWorkspace: true,
					workspace:   ws,
					prefix:      adjustedPrefix,
					depth:       ws.Depth + 1, // Increase depth since it's under "Ungrouped"
				}

				// Assign jump key for ungrouped workspaces
				if jumpCounter <= '9' {
					node.jumpKey = jumpCounter
					m.jumpMap[jumpCounter] = len(nodes)
					jumpCounter++
				}
				nodes = append(nodes, node)

				// Skip children if workspace is collapsed
				wsNodeID := node.nodeID()
				if m.collapsedNodes[wsNodeID] {
					continue
				}

				// Render notes for this ungrouped workspace
				if noteGroups, ok := notesByWorkspace[ws.Name]; ok {
					// Separate regular groups, plan groups, and archive groups
					var regularGroups []string
					planGroups := make(map[string][]*models.Note)
					archiveGroups := make(map[string][]string)

					for name, notes := range noteGroups {
						if strings.HasSuffix(name, "/.archive") {
							parent := strings.TrimSuffix(name, "/.archive")
							archiveGroups[parent] = append(archiveGroups[parent], name)
						} else if strings.Contains(name, "/.archive/") {
							if m.showArchives {
								regularGroups = append(regularGroups, name)
							}
						} else if strings.HasPrefix(name, "plans/") {
							planName := strings.TrimPrefix(name, "plans/")
							planGroups[planName] = notes
						} else {
							regularGroups = append(regularGroups, name)
						}
					}
					sort.Strings(regularGroups)

					hasPlans := len(planGroups) > 0

					for i, groupName := range regularGroups {
						isLastGroup := i == len(regularGroups)-1 && !hasPlans
						notesInGroup := noteGroups[groupName]

						// Calculate group prefix (with extra indentation for ungrouped)
						var groupPrefix strings.Builder
						indentPrefix := strings.ReplaceAll(adjustedPrefix, "├─", "│ ")
						indentPrefix = strings.ReplaceAll(indentPrefix, "└─", "  ")
						groupPrefix.WriteString(indentPrefix)
						groupPrefix.WriteString("  ")
						if isLastGroup {
							groupPrefix.WriteString("└─ ")
						} else {
							groupPrefix.WriteString("├─ ")
						}

						// Add group node
						groupNode := &displayNode{
							isGroup:       true,
							groupName:     groupName,
							workspaceName: ws.Name,
							prefix:        groupPrefix.String(),
							depth:         ws.Depth + 2,
						}
						nodes = append(nodes, groupNode)

						// Skip notes if group is collapsed
						groupNodeID := groupNode.nodeID()
						if m.collapsedNodes[groupNodeID] {
							continue
						}

						// Add note nodes
						hasArchive := len(archiveGroups[groupName]) > 0 && m.showArchives

						for j, note := range notesInGroup {
							isLastNote := j == len(notesInGroup)-1 && !hasArchive
							var notePrefix strings.Builder
							noteIndent := strings.ReplaceAll(groupPrefix.String(), "├─", "│ ")
							noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
							notePrefix.WriteString(noteIndent)
							if isLastNote {
								notePrefix.WriteString("└─ ")
							} else {
								notePrefix.WriteString("├─ ")
							}
							nodes = append(nodes, &displayNode{
								isNote: true,
								note:   note,
								prefix: notePrefix.String(),
								depth:  ws.Depth + 3,
							})
						}

						// Add archive subgroup if it exists
						if hasArchive {
							for _, archiveName := range archiveGroups[groupName] {
								archiveNotes := noteGroups[archiveName]

								var archivePrefix strings.Builder
								archiveIndent := strings.ReplaceAll(groupPrefix.String(), "├─", "│ ")
								archiveIndent = strings.ReplaceAll(archiveIndent, "└─", "  ")
								archivePrefix.WriteString(archiveIndent)
								archivePrefix.WriteString("└─ ")

								archiveNode := &displayNode{
									isGroup:       true,
									groupName:     ".archive",
									workspaceName: ws.Name,
									prefix:        archivePrefix.String(),
									depth:         ws.Depth + 3,
								}
								nodes = append(nodes, archiveNode)

								archiveNodeID := archiveNode.nodeID()
								if m.collapsedNodes[archiveNodeID] {
									continue
								}

								for k, note := range archiveNotes {
									isLastArchiveNote := k == len(archiveNotes)-1
									var archiveNotePrefix strings.Builder
									archiveNoteIndent := strings.ReplaceAll(archivePrefix.String(), "├─", "│ ")
									archiveNoteIndent = strings.ReplaceAll(archiveNoteIndent, "└─", "  ")
									archiveNotePrefix.WriteString(archiveNoteIndent)
									if isLastArchiveNote {
										archiveNotePrefix.WriteString("└─ ")
									} else {
										archiveNotePrefix.WriteString("├─ ")
									}
									nodes = append(nodes, &displayNode{
										isNote: true,
										note:   note,
										prefix: archiveNotePrefix.String(),
										depth:  ws.Depth + 4,
									})
								}
							}
						}
					}

					// Add "plans" parent group if there are any plans
					if hasPlans {
						var plansPrefix strings.Builder
						indentPrefix := strings.ReplaceAll(adjustedPrefix, "├─", "│ ")
						indentPrefix = strings.ReplaceAll(indentPrefix, "└─", "  ")
						plansPrefix.WriteString(indentPrefix)
						plansPrefix.WriteString("  ")
						plansPrefix.WriteString("└─ ")

						plansParentNode := &displayNode{
							isGroup:       true,
							groupName:     "plans",
							workspaceName: ws.Name,
							prefix:        plansPrefix.String(),
							depth:         ws.Depth + 2,
						}
						nodes = append(nodes, plansParentNode)

						plansParentNodeID := plansParentNode.nodeID()
						if !m.collapsedNodes[plansParentNodeID] {
							var planNames []string
							for planName := range planGroups {
								planNames = append(planNames, planName)
							}
							sort.Strings(planNames)

							for pi, planName := range planNames {
								isLastPlan := pi == len(planNames)-1
								planNotes := planGroups[planName]

								var planPrefix strings.Builder
								planIndent := strings.ReplaceAll(plansPrefix.String(), "├─", "│ ")
								planIndent = strings.ReplaceAll(planIndent, "└─", "  ")
								planPrefix.WriteString(planIndent)
								if isLastPlan {
									planPrefix.WriteString("└─ ")
								} else {
									planPrefix.WriteString("├─ ")
								}

								planNode := &displayNode{
									isGroup:       true,
									groupName:     "plans/" + planName,
									workspaceName: ws.Name,
									prefix:        planPrefix.String(),
									depth:         ws.Depth + 3,
								}
								nodes = append(nodes, planNode)

								planNodeID := planNode.nodeID()
								if !m.collapsedNodes[planNodeID] {
									for ni, note := range planNotes {
										isLastNote := ni == len(planNotes)-1
										var notePrefix strings.Builder
										noteIndent := strings.ReplaceAll(planPrefix.String(), "├─", "│ ")
										noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
										notePrefix.WriteString(noteIndent)
										if isLastNote {
											notePrefix.WriteString("└─ ")
										} else {
											notePrefix.WriteString("├─ ")
										}
										nodes = append(nodes, &displayNode{
											isNote: true,
											note:   note,
											prefix: notePrefix.String(),
											depth:  ws.Depth + 4,
										})
									}
								}
							}
						}
					}
				}
			}
		}
	}

	m.displayNodes = nodes
	m.clampCursor()
}

// clampCursor ensures the cursor is within the valid range of display nodes.
func (m *Model) clampCursor() {
	if m.cursor >= len(m.displayNodes) {
		if len(m.displayNodes) > 0 {
			m.cursor = len(m.displayNodes) - 1
		} else {
			m.cursor = 0
		}
	}
}

// getViewportHeight calculates how many lines are available for the list.
func (m *Model) getViewportHeight() int {
	// Account for:
	// - Top margin: 1 line
	// - Header: 1 line
	// - Blank line after header: 1 line
	// - Blank line before footer: 1 line
	// - Status bar: 1 line
	// - Footer (help): 1 line
	// - Scroll indicator (when shown): 2 lines (blank + indicator)
	const fixedLines = 8
	availableHeight := m.height - fixedLines
	if availableHeight < 1 {
		return 1
	}
	return availableHeight
}

// adjustScroll ensures the cursor is visible in the viewport.
func (m *Model) adjustScroll() {
	viewportHeight := m.getViewportHeight()
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	} else if m.cursor >= m.scrollOffset+viewportHeight {
		m.scrollOffset = m.cursor - viewportHeight + 1
	}
	// Ensure scrollOffset never goes negative
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// applyFilterAndSort filters and sorts notes for the table view.
func (m *Model) applyFilterAndSort() {
	var notesToConsider []*models.Note
	if m.focusedWorkspace != nil {
		for _, note := range m.allNotes {
			// Filter to notes within the focused workspace
			if strings.HasPrefix(note.Path, m.focusedWorkspace.Path) {
				notesToConsider = append(notesToConsider, note)
			}
		}
	} else {
		notesToConsider = m.allNotes
	}

	filter := strings.ToLower(m.filterInput.Value())
	if filter == "" {
		m.filteredNotes = notesToConsider
	} else {
		var filtered []*models.Note
		for _, note := range notesToConsider {
			if strings.Contains(strings.ToLower(note.Title), filter) ||
				strings.Contains(strings.ToLower(note.Workspace), filter) ||
				strings.Contains(strings.ToLower(string(note.Type)), filter) ||
				strings.Contains(strings.ToLower(note.Group), filter) {
				filtered = append(filtered, note)
			}
		}
		m.filteredNotes = filtered
	}

	// Sort the notes
	sort.SliceStable(m.filteredNotes, func(i, j int) bool {
		a, b := m.filteredNotes[i], m.filteredNotes[j]
		var less bool
		switch m.sortColumn {
		case 0: // Selection indicator (no sorting)
			return false
		case 1: // Workspace
			less = strings.ToLower(a.Workspace) < strings.ToLower(b.Workspace)
		case 2: // Type
			less = strings.ToLower(string(a.Type)) < strings.ToLower(string(b.Type))
		case 3: // Title
			less = strings.ToLower(a.Title) < strings.ToLower(b.Title)
		default: // Modified (case 4)
			less = a.ModifiedAt.Before(b.ModifiedAt)
		}
		if m.sortAsc {
			return less
		}
		return !less
	})

	m.buildTableRows()
}

// buildTableRows converts filtered notes into table rows.
func (m *Model) buildTableRows() {
	rows := make([]table.Row, len(m.filteredNotes))
	for i, note := range m.filteredNotes {
		selIndicator := " "
		if _, ok := m.selected[note.Path]; ok {
			selIndicator = "✓"
		}
		rows[i] = table.Row{
			selIndicator,
			note.Workspace,
			string(note.Type),
			note.Title,
			note.ModifiedAt.Format(time.RFC822),
		}
	}
	m.table.SetRows(rows)
}

// toggleFold toggles the fold state of the node under the cursor
func (m *Model) toggleFold() {
	if m.cursor >= len(m.displayNodes) {
		return
	}
	node := m.displayNodes[m.cursor]
	if !node.isFoldable() {
		return
	}
	nodeID := node.nodeID()
	if m.collapsedNodes[nodeID] {
		delete(m.collapsedNodes, nodeID)
	} else {
		m.collapsedNodes[nodeID] = true
	}
	m.buildDisplayTree()
}

// openFold opens the fold of the node under the cursor
func (m *Model) openFold() {
	if m.cursor >= len(m.displayNodes) {
		return
	}
	node := m.displayNodes[m.cursor]
	if !node.isFoldable() {
		return
	}
	delete(m.collapsedNodes, node.nodeID())
	m.buildDisplayTree()
}

// closeFold closes the fold of the node under the cursor
func (m *Model) closeFold() {
	if m.cursor >= len(m.displayNodes) {
		return
	}
	node := m.displayNodes[m.cursor]
	if !node.isFoldable() {
		return
	}
	m.collapsedNodes[node.nodeID()] = true
	m.buildDisplayTree()
}

// closeAllFolds closes all foldable nodes
func (m *Model) closeAllFolds() {
	for _, node := range m.displayNodes {
		if node.isFoldable() {
			m.collapsedNodes[node.nodeID()] = true
		}
	}
	m.buildDisplayTree()
}

// openAllFolds opens all foldable nodes
func (m *Model) openAllFolds() {
	m.collapsedNodes = make(map[string]bool)
	m.buildDisplayTree()
}

// collapseFocusedWorkspaceGroups collapses individual plans and child workspaces
func (m *Model) collapseFocusedWorkspaceGroups() {
	if m.focusedWorkspace == nil {
		return
	}

	// If focusing on an ecosystem, collapse all child workspaces
	if m.focusedWorkspace.IsEcosystem() {
		for _, ws := range m.workspaces {
			// Collapse child workspaces (those under this ecosystem)
			isSame, _ := pathutil.ComparePaths(ws.Path, m.focusedWorkspace.Path)
			if !isSame && strings.HasPrefix(strings.ToLower(ws.Path), strings.ToLower(m.focusedWorkspace.Path)+string(filepath.Separator)) {
				wsNodeID := "ws:" + ws.Path
				m.collapsedNodes[wsNodeID] = true
			}
		}
	}

	// Collapse only individual plans (plans/*) for this workspace
	// This keeps "current", "issues", etc. expanded while showing plan names collapsed
	for _, note := range m.allNotes {
		if note.Workspace == m.focusedWorkspace.Name {
			// Only collapse individual plans (anything starting with "plans/")
			if strings.HasPrefix(note.Group, "plans/") {
				groupNodeID := "grp:" + note.Group
				m.collapsedNodes[groupNodeID] = true
			}
		}
	}
}

// getGroupKey returns a unique key for a group node (workspace:groupName)
func (m *Model) getGroupKey(node *displayNode) string {
	return node.workspaceName + ":" + node.groupName
}

// archivePlanDirectory moves a plan directory to the .archive subdirectory
// and returns the paths of all notes that were in the plan directory
func (m *Model) archivePlanDirectory(ctx *service.WorkspaceContext, planGroup string) ([]string, error) {
	// planGroup is like "plans/my-plan"
	// We need to move it to "plans/.archive/my-plan"

	// Get the plans base directory
	plansBaseDir, err := m.service.GetNotebookLocator().GetPlansDir(ctx.NotebookContextWorkspace)
	if err != nil {
		return nil, fmt.Errorf("get plans directory: %w", err)
	}

	// Extract plan name from group (remove "plans/" prefix)
	planName := strings.TrimPrefix(planGroup, "plans/")

	// Source path: plans/<planName>
	sourcePath := filepath.Join(plansBaseDir, planName)

	// Collect paths of notes in this plan directory
	var notePaths []string
	for _, note := range m.allNotes {
		if strings.HasPrefix(note.Path, sourcePath+string(filepath.Separator)) {
			notePaths = append(notePaths, note.Path)
		}
	}

	// Create archive directory if it doesn't exist
	archiveDir := filepath.Join(plansBaseDir, ".archive")
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return nil, fmt.Errorf("create archive directory: %w", err)
	}

	// Destination path: plans/.archive/<planName>
	destPath := filepath.Join(archiveDir, planName)

	// Check if destination already exists, if so append timestamp
	if _, err := os.Stat(destPath); err == nil {
		// Destination exists, create unique name with timestamp
		timestamp := time.Now().Format("20060102150405")
		destPath = filepath.Join(archiveDir, fmt.Sprintf("%s-%s", planName, timestamp))
	}

	// Move the plan directory
	if err := os.Rename(sourcePath, destPath); err != nil {
		return nil, fmt.Errorf("move plan directory: %w", err)
	}

	return notePaths, nil
}

// archiveSelectedNotesCmd creates a command to archive the selected notes and plan groups
func (m *Model) archiveSelectedNotesCmd() tea.Cmd {
	return func() tea.Msg {
		// Group notes by workspace
		notesByWorkspace := make(map[string][]*models.Note)
		for _, note := range m.allNotes {
			if _, ok := m.selected[note.Path]; ok {
				notesByWorkspace[note.Workspace] = append(notesByWorkspace[note.Workspace], note)
			}
		}

		var archivedPaths []string
		var archivedPlans int
		var archiveErr error

		// Archive notes workspace by workspace
		for workspaceName, notes := range notesByWorkspace {
			// Get workspace context
			wsCtx, err := m.service.GetWorkspaceContext(workspaceName)
			if err != nil {
				archiveErr = fmt.Errorf("failed to get workspace context for %s: %w", workspaceName, err)
				break
			}

			// Extract paths from notes
			paths := make([]string, len(notes))
			for i, note := range notes {
				paths[i] = note.Path
			}

			// Archive the notes
			if err := m.service.ArchiveNotes(wsCtx, paths); err != nil {
				archiveErr = fmt.Errorf("failed to archive notes in workspace %s: %w", workspaceName, err)
				break
			}

			archivedPaths = append(archivedPaths, paths...)
		}

		// Archive selected plan groups
		if archiveErr == nil && len(m.selectedGroups) > 0 {
			// Group plans by workspace
			plansByWorkspace := make(map[string][]string)
			for groupKey := range m.selectedGroups {
				parts := strings.SplitN(groupKey, ":", 2)
				if len(parts) == 2 {
					workspaceName := parts[0]
					groupName := parts[1]
					plansByWorkspace[workspaceName] = append(plansByWorkspace[workspaceName], groupName)
				}
			}

			// Archive plans workspace by workspace
			for workspaceName, planNames := range plansByWorkspace {
				// Find the workspace node by name
				var wsNode *workspace.WorkspaceNode
				for _, node := range m.service.GetWorkspaceProvider().All() {
					if node.Name == workspaceName {
						wsNode = node
						break
					}
				}
				if wsNode == nil {
					archiveErr = fmt.Errorf("workspace not found: %s", workspaceName)
					break
				}

				wsCtx, err := m.service.GetWorkspaceContext(wsNode.Path)
				if err != nil {
					archiveErr = fmt.Errorf("failed to get workspace context for %s: %w", workspaceName, err)
					break
				}

				for _, planName := range planNames {
					// Archive the plan directory and collect archived note paths
					planNotePaths, err := m.archivePlanDirectory(wsCtx, planName)
					if err != nil {
						archiveErr = fmt.Errorf("failed to archive plan %s in workspace %s: %w", planName, workspaceName, err)
						break
					}
					archivedPaths = append(archivedPaths, planNotePaths...)
					archivedPlans++
				}

				if archiveErr != nil {
					break
				}
			}
		}

		return notesArchivedMsg{
			archivedPaths: archivedPaths,
			archivedPlans: archivedPlans,
			err:           archiveErr,
		}
	}
}
