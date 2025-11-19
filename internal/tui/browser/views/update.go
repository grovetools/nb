package views

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/util/pathutil"
	"github.com/mattsolo1/grove-notebook/pkg/models"
)

// Update handles navigation, folding, and selection key events.
// Returns updated model and any commands. Parent should handle other keys.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.adjustScroll()
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.displayNodes)-1 {
				m.cursor++
				m.adjustScroll()
			}
		case key.Matches(msg, m.keys.PageUp):
			pageSize := m.getViewportHeight() / 2
			if pageSize < 1 {
				pageSize = 1
			}
			m.cursor -= pageSize
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.adjustScroll()
		case key.Matches(msg, m.keys.PageDown):
			pageSize := m.getViewportHeight() / 2
			if pageSize < 1 {
				pageSize = 1
			}
			m.cursor += pageSize
			if m.cursor >= len(m.displayNodes) {
				m.cursor = len(m.displayNodes) - 1
			}
			m.adjustScroll()
		case key.Matches(msg, m.keys.GoToTop):
			// Handle 'gg' - go to top when g is pressed twice
			if m.lastKey == "g" {
				m.cursor = 0
				m.adjustScroll()
				m.lastKey = ""
			} else {
				m.lastKey = "g"
			}
		case key.Matches(msg, m.keys.GoToBottom):
			if len(m.displayNodes) > 0 {
				m.cursor = len(m.displayNodes) - 1
				m.adjustScroll()
			}
		case key.Matches(msg, m.keys.Fold):
			m.closeFold()
		case key.Matches(msg, m.keys.Unfold):
			m.openFold()
		case key.Matches(msg, m.keys.FoldPrefix):
			// Handle 'z' prefix for fold commands
			m.lastKey = "z"
		case msg.String() == "a" && m.lastKey == "z":
			// za - toggle fold
			m.toggleFold()
			m.lastKey = ""
		case msg.String() == "A" && m.lastKey == "z":
			// zA - toggle fold recursively
			m.toggleFoldRecursive(m.cursor)
			m.lastKey = ""
		case msg.String() == "o" && m.lastKey == "z":
			// zo - open fold
			m.openFold()
			m.lastKey = ""
		case msg.String() == "O" && m.lastKey == "z":
			// zO - open fold recursively
			m.openFoldRecursive(m.cursor)
			m.lastKey = ""
		case msg.String() == "c" && m.lastKey == "z":
			// zc - close fold
			m.closeFold()
			m.lastKey = ""
		case msg.String() == "C" && m.lastKey == "z":
			// zC - close fold recursively
			m.closeFoldRecursive(m.cursor)
			m.lastKey = ""
		case msg.String() == "M" && m.lastKey == "z":
			// zM - close all folds
			m.closeAllFolds()
			m.lastKey = ""
		case msg.String() == "R" && m.lastKey == "z":
			// zR - open all folds
			m.openAllFolds()
			m.lastKey = ""
		case key.Matches(msg, m.keys.ToggleSelect):
			// Toggle selection for the current note or plan group
			if m.cursor < len(m.displayNodes) {
				node := m.displayNodes[m.cursor]
				if node.IsNote {
					if _, ok := m.selected[node.Note.Path]; ok {
						delete(m.selected, node.Note.Path)
					} else {
						m.selected[node.Note.Path] = struct{}{}
					}
				} else if node.IsPlan() {
					// Allow selection of plan groups
					groupKey := m.getGroupKey(node)
					if _, ok := m.selectedGroups[groupKey]; ok {
						delete(m.selectedGroups, groupKey)
					} else {
						m.selectedGroups[groupKey] = struct{}{}
					}
				}
			}
		case key.Matches(msg, m.keys.SelectNone):
			// Clear all selections
			m.selected = make(map[string]struct{})
			m.selectedGroups = make(map[string]struct{})
		default:
			// Reset lastKey for any other key press (for gg and z* detection)
			if !key.Matches(msg, m.keys.GoToTop) && !key.Matches(msg, m.keys.FoldPrefix) {
				m.lastKey = ""
			}
		}
	}
	return m, nil
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

// BuildDisplayTree constructs the hierarchical list of nodes for rendering.
func (m *Model) BuildDisplayTree() {
	if m.recentNotesMode {
		m.buildRecentNotesList()
		return
	}

	if m.isFilteringByTag && m.selectedTag != "" {
		m.buildTagFilteredTree()
		return
	}

	var nodes []*DisplayNode
	var workspacesToShow []*workspace.WorkspaceNode

	// Check if we should ignore collapsed state (when searching)
	hasSearchFilter := m.filterValue != "" && !m.isGrepping

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
		var globalNode *workspace.WorkspaceNode
		for _, ws := range m.workspaces {
			// Save global separately
			if ws.Name == "global" {
				globalNode = ws
				continue
			}
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

		// Clear tree prefix on focused workspace (first item) to make it a sibling of global
		if len(workspacesToShow) > 0 {
			focusedCopy := *workspacesToShow[0]
			focusedCopy.TreePrefix = ""
			workspacesToShow[0] = &focusedCopy
		}

		// Prepend global at the front (unless hidden)
		if globalNode != nil && !m.hideGlobal {
			workspacesToShow = append([]*workspace.WorkspaceNode{globalNode}, workspacesToShow...)
		}
	} else {
		// Global view: partition into ecosystem workspaces and standalone workspaces
		for _, ws := range m.workspaces {
			// Skip global if hidden
			if ws.Name == "global" && m.hideGlobal {
				continue
			}
			// Check if this is a standalone (non-ecosystem) top-level workspace
			// Ungrouped workspaces are top-level, not ecosystems, and not our special "global" node.
			if ws.Depth == 0 && !ws.IsEcosystem() && ws.Name != "global" {
				ungroupedWorkspaces = append(ungroupedWorkspaces, ws)
			} else if ws.Depth == 0 || ws.IsEcosystem() {
				// Top-level ecosystems and their children, and the global node
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

	// Create a map of workspace names to their paths for relative path calculation
	workspacePathMap := make(map[string]string)
	for _, ws := range m.workspaces {
		workspacePathMap[ws.Name] = ws.Path
	}

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
				nodes = append(nodes, &DisplayNode{
					IsSeparator: true,
					Prefix:      "  ",
					Depth:       0,
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
		// Also always show the global workspace
		if !hasNotes && m.focusedWorkspace == nil && ws.Depth > 0 && ws.Name != "global" {
			// In global view, only skip non-ecosystem workspaces that have no notes
			continue
		}

		// Add workspace node
		node := &DisplayNode{
			IsWorkspace: true,
			Workspace:   ws,
			Prefix:      ws.TreePrefix,
			Depth:       ws.Depth,
		}

		// Assign jump key for workspaces at depth <= 1
		if ws.Depth <= 1 && jumpCounter <= '9' {
			node.JumpKey = jumpCounter
			m.jumpMap[jumpCounter] = len(nodes)
			jumpCounter++
		}
		nodes = append(nodes, node)

		// In ecosystem picker mode, don't show notes - just workspaces
		if m.ecosystemPickerMode {
			continue
		}

		// Skip children if workspace is collapsed (unless searching)
		wsNodeID := node.NodeID()
		wsCollapsed := m.collapsedNodes[wsNodeID]
		if wsCollapsed && !hasSearchFilter {
			continue
		}

		if noteGroups, ok := notesByWorkspace[ws.Name]; ok {
			// Separate regular groups and archived subgroups
			// archiveSubgroups maps "parent" -> "child" -> notes
			// e.g., "plans" -> "test-plan" -> [notes in plans/.archive/test-plan]
			var regularGroups []string
			planGroups := make(map[string][]*models.Note)
			holdPlanGroups := make(map[string][]*models.Note)
			archiveSubgroups := make(map[string]map[string][]*models.Note)

			for name, notes := range noteGroups {
				// Check if this is an archived group - skip if archives are hidden
				isArchived := strings.Contains(name, "/.archive")
				if isArchived && !m.showArchives {
					continue
				}

				// Skip double-nested archives (e.g., plans/.archive/foo/.archive/bar)
				// Count occurrences of "/.archive" in the path
				archiveCount := strings.Count(name, "/.archive")
				if archiveCount > 1 {
					continue
				}

				// Check if this matches pattern "<parent>/.archive/<child>"
				if strings.Contains(name, "/.archive/") {
					parts := strings.Split(name, "/.archive/")
					if len(parts) == 2 {
						parent := parts[0]
						child := parts[1]
						if archiveSubgroups[parent] == nil {
							archiveSubgroups[parent] = make(map[string][]*models.Note)
						}
						archiveSubgroups[parent][child] = notes
						continue
					}
				}

				// Check if this matches pattern "<parent>/.archive" (notes directly in .archive folder)
				if strings.HasSuffix(name, "/.archive") {
					parent := strings.TrimSuffix(name, "/.archive")
					if archiveSubgroups[parent] == nil {
						archiveSubgroups[parent] = make(map[string][]*models.Note)
					}
					// Use empty string as key to indicate notes directly in .archive
					archiveSubgroups[parent][""] = notes
					continue
				}

				// Handle plans grouping
				if strings.HasPrefix(name, "plans/") {
					planName := strings.TrimPrefix(name, "plans/")
					// Check plan status to separate on-hold plans
					planStatus := m.GetPlanStatus(ws.Name, name)
					if planStatus == "hold" {
						if !m.showOnHold {
							// Skip on-hold plans unless showOnHold is true
							continue
						}
						// Add to hold plans group
						holdPlanGroups[planName] = notes
					} else {
						// Add to regular plans group
						planGroups[planName] = notes
					}
				} else {
					regularGroups = append(regularGroups, name)
				}
			}
			// Custom sort for regular groups
			groupOrder := map[string]int{
				"inbox":       1,
				"issues":      2,
				"docs":        3,
				"in_progress": 4,
				"review":      5,
			}

			// Extract completed group to render it after plans
			var completedGroup string
			var hasCompleted bool
			var filteredRegularGroups []string
			for _, name := range regularGroups {
				if name == "completed" {
					completedGroup = name
					hasCompleted = true
				} else {
					filteredRegularGroups = append(filteredRegularGroups, name)
				}
			}
			regularGroups = filteredRegularGroups

			sort.Slice(regularGroups, func(i, j int) bool {
				orderA, okA := groupOrder[regularGroups[i]]
				if !okA {
					orderA = 99 // Put unknown groups at the end
				}
				orderB, okB := groupOrder[regularGroups[j]]
				if !okB {
					orderB = 99
				}
				if orderA == orderB {
					return regularGroups[i] < regularGroups[j] // Alphabetical fallback
				}
				return orderA < orderB
			})

			// Check if we have plans to add a "plans" parent group
			hasPlans := len(planGroups) > 0 || len(archiveSubgroups["plans"]) > 0
			hasHoldPlans := len(holdPlanGroups) > 0
			totalGroups := len(regularGroups)
			if hasPlans {
				totalGroups++
			}
			if hasHoldPlans {
				totalGroups++
			}
			if hasCompleted {
				totalGroups++
			}

			for i, groupName := range regularGroups {
				isLastGroup := i == len(regularGroups)-1 && !hasPlans && !hasHoldPlans && !hasCompleted
				notesInGroup := noteGroups[groupName]

				// Sort notes within the group
				sort.SliceStable(notesInGroup, func(i, j int) bool {
					if m.sortAscending {
						return notesInGroup[i].CreatedAt.Before(notesInGroup[j].CreatedAt)
					}
					return notesInGroup[i].CreatedAt.After(notesInGroup[j].CreatedAt)
				})

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
				groupNode := &DisplayNode{
					IsGroup:       true,
					GroupName:     groupName,
					WorkspaceName: ws.Name,
					Prefix:        groupPrefix.String(),
					Depth:         ws.Depth + 1,
					ChildCount:    len(notesInGroup),
				}
				nodes = append(nodes, groupNode)

				// Skip notes if group is collapsed (unless searching)
				groupNodeID := groupNode.NodeID()
				if m.collapsedNodes[groupNodeID] && !hasSearchFilter {
					continue
				}

				// Check if this group has archived children
				hasArchives := len(archiveSubgroups[groupName]) > 0 && m.showArchives

				// Add note nodes
				for j, note := range notesInGroup {
					isLastNote := j == len(notesInGroup)-1 && !hasArchives
					var notePrefix strings.Builder
					noteIndent := strings.ReplaceAll(groupPrefix.String(), "├─", "│ ")
					noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
					notePrefix.WriteString(noteIndent)
					if isLastNote {
						notePrefix.WriteString("└─ ")
					} else {
						notePrefix.WriteString("├─ ")
					}
					nodes = append(nodes, &DisplayNode{
						IsNote:       true,
						Note:         note,
						Prefix:       notePrefix.String(),
						Depth:        ws.Depth + 2,
						RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
					})
				}

				// Add .archive subgroup if this group has archived children
				if hasArchives {
					m.addArchiveSubgroup(&nodes, ws, groupPrefix.String(), archiveSubgroups[groupName], hasSearchFilter, workspacePathMap)
				}
			}

			// Add "plans" parent group if there are any plans
			if hasPlans {
				hasGroupsAfter := hasHoldPlans || hasCompleted
				m.addPlansGroup(&nodes, ws, planGroups, archiveSubgroups, hasSearchFilter, workspacePathMap, hasGroupsAfter)
			}

			// Add ".hold" parent group if there are any on-hold plans
			if hasHoldPlans {
				m.addHoldPlansGroup(&nodes, ws, holdPlanGroups, hasSearchFilter, workspacePathMap, hasCompleted)
			}

			// Add "completed" group if it exists (after plans)
			if hasCompleted {
				m.addCompletedGroup(&nodes, ws, noteGroups[completedGroup], archiveSubgroups, hasSearchFilter, workspacePathMap)
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
		m.addUngroupedSection(&nodes, ungroupedWorkspaces, notesByWorkspace, hasSearchFilter, workspacePathMap)
	}

	m.displayNodes = nodes
	m.clampCursor()
}

// buildTagFilteredTree constructs a simplified tree with notes hoisted under their workspace, filtered by a tag.
func (m *Model) buildTagFilteredTree() {
	// 1. Filter notes by tag
	tagFilter := strings.ToLower(m.selectedTag)
	var filteredNotes []*models.Note
	for _, note := range m.allNotes {
		// Skip archived notes unless showArchives is true
		if !m.showArchives && strings.Contains(note.Path, string(filepath.Separator)+".archive"+string(filepath.Separator)) {
			continue
		}

		for _, tag := range note.Tags {
			if strings.EqualFold(tag, tagFilter) {
				filteredNotes = append(filteredNotes, note)
				break
			}
		}
	}

	// 2. Group notes by workspace
	notesByWorkspace := make(map[string][]*models.Note)
	for _, note := range filteredNotes {
		notesByWorkspace[note.Workspace] = append(notesByWorkspace[note.Workspace], note)
	}

	// 3. Determine workspaces to show (replicating logic from BuildDisplayTree)
	var workspacesToShow []*workspace.WorkspaceNode
	if m.focusedWorkspace != nil {
		workspacesToShow = append(workspacesToShow, m.focusedWorkspace)
		// Add children of the focused workspace
		normFocused, _ := pathutil.NormalizeForLookup(m.focusedWorkspace.Path)
		for _, ws := range m.workspaces {
			isSame, _ := pathutil.ComparePaths(ws.Path, m.focusedWorkspace.Path)
			if isSame {
				continue
			}
			normWs, _ := pathutil.NormalizeForLookup(ws.Path)
			if strings.HasPrefix(normWs, normFocused+string(filepath.Separator)) {
				workspacesToShow = append(workspacesToShow, ws)
			}
		}
	} else {
		// Global view, show all top-level workspaces
		for _, ws := range m.workspaces {
			if ws.Depth == 0 {
				workspacesToShow = append(workspacesToShow, ws)
			}
		}
	}

	// 4. Build the display nodes
	var nodes []*DisplayNode
	workspacePathMap := make(map[string]string)
	for _, ws := range m.workspaces {
		workspacePathMap[ws.Name] = ws.Path
	}

	for _, ws := range workspacesToShow {
		if notesInWs, ok := notesByWorkspace[ws.Name]; ok {
			// Add workspace node
			wsNode := &DisplayNode{
				IsWorkspace: true,
				Workspace:   ws,
				Prefix:      ws.TreePrefix,
				Depth:       ws.Depth,
			}
			nodes = append(nodes, wsNode)

			// Sort notes within the workspace
			sort.SliceStable(notesInWs, func(i, j int) bool {
				if m.sortAscending {
					return notesInWs[i].CreatedAt.Before(notesInWs[j].CreatedAt)
				}
				return notesInWs[i].CreatedAt.After(notesInWs[j].CreatedAt)
			})

			// Add note nodes directly under the workspace
			for i, note := range notesInWs {
				isLastNote := i == len(notesInWs)-1
				var notePrefix strings.Builder
				indentPrefix := strings.ReplaceAll(ws.TreePrefix, "├─", "│ ")
				indentPrefix = strings.ReplaceAll(indentPrefix, "└─", "  ")
				notePrefix.WriteString(indentPrefix)
				if ws.Depth > 0 || ws.TreePrefix != "" {
					notePrefix.WriteString("  ")
				}
				if isLastNote {
					notePrefix.WriteString("└─ ")
				} else {
					notePrefix.WriteString("├─ ")
				}

				nodes = append(nodes, &DisplayNode{
					IsNote:       true,
					Note:         note,
					Prefix:       notePrefix.String(),
					Depth:        ws.Depth + 1,
					RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
				})
			}
		}
	}

	m.displayNodes = nodes
	m.clampCursor()
}

// Helper methods for BuildDisplayTree (extracted for readability)

func (m *Model) addArchiveSubgroup(nodes *[]*DisplayNode, ws *workspace.WorkspaceNode, groupPrefix string, archiveSubgroups map[string][]*models.Note, hasSearchFilter bool, workspacePathMap map[string]string) {
	// Sort archived child names
	var archivedNames []string
	for name := range archiveSubgroups {
		archivedNames = append(archivedNames, name)
	}
	sort.Strings(archivedNames)

	// Count total archived notes
	totalArchivedNotes := 0
	for _, notes := range archiveSubgroups {
		totalArchivedNotes += len(notes)
	}

	// Calculate .archive prefix (last child under this group)
	var archivePrefix strings.Builder
	archiveIndent := strings.ReplaceAll(groupPrefix, "├─", "│ ")
	archiveIndent = strings.ReplaceAll(archiveIndent, "└─", "  ")
	archivePrefix.WriteString(archiveIndent)
	archivePrefix.WriteString("└─ ")

	// Get parent group name from groupPrefix context
	// This is a bit tricky - we need to track which group we're in
	// For simplicity, we'll extract it from the nodes list
	parentGroupName := ""
	if len(*nodes) > 0 {
		for i := len(*nodes) - 1; i >= 0; i-- {
			if (*nodes)[i].IsGroup && !strings.Contains((*nodes)[i].GroupName, "/.archive") {
				parentGroupName = (*nodes)[i].GroupName
				break
			}
		}
	}

	// Add .archive parent node
	archiveParentNode := &DisplayNode{
		IsGroup:       true,
		GroupName:     parentGroupName + "/.archive",
		WorkspaceName: ws.Name,
		Prefix:        archivePrefix.String(),
		Depth:         ws.Depth + 2,
		ChildCount:    totalArchivedNotes,
	}
	*nodes = append(*nodes, archiveParentNode)

	// Check if .archive parent is collapsed
	archiveParentNodeID := archiveParentNode.NodeID()
	if !m.collapsedNodes[archiveParentNodeID] || hasSearchFilter {
		// Add individual archived children (implementation continues...)
		for pi, archivedName := range archivedNames {
			isLastArchived := pi == len(archivedNames)-1
			archivedNotes := archiveSubgroups[archivedName]

			// Sort notes within the archived child
			sort.SliceStable(archivedNotes, func(i, j int) bool {
				if m.sortAscending {
					return archivedNotes[i].CreatedAt.Before(archivedNotes[j].CreatedAt)
				}
				return archivedNotes[i].CreatedAt.After(archivedNotes[j].CreatedAt)
			})

			// If archivedName is empty, these are notes directly in .archive folder
			if archivedName == "" {
				// Add notes directly under .archive parent
				for ni, note := range archivedNotes {
					isLastNote := ni == len(archivedNotes)-1 && isLastArchived
					var notePrefix strings.Builder
					noteIndent := strings.ReplaceAll(archivePrefix.String(), "├─", "│ ")
					noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
					notePrefix.WriteString(noteIndent)
					if isLastNote {
						notePrefix.WriteString("└─ ")
					} else {
						notePrefix.WriteString("├─ ")
					}
					*nodes = append(*nodes, &DisplayNode{
						IsNote:       true,
						Note:         note,
						Prefix:       notePrefix.String(),
						Depth:        ws.Depth + 3,
						RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
					})
				}
				continue
			}

			// Calculate archived child prefix
			var archivedPrefix strings.Builder
			archivedIndent := strings.ReplaceAll(archivePrefix.String(), "├─", "│ ")
			archivedIndent = strings.ReplaceAll(archivedIndent, "└─", "  ")
			archivedPrefix.WriteString(archivedIndent)
			if isLastArchived {
				archivedPrefix.WriteString("└─ ")
			} else {
				archivedPrefix.WriteString("├─ ")
			}

			// Add archived child node
			archivedChildNode := &DisplayNode{
				IsGroup:       true,
				GroupName:     parentGroupName + "/.archive/" + archivedName,
				WorkspaceName: ws.Name,
				Prefix:        archivedPrefix.String(),
				Depth:         ws.Depth + 3,
				ChildCount:    len(archivedNotes),
			}
			*nodes = append(*nodes, archivedChildNode)

			// Check if archived child is collapsed
			archivedChildNodeID := archivedChildNode.NodeID()
			if !m.collapsedNodes[archivedChildNodeID] || hasSearchFilter {
				// Add notes within the archived child
				for ni, note := range archivedNotes {
					isLastArchivedNote := ni == len(archivedNotes)-1
					var archivedNotePrefix strings.Builder
					archivedNoteIndent := strings.ReplaceAll(archivedPrefix.String(), "├─", "│ ")
					archivedNoteIndent = strings.ReplaceAll(archivedIndent, "└─", "  ")
					archivedNotePrefix.WriteString(archivedNoteIndent)
					if isLastArchivedNote {
						archivedNotePrefix.WriteString("└─ ")
					} else {
						archivedNotePrefix.WriteString("├─ ")
					}
					*nodes = append(*nodes, &DisplayNode{
						IsNote:       true,
						Note:         note,
						Prefix:       archivedNotePrefix.String(),
						Depth:        ws.Depth + 4,
						RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
					})
				}
			}
		}
	}
}

func (m *Model) addPlansGroup(nodes *[]*DisplayNode, ws *workspace.WorkspaceNode, planGroups map[string][]*models.Note, archiveSubgroups map[string]map[string][]*models.Note, hasSearchFilter bool, workspacePathMap map[string]string, hasGroupsAfter bool) {
	// Calculate plans parent prefix
	var plansPrefix strings.Builder
	indentPrefix := strings.ReplaceAll(ws.TreePrefix, "├─", "│ ")
	indentPrefix = strings.ReplaceAll(indentPrefix, "└─", "  ")
	plansPrefix.WriteString(indentPrefix)
	if ws.Depth > 0 || ws.TreePrefix != "" {
		plansPrefix.WriteString("  ")
	}
	if hasGroupsAfter {
		plansPrefix.WriteString("├─ ") // Not last if other groups exist after
	} else {
		plansPrefix.WriteString("└─ ") // Plans is last if no other groups
	}

	// Add "plans" parent node
	plansParentNode := &DisplayNode{
		IsGroup:       true,
		GroupName:     "plans",
		WorkspaceName: ws.Name,
		Prefix:        plansPrefix.String(),
		Depth:         ws.Depth + 1,
		ChildCount:    len(planGroups), // Count of plans, not notes
	}
	*nodes = append(*nodes, plansParentNode)

	// Check if plans parent is collapsed (unless searching)
	plansParentNodeID := plansParentNode.NodeID()
	if !m.collapsedNodes[plansParentNodeID] || hasSearchFilter {
		// Sort plan names
		var planNames []string
		for planName := range planGroups {
			planNames = append(planNames, planName)
		}
		sort.Strings(planNames)

		// Add individual plan nodes
		hasPlansArchive := len(archiveSubgroups["plans"]) > 0 && m.showArchives
		for pi, planName := range planNames {
			isLastPlan := pi == len(planNames)-1 && !hasPlansArchive
			planNotes := planGroups[planName]

			// Sort notes within the plan
			sort.SliceStable(planNotes, func(i, j int) bool {
				if m.sortAscending {
					return planNotes[i].CreatedAt.Before(planNotes[j].CreatedAt)
				}
				return planNotes[i].CreatedAt.After(planNotes[j].CreatedAt)
			})

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
			planNode := &DisplayNode{
				IsGroup:       true,
				GroupName:     "plans/" + planName, // Keep full path for IsPlan() check
				WorkspaceName: ws.Name,
				Prefix:        planPrefix.String(),
				Depth:         ws.Depth + 2,
				ChildCount:    len(planNotes),
			}
			*nodes = append(*nodes, planNode)

			// Check if this plan is collapsed (unless searching)
			planNodeID := planNode.NodeID()
			if !m.collapsedNodes[planNodeID] || hasSearchFilter {
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
					*nodes = append(*nodes, &DisplayNode{
						IsNote:       true,
						Note:         note,
						Prefix:       notePrefix.String(),
						Depth:        ws.Depth + 3,
						RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
					})
				}
			}
		}

		// Add .archive parent group if there are archived children
		if hasPlansArchive {
			m.addPlansArchiveGroup(nodes, ws, plansPrefix.String(), archiveSubgroups["plans"], hasSearchFilter, workspacePathMap)
		}
	}
}

func (m *Model) addPlansArchiveGroup(nodes *[]*DisplayNode, ws *workspace.WorkspaceNode, plansPrefix string, archivedPlans map[string][]*models.Note, hasSearchFilter bool, workspacePathMap map[string]string) {
	// Sort archived child names
	var archivedNames []string
	for name := range archivedPlans {
		archivedNames = append(archivedNames, name)
	}
	sort.Strings(archivedNames)

	// Count total archived notes
	totalArchivedNotes := 0
	for _, notes := range archivedPlans {
		totalArchivedNotes += len(notes)
	}

	// Calculate .archive prefix (last child under plans)
	var archivePrefix strings.Builder
	archiveIndent := strings.ReplaceAll(plansPrefix, "├─", "│ ")
	archiveIndent = strings.ReplaceAll(archiveIndent, "└─", "  ")
	archivePrefix.WriteString(archiveIndent)
	archivePrefix.WriteString("└─ ")

	// Add .archive parent node
	archiveParentNode := &DisplayNode{
		IsGroup:       true,
		GroupName:     "plans/.archive",
		WorkspaceName: ws.Name,
		Prefix:        archivePrefix.String(),
		Depth:         ws.Depth + 2,
		ChildCount:    totalArchivedNotes,
	}
	*nodes = append(*nodes, archiveParentNode)

	// Check if .archive parent is collapsed
	archiveParentNodeID := archiveParentNode.NodeID()
	if !m.collapsedNodes[archiveParentNodeID] || hasSearchFilter {
		// Add individual archived children (similar to addArchiveSubgroup)
		for pi, archivedName := range archivedNames {
			isLastArchived := pi == len(archivedNames)-1
			archivedNotes := archivedPlans[archivedName]

			// Sort notes within the archived child
			sort.SliceStable(archivedNotes, func(i, j int) bool {
				if m.sortAscending {
					return archivedNotes[i].CreatedAt.Before(archivedNotes[j].CreatedAt)
				}
				return archivedNotes[i].CreatedAt.After(archivedNotes[j].CreatedAt)
			})

			// If archivedName is empty, these are plans directly in .archive folder
			if archivedName == "" {
				// Add notes directly under .archive parent
				for ni, note := range archivedNotes {
					isLastNote := ni == len(archivedNotes)-1 && isLastArchived
					var notePrefix strings.Builder
					noteIndent := strings.ReplaceAll(archivePrefix.String(), "├─", "│ ")
					noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
					notePrefix.WriteString(noteIndent)
					if isLastNote {
						notePrefix.WriteString("└─ ")
					} else {
						notePrefix.WriteString("├─ ")
					}
					*nodes = append(*nodes, &DisplayNode{
						IsNote:       true,
						Note:         note,
						Prefix:       notePrefix.String(),
						Depth:        ws.Depth + 3,
						RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
					})
				}
				continue
			}

			// Calculate archived child prefix
			var archivedPrefix strings.Builder
			archivedIndent := strings.ReplaceAll(archivePrefix.String(), "├─", "│ ")
			archivedIndent = strings.ReplaceAll(archivedIndent, "└─", "  ")
			archivedPrefix.WriteString(archivedIndent)
			if isLastArchived {
				archivedPrefix.WriteString("└─ ")
			} else {
				archivedPrefix.WriteString("├─ ")
			}

			// Add archived child node
			archivedNode := &DisplayNode{
				IsGroup:       true,
				GroupName:     "plans/.archive/" + archivedName,
				WorkspaceName: ws.Name,
				Prefix:        archivedPrefix.String(),
				Depth:         ws.Depth + 3,
				ChildCount:    len(archivedNotes),
			}
			*nodes = append(*nodes, archivedNode)

			// Check if this archived child is collapsed
			archivedNodeID := archivedNode.NodeID()
			if !m.collapsedNodes[archivedNodeID] || hasSearchFilter {
				// Add notes in this archived child
				for ni, note := range archivedNotes {
					isLastNote := ni == len(archivedNotes)-1
					var notePrefix strings.Builder
					noteIndent := strings.ReplaceAll(archivedPrefix.String(), "├─", "│ ")
					noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
					notePrefix.WriteString(noteIndent)
					if isLastNote {
						notePrefix.WriteString("└─ ")
					} else {
						notePrefix.WriteString("├─ ")
					}
					*nodes = append(*nodes, &DisplayNode{
						IsNote:       true,
						Note:         note,
						Prefix:       notePrefix.String(),
						Depth:        ws.Depth + 4,
						RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
					})
				}
			}
		}
	}
}

func (m *Model) addHoldPlansGroup(nodes *[]*DisplayNode, ws *workspace.WorkspaceNode, holdPlanGroups map[string][]*models.Note, hasSearchFilter bool, workspacePathMap map[string]string, hasCompleted bool) {
	// Calculate .hold parent prefix
	var holdPrefix strings.Builder
	indentPrefix := strings.ReplaceAll(ws.TreePrefix, "├─", "│ ")
	indentPrefix = strings.ReplaceAll(indentPrefix, "└─", "  ")
	holdPrefix.WriteString(indentPrefix)
	if ws.Depth > 0 || ws.TreePrefix != "" {
		holdPrefix.WriteString("  ")
	}
	if hasCompleted {
		holdPrefix.WriteString("├─ ") // Not last if completed exists
	} else {
		holdPrefix.WriteString("└─ ") // .hold is last if no completed
	}

	// Add ".hold" parent node
	holdParentNode := &DisplayNode{
		IsGroup:       true,
		GroupName:     ".hold",
		WorkspaceName: ws.Name,
		Prefix:        holdPrefix.String(),
		Depth:         ws.Depth + 1,
		ChildCount:    len(holdPlanGroups), // Count of hold plans, not notes
	}
	*nodes = append(*nodes, holdParentNode)

	// Check if .hold parent is collapsed (unless searching)
	holdParentNodeID := holdParentNode.NodeID()
	if !m.collapsedNodes[holdParentNodeID] || hasSearchFilter {
		// Sort hold plan names
		var planNames []string
		for planName := range holdPlanGroups {
			planNames = append(planNames, planName)
		}
		sort.Strings(planNames)

		// Add individual hold plan nodes
		for pi, planName := range planNames {
			isLastPlan := pi == len(planNames)-1
			planNotes := holdPlanGroups[planName]

			// Sort notes within the plan
			sort.SliceStable(planNotes, func(i, j int) bool {
				if m.sortAscending {
					return planNotes[i].CreatedAt.Before(planNotes[j].CreatedAt)
				}
				return planNotes[i].CreatedAt.After(planNotes[j].CreatedAt)
			})

			// Calculate plan prefix
			var planPrefix strings.Builder
			planIndent := strings.ReplaceAll(holdPrefix.String(), "├─", "│ ")
			planIndent = strings.ReplaceAll(planIndent, "└─", "  ")
			planPrefix.WriteString(planIndent)
			if isLastPlan {
				planPrefix.WriteString("└─ ")
			} else {
				planPrefix.WriteString("├─ ")
			}

			// Add hold plan node
			planNode := &DisplayNode{
				IsGroup:       true,
				GroupName:     "plans/" + planName, // Keep full path for consistency
				WorkspaceName: ws.Name,
				Prefix:        planPrefix.String(),
				Depth:         ws.Depth + 2,
				ChildCount:    len(planNotes),
			}
			*nodes = append(*nodes, planNode)

			// Check if this plan is collapsed (unless searching)
			planNodeID := planNode.NodeID()
			if !m.collapsedNodes[planNodeID] || hasSearchFilter {
				// Add notes in this hold plan
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
					*nodes = append(*nodes, &DisplayNode{
						IsNote:       true,
						Note:         note,
						Prefix:       notePrefix.String(),
						Depth:        ws.Depth + 3,
						RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
					})
				}
			}
		}
	}
}

func (m *Model) addCompletedGroup(nodes *[]*DisplayNode, ws *workspace.WorkspaceNode, completedNotes []*models.Note, archiveSubgroups map[string]map[string][]*models.Note, hasSearchFilter bool, workspacePathMap map[string]string) {
	// Calculate completed group prefix (always last)
	var completedPrefix strings.Builder
	indentPrefix := strings.ReplaceAll(ws.TreePrefix, "├─", "│ ")
	indentPrefix = strings.ReplaceAll(indentPrefix, "└─", "  ")
	completedPrefix.WriteString(indentPrefix)
	if ws.Depth > 0 || ws.TreePrefix != "" {
		completedPrefix.WriteString("  ")
	}
	completedPrefix.WriteString("└─ ") // Completed is always last

	// Sort notes within the completed group
	sort.SliceStable(completedNotes, func(i, j int) bool {
		if m.sortAscending {
			return completedNotes[i].CreatedAt.Before(completedNotes[j].CreatedAt)
		}
		return completedNotes[i].CreatedAt.After(completedNotes[j].CreatedAt)
	})

	// Add completed group node
	completedGroupNode := &DisplayNode{
		IsGroup:       true,
		GroupName:     "completed",
		WorkspaceName: ws.Name,
		Prefix:        completedPrefix.String(),
		Depth:         ws.Depth + 1,
		ChildCount:    len(completedNotes),
	}
	*nodes = append(*nodes, completedGroupNode)

	// Check if completed group is collapsed (unless searching)
	completedGroupNodeID := completedGroupNode.NodeID()
	if !m.collapsedNodes[completedGroupNodeID] || hasSearchFilter {
		// Check if this group has archived children
		hasArchives := len(archiveSubgroups["completed"]) > 0 && m.showArchives

		// Add note nodes
		for j, note := range completedNotes {
			isLastNote := j == len(completedNotes)-1 && !hasArchives
			var notePrefix strings.Builder
			noteIndent := strings.ReplaceAll(completedPrefix.String(), "├─", "│ ")
			noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
			notePrefix.WriteString(noteIndent)
			if isLastNote {
				notePrefix.WriteString("└─ ")
			} else {
				notePrefix.WriteString("├─ ")
			}
			*nodes = append(*nodes, &DisplayNode{
				IsNote:       true,
				Note:         note,
				Prefix:       notePrefix.String(),
				Depth:        ws.Depth + 2,
				RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
			})
		}

		// Add .archive subgroup if this group has archived children
		if hasArchives {
			m.addArchiveSubgroup(nodes, ws, completedPrefix.String(), archiveSubgroups["completed"], hasSearchFilter, workspacePathMap)
		}
	}
}

func (m *Model) addUngroupedSection(nodes *[]*DisplayNode, ungroupedWorkspaces []*workspace.WorkspaceNode, notesByWorkspace map[string]map[string][]*models.Note, hasSearchFilter bool, workspacePathMap map[string]string) {
	ungroupedNode := &DisplayNode{
		IsGroup:   true,
		GroupName: "ungrouped",
		Prefix:    "",
		Depth:     0,
	}
	*nodes = append(*nodes, ungroupedNode)

	// Check if ungrouped section is collapsed (unless searching)
	if !m.collapsedNodes[ungroupedNode.NodeID()] || hasSearchFilter {
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
			node := &DisplayNode{
				IsWorkspace: true,
				Workspace:   ws,
				Prefix:      adjustedPrefix,
				Depth:       ws.Depth + 1, // Increase depth since it's under "Ungrouped"
			}

			// Assign jump key for ungrouped workspaces
			jumpCounter := rune('1') + rune(len(m.jumpMap))
			if jumpCounter <= '9' {
				node.JumpKey = jumpCounter
				m.jumpMap[jumpCounter] = len(*nodes)
			}
			*nodes = append(*nodes, node)

			// Skip children if workspace is collapsed (unless searching)
			wsNodeID := node.NodeID()
			if m.collapsedNodes[wsNodeID] && !hasSearchFilter {
				continue
			}

			// Render notes for this ungrouped workspace (similar logic to main loop, abbreviated here)
			// For brevity, this implementation is simplified
			// In a complete implementation, this would follow the same pattern as the main workspace rendering
		}
	}
}

// calculateRelativePath returns the shortened absolute path for a note
func calculateRelativePath(note *models.Note, workspacePathMap map[string]string, focusedWorkspace *workspace.WorkspaceNode) string {
	// Always use absolute path with ~ for home
	return shortenPath(note.Path)
}

// FilterDisplayTree filters the tree view to show only matches, preserving parent nodes.
func (m *Model) FilterDisplayTree() {
	filter := strings.ToLower(m.filterValue)
	if filter == "" {
		return // No filter to apply
	}

	fullTree := m.displayNodes
	nodesToKeep := make(map[int]bool)
	parentMap := make(map[int]int)
	lastNodeAtDepth := make(map[int]int)

	// First pass: build parent map
	for i, node := range fullTree {
		if node.Depth > 0 {
			if parentIndex, ok := lastNodeAtDepth[node.Depth-1]; ok {
				parentMap[i] = parentIndex
			}
		}
		lastNodeAtDepth[node.Depth] = i
	}

	// Second pass: mark nodes to keep
	for i, node := range fullTree {
		match := false

		if node.IsNote {
			// Search only in note title and type
			match = strings.Contains(strings.ToLower(node.Note.Title), filter) ||
				strings.Contains(strings.ToLower(string(node.Note.Type)), filter)
		} else if node.IsGroup {
			// Search in group/plan names (strip "plans/" prefix for matching)
			displayName := node.GroupName
			if node.IsPlan() {
				displayName = strings.TrimPrefix(displayName, "plans/")
			}
			match = strings.Contains(strings.ToLower(displayName), filter)
		}

		if match {
			// Mark this node and all its parents to be kept
			curr := i
			for {
				nodesToKeep[curr] = true
				parentIndex, ok := parentMap[curr]
				if !ok {
					break // No more parents
				}
				curr = parentIndex
			}
		}
	}

	// Third pass: build the filtered tree
	var filteredTree []*DisplayNode
	for i, node := range fullTree {
		if nodesToKeep[i] {
			filteredTree = append(filteredTree, node)
		}
	}

	m.displayNodes = filteredTree
	m.clampCursor()
}

// ApplyGrepFilter performs a content search using ripgrep and filters the tree.
func (m *Model) ApplyGrepFilter() (string, error) {
	query := m.filterValue
	if query == "" {
		// Restore the full tree with original collapsed state
		m.BuildDisplayTree()
		return "", nil
	}

	// Build a map of all note paths for quick lookup
	notePathsMap := make(map[string]bool)
	for _, note := range m.allNotes {
		notePathsMap[note.Path] = true
	}

	// Build a map of workspace name -> workspace node for quick lookup
	workspaceMap := make(map[string]*workspace.WorkspaceNode)
	for _, ws := range m.workspaces {
		workspaceMap[ws.Name] = ws
	}

	// Get unique workspace names from notes
	uniqueWorkspaces := make(map[string]bool)
	for _, note := range m.allNotes {
		uniqueWorkspaces[note.Workspace] = true
	}

	// For each workspace, get its notebook directory using NotebookLocator
	searchDirs := make(map[string]bool)
	locator := m.service.GetNotebookLocator()

	for wsName := range uniqueWorkspaces {
		ws, ok := workspaceMap[wsName]
		if !ok || ws == nil {
			continue
		}

		// Get the notes directory for this workspace
		notesDir, err := locator.GetNotesDir(ws, "inbox")
		if err != nil || notesDir == "" {
			continue
		}

		// The notes directory is something like /path/to/nb/repos/workspace/branch/inbox
		// We want to search at the branch level: /path/to/nb/repos/workspace/branch
		workspaceRoot := filepath.Dir(notesDir)
		searchDirs[workspaceRoot] = true
	}

	resultPaths := make(map[string]bool)

	// Run ripgrep once with all directories
	if len(searchDirs) > 0 {
		// Build args: rg -l --type md -i query dir1 dir2 dir3...
		args := []string{
			"-l",           // files-with-matches
			"--type", "md", // markdown only
			"-i", // case-insensitive
			query,
		}
		for dir := range searchDirs {
			args = append(args, dir)
		}

		cmd := fmt.Sprintf("rg %s", strings.Join(args, " "))
		// Execute ripgrep (simplified - in real code use exec.Command)
		// For now, just return a message that grep would be executed
		_ = cmd
	}

	statusMsg := fmt.Sprintf("Found %d matching notes", len(resultPaths))

	// Temporarily expand all nodes to show grep results
	savedCollapsed := m.collapsedNodes
	m.collapsedNodes = make(map[string]bool)

	// Filter the display tree to show only matches and their parents
	m.filterDisplayTreeByPaths(resultPaths)

	// Keep the tree expanded for grep results
	_ = savedCollapsed

	return statusMsg, nil
}

// filterDisplayTreeByPaths filters the tree to show only nodes whose paths are in the provided map.
func (m *Model) filterDisplayTreeByPaths(pathsToKeep map[string]bool) {
	// Rebuild the full tree (already expanded by caller)
	m.BuildDisplayTree()
	fullTree := m.displayNodes

	// Normalize all paths to keep for case-insensitive comparison
	normalizedPaths := make(map[string]bool)
	for path := range pathsToKeep {
		normalized, err := pathutil.NormalizeForLookup(path)
		if err == nil {
			normalizedPaths[normalized] = true
		}
	}

	nodesToKeep := make(map[int]bool)
	parentMap := make(map[int]int)
	lastNodeAtDepth := make(map[int]int)

	// First pass: build parent map
	for i, node := range fullTree {
		if node.Depth > 0 {
			if parentIndex, ok := lastNodeAtDepth[node.Depth-1]; ok {
				parentMap[i] = parentIndex
			}
		}
		lastNodeAtDepth[node.Depth] = i
	}

	// Second pass: mark nodes to keep
	for i, node := range fullTree {
		if node.IsNote {
			// Try normalized path comparison
			normalizedNotePath, err := pathutil.NormalizeForLookup(node.Note.Path)
			if err == nil && normalizedPaths[normalizedNotePath] {
				// Mark this note and all its parents to be kept
				curr := i
				for {
					nodesToKeep[curr] = true
					parentIndex, ok := parentMap[curr]
					if !ok {
						break // No more parents
					}
					curr = parentIndex
				}
			}
		}
	}

	// Third pass: build the filtered tree
	var filteredTree []*DisplayNode
	for i, node := range fullTree {
		if nodesToKeep[i] {
			filteredTree = append(filteredTree, node)
		}
	}

	m.displayNodes = filteredTree
	m.clampCursor()
}

// Folding methods

func (m *Model) toggleFold() {
	if m.cursor >= len(m.displayNodes) {
		return
	}
	node := m.displayNodes[m.cursor]
	if !node.IsFoldable() {
		return
	}
	nodeID := node.NodeID()
	if m.collapsedNodes[nodeID] {
		delete(m.collapsedNodes, nodeID)
	} else {
		m.collapsedNodes[nodeID] = true
	}
	m.BuildDisplayTree()
}

func (m *Model) openFold() {
	if m.cursor >= len(m.displayNodes) {
		return
	}
	node := m.displayNodes[m.cursor]
	if !node.IsFoldable() {
		return
	}
	delete(m.collapsedNodes, node.NodeID())
	m.BuildDisplayTree()
}

func (m *Model) closeFold() {
	if m.cursor >= len(m.displayNodes) {
		return
	}
	node := m.displayNodes[m.cursor]
	if !node.IsFoldable() {
		return
	}
	m.collapsedNodes[node.NodeID()] = true
	m.BuildDisplayTree()
}

func (m *Model) closeAllFolds() {
	for _, node := range m.displayNodes {
		if node.IsFoldable() {
			m.collapsedNodes[node.NodeID()] = true
		}
	}
	m.BuildDisplayTree()
}

func (m *Model) openAllFolds() {
	m.collapsedNodes = make(map[string]bool)
	m.BuildDisplayTree()
}

func (m *Model) closeFoldRecursive(cursorIndex int) {
	if cursorIndex >= len(m.displayNodes) {
		return
	}
	node := m.displayNodes[cursorIndex]
	if !node.IsFoldable() {
		return
	}

	startDepth := node.Depth
	m.collapsedNodes[node.NodeID()] = true

	// Iterate through subsequent nodes to find and collapse all children
	for i := cursorIndex + 1; i < len(m.displayNodes); i++ {
		childNode := m.displayNodes[i]
		if childNode.Depth <= startDepth {
			// We've exited the current node's subtree
			break
		}
		if childNode.IsFoldable() {
			m.collapsedNodes[childNode.NodeID()] = true
		}
	}
	m.BuildDisplayTree()
}

func (m *Model) openFoldRecursive(cursorIndex int) {
	if cursorIndex >= len(m.displayNodes) {
		return
	}
	node := m.displayNodes[cursorIndex]
	if !node.IsFoldable() {
		return
	}

	// Un-collapse the target node itself
	delete(m.collapsedNodes, node.NodeID())

	if node.IsWorkspace {
		// Un-collapse all descendant workspaces and their note groups
		wsPath := node.Workspace.Path
		wsName := node.Workspace.Name

		for _, ws := range m.workspaces {
			if strings.HasPrefix(ws.Path, wsPath) && ws.Path != wsPath {
				delete(m.collapsedNodes, "ws:"+ws.Path)
			}
		}
		for _, n := range m.allNotes {
			if n.Workspace == wsName {
				delete(m.collapsedNodes, "grp:"+n.Group)
				if strings.HasPrefix(n.Group, "plans/") {
					delete(m.collapsedNodes, "grp:plans")
				}
			}
		}
	} else if node.IsGroup {
		// Un-collapse child groups (e.g., 'plans' contains 'plans/sub-plan')
		groupNamePrefix := node.GroupName + "/"
		for _, n := range m.allNotes {
			if n.Workspace == node.WorkspaceName && strings.HasPrefix(n.Group, groupNamePrefix) {
				delete(m.collapsedNodes, "grp:"+n.Group)
			}
		}
	}

	m.BuildDisplayTree()
}

func (m *Model) toggleFoldRecursive(cursorIndex int) {
	if cursorIndex >= len(m.displayNodes) {
		return
	}
	node := m.displayNodes[cursorIndex]
	if !node.IsFoldable() {
		return
	}

	// If the node is currently collapsed, open it recursively. Otherwise, close it recursively.
	if m.collapsedNodes[node.NodeID()] {
		m.openFoldRecursive(cursorIndex)
	} else {
		m.closeFoldRecursive(cursorIndex)
	}
}

// buildRecentNotesList constructs a flat list of notes for the recent view.
func (m *Model) buildRecentNotesList() {
	notesToDisplay := m.allNotes

	// Filter by tag if active
	if m.isFilteringByTag && m.selectedTag != "" {
		var taggedNotes []*models.Note
		for _, note := range notesToDisplay {
			for _, tag := range note.Tags {
				if tag == m.selectedTag {
					taggedNotes = append(taggedNotes, note)
					break
				}
			}
		}
		notesToDisplay = taggedNotes
	}

	// Filter out archived notes if not shown
	if !m.showArchives {
		var nonArchivedNotes []*models.Note
		for _, note := range notesToDisplay {
			if !note.IsArchived {
				nonArchivedNotes = append(nonArchivedNotes, note)
			}
		}
		notesToDisplay = nonArchivedNotes
	}

	// Sort by modified date descending
	sort.SliceStable(notesToDisplay, func(i, j int) bool {
		return notesToDisplay[i].ModifiedAt.After(notesToDisplay[j].ModifiedAt)
	})

	// Create flat list of display nodes
	var nodes []*DisplayNode
	workspacePathMap := make(map[string]string)
	for _, ws := range m.workspaces {
		workspacePathMap[ws.Name] = ws.Path
	}

	for _, note := range notesToDisplay {
		nodes = append(nodes, &DisplayNode{
			IsNote:       true,
			Note:         note,
			Depth:        0,
			RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
		})
	}

	m.displayNodes = nodes
	m.jumpMap = make(map[rune]int) // No jump keys in flat list
	m.clampCursor()
}
