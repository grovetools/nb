package browser

import (
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-notebook/pkg/models"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetSize(msg.Width, msg.Height)
		m.table.SetHeight(m.getViewportHeight())
		return m, nil

	case workspacesLoadedMsg:
		m.workspaces = msg.workspaces
		m.buildDisplayTree()
		m.applyFilterAndSort()
		return m, nil

	case notesLoadedMsg:
		m.allNotes = msg.notes
		m.buildDisplayTree()
		m.applyFilterAndSort()
		return m, nil

	case tea.KeyMsg:
		if m.help.ShowAll {
			m.help.Toggle()
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
			if m.focusedWorkspace != nil {
				m.focusedWorkspace = nil
				m.ecosystemPickerMode = false
				m.buildDisplayTree()
				m.applyFilterAndSort()
				m.cursor = 0
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
				m.sortColumn = (m.sortColumn + 1) % 4
				m.applyFilterAndSort()
			}
		case key.Matches(msg, m.keys.ToggleArchives):
			if m.viewMode == treeView {
				m.showArchives = !m.showArchives
				m.buildDisplayTree()
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
			if ws.Path == m.focusedWorkspace.Path || strings.HasPrefix(ws.Path, m.focusedWorkspace.Path+"/") {
				workspacesToShow = append(workspacesToShow, ws)
			}
		}
	} else {
		workspacesToShow = m.workspaces
	}

	// 2. Group notes by workspace path, then by note type
	notesByWorkspace := make(map[string]map[string][]*models.Note)
	for _, note := range m.allNotes {
		if _, ok := notesByWorkspace[note.Workspace]; !ok {
			notesByWorkspace[note.Workspace] = make(map[string][]*models.Note)
		}
		notesByWorkspace[note.Workspace][string(note.Type)] = append(notesByWorkspace[note.Workspace][string(note.Type)], note)
	}

	// 3. Build the display node list and jump map
	m.jumpMap = make(map[rune]int)
	jumpCounter := '1'
	for _, ws := range workspacesToShow {
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
		if m.collapsedNodes[wsNodeID] {
			continue
		}

		if noteGroups, ok := notesByWorkspace[ws.Name]; ok {
			// Separate archive groups from regular groups
			var regularGroups []string
			archiveGroups := make(map[string][]string) // parent -> archive path

			for name := range noteGroups {
				if strings.HasSuffix(name, "/.archive") {
					// This is an archive group - associate it with its parent
					parent := strings.TrimSuffix(name, "/.archive")
					archiveGroups[parent] = append(archiveGroups[parent], name)
				} else {
					regularGroups = append(regularGroups, name)
				}
			}
			sort.Strings(regularGroups)

			for i, groupName := range regularGroups {
				isLastGroup := i == len(regularGroups)-1
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
					isGroup:   true,
					groupName: groupName,
					prefix:    groupPrefix.String(),
					depth:     ws.Depth + 1,
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
							isGroup:   true,
							groupName: ".archive",
							prefix:    archivePrefix.String(),
							depth:     ws.Depth + 2,
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
	// - Footer (help): 1 line
	// - Scroll indicator (when shown): 2 lines (blank + indicator)
	const fixedLines = 7
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
				strings.Contains(strings.ToLower(string(note.Type)), filter) {
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
		case 0: // Workspace
			less = strings.ToLower(a.Workspace) < strings.ToLower(b.Workspace)
		case 1: // Type
			less = strings.ToLower(string(a.Type)) < strings.ToLower(string(b.Type))
		case 2: // Title
			less = strings.ToLower(a.Title) < strings.ToLower(b.Title)
		default: // Modified (case 3)
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
		rows[i] = table.Row{
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
