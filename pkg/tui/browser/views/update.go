package views

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/core/tui/theme"
	"github.com/grovetools/core/util/pathutil"

	"github.com/grovetools/nb/pkg/models"
	"github.com/grovetools/nb/pkg/tree"
)

// groupTreeNode is a helper struct to build an in-memory tree of group paths.
type groupTreeNode struct {
	name      string
	fullName  string // The full relative path for this node, e.g., "architecture/decisions"
	children  map[string]*groupTreeNode
	notes     []*models.Note
	childKeys []string // For sorted iteration
}

func newGroupTreeNode(name, fullName string) *groupTreeNode {
	return &groupTreeNode{
		name:     name,
		fullName: fullName,
		children: make(map[string]*groupTreeNode),
	}
}

// treeRenderConfig holds configuration for the generic renderTree function.
type treeRenderConfig struct {
	itemType            tree.ItemType
	groupMetadataPrefix string
	nameUsesPrefix      bool
	includeArtifacts    bool
	includeArchives     bool
	includeClosed       bool
}

// Update handles navigation, folding, and selection key events.
// Returns updated model and any commands. Parent should handle other keys.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Define bindings that use sequences
		sequenceBindings := []key.Binding{
			m.keys.Top,          // gg
			m.keys.FoldOpen,     // zo
			m.keys.FoldClose,    // zc
			m.keys.FoldToggle,   // za
			m.keys.FoldOpenAll,  // zR
			m.keys.FoldCloseAll, // zM
		}

		// Process the key through sequence state
		result, _ := m.sequence.Process(msg, sequenceBindings...)

		// Get the current buffer for checking fold sequences
		buffer := m.sequence.Buffer()

		switch {
		case key.Matches(msg, m.keys.Up):
			m.sequence.Clear()
			if m.cursor > 0 {
				m.cursor--
				m.adjustScroll()
			}
		case key.Matches(msg, m.keys.Down):
			m.sequence.Clear()
			if m.cursor < len(m.displayNodes)-1 {
				m.cursor++
				m.adjustScroll()
			}
		case key.Matches(msg, m.keys.PageUp):
			m.sequence.Clear()
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
			m.sequence.Clear()
			pageSize := m.getViewportHeight() / 2
			if pageSize < 1 {
				pageSize = 1
			}
			m.cursor += pageSize
			if m.cursor >= len(m.displayNodes) {
				m.cursor = len(m.displayNodes) - 1
			}
			m.adjustScroll()
		case result == keymap.SequenceMatch && keymap.Matches(buffer, m.keys.Top):
			// gg - go to top
			m.cursor = 0
			m.adjustScroll()
			m.sequence.Clear()
		case key.Matches(msg, m.keys.Bottom):
			m.sequence.Clear()
			if len(m.displayNodes) > 0 {
				m.cursor = len(m.displayNodes) - 1
				m.adjustScroll()
			}
		case key.Matches(msg, m.keys.Left):
			m.sequence.Clear()
			m.closeFold()
		case key.Matches(msg, m.keys.Right):
			m.sequence.Clear()
			m.openFold()
		// Fold sequence commands (z*)
		case result == keymap.SequenceMatch && keymap.Matches(buffer, m.keys.FoldToggle):
			m.toggleFold()
			m.sequence.Clear()
		case buffer == "zA":
			m.toggleFoldRecursive(m.cursor)
			m.sequence.Clear()
		case result == keymap.SequenceMatch && keymap.Matches(buffer, m.keys.FoldOpen):
			m.openFold()
			m.sequence.Clear()
		case buffer == "zO":
			m.openFoldRecursive(m.cursor)
			m.sequence.Clear()
		case result == keymap.SequenceMatch && keymap.Matches(buffer, m.keys.FoldClose):
			m.closeFold()
			m.sequence.Clear()
		case buffer == "zC":
			m.closeFoldRecursive(m.cursor)
			m.sequence.Clear()
		case result == keymap.SequenceMatch && keymap.Matches(buffer, m.keys.FoldCloseAll):
			m.closeAllFolds()
			m.sequence.Clear()
		case result == keymap.SequenceMatch && keymap.Matches(buffer, m.keys.FoldOpenAll):
			m.openAllFolds()
			m.sequence.Clear()
		case result == keymap.SequencePending:
			// z was pressed, sequence state already has it, just wait for more input
		case key.Matches(msg, m.keys.Select):
			m.sequence.Clear()
			// Toggle selection for the current note or plan group
			if m.cursor < len(m.displayNodes) {
				node := m.displayNodes[m.cursor]
				if node.Item != nil && !node.Item.IsDir {
					// It's a note (file)
					if _, ok := m.selected[node.Item.Path]; ok {
						delete(m.selected, node.Item.Path)
					} else {
						m.selected[node.Item.Path] = struct{}{}
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
			m.sequence.Clear()
			// Clear all selections
			m.selected = make(map[string]struct{})
			m.selectedGroups = make(map[string]struct{})
		default:
			// Clear sequence buffer for keys that aren't part of sequences
			// unless we're in the middle of a potential sequence
			if result != keymap.SequencePending {
				m.sequence.Clear()
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

// priorityRank maps a priority string to a sortable key. Empty priority sorts
// LAST (rank "z") so prioritized notes float to the top when ordering by
// priority. p0 < p1 < p2 < p3 < "" lexically, which matches "most critical
// first". This is the priority comparator used by the group-by-priority axis
// (partitionByPriority).
func priorityRank(priority string) string {
	if priority == "" {
		return "z"
	}
	return priority
}

// sortNotes sorts a slice of notes in place by creation time, honoring
// m.sortAscending. Priority no longer affects flat ordering — it is surfaced
// exclusively through the group-by-priority axis (partitionByPriority, which
// orders buckets most-critical-first) and the per-priority filename coloring.
//
// This is the single canonical note comparator; all tree-building sort sites
// route through it so sort behavior stays consistent.
func (m *Model) sortNotes(notes []*models.Note) {
	sort.SliceStable(notes, func(i, j int) bool {
		if m.sortAscending {
			return notes[i].CreatedAt.Before(notes[j].CreatedAt)
		}
		return notes[i].CreatedAt.After(notes[j].CreatedAt)
	})
}

// BuildDisplayTree constructs the hierarchical list of nodes for rendering.
func (m *Model) BuildDisplayTree() { //nolint:gocyclo
	if m.recentNotesMode {
		m.buildRecentNotesList()
		m.ApplyLinks()
		return
	}

	if m.archiveViewMode {
		m.buildArchiveNotesList()
		m.ApplyLinks()
		return
	}

	if m.isFilteringByTag && m.selectedTag != "" {
		m.buildTagFilteredTree()
		m.ApplyLinks()
		return
	}

	var nodes []*DisplayNode
	var workspacesToShow []*workspace.WorkspaceNode

	// Check if we should ignore collapsed state (when searching or filtering by git status)
	hasSearchFilter := (m.filterValue != "" && !m.isGrepping) || m.showGitModifiedOnly

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
		var baseDepth int
		for _, ws := range m.workspaces {
			// Save global separately
			if ws.Name == "global" { //nolint:goconst
				globalNode = ws
				continue
			}
			// Use case-insensitive path comparison
			isSame, _ := pathutil.ComparePaths(ws.Path, m.focusedWorkspace.Path)
			if isSame {
				baseDepth = ws.Depth // Save the focused workspace's depth for adjustment
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

		// Adjust depths to make focused workspace a root-level item
		if len(workspacesToShow) > 0 {
			for i, ws := range workspacesToShow {
				wsCopy := *ws
				wsCopy.Depth = ws.Depth - baseDepth
				wsCopy.TreePrefix = "" // Clear prefix as depth is adjusted
				workspacesToShow[i] = &wsCopy
			}
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

	// 2. Convert items to notes (temporary bridge during refactoring)
	// TODO: Refactor BuildDisplayTree to work directly with tree.Item
	var allNotes []*models.Note
	for _, item := range m.allItems {
		if !item.IsDir { // Only convert file items (notes), not directories
			allNotes = append(allNotes, ItemToNote(item))
		}
	}

	// Group notes by workspace path, then by group (directory)
	notesByWorkspace := make(map[string]map[string][]*models.Note)

	// Create a map of workspace names to their paths for relative path calculation
	workspacePathMap := make(map[string]string)
	for _, ws := range m.workspaces {
		workspacePathMap[ws.Name] = ws.Path
	}

	for _, note := range allNotes {
		// Normalize workspace name to lowercase for case-insensitive matching
		wsKey := strings.ToLower(note.Workspace)
		if _, ok := notesByWorkspace[wsKey]; !ok {
			notesByWorkspace[wsKey] = make(map[string][]*models.Note)
		}
		notesByWorkspace[wsKey][note.Group] = append(notesByWorkspace[wsKey][note.Group], note)
	}

	// 3. Build the display node list and jump map
	m.jumpMap = make(map[rune]int)
	jumpCounter := '1'
	needsSeparator := false // Track if we need to add a separator before the next workspace

	for _, ws := range workspacesToShow {
		// Normalize workspace name to lowercase for case-insensitive matching
		wsKey := strings.ToLower(ws.Name)

		// Add separator between ecosystem's own notes and child workspaces
		if needsSeparator && m.focusedWorkspace != nil && m.focusedWorkspace.IsEcosystem() {
			isSame, _ := pathutil.ComparePaths(ws.Path, m.focusedWorkspace.Path)
			if !isSame {
				// This is a child workspace, add separator
				// Separator nodes have nil Item
				nodes = append(nodes, &DisplayNode{
					Item:   nil, // Separator
					Prefix: "  ",
					Depth:  0,
				})
				needsSeparator = false // Only add separator once
			}
		}
		// Skip worktrees - they never have their own notes
		if ws.IsWorktree() {
			continue
		}

		hasNotes := len(notesByWorkspace[wsKey]) > 0
		// Always show ecosystem nodes at depth 0, even if they have no direct notes
		// (their children may have notes)
		// Also always show the global workspace
		if !hasNotes && m.focusedWorkspace == nil && ws.Depth > 0 && ws.Name != "global" {
			// In global view, only skip non-ecosystem workspaces that have no notes
			continue
		}

		// Add workspace node
		wsItem := &tree.Item{
			Path:     ws.Path,
			Name:     ws.Name,
			IsDir:    true, // All workspaces, including global, are foldable directories
			Type:     tree.TypeWorkspace,
			Metadata: make(map[string]interface{}),
		}
		wsItem.Metadata["Workspace"] = ws
		node := &DisplayNode{
			Item:   wsItem,
			Prefix: ws.TreePrefix,
			Depth:  ws.Depth,
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

		if noteGroups, ok := notesByWorkspace[wsKey]; ok {
			// Separate regular groups, archived subgroups, and artifact subgroups
			// archiveSubgroups maps "parent" -> "child" -> notes
			// e.g., "plans" -> "test-plan" -> [notes in plans/.archive/test-plan]
			// artifactSubgroups maps "parent" -> "jobName" -> notes
			// e.g., "plans/binary-test" -> "impl-foo-abc123" -> [briefing/log/etc files]
			// jobName == "" means files directly under .artifacts (no per-job subdir).
			var regularGroups []string
			var rootNotes []*models.Note
			planGroups := make(map[string][]*models.Note)
			holdPlanGroups := make(map[string][]*models.Note)
			archiveSubgroups := make(map[string]map[string][]*models.Note)
			closedSubgroups := make(map[string]map[string][]*models.Note)
			artifactSubgroups := make(map[string]map[string][]*models.Note)

			for name, notes := range noteGroups {
				// Handle root notes (e.g. grove.toml)
				if name == "" {
					rootNotes = append(rootNotes, notes...)
					continue
				}

				// Check if this is an archived or closed group - skip if archives are hidden
				isArchived := strings.Contains(name, "/.archive")
				isClosed := strings.Contains(name, "/.closed")
				if (isArchived || isClosed) && !m.showArchives {
					continue
				}

				// Check if this is an artifact group - skip if artifacts are hidden
				isArtifact := strings.Contains(name, "/.artifacts")
				if isArtifact && !m.showArtifacts {
					continue
				}

				// Skip double-nested archives (e.g., plans/.archive/foo/.archive/bar)
				// Count occurrences of "/.archive" in the path
				archiveCount := strings.Count(name, "/.archive")
				if archiveCount > 1 {
					continue
				}

				// Skip double-nested closed (e.g., issues/.closed/foo/.closed/bar)
				closedCount := strings.Count(name, "/.closed")
				if closedCount > 1 {
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

				// Check if this matches pattern "<parent>/.closed/<child>"
				if strings.Contains(name, "/.closed/") {
					parts := strings.Split(name, "/.closed/")
					if len(parts) == 2 {
						parent := parts[0]
						child := parts[1]
						if closedSubgroups[parent] == nil {
							closedSubgroups[parent] = make(map[string][]*models.Note)
						}
						closedSubgroups[parent][child] = notes
						continue
					}
				}

				// Check if this matches pattern "<parent>/.closed" (notes directly in .closed folder)
				if strings.HasSuffix(name, "/.closed") {
					parent := strings.TrimSuffix(name, "/.closed")
					if closedSubgroups[parent] == nil {
						closedSubgroups[parent] = make(map[string][]*models.Note)
					}
					// Use empty string as key to indicate notes directly in .closed
					closedSubgroups[parent][""] = notes
					continue
				}

				// Check if this matches pattern "<parent>/.artifacts/<jobName>"
				// (each job dir under .artifacts becomes its own subgroup).
				if idx := strings.Index(name, "/.artifacts/"); idx >= 0 {
					parent := name[:idx]
					jobName := name[idx+len("/.artifacts/"):]
					if artifactSubgroups[parent] == nil {
						artifactSubgroups[parent] = make(map[string][]*models.Note)
					}
					artifactSubgroups[parent][jobName] = notes
					continue
				}

				// Check if this matches pattern "<parent>/.artifacts" (files directly in .artifacts folder)
				if strings.HasSuffix(name, "/.artifacts") {
					parent := strings.TrimSuffix(name, "/.artifacts")
					if artifactSubgroups[parent] == nil {
						artifactSubgroups[parent] = make(map[string][]*models.Note)
					}
					artifactSubgroups[parent][""] = notes
					continue
				}

				// Handle plans grouping
				if strings.HasPrefix(name, "plans/") {
					planName := strings.TrimPrefix(name, "plans/")
					// Check plan status to separate on-hold plans
					planStatus := m.GetPlanStatus(ws.Name, name)
					if planStatus == "hold" { //nolint:goconst
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
			ensureParentGroup := func(parent string) {
				if parent == "plans" || parent == "" { //nolint:goconst
					return
				}
				if strings.HasPrefix(parent, "plans/") {
					planName := strings.TrimPrefix(parent, "plans/")
					if planGroups[planName] == nil && holdPlanGroups[planName] == nil {
						planStatus := m.GetPlanStatus(ws.Name, parent)
						if planStatus == "hold" {
							if m.showOnHold {
								holdPlanGroups[planName] = []*models.Note{}
							}
						} else {
							planGroups[planName] = []*models.Note{}
						}
					}
				} else {
					found := false
					for _, rg := range regularGroups {
						if rg == parent {
							found = true
							break
						}
					}
					if !found {
						regularGroups = append(regularGroups, parent)
					}
				}
			}

			for parent := range archiveSubgroups {
				ensureParentGroup(parent)
			}
			for parent := range closedSubgroups {
				ensureParentGroup(parent)
			}
			for parent := range artifactSubgroups {
				ensureParentGroup(parent)
			}

			// Sort groups using SortOrder from NoteTypes registry
			// Groups with lower SortOrder appear first, then alphabetically by name
			sort.SliceStable(regularGroups, func(i, j int) bool {
				nameA := regularGroups[i]
				nameB := regularGroups[j]

				// Get SortOrder from registry, default to 100 if not found or if SortOrder is 0
				sortOrderA := 100
				sortOrderB := 100
				if typeConfig, ok := m.service.NoteTypes[nameA]; ok && typeConfig.SortOrder != 0 {
					sortOrderA = typeConfig.SortOrder
				}
				if typeConfig, ok := m.service.NoteTypes[nameB]; ok && typeConfig.SortOrder != 0 {
					sortOrderB = typeConfig.SortOrder
				}

				// Sort by SortOrder first, then alphabetically
				if sortOrderA != sortOrderB {
					return sortOrderA < sortOrderB
				}
				return nameA < nameB
			})

			// Check if we have plans to add a "plans" parent group
			hasPlans := len(planGroups) > 0 || len(archiveSubgroups["plans"]) > 0
			hasHoldPlans := len(holdPlanGroups) > 0

			// Render groups in the sorted order
			notesRootDir, err := m.service.GetNotebookLocator().GetNotesDir(ws, "")
			if err == nil { // Proceed only if we can get the notes root directory
				// Determine where to insert plans based on SortOrder
				plansSortOrder := 100
				if typeConfig, ok := m.service.NoteTypes["plans"]; ok && typeConfig.SortOrder != 0 {
					plansSortOrder = typeConfig.SortOrder
				}

				// Split regular groups into those that come before and after plans
				var groupsBeforePlans []string
				var groupsAfterPlans []string
				for _, groupName := range regularGroups {
					groupSortOrder := 100
					if typeConfig, ok := m.service.NoteTypes[groupName]; ok && typeConfig.SortOrder != 0 {
						groupSortOrder = typeConfig.SortOrder
					}
					if groupSortOrder < plansSortOrder {
						groupsBeforePlans = append(groupsBeforePlans, groupName)
					} else {
						groupsAfterPlans = append(groupsAfterPlans, groupName)
					}
				}

				// Render groups before plans
				if len(groupsBeforePlans) > 0 {
					rootGroupNode := buildGroupTree(noteGroups, groupsBeforePlans)
					hasFollowingTopLevelSiblings := hasPlans || len(groupsAfterPlans) > 0 || hasHoldPlans || len(rootNotes) > 0
					config := treeRenderConfig{
						itemType:            tree.TypeGroup,
						groupMetadataPrefix: "",
						nameUsesPrefix:      false,
						includeArchives:     true,
						includeClosed:       true,
						includeArtifacts:    true,
					}
					m.renderTree(&nodes, ws, rootGroupNode, ws.TreePrefix+"  ", ws.Depth+1, hasSearchFilter, workspacePathMap, notesRootDir, config, hasFollowingTopLevelSiblings, archiveSubgroups, closedSubgroups, artifactSubgroups)
				}

				// Render Plans Group in its sorted position
				if hasPlans {
					hasGroupsAfter := len(groupsAfterPlans) > 0 || hasHoldPlans || len(rootNotes) > 0
					m.addPlansGroup(&nodes, ws, planGroups, archiveSubgroups, artifactSubgroups, hasSearchFilter, workspacePathMap, hasGroupsAfter)
				}

				// Render groups after plans
				if len(groupsAfterPlans) > 0 {
					rootGroupNode := buildGroupTree(noteGroups, groupsAfterPlans)
					hasFollowingTopLevelSiblings := hasHoldPlans || len(rootNotes) > 0
					config := treeRenderConfig{
						itemType:            tree.TypeGroup,
						groupMetadataPrefix: "",
						nameUsesPrefix:      false,
						includeArchives:     true,
						includeClosed:       true,
						includeArtifacts:    true,
					}
					m.renderTree(&nodes, ws, rootGroupNode, ws.TreePrefix+"  ", ws.Depth+1, hasSearchFilter, workspacePathMap, notesRootDir, config, hasFollowingTopLevelSiblings, archiveSubgroups, closedSubgroups, artifactSubgroups)
				}

				// Render On-Hold Plans (always last before root notes)
				if hasHoldPlans {
					m.addHoldPlansGroup(&nodes, ws, holdPlanGroups, hasSearchFilter, workspacePathMap, len(rootNotes) > 0)
				}

				// Render root notes (e.g. grove.toml) directly under workspace
				if len(rootNotes) > 0 {
					m.sortNotes(rootNotes)
					for ni, note := range rootNotes {
						isLastRootNote := ni == len(rootNotes)-1
						var notePrefix strings.Builder
						noteIndent := strings.ReplaceAll(ws.TreePrefix, "├ ", "│ ")
						noteIndent = strings.ReplaceAll(noteIndent, "└ ", "  ")
						notePrefix.WriteString(noteIndent)
						if isLastRootNote {
							notePrefix.WriteString("└ ")
						} else {
							notePrefix.WriteString("├ ")
						}
						nodes = append(nodes, &DisplayNode{
							Item:         noteToItem(note),
							Prefix:       notePrefix.String(),
							Depth:        ws.Depth + 1,
							RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
						})
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
		m.addUngroupedSection(&nodes, ungroupedWorkspaces, notesByWorkspace, hasSearchFilter, workspacePathMap)
	}

	m.displayNodes = nodes
	m.ApplyLinks()

	// Handle pending workspace initialization
	// We need to check if any collapse state was actually set
	needsRebuild := false
	if m.pendingWorkspaceInit != "" {
		needsRebuild = m.finalizePendingWorkspaceInit()
		m.pendingWorkspaceInit = ""
	}

	// If we initialized collapse state, rebuild the tree to reflect the changes
	if needsRebuild {
		// Recursive call - but pendingWorkspaceInit is now empty so won't loop
		m.BuildDisplayTree()
		return
	}

	// Add deleted files to the tree
	m.AddDeletedFilesToTree()

	m.clampCursor()
}

// buildTagFilteredTree constructs a simplified tree with notes hoisted under their workspace, filtered by a tag.
func (m *Model) buildTagFilteredTree() {
	// 1. Convert items to notes
	var allNotes []*models.Note
	for _, item := range m.allItems {
		if !item.IsDir {
			allNotes = append(allNotes, ItemToNote(item))
		}
	}

	// 2. Filter notes by tag
	tagFilter := strings.ToLower(m.selectedTag)
	var filteredNotes []*models.Note
	for _, note := range allNotes {
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
		// Normalize workspace name to lowercase for case-insensitive matching
		wsKey := strings.ToLower(note.Workspace)
		notesByWorkspace[wsKey] = append(notesByWorkspace[wsKey], note)
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
		// Normalize workspace name to lowercase for case-insensitive matching
		wsKey := strings.ToLower(ws.Name)
		if notesInWs, ok := notesByWorkspace[wsKey]; ok {
			// Add workspace node
			wsItem := &tree.Item{
				Path:     ws.Path,
				Name:     ws.Name,
				IsDir:    true,
				Type:     tree.TypeWorkspace,
				Metadata: make(map[string]interface{}),
			}
			wsItem.Metadata["Workspace"] = ws
			wsNode := &DisplayNode{
				Item:   wsItem,
				Prefix: ws.TreePrefix,
				Depth:  ws.Depth,
			}
			nodes = append(nodes, wsNode)

			// Sort notes within the workspace
			m.sortNotes(notesInWs)

			// Add note nodes directly under the workspace
			for i, note := range notesInWs {
				isLastNote := i == len(notesInWs)-1
				var notePrefix strings.Builder
				indentPrefix := strings.ReplaceAll(ws.TreePrefix, "├ ", "│ ")
				indentPrefix = strings.ReplaceAll(indentPrefix, "└ ", "  ")
				notePrefix.WriteString(indentPrefix)
				if ws.Depth > 0 || ws.TreePrefix != "" {
					notePrefix.WriteString("  ")
				}
				if isLastNote {
					notePrefix.WriteString("└ ")
				} else {
					notePrefix.WriteString("├ ")
				}

				nodes = append(nodes, &DisplayNode{
					Item:         noteToItem(note),
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

func (m *Model) addArchiveSubgroup(
	nodes *[]*DisplayNode,
	ws *workspace.WorkspaceNode,
	groupPrefix string,
	archiveSubgroups map[string][]*models.Note,
	hasSearchFilter bool,
	workspacePathMap map[string]string,
	parentPath string,
	parentName string,
	parentGroup string,
) {
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
	archiveIndent := strings.ReplaceAll(groupPrefix, "├ ", "│ ")
	archiveIndent = strings.ReplaceAll(archiveIndent, "└ ", "  ")
	archivePrefix.WriteString(archiveIndent)
	archivePrefix.WriteString("└ ")

	// Add .archive parent node
	archiveParentItem := &tree.Item{
		Path:     filepath.Join(parentPath, ".archive"),
		Name:     parentName + "/.archive",
		IsDir:    true,
		Type:     tree.TypeGroup,
		Metadata: make(map[string]interface{}),
	}
	archiveParentItem.Metadata["Workspace"] = ws.Name
	archiveParentItem.Metadata["Group"] = parentGroup + "/.archive"
	archiveParentNode := &DisplayNode{
		Item:       archiveParentItem,
		Prefix:     archivePrefix.String(),
		Depth:      ws.Depth + 2,
		ChildCount: totalArchivedNotes,
	}
	*nodes = append(*nodes, archiveParentNode)

	// Check if .archive parent is collapsed (collapsed by default on first sight)
	archiveParentNodeID := archiveParentNode.NodeID()
	m.seedCollapsedDefault(archiveParentNodeID)
	if !m.collapsedNodes[archiveParentNodeID] || hasSearchFilter {
		// Add individual archived children (implementation continues...)
		for pi, archivedName := range archivedNames {
			isLastArchived := pi == len(archivedNames)-1
			archivedNotes := archiveSubgroups[archivedName]

			// Sort notes within the archived child
			m.sortNotes(archivedNotes)

			// If archivedName is empty, these are notes directly in .archive folder
			if archivedName == "" {
				// Add notes directly under .archive parent
				for ni, note := range archivedNotes {
					isLastNote := ni == len(archivedNotes)-1 && isLastArchived
					var notePrefix strings.Builder
					noteIndent := strings.ReplaceAll(archivePrefix.String(), "├ ", "│ ")
					noteIndent = strings.ReplaceAll(noteIndent, "└ ", "  ")
					notePrefix.WriteString(noteIndent)
					if isLastNote {
						notePrefix.WriteString("└ ")
					} else {
						notePrefix.WriteString("├ ")
					}
					*nodes = append(*nodes, &DisplayNode{Item: noteToItem(note), Prefix: notePrefix.String(), Depth: ws.Depth + 3, RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace)})
				}
				continue
			}

			// Calculate archived child prefix
			var archivedPrefix strings.Builder
			archivedIndent := strings.ReplaceAll(archivePrefix.String(), "├ ", "│ ")
			archivedIndent = strings.ReplaceAll(archivedIndent, "└ ", "  ")
			archivedPrefix.WriteString(archivedIndent)
			if isLastArchived {
				archivedPrefix.WriteString("└ ")
			} else {
				archivedPrefix.WriteString("├ ")
			}

			// Add archived child node
			archivedChildItem := &tree.Item{
				Path:     filepath.Join(parentPath, ".archive", archivedName),
				Name:     parentName + "/.archive/" + archivedName,
				IsDir:    true,
				Type:     tree.TypeGroup,
				Metadata: make(map[string]interface{}),
			}
			archivedChildItem.Metadata["Workspace"] = ws.Name
			archivedChildItem.Metadata["Group"] = parentGroup + "/.archive/" + archivedName
			archivedChildNode := &DisplayNode{
				Item:       archivedChildItem,
				Prefix:     archivedPrefix.String(),
				Depth:      ws.Depth + 3,
				ChildCount: len(archivedNotes),
			}
			*nodes = append(*nodes, archivedChildNode)

			// Check if archived child is collapsed
			archivedChildNodeID := archivedChildNode.NodeID()
			if !m.collapsedNodes[archivedChildNodeID] || hasSearchFilter {
				// Add notes within the archived child
				for ni, note := range archivedNotes {
					isLastArchivedNote := ni == len(archivedNotes)-1
					var archivedNotePrefix strings.Builder
					archivedNoteIndent := strings.ReplaceAll(archivedPrefix.String(), "├ ", "│ ")
					archivedNoteIndent = strings.ReplaceAll(archivedNoteIndent, "└ ", "  ")
					archivedNotePrefix.WriteString(archivedNoteIndent)
					if isLastArchivedNote {
						archivedNotePrefix.WriteString("└ ")
					} else {
						archivedNotePrefix.WriteString("├ ")
					}
					*nodes = append(*nodes, &DisplayNode{Item: noteToItem(note), Prefix: archivedNotePrefix.String(), Depth: ws.Depth + 4, RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace)})
				}
			}
		}
	}
}

func (m *Model) addClosedSubgroup(
	nodes *[]*DisplayNode,
	ws *workspace.WorkspaceNode,
	groupPrefix string,
	closedSubgroups map[string][]*models.Note,
	hasSearchFilter bool,
	workspacePathMap map[string]string,
	parentPath string,
	parentName string,
	parentGroup string,
) {
	// Sort closed child names
	var closedNames []string
	for name := range closedSubgroups {
		closedNames = append(closedNames, name)
	}
	sort.Strings(closedNames)

	// Count total closed notes
	totalClosedNotes := 0
	for _, notes := range closedSubgroups {
		totalClosedNotes += len(notes)
	}

	// Calculate .closed prefix (last child under this group)
	var closedPrefix strings.Builder
	closedIndent := strings.ReplaceAll(groupPrefix, "├ ", "│ ")
	closedIndent = strings.ReplaceAll(closedIndent, "└ ", "  ")
	closedPrefix.WriteString(closedIndent)
	closedPrefix.WriteString("└ ")

	// Add .closed parent node
	closedParentItem := &tree.Item{
		Path:     filepath.Join(parentPath, ".closed"),
		Name:     parentName + "/.closed",
		IsDir:    true,
		Type:     tree.TypeGroup,
		Metadata: make(map[string]interface{}),
	}
	closedParentItem.Metadata["Workspace"] = ws.Name
	closedParentItem.Metadata["Group"] = parentGroup + "/.closed"
	closedParentNode := &DisplayNode{
		Item:       closedParentItem,
		Prefix:     closedPrefix.String(),
		Depth:      ws.Depth + 2,
		ChildCount: totalClosedNotes,
	}
	*nodes = append(*nodes, closedParentNode)

	// Check if .closed parent is collapsed (collapsed by default on first sight)
	closedParentNodeID := closedParentNode.NodeID()
	m.seedCollapsedDefault(closedParentNodeID)
	if !m.collapsedNodes[closedParentNodeID] || hasSearchFilter {
		// Add individual closed children
		for pi, closedName := range closedNames {
			isLastClosed := pi == len(closedNames)-1
			closedNotes := closedSubgroups[closedName]

			// Sort notes within the closed child
			m.sortNotes(closedNotes)

			// If closedName is empty, these are notes directly in .closed folder
			if closedName == "" {
				// Add notes directly under .closed parent
				for ni, note := range closedNotes {
					isLastNote := ni == len(closedNotes)-1 && isLastClosed
					var notePrefix strings.Builder
					noteIndent := strings.ReplaceAll(closedPrefix.String(), "├ ", "│ ")
					noteIndent = strings.ReplaceAll(noteIndent, "└ ", "  ")
					notePrefix.WriteString(noteIndent)
					if isLastNote {
						notePrefix.WriteString("└ ")
					} else {
						notePrefix.WriteString("├ ")
					}
					*nodes = append(*nodes, &DisplayNode{Item: noteToItem(note), Prefix: notePrefix.String(), Depth: ws.Depth + 3, RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace)})
				}
				continue
			}

			// Calculate closed child prefix
			var closedChildPrefix strings.Builder
			closedChildIndent := strings.ReplaceAll(closedPrefix.String(), "├ ", "│ ")
			closedChildIndent = strings.ReplaceAll(closedChildIndent, "└ ", "  ")
			closedChildPrefix.WriteString(closedChildIndent)
			if isLastClosed {
				closedChildPrefix.WriteString("└ ")
			} else {
				closedChildPrefix.WriteString("├ ")
			}

			// Add closed child node
			closedChildItem := &tree.Item{
				Path:     filepath.Join(parentPath, ".closed", closedName),
				Name:     parentName + "/.closed/" + closedName,
				IsDir:    true,
				Type:     tree.TypeGroup,
				Metadata: make(map[string]interface{}),
			}
			closedChildItem.Metadata["Workspace"] = ws.Name
			closedChildItem.Metadata["Group"] = parentGroup + "/.closed/" + closedName
			closedChildNode := &DisplayNode{
				Item:       closedChildItem,
				Prefix:     closedChildPrefix.String(),
				Depth:      ws.Depth + 3,
				ChildCount: len(closedNotes),
			}
			*nodes = append(*nodes, closedChildNode)

			// Check if closed child is collapsed
			closedChildNodeID := closedChildNode.NodeID()
			if !m.collapsedNodes[closedChildNodeID] || hasSearchFilter {
				// Add notes within the closed child
				for ni, note := range closedNotes {
					isLastClosedNote := ni == len(closedNotes)-1
					var closedNotePrefix strings.Builder
					closedNoteIndent := strings.ReplaceAll(closedChildPrefix.String(), "├ ", "│ ")
					closedNoteIndent = strings.ReplaceAll(closedNoteIndent, "└ ", "  ")
					closedNotePrefix.WriteString(closedNoteIndent)
					if isLastClosedNote {
						closedNotePrefix.WriteString("└ ")
					} else {
						closedNotePrefix.WriteString("├ ")
					}
					*nodes = append(*nodes, &DisplayNode{Item: noteToItem(note), Prefix: closedNotePrefix.String(), Depth: ws.Depth + 4, RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace)})
				}
			}
		}
	}
}

func (m *Model) addArtifactSubgroup(
	nodes *[]*DisplayNode,
	ws *workspace.WorkspaceNode,
	groupPrefix string,
	artifactJobs map[string][]*models.Note,
	hasSearchFilter bool,
	workspacePathMap map[string]string,
	parentPath string,
	parentName string,
	parentGroup string,
) {
	// The keys of artifactJobs are full relative paths under .artifacts/ (e.g.
	// "test-2b3a8ac9/workflows/wf_xxx/agents"). Build an in-memory tree from
	// those segments so nested directories render as a properly indented tree,
	// preserving intermediate dirs (e.g. "workflows/") that would otherwise be
	// dropped. The empty key ("") means files sit directly under .artifacts/.
	artifactsTree := newGroupTreeNode("", "")
	var rootNotes []*models.Note // notes directly under .artifacts/ (jobName == "")
	for jobName, notes := range artifactJobs {
		if jobName == "" {
			rootNotes = append(rootNotes, notes...)
			continue
		}
		parts := strings.Split(jobName, "/")
		currentNode := artifactsTree
		for i, part := range parts {
			if _, ok := currentNode.children[part]; !ok {
				fullName := strings.Join(parts[:i+1], "/")
				currentNode.children[part] = newGroupTreeNode(part, fullName)
				currentNode.childKeys = append(currentNode.childKeys, part)
			}
			currentNode = currentNode.children[part]
		}
		currentNode.notes = notes
	}
	// Sort child keys at every level for stable rendering.
	var sortNodes func(*groupTreeNode)
	sortNodes = func(n *groupTreeNode) {
		sort.Strings(n.childKeys)
		for _, c := range n.children {
			sortNodes(c)
		}
	}
	sortNodes(artifactsTree)

	// Count total artifact files across the whole subtree.
	var countAll func(*groupTreeNode) int
	countAll = func(n *groupTreeNode) int {
		total := len(n.notes)
		for _, c := range n.children {
			total += countAll(c)
		}
		return total
	}
	totalArtifacts := countAll(artifactsTree) + len(rootNotes)

	// Calculate .artifacts prefix (last child under this group)
	var artifactsPrefix strings.Builder
	artifactsIndent := strings.ReplaceAll(groupPrefix, "├ ", "│ ")
	artifactsIndent = strings.ReplaceAll(artifactsIndent, "└ ", "  ")
	artifactsPrefix.WriteString(artifactsIndent)
	artifactsPrefix.WriteString("└ ")

	// Add .artifacts parent node
	artifactsParentItem := &tree.Item{
		Path:     filepath.Join(parentPath, ".artifacts"),
		Name:     parentName + "/.artifacts",
		IsDir:    true,
		Type:     tree.TypeGroup,
		Metadata: make(map[string]interface{}),
	}
	artifactsParentItem.Metadata["Workspace"] = ws.Name
	artifactsParentItem.Metadata["Group"] = parentGroup + "/.artifacts"
	artifactsParentNode := &DisplayNode{
		Item:       artifactsParentItem,
		Prefix:     artifactsPrefix.String(),
		Depth:      ws.Depth + 2,
		ChildCount: totalArtifacts,
	}
	*nodes = append(*nodes, artifactsParentNode)

	// Check if .artifacts parent is collapsed (collapsed by default on first sight)
	artifactsParentNodeID := artifactsParentNode.NodeID()
	m.seedCollapsedDefault(artifactsParentNodeID)
	if m.collapsedNodes[artifactsParentNodeID] && !hasSearchFilter {
		return
	}

	// Render the directory subtree beneath .artifacts. Files directly under
	// .artifacts (rootNotes) render after the dir children, and govern the
	// last-child glyph of the deepest dir branch.
	hasRootNotes := len(rootNotes) > 0
	m.renderArtifactTree(nodes, ws, artifactsTree, artifactsPrefix.String(), ws.Depth+3,
		hasSearchFilter, workspacePathMap, parentPath, parentGroup, hasRootNotes)

	if hasRootNotes {
		m.sortNotes(rootNotes)
		noteIndent := strings.ReplaceAll(artifactsPrefix.String(), "├ ", "│ ")
		noteIndent = strings.ReplaceAll(noteIndent, "└ ", "  ")
		for ni, note := range rootNotes {
			isLastNote := ni == len(rootNotes)-1
			var notePrefix strings.Builder
			notePrefix.WriteString(noteIndent)
			if isLastNote {
				notePrefix.WriteString("└ ")
			} else {
				notePrefix.WriteString("├ ")
			}
			*nodes = append(*nodes, &DisplayNode{Item: noteToItem(note), Prefix: notePrefix.String(), Depth: ws.Depth + 3, RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace)})
		}
	}
}

// renderArtifactTree recursively renders the directory tree built from the
// post-`.artifacts/` path segments. Each segment becomes its own group node
// (including intermediate dirs like "workflows/"), indented one level deeper,
// with ├/└ branch glyphs. Leaf dirs render their artifact files beneath them.
// Group metadata carries the full path under .artifacts so labels can resolve
// the job-dir title (top segment) and NodeIDs stay stable across rebuilds.
func (m *Model) renderArtifactTree(
	nodes *[]*DisplayNode,
	ws *workspace.WorkspaceNode,
	n *groupTreeNode,
	parentPrefix string,
	depth int,
	hasSearchFilter bool,
	workspacePathMap map[string]string,
	parentPath string,
	parentGroup string,
	hasFollowingSiblings bool,
) {
	numChildren := len(n.childKeys)
	for i, key := range n.childKeys {
		child := n.children[key]
		isLastChild := (i == numChildren-1) && !hasFollowingSiblings

		var childPrefix strings.Builder
		childPrefix.WriteString(parentPrefix)
		if isLastChild {
			childPrefix.WriteString("└ ")
		} else {
			childPrefix.WriteString("├ ")
		}

		// child.fullName is the relative path under .artifacts (e.g.
		// "test-2b3a8ac9/workflows"). Metadata["Group"] carries the full
		// path so view.go can label by the last segment / resolve the job
		// title for the top segment.
		fullPath := parentGroup + "/.artifacts/" + child.fullName
		dirItem := &tree.Item{
			Path:     filepath.Join(parentPath, ".artifacts", filepath.FromSlash(child.fullName)),
			Name:     fullPath,
			IsDir:    true,
			Type:     tree.TypeGroup,
			Metadata: make(map[string]interface{}),
		}
		dirItem.Metadata["Workspace"] = ws.Name
		dirItem.Metadata["Group"] = fullPath

		childCount := len(child.notes)
		if childCount == 0 {
			var countChildren func(*groupTreeNode) int
			countChildren = func(node *groupTreeNode) int {
				count := len(node.notes)
				for _, c := range node.children {
					count += countChildren(c)
				}
				return count
			}
			childCount = countChildren(child)
		}

		dirNode := &DisplayNode{
			Item:       dirItem,
			Prefix:     childPrefix.String(),
			Depth:      depth,
			ChildCount: childCount,
		}
		*nodes = append(*nodes, dirNode)

		dirNodeID := dirNode.NodeID()
		if m.collapsedNodes[dirNodeID] && !hasSearchFilter {
			continue
		}

		var nextParentPrefix string
		if isLastChild {
			nextParentPrefix = parentPrefix + "  "
		} else {
			nextParentPrefix = parentPrefix + "│ "
		}

		// Recurse into subdirectories first; the leaf's own files (if any)
		// follow, so any subdirs are non-last when files exist beneath them.
		hasOwnNotes := len(child.notes) > 0
		m.renderArtifactTree(nodes, ws, child, nextParentPrefix, depth+1,
			hasSearchFilter, workspacePathMap, parentPath, parentGroup, hasOwnNotes)

		if hasOwnNotes {
			m.sortNotes(child.notes)
			noteIndent := strings.ReplaceAll(childPrefix.String(), "├ ", "│ ")
			noteIndent = strings.ReplaceAll(noteIndent, "└ ", "  ")
			for ni, note := range child.notes {
				isLastNote := ni == len(child.notes)-1
				var notePrefix strings.Builder
				notePrefix.WriteString(noteIndent)
				if isLastNote {
					notePrefix.WriteString("└ ")
				} else {
					notePrefix.WriteString("├ ")
				}
				*nodes = append(*nodes, &DisplayNode{Item: noteToItem(note), Prefix: notePrefix.String(), Depth: depth + 1, RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace)})
			}
		}
	}
}

func (m *Model) addPlansGroup(nodes *[]*DisplayNode, ws *workspace.WorkspaceNode, planGroups map[string][]*models.Note, archiveSubgroups map[string]map[string][]*models.Note, artifactSubgroups map[string]map[string][]*models.Note, hasSearchFilter bool, workspacePathMap map[string]string, hasGroupsAfter bool) {
	// Calculate plans parent prefix
	var plansPrefix strings.Builder
	indentPrefix := strings.ReplaceAll(ws.TreePrefix, "├ ", "│ ")
	indentPrefix = strings.ReplaceAll(indentPrefix, "└ ", "  ")
	plansPrefix.WriteString(indentPrefix)
	if ws.Depth > 0 || ws.TreePrefix != "" {
		plansPrefix.WriteString("  ")
	}
	if hasGroupsAfter {
		plansPrefix.WriteString("├ ") // Not last if other groups exist after
	} else {
		plansPrefix.WriteString("└ ") // Plans is last if no other groups
	}

	// Use GetGroupDir for centralized path resolution
	plansPath, err := m.service.GetNotebookLocator().GetGroupDir(ws, "plans")
	if err != nil {
		// Skip if we can't resolve path
		return
	}

	// Add "plans" parent node
	plansParentItem := &tree.Item{
		Path:     plansPath,
		Name:     "plans",
		IsDir:    true,
		Type:     tree.TypeGroup,
		Metadata: make(map[string]interface{}),
	}
	plansParentItem.Metadata["Workspace"] = ws.Name
	plansParentItem.Metadata["Group"] = "plans"
	plansParentNode := &DisplayNode{
		Item:       plansParentItem,
		Prefix:     plansPrefix.String(),
		Depth:      ws.Depth + 1,
		ChildCount: len(planGroups),
	}
	*nodes = append(*nodes, plansParentNode)

	// Check if plans parent is collapsed (unless searching)
	plansParentNodeID := plansParentNode.NodeID()
	if !m.collapsedNodes[plansParentNodeID] || hasSearchFilter {
		// Build tree structure for plans (same as regular groups)
		var planNames []string
		for planName := range planGroups {
			planNames = append(planNames, planName)
		}
		sort.Strings(planNames)

		// Build hierarchical tree for plan names
		planTree := buildGroupTree(planGroups, planNames)
		hasPlansArchive := len(archiveSubgroups["plans"]) > 0 && m.showArchives

		// Render the plan tree hierarchically
		config := treeRenderConfig{
			itemType:            tree.TypePlan,
			groupMetadataPrefix: "plans/",
			nameUsesPrefix:      false,
			includeArtifacts:    true,
			includeArchives:     false,
			includeClosed:       false,
		}
		m.renderTree(nodes, ws, planTree, plansPrefix.String(), ws.Depth+2, hasSearchFilter, workspacePathMap, plansPath, config, hasPlansArchive, nil, nil, artifactSubgroups)

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
	archiveIndent := strings.ReplaceAll(plansPrefix, "├ ", "│ ")
	archiveIndent = strings.ReplaceAll(archiveIndent, "└ ", "  ")
	archivePrefix.WriteString(archiveIndent)
	archivePrefix.WriteString("└ ")

	// Use GetGroupDir for centralized path resolution
	archivePath, err := m.service.GetNotebookLocator().GetGroupDir(ws, "plans/.archive")
	if err != nil {
		// Skip if we can't resolve path
		return
	}

	// Add .archive parent node
	// Create tree.Item for group
	archiveParentNodeItem := &tree.Item{
		Path:     archivePath,
		Name:     "plans/.archive",
		IsDir:    true,
		Type:     tree.TypeGroup,
		Metadata: make(map[string]interface{}),
	}
	archiveParentNodeItem.Metadata["Workspace"] = ws.Name
	archiveParentNodeItem.Metadata["Group"] = "plans/.archive"
	archiveParentNode := &DisplayNode{
		Item:       archiveParentNodeItem,
		Prefix:     archivePrefix.String(),
		Depth:      ws.Depth + 2,
		ChildCount: totalArchivedNotes,
	}
	*nodes = append(*nodes, archiveParentNode)

	// Check if .archive parent is collapsed (collapsed by default on first sight)
	archiveParentNodeID := archiveParentNode.NodeID()
	m.seedCollapsedDefault(archiveParentNodeID)
	if !m.collapsedNodes[archiveParentNodeID] || hasSearchFilter {
		// Build hierarchical tree for archived plan names
		archivedPlanTree := buildGroupTree(archivedPlans, archivedNames)

		// Render the archived plan tree hierarchically
		config := treeRenderConfig{
			itemType:            tree.TypeGroup, // An archived plan is just a group
			groupMetadataPrefix: "plans/.archive/",
			nameUsesPrefix:      false,
			includeArtifacts:    false,
			includeArchives:     false,
			includeClosed:       false,
		}
		m.renderTree(nodes, ws, archivedPlanTree, archivePrefix.String(), ws.Depth+2, hasSearchFilter, workspacePathMap, archivePath, config, false, nil, nil, nil)
	}
}

func (m *Model) addHoldPlansGroup(nodes *[]*DisplayNode, ws *workspace.WorkspaceNode, holdPlanGroups map[string][]*models.Note, hasSearchFilter bool, workspacePathMap map[string]string, hasFollowingSiblings bool) {
	// Calculate .hold parent prefix
	var holdPrefix strings.Builder
	indentPrefix := strings.ReplaceAll(ws.TreePrefix, "├ ", "│ ")
	indentPrefix = strings.ReplaceAll(indentPrefix, "└ ", "  ")
	holdPrefix.WriteString(indentPrefix)
	if ws.Depth > 0 || ws.TreePrefix != "" {
		holdPrefix.WriteString("  ")
	}
	if hasFollowingSiblings {
		holdPrefix.WriteString("├ ") // Not last if other siblings exist after
	} else {
		holdPrefix.WriteString("└ ") // .hold is last if no other siblings
	}

	// Use GetNotesDir to get the base path for notes, then create .hold virtual path
	// .hold is a virtual grouping, not a real directory, but we need a consistent path for NodeID
	notesDir, err := m.service.GetNotebookLocator().GetNotesDir(ws, "")
	if err != nil {
		// Skip if we can't resolve path
		return
	}
	holdPath := filepath.Join(filepath.Dir(notesDir), ".hold")

	// Add ".hold" parent node
	holdParentItem := &tree.Item{
		Path:     holdPath,
		Name:     ".hold",
		IsDir:    true,
		Type:     tree.TypeGroup,
		Metadata: make(map[string]interface{}),
	}
	holdParentItem.Metadata["Workspace"] = ws.Name
	holdParentItem.Metadata["Group"] = ".hold"
	holdParentNode := &DisplayNode{
		Item:       holdParentItem,
		Prefix:     holdPrefix.String(),
		Depth:      ws.Depth + 1,
		ChildCount: len(holdPlanGroups), // Count of hold plans, not notes
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
			m.sortNotes(planNotes)

			// Calculate plan prefix
			var planPrefix strings.Builder
			planIndent := strings.ReplaceAll(holdPrefix.String(), "├ ", "│ ")
			planIndent = strings.ReplaceAll(planIndent, "└ ", "  ")
			planPrefix.WriteString(planIndent)
			if isLastPlan {
				planPrefix.WriteString("└ ")
			} else {
				planPrefix.WriteString("├ ")
			}

			// Use GetGroupDir for centralized path resolution
			planPath, err := m.service.GetNotebookLocator().GetGroupDir(ws, "plans/"+planName)
			if err != nil {
				// Skip this plan if we can't resolve its path
				continue
			}

			// Add hold plan node
			planItem := &tree.Item{
				Path:     planPath,
				Name:     "plans/" + planName, // Keep full path for consistency
				IsDir:    true,
				Type:     tree.TypePlan,
				Metadata: make(map[string]interface{}),
			}
			planItem.Metadata["Workspace"] = ws.Name
			planItem.Metadata["Group"] = "plans/" + planName
			planNode := &DisplayNode{
				Item:       planItem,
				Prefix:     planPrefix.String(),
				Depth:      ws.Depth + 2,
				ChildCount: len(planNotes),
			}
			*nodes = append(*nodes, planNode)

			// Check if this plan is collapsed (unless searching)
			planNodeID := planNode.NodeID()
			if !m.collapsedNodes[planNodeID] || hasSearchFilter {
				// Add notes in this hold plan
				for ni, note := range planNotes {
					isLastNote := ni == len(planNotes)-1
					var notePrefix strings.Builder
					noteIndent := strings.ReplaceAll(planPrefix.String(), "├ ", "│ ")
					noteIndent = strings.ReplaceAll(noteIndent, "└ ", "  ")
					notePrefix.WriteString(noteIndent)
					if isLastNote {
						notePrefix.WriteString("└ ")
					} else {
						notePrefix.WriteString("├ ")
					}
					*nodes = append(*nodes, &DisplayNode{Item: noteToItem(note), Prefix: notePrefix.String(), Depth: ws.Depth + 3, RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace)})
				}
			}
		}
	}
}

func (m *Model) addUngroupedSection(nodes *[]*DisplayNode, ungroupedWorkspaces []*workspace.WorkspaceNode, notesByWorkspace map[string]map[string][]*models.Note, hasSearchFilter bool, workspacePathMap map[string]string) {
	ungroupedItem := &tree.Item{
		Path:     "ungrouped",
		Name:     "ungrouped",
		IsDir:    true,
		Type:     tree.TypeGroup,
		Metadata: make(map[string]interface{}),
	}
	ungroupedItem.Metadata["Group"] = "ungrouped"
	ungroupedNode := &DisplayNode{
		Item:   ungroupedItem,
		Prefix: "",
		Depth:  0,
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

			// Normalize workspace name to lowercase for case-insensitive matching
			wsKey := strings.ToLower(ws.Name)
			hasNotes := len(notesByWorkspace[wsKey]) > 0
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
					adjustedPrefix = "  └ "
				} else {
					adjustedPrefix = "  ├ "
				}
			}

			// Add workspace node
			wsItem := &tree.Item{
				Path:     ws.Path,
				Name:     ws.Name,
				IsDir:    true,
				Type:     tree.TypeWorkspace,
				Metadata: make(map[string]interface{}),
			}
			wsItem.Metadata["Workspace"] = ws
			node := &DisplayNode{
				Item:   wsItem,
				Prefix: adjustedPrefix,
				Depth:  ws.Depth + 1, // Increase depth since it's under "Ungrouped"
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

// groupHasArtifactOrphans reports whether the artifact jobs map for a group
// contains at least one jobID that does NOT correspond to a job note present in
// notesInGroup. Those orphans (UUID-only dirs / deleted jobs) still render under
// a standalone .artifacts subgroup after nesting, so this drives the trailing
// branch-glyph decision for the note list.
func (m *Model) groupHasArtifactOrphans(notesInGroup []*models.Note, jobsForGroup map[string][]*models.Note) bool {
	if len(jobsForGroup) == 0 {
		return false
	}
	owned := make(map[string]struct{}, len(notesInGroup))
	for _, note := range notesInGroup {
		if id, ok := m.jobFileToID[filepath.Base(note.Path)]; ok {
			owned[id] = struct{}{}
		}
	}
	for jobID, arts := range jobsForGroup {
		if len(arts) == 0 {
			continue
		}
		if _, isOwned := owned[jobID]; !isOwned {
			return true
		}
	}
	return false
}

// addNoteNodes renders the note items for a given group. When artifactSubgroups
// (keyed by artifactGroupKey -> jobID -> artifact notes) is supplied, any note
// that is a plan job with a matching jobID gets its artifacts nested directly
// beneath it (Phase 3). Consumed jobIDs are deleted from the subgroup map so the
// later addArtifactSubgroup() call only renders orphaned/UUID-only leftovers.
func (m *Model) addNoteNodes(
	nodes *[]*DisplayNode,
	notesInGroup []*models.Note,
	ws *workspace.WorkspaceNode,
	groupPrefix string,
	depth int,
	workspacePathMap map[string]string,
	hasFollowingSiblings bool,
	artifactSubgroups map[string]map[string][]*models.Note,
	artifactGroupKey string,
) {
	// Sort notes within the group
	m.sortNotes(notesInGroup)

	jobsForGroup := artifactSubgroups[artifactGroupKey]

	// Add note nodes
	for j, note := range notesInGroup {
		isLastNote := j == len(notesInGroup)-1 && !hasFollowingSiblings
		var notePrefix strings.Builder
		noteIndent := strings.ReplaceAll(groupPrefix, "├ ", "│ ")
		noteIndent = strings.ReplaceAll(noteIndent, "└ ", "  ")
		notePrefix.WriteString(noteIndent)
		if isLastNote {
			notePrefix.WriteString("└ ")
		} else {
			notePrefix.WriteString("├ ")
		}

		// Resolve nested artifacts for this note, if any.
		var jobID string
		var jobArtifacts []*models.Note
		if jobsForGroup != nil && m.showArtifacts {
			if id, ok := m.jobFileToID[filepath.Base(note.Path)]; ok {
				if arts, ok := jobsForGroup[id]; ok && len(arts) > 0 {
					jobID = id
					jobArtifacts = arts
				}
			}
		}

		noteNode := &DisplayNode{
			Item:               noteToItem(note),
			Prefix:             notePrefix.String(),
			Depth:              depth + 1,
			RelativePath:       calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
			HasNestedArtifacts: len(jobArtifacts) > 0,
		}
		*nodes = append(*nodes, noteNode)

		if len(jobArtifacts) == 0 {
			continue
		}

		// Consume this jobID so the orphan-mop-up subgroup won't re-render it,
		// even when the note row is folded (so it can't reappear as a sibling).
		delete(jobsForGroup, jobID)

		// Folding the job row itself hides its nested artifacts entirely.
		if m.collapsedNodes[noteNode.NodeID()] {
			continue
		}

		m.addNestedArtifacts(nodes, ws, note, notePrefix.String(), depth+1, workspacePathMap, jobArtifacts)
	}
}

// addNestedArtifacts renders an "artifacts" parent node plus its files directly
// beneath the owning job note row (Phase 3). The parent collapses by default and
// honors any later user toggle via seedCollapsedDefault.
func (m *Model) addNestedArtifacts(
	nodes *[]*DisplayNode,
	ws *workspace.WorkspaceNode,
	note *models.Note,
	notePrefix string,
	noteDepth int,
	workspacePathMap map[string]string,
	jobArtifacts []*models.Note,
) {
	// The artifacts node sits one level beneath the note; convert the note's
	// branch glyphs into vertical continuations, then attach a last-child glyph.
	childIndent := strings.ReplaceAll(notePrefix, "├ ", "│ ")
	childIndent = strings.ReplaceAll(childIndent, "└ ", "  ")

	var artifactsPrefix strings.Builder
	artifactsPrefix.WriteString(childIndent)
	artifactsPrefix.WriteString("└ ")

	artifactsItem := &tree.Item{
		Path:     filepath.Join(filepath.Dir(note.Path), ".artifacts-nested", filepath.Base(note.Path)),
		Name:     "artifacts",
		IsDir:    true,
		Type:     tree.TypeGroup,
		Metadata: make(map[string]interface{}),
	}
	artifactsItem.Metadata["Workspace"] = ws.Name
	artifactsItem.Metadata["Group"] = note.Group + "/.artifacts-nested"
	if icon := getGroupIcon(".artifacts", m.service.NoteTypes); icon != "" {
		artifactsItem.Metadata["Icon"] = icon
	}
	artifactsNode := &DisplayNode{
		Item:       artifactsItem,
		Prefix:     artifactsPrefix.String(),
		Depth:      noteDepth + 1,
		ChildCount: len(jobArtifacts),
	}
	*nodes = append(*nodes, artifactsNode)

	artifactsNodeID := artifactsNode.NodeID()
	m.seedCollapsedDefault(artifactsNodeID)
	if m.collapsedNodes[artifactsNodeID] {
		return
	}

	m.sortNotes(jobArtifacts)

	fileIndent := strings.ReplaceAll(artifactsPrefix.String(), "├ ", "│ ")
	fileIndent = strings.ReplaceAll(fileIndent, "└ ", "  ")
	for ai, art := range jobArtifacts {
		isLast := ai == len(jobArtifacts)-1
		var filePrefix strings.Builder
		filePrefix.WriteString(fileIndent)
		if isLast {
			filePrefix.WriteString("└ ")
		} else {
			filePrefix.WriteString("├ ")
		}
		*nodes = append(*nodes, &DisplayNode{
			Item:         noteToItem(art),
			Prefix:       filePrefix.String(),
			Depth:        noteDepth + 2,
			RelativePath: calculateRelativePath(art, workspacePathMap, m.focusedWorkspace),
		})
	}
}

// syntheticBucket is one bucket produced by a "Group By" axis. The id is used
// to build a stable synthetic path so the bucket's fold state survives restarts.
type syntheticBucket struct {
	id    string // stable, axis-local identifier (e.g. "today", "tag-frontend")
	label string // display label (e.g. "Today", "#frontend")
	icon  string // icon string for Item.Metadata["Icon"]
	notes []*models.Note
}

// renderSyntheticGroups partitions a group's notes by the active m.groupBy axis
// into synthetic collapsible bucket nodes inserted beneath the directory group.
// Each bucket gets a stable synthetic path (".synthetic-<axis>-<id>") so its
// NodeID() collapse key is unique and persistent. Notes inside an expanded
// bucket are rendered via addNoteNodes (which forces no further sub-grouping).
func (m *Model) renderSyntheticGroups(
	nodes *[]*DisplayNode,
	notesInGroup []*models.Note,
	ws *workspace.WorkspaceNode,
	groupPath string,
	groupName string,
	groupPrefix string,
	depth int,
	workspacePathMap map[string]string,
	hasFollowingSiblings bool,
	hasSearchFilter bool,
) {
	buckets := m.partitionNotes(notesInGroup)
	if len(buckets) == 0 {
		// Nothing to bucket: fall back to a flat note list. Artifacts stay as a
		// sibling subgroup under group-by, so no nesting map is passed.
		m.addNoteNodes(nodes, notesInGroup, ws, groupPrefix, depth, workspacePathMap, hasFollowingSiblings, nil, "")
		return
	}

	// The notes live one indentation level beneath the directory group; convert
	// the group's branch glyphs into vertical continuations for the bucket level.
	bucketParentPrefix := strings.ReplaceAll(groupPrefix, "├ ", "│ ")
	bucketParentPrefix = strings.ReplaceAll(bucketParentPrefix, "└ ", "  ")

	numBuckets := len(buckets)
	for i, bucket := range buckets {
		isLastBucket := i == numBuckets-1 && !hasFollowingSiblings

		var bucketPrefix strings.Builder
		bucketPrefix.WriteString(bucketParentPrefix)
		if isLastBucket {
			bucketPrefix.WriteString("└ ")
		} else {
			bucketPrefix.WriteString("├ ")
		}

		// Stable synthetic path -> stable NodeID() collapse key.
		synthPath := filepath.Join(groupPath, ".synthetic-"+m.groupBy+"-"+bucket.id)
		bucketItem := &tree.Item{
			Path:  synthPath,
			Name:  bucket.label,
			IsDir: true,
			Type:  tree.TypeGroup,
			Metadata: map[string]interface{}{
				"Workspace": ws.Name,
				"Icon":      bucket.icon,
				// The enclosing on-disk group, so actions targeting the bucket
				// (e.g. note creation) resolve to a real directory instead of
				// the bucket label ("Today", "P0", ...).
				"Group": groupName,
			},
		}
		bucketNode := &DisplayNode{
			Item:       bucketItem,
			Prefix:     bucketPrefix.String(),
			Depth:      depth + 1,
			ChildCount: len(bucket.notes),
		}
		*nodes = append(*nodes, bucketNode)

		// Render the bucket's notes when expanded. Artifacts remain a sibling
		// subgroup under group-by, so no nesting map is passed.
		if !m.collapsedNodes[bucketNode.NodeID()] || hasSearchFilter {
			m.addNoteNodes(nodes, bucket.notes, ws, bucketPrefix.String(), depth+1, workspacePathMap, false, nil, "")
		}
	}
}

// partitionNotes splits notes into ordered synthetic buckets according to the
// active m.groupBy axis. Empty buckets are omitted to avoid clutter.
func (m *Model) partitionNotes(notes []*models.Note) []syntheticBucket {
	switch m.groupBy {
	case "date":
		return partitionByDate(notes)
	case "status":
		return partitionByStatus(notes)
	case "tag":
		return partitionByTag(notes)
	case "priority":
		return partitionByPriority(notes)
	default:
		return nil
	}
}

// partitionByPriority buckets notes by their priority frontmatter field, most
// critical first (p0, p1, p2, p3), with unset/empty priority collected into a
// trailing "none" bucket. Empty buckets are dropped. Intra-bucket order is left
// to addNoteNodes -> sortNotes (date order), which is sufficient because every
// note in a bucket shares the same priority.
func partitionByPriority(notes []*models.Note) []syntheticBucket {
	// Ladder ordered most-critical first; "" (none) handled separately so it
	// always sorts last.
	order := []string{"p0", "p1", "p2", "p3"}
	byPriority := map[string][]*models.Note{}
	var none []*models.Note

	for _, note := range notes {
		switch priorityRank(note.Priority) {
		case "p0", "p1", "p2", "p3":
			byPriority[note.Priority] = append(byPriority[note.Priority], note)
		default:
			none = append(none, note)
		}
	}

	var buckets []syntheticBucket
	for _, p := range order {
		if len(byPriority[p]) == 0 {
			continue
		}
		buckets = append(buckets, syntheticBucket{
			id:    p,
			label: strings.ToUpper(p), // "P0".."P3"
			icon:  theme.IconFire,
			notes: byPriority[p],
		})
	}
	if len(none) > 0 {
		buckets = append(buckets, syntheticBucket{
			id:    "none",
			label: "No Priority",
			icon:  theme.IconClock,
			notes: none,
		})
	}
	return buckets
}

// partitionByDate buckets notes by CreatedAt into granular relative buckets:
// Today / Yesterday / 2 Days Ago / 3 Days Ago / This Week / Last Week /
// This Month / Icebox, relative to now. Order is fixed (most recent first);
// empty buckets are dropped.
//
// Each note lands in the first (most-recent) bucket whose threshold it is at or
// after. The relative day-buckets (today..3 days ago) take precedence over the
// calendar-week buckets, so early in the week the "3 days ago" bucket may reach
// into last week's calendar days and "This Week" may be empty — that is by
// design and produces no misrouting, since first-match gives every note exactly
// one bucket and empty buckets are dropped.
func partitionByDate(notes []*models.Note) []syntheticBucket {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1)
	twoDaysAgo := today.AddDate(0, 0, -2)
	threeDaysAgo := today.AddDate(0, 0, -3)
	weekStart := today.AddDate(0, 0, -int(today.Weekday()))
	lastWeekStart := weekStart.AddDate(0, 0, -7)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	var todayN, yesterdayN, twoN, threeN, weekN, lastWeekN, monthN, iceboxN []*models.Note
	for _, note := range notes {
		c := note.CreatedAt
		switch {
		case !c.Before(today):
			todayN = append(todayN, note)
		case !c.Before(yesterday):
			yesterdayN = append(yesterdayN, note)
		case !c.Before(twoDaysAgo):
			twoN = append(twoN, note)
		case !c.Before(threeDaysAgo):
			threeN = append(threeN, note)
		case !c.Before(weekStart):
			weekN = append(weekN, note)
		case !c.Before(lastWeekStart):
			lastWeekN = append(lastWeekN, note)
		case !c.Before(monthStart):
			monthN = append(monthN, note)
		default:
			iceboxN = append(iceboxN, note)
		}
	}

	var buckets []syntheticBucket
	appendBucket := func(id, label string, ns []*models.Note) {
		if len(ns) > 0 {
			buckets = append(buckets, syntheticBucket{id: id, label: label, icon: theme.IconCalendar, notes: ns})
		}
	}
	appendBucket("today", "Today", todayN)
	appendBucket("yesterday", "Yesterday", yesterdayN)
	appendBucket("2-days-ago", "2 Days Ago", twoN)
	appendBucket("3-days-ago", "3 Days Ago", threeN)
	appendBucket("week", "This Week", weekN)
	appendBucket("last-week", "Last Week", lastWeekN)
	appendBucket("month", "This Month", monthN)
	appendBucket("icebox", "Icebox", iceboxN)
	return buckets
}

// partitionByStatus buckets notes by note.Remote.State when present. Local notes
// with no synced remote state go in a single neutral "No Status" bucket; HasTodos
// is intentionally NOT used as a status proxy.
func partitionByStatus(notes []*models.Note) []syntheticBucket {
	stateOrder := []string{}
	byState := map[string][]*models.Note{}
	var noStatus []*models.Note

	for _, note := range notes {
		if note.Remote != nil && note.Remote.State != "" {
			state := note.Remote.State
			if _, seen := byState[state]; !seen {
				stateOrder = append(stateOrder, state)
			}
			byState[state] = append(byState[state], note)
		} else {
			noStatus = append(noStatus, note)
		}
	}
	sort.Strings(stateOrder)

	var buckets []syntheticBucket
	for _, state := range stateOrder {
		// Use the raw state as both id and a title-cased label.
		label := strings.ToUpper(state[:1]) + state[1:]
		buckets = append(buckets, syntheticBucket{
			id:    "state-" + state,
			label: label,
			icon:  theme.IconStatusCompleted,
			notes: byState[state],
		})
	}
	if len(noStatus) > 0 {
		buckets = append(buckets, syntheticBucket{
			id:    "none",
			label: "No Status",
			icon:  theme.IconClock,
			notes: noStatus,
		})
	}
	return buckets
}

// partitionByTag fans each note out across EVERY tag it carries (so a multi-tag
// note appears under each tag bucket); notes with no tags go to "untagged".
// Per-tag bucket counts therefore sum to more than the distinct note count.
func partitionByTag(notes []*models.Note) []syntheticBucket {
	tagOrder := []string{}
	byTag := map[string][]*models.Note{}
	var untagged []*models.Note

	for _, note := range notes {
		if len(note.Tags) == 0 {
			untagged = append(untagged, note)
			continue
		}
		for _, tag := range note.Tags {
			if tag == "" {
				continue
			}
			if _, seen := byTag[tag]; !seen {
				tagOrder = append(tagOrder, tag)
			}
			byTag[tag] = append(byTag[tag], note)
		}
	}
	sort.Strings(tagOrder)

	var buckets []syntheticBucket
	for _, tag := range tagOrder {
		buckets = append(buckets, syntheticBucket{
			id:    "tag-" + tag,
			label: "#" + tag,
			icon:  theme.IconFilter,
			notes: byTag[tag],
		})
	}
	if len(untagged) > 0 {
		buckets = append(buckets, syntheticBucket{
			id:    "untagged",
			label: "untagged",
			icon:  theme.IconFilter,
			notes: untagged,
		})
	}
	return buckets
}

// renderTree recursively traverses a group tree and generates DisplayNodes.
// It is a generic function configurable for rendering different types of hierarchical groups.
func (m *Model) renderTree(
	nodes *[]*DisplayNode,
	ws *workspace.WorkspaceNode,
	n *groupTreeNode,
	parentPrefix string,
	depth int,
	hasSearchFilter bool,
	workspacePathMap map[string]string,
	rootDir string,
	config treeRenderConfig,
	hasFollowingSiblings bool,
	// Subgroups are passed from the top-level call site
	archiveSubgroups map[string]map[string][]*models.Note,
	closedSubgroups map[string]map[string][]*models.Note,
	artifactSubgroups map[string]map[string][]*models.Note,
) {
	numChildren := len(n.childKeys)
	for i, key := range n.childKeys {
		child := n.children[key]
		isLastChild := (i == numChildren-1) && !hasFollowingSiblings

		// 1. Calculate prefix for this child node
		var childPrefix strings.Builder
		childPrefix.WriteString(parentPrefix)
		if isLastChild {
			childPrefix.WriteString("└ ")
		} else {
			childPrefix.WriteString("├ ")
		}

		// 2. Create DisplayNode for this directory part
		itemName := child.name
		if config.nameUsesPrefix {
			itemName = config.groupMetadataPrefix + child.fullName
		}

		groupPath := filepath.Join(rootDir, child.fullName)

		groupItem := &tree.Item{
			Path:  groupPath, // Absolute path for NodeID
			Name:  itemName,
			IsDir: true,
			Type:  config.itemType,
			Metadata: map[string]interface{}{
				"Workspace": ws.Name,
				"Group":     config.groupMetadataPrefix + child.fullName, // Full relative path
			},
		}
		childCount := len(child.notes)
		// Recursively count children notes if it's an intermediate node
		if childCount == 0 {
			var countChildren func(*groupTreeNode) int
			countChildren = func(node *groupTreeNode) int {
				count := len(node.notes)
				for _, c := range node.children {
					count += countChildren(c)
				}
				return count
			}
			childCount = countChildren(child)
		}

		groupNode := &DisplayNode{
			Item:       groupItem,
			Prefix:     childPrefix.String(),
			Depth:      depth,
			ChildCount: childCount,
		}
		*nodes = append(*nodes, groupNode)

		// 3. Recurse if not collapsed, and render notes/subgroups
		nodeID := groupNode.NodeID()
		if !m.collapsedNodes[nodeID] || hasSearchFilter {
			var nextParentPrefix string
			if isLastChild {
				nextParentPrefix = parentPrefix + "  "
			} else {
				nextParentPrefix = parentPrefix + "│ "
			}

			// Prepare recursive config: subdirectories of a plan should be TypeGroup
			recursiveConfig := config
			if config.itemType == tree.TypePlan {
				recursiveConfig.itemType = tree.TypeGroup
			}

			// Recurse for subdirectories
			m.renderTree(nodes, ws, child, nextParentPrefix, depth+1, hasSearchFilter, workspacePathMap, rootDir, recursiveConfig, false, archiveSubgroups, closedSubgroups, artifactSubgroups)

			// Render notes and special subgroups if this node corresponds to an original group
			artifactGroupKey := child.fullName
			if config.itemType == tree.TypePlan {
				artifactGroupKey = config.groupMetadataPrefix + child.fullName
			}

			hasArchives := config.includeArchives && len(archiveSubgroups[child.fullName]) > 0 && m.showArchives
			hasClosed := config.includeClosed && len(closedSubgroups[child.fullName]) > 0 && m.showArchives
			hasArtifacts := config.includeArtifacts && len(artifactSubgroups[artifactGroupKey]) > 0 && m.showArtifacts

			hasNotes := len(child.notes) > 0

			// Phase 3 nesting only applies in the flat (non-grouped) note layout.
			// When a "group by" axis is active we keep artifacts as a sibling
			// subgroup so the synthetic buckets stay legible.
			nestArtifacts := hasArtifacts && config.includeArtifacts && (m.groupBy == "" || m.groupBy == "none")

			// When nesting, determine up front whether any artifacts will remain
			// as orphans (jobIDs with no matching job note in this group). Only
			// then does a trailing standalone .artifacts subgroup render — which
			// governs the last note row's branch glyph.
			hasArtifactOrphans := hasArtifacts
			if nestArtifacts {
				hasArtifactOrphans = m.groupHasArtifactOrphans(child.notes, artifactSubgroups[artifactGroupKey])
			}

			if hasNotes || hasArchives || hasClosed || hasArtifacts {
				hasFollowingNoteSiblings := hasArchives || hasClosed || hasArtifactOrphans

				if hasNotes {
					if m.groupBy != "" && m.groupBy != "none" {
						m.renderSyntheticGroups(nodes, child.notes, ws, groupPath, config.groupMetadataPrefix+child.fullName, childPrefix.String(), depth, workspacePathMap, hasFollowingNoteSiblings, hasSearchFilter)
					} else {
						// Pass the artifact subgroups so owning job rows nest their
						// artifacts directly; consumed jobIDs are removed in place.
						subgroupsForNesting := artifactSubgroups
						if !nestArtifacts {
							subgroupsForNesting = nil
						}
						m.addNoteNodes(nodes, child.notes, ws, childPrefix.String(), depth, workspacePathMap, hasFollowingNoteSiblings, subgroupsForNesting, artifactGroupKey)
					}
				}

				fullGroupMetadata := config.groupMetadataPrefix + child.fullName

				if hasArchives {
					m.addArchiveSubgroup(nodes, ws, childPrefix.String(), archiveSubgroups[child.fullName], hasSearchFilter, workspacePathMap, groupPath, itemName, fullGroupMetadata)
				}
				if hasClosed {
					m.addClosedSubgroup(nodes, ws, childPrefix.String(), closedSubgroups[child.fullName], hasSearchFilter, workspacePathMap, groupPath, itemName, fullGroupMetadata)
				}
				// Mop up any artifacts not nested under a job row (orphans /
				// UUID-only dirs / deleted jobs) under the standalone .artifacts
				// node. After nesting consumed matched jobIDs, re-check the count.
				if config.includeArtifacts && m.showArtifacts && len(artifactSubgroups[artifactGroupKey]) > 0 {
					m.addArtifactSubgroup(nodes, ws, childPrefix.String(), artifactSubgroups[artifactGroupKey], hasSearchFilter, workspacePathMap, groupPath, itemName, fullGroupMetadata)
				}
			}
		}
	}
}

// buildGroupTree builds an in-memory tree from a flat list of group paths.
// It preserves the order of top-level groups as passed in regularGroups,
// but sorts nested children alphabetically.
func buildGroupTree(noteGroups map[string][]*models.Note, regularGroups []string) *groupTreeNode {
	root := newGroupTreeNode("", "")
	for _, groupName := range regularGroups {
		parts := strings.Split(groupName, "/")
		currentNode := root
		for i, part := range parts {
			if _, ok := currentNode.children[part]; !ok {
				fullName := strings.Join(parts[:i+1], "/")
				currentNode.children[part] = newGroupTreeNode(part, fullName)
				currentNode.childKeys = append(currentNode.childKeys, part)
			}
			currentNode = currentNode.children[part]
		}
		currentNode.notes = noteGroups[groupName]
	}

	// Sort child keys at each level for consistent rendering order
	// BUT preserve the top-level order (root.childKeys should not be sorted)
	var sortNodes func(*groupTreeNode, bool)
	sortNodes = func(n *groupTreeNode, isRoot bool) {
		if !isRoot {
			sort.Strings(n.childKeys)
		}
		for _, child := range n.children {
			sortNodes(child, false)
		}
	}
	sortNodes(root, true)

	return root
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
	matchedGroups := make(map[int]bool)
	for i, node := range fullTree {
		match := false

		if node.IsNote() {
			// Search only in note title
			title, _ := node.Item.Metadata["Title"].(string)
			match = strings.Contains(strings.ToLower(title), filter)
		} else if node.IsGroup() {
			// Search in group/plan names (strip "plans/" prefix for matching)
			displayName := node.Item.Name
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
			if node.IsGroup() {
				matchedGroups[i] = true
			}
		}
	}

	// Third pass: for directly matched groups, include all descendants
	for i, node := range fullTree {
		if !matchedGroups[i] {
			continue
		}
		// Walk forward to find all descendants (nodes with Depth > this node's depth)
		for j := i + 1; j < len(fullTree); j++ {
			if fullTree[j].Depth <= node.Depth {
				break // Exited this node's subtree
			}
			nodesToKeep[j] = true
		}
	}

	// Fourth pass: build the filtered tree
	var filteredTree []*DisplayNode
	for i, node := range fullTree {
		if nodesToKeep[i] {
			filteredTree = append(filteredTree, node)
		}
	}

	m.displayNodes = filteredTree
	m.clampCursor()
}

// FilterDisplayTreeByGitStatus filters the tree view to show only notes with git changes, preserving parent nodes.
func (m *Model) FilterDisplayTreeByGitStatus() {
	if !m.showGitModifiedOnly || m.gitFileStatus == nil {
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
		if node.IsNote() {
			normalizedPath, err := pathutil.NormalizeForLookup(node.Item.Path)
			if err != nil {
				continue
			}

			if status, exists := m.gitFileStatus[normalizedPath]; exists && strings.TrimSpace(status) != "" {
				// This note is modified, mark it and its parents to be kept
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

	// Add deleted files to the filtered tree
	m.AddDeletedFilesToTree()

	m.clampCursor()
}

// AddDeletedFilesToTree inserts synthetic entries for git-deleted files into the display tree.
// It places deleted files in their proper parent folder location in the tree.
func (m *Model) AddDeletedFilesToTree() {
	if len(m.gitDeletedFiles) == 0 {
		return
	}

	// Build set of existing paths
	existingPaths := make(map[string]bool)
	for _, node := range m.displayNodes {
		if node.Item != nil {
			normalizedPath, err := pathutil.NormalizeForLookup(node.Item.Path)
			if err == nil {
				existingPaths[normalizedPath] = true
			}
		}
	}

	// Build map of parent paths to their node index, depth, and collapsed state
	parentNodes := make(map[string]struct {
		index     int
		depth     int
		collapsed bool
	})
	for i, node := range m.displayNodes {
		if node.Item != nil && node.Item.IsDir {
			normalizedPath, err := pathutil.NormalizeForLookup(node.Item.Path)
			if err == nil {
				nodeID := node.NodeID()
				parentNodes[normalizedPath] = struct {
					index     int
					depth     int
					collapsed bool
				}{i, node.Depth, m.collapsedNodes[nodeID]}
			}
		}
	}

	// Collect deleted files to insert
	type deletedEntry struct {
		path        string
		parentDepth int
		insertAfter int
	}
	var deletedEntries []deletedEntry

	for _, deletedPath := range m.gitDeletedFiles {
		if existingPaths[deletedPath] {
			continue
		}
		existingPaths[deletedPath] = true

		parentPath := filepath.Dir(deletedPath)
		normalizedParent, _ := pathutil.NormalizeForLookup(parentPath)

		if parent, ok := parentNodes[normalizedParent]; ok {
			// Skip if parent folder is collapsed - don't show deleted files under collapsed parents
			if parent.collapsed {
				continue
			}
			insertAfter := parent.index
			for j := parent.index + 1; j < len(m.displayNodes); j++ {
				if m.displayNodes[j].Depth <= parent.depth {
					break
				}
				insertAfter = j
			}
			deletedEntries = append(deletedEntries, deletedEntry{
				path:        deletedPath,
				parentDepth: parent.depth,
				insertAfter: insertAfter,
			})
		}
		// If parent folder not found in display tree (ancestor collapsed), skip this deleted file
	}

	// Sort by insert position descending
	sort.Slice(deletedEntries, func(i, j int) bool {
		return deletedEntries[i].insertAfter > deletedEntries[j].insertAfter
	})

	// Insert deleted files
	for _, entry := range deletedEntries {
		name := filepath.Base(entry.path)
		syntheticItem := &tree.Item{
			Path:  entry.path,
			Name:  name,
			IsDir: false,
			Type:  tree.TypeNote,
		}

		depth := entry.parentDepth + 1
		if depth < 1 {
			depth = 1
		}

		syntheticNode := &DisplayNode{
			Item:         syntheticItem,
			Prefix:       strings.Repeat("│ ", depth-1) + "│ ",
			Depth:        depth,
			RelativePath: entry.path,
		}

		insertPos := entry.insertAfter + 1
		if insertPos >= len(m.displayNodes) {
			m.displayNodes = append(m.displayNodes, syntheticNode)
		} else {
			m.displayNodes = append(m.displayNodes[:insertPos], append([]*DisplayNode{syntheticNode}, m.displayNodes[insertPos:]...)...)
		}
	}
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
	for _, item := range m.allItems {
		if !item.IsDir {
			notePathsMap[item.Path] = true
		}
	}

	// Build a map of workspace name -> workspace node for quick lookup
	workspaceMap := make(map[string]*workspace.WorkspaceNode)
	for _, ws := range m.workspaces {
		workspaceMap[ws.Name] = ws
	}

	// Get unique workspace names from items
	uniqueWorkspaces := make(map[string]bool)
	for _, item := range m.allItems {
		if !item.IsDir {
			if ws, ok := item.Metadata["Workspace"].(string); ok {
				uniqueWorkspaces[ws] = true
			}
		}
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
		if node.IsNote() {
			// Try normalized path comparison
			normalizedNotePath, err := pathutil.NormalizeForLookup(node.Item.Path)
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
	m.FilterDisplayTreeByGitStatus()
	m.FilterDisplayTree()
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
	m.FilterDisplayTreeByGitStatus()
	m.FilterDisplayTree()
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
	m.FilterDisplayTreeByGitStatus()
	m.FilterDisplayTree()
}

func (m *Model) closeAllFolds() {
	for _, node := range m.displayNodes {
		if node.IsFoldable() {
			m.collapsedNodes[node.NodeID()] = true
		}
	}
	m.BuildDisplayTree()
	m.FilterDisplayTreeByGitStatus()
	m.FilterDisplayTree()
}

func (m *Model) openAllFolds() {
	m.collapsedNodes = make(map[string]bool)
	m.BuildDisplayTree()
	m.FilterDisplayTreeByGitStatus()
	m.FilterDisplayTree()
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
	m.FilterDisplayTreeByGitStatus()
	m.FilterDisplayTree()
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

	if node.IsWorkspace() {
		// Un-collapse all descendant workspaces and their note groups
		wsPath := node.Item.Path
		var wsName string
		if ws, ok := node.Item.Metadata["Workspace"].(*workspace.WorkspaceNode); ok {
			wsName = ws.Name
		}

		for _, ws := range m.workspaces {
			if strings.HasPrefix(ws.Path, wsPath) && ws.Path != wsPath {
				delete(m.collapsedNodes, "ws:"+ws.Path)
			}
		}
		for _, item := range m.allItems {
			if !item.IsDir {
				if itemWs, ok := item.Metadata["Workspace"].(string); ok && itemWs == wsName {
					if group, ok := item.Metadata["Group"].(string); ok {
						delete(m.collapsedNodes, "grp:"+group)
						if strings.HasPrefix(group, "plans/") {
							delete(m.collapsedNodes, "grp:plans")
						}
					}
				}
			}
		}
	} else if node.IsGroup() {
		// Un-collapse child groups (e.g., 'plans' contains 'plans/sub-plan')
		groupNamePrefix := node.Item.Name + "/"
		wsName, _ := node.Item.Metadata["Workspace"].(string)
		for _, item := range m.allItems {
			if !item.IsDir {
				if itemWs, ok := item.Metadata["Workspace"].(string); ok && itemWs == wsName {
					if group, ok := item.Metadata["Group"].(string); ok && strings.HasPrefix(group, groupNamePrefix) {
						delete(m.collapsedNodes, "grp:"+group)
					}
				}
			}
		}
	}

	m.BuildDisplayTree()
	m.FilterDisplayTreeByGitStatus()
	m.FilterDisplayTree()
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
	// Convert items to notes
	var allNotes []*models.Note
	for _, item := range m.allItems {
		if !item.IsDir {
			allNotes = append(allNotes, ItemToNote(item))
		}
	}
	notesToDisplay := allNotes

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
			Item:         noteToItem(note),
			Depth:        0,
			RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
		})
	}

	m.displayNodes = nodes
	m.jumpMap = make(map[rune]int) // No jump keys in flat list
	m.clampCursor()
}

// buildArchiveNotesList constructs a flat list of archived/closed notes for the
// dedicated archive view. Only notes living under a /.archive/ or /.closed/ path
// are included; everything else is hidden. Sorted by modified date descending.
func (m *Model) buildArchiveNotesList() {
	var archivedNotes []*models.Note
	for _, item := range m.allItems {
		if item.IsDir {
			continue
		}
		if strings.Contains(item.Path, "/.archive/") || strings.Contains(item.Path, "/.closed/") {
			archivedNotes = append(archivedNotes, ItemToNote(item))
		}
	}

	// Filter by tag if active
	if m.isFilteringByTag && m.selectedTag != "" {
		var taggedNotes []*models.Note
		for _, note := range archivedNotes {
			for _, tag := range note.Tags {
				if tag == m.selectedTag {
					taggedNotes = append(taggedNotes, note)
					break
				}
			}
		}
		archivedNotes = taggedNotes
	}

	sort.SliceStable(archivedNotes, func(i, j int) bool {
		return archivedNotes[i].ModifiedAt.After(archivedNotes[j].ModifiedAt)
	})

	var nodes []*DisplayNode
	workspacePathMap := make(map[string]string)
	for _, ws := range m.workspaces {
		workspacePathMap[ws.Name] = ws.Path
	}

	for _, note := range archivedNotes {
		nodes = append(nodes, &DisplayNode{
			Item:         noteToItem(note),
			Depth:        0,
			RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
		})
	}

	m.displayNodes = nodes
	m.jumpMap = make(map[rune]int) // No jump keys in flat list
	m.clampCursor()
}

// Temporary converter function to bridge the refactoring gap
// TODO: Remove this once BuildDisplayTree is fully refactored to work with tree.Item
// ItemToNote converts a tree.Item to a models.Note for backward compatibility
func ItemToNote(item *tree.Item) *models.Note {
	if item == nil {
		return nil
	}

	note := &models.Note{
		Path: item.Path,
	}

	// Extract metadata fields
	if title, ok := item.Metadata["Title"].(string); ok {
		note.Title = title
	}
	if noteType, ok := item.Metadata["Type"].(string); ok {
		note.Type = models.NoteType(noteType)
	}
	if ws, ok := item.Metadata["Workspace"].(string); ok {
		note.Workspace = ws
	}
	if group, ok := item.Metadata["Group"].(string); ok {
		note.Group = group
	}
	if branch, ok := item.Metadata["Branch"].(string); ok {
		note.Branch = branch
	}
	if tags, ok := item.Metadata["Tags"].([]string); ok {
		note.Tags = tags
	}
	if planRef, ok := item.Metadata["PlanRef"].(string); ok {
		note.PlanRef = planRef
	}
	if priority, ok := item.Metadata["Priority"].(string); ok {
		note.Priority = priority
	}
	if created, ok := item.Metadata["Created"].(time.Time); ok {
		note.CreatedAt = created
	} else {
		note.CreatedAt = item.ModTime
	}
	note.ModifiedAt = item.ModTime

	// Set note type from item type
	switch item.Type {
	case tree.TypeArtifact:
		note.IsArtifact = true
	}

	// Check if archived based on path
	note.IsArchived = strings.Contains(item.Path, "/.archive/") || strings.Contains(item.Path, "/.closed/")

	return note
}

// noteToItem converts a models.Note back to a tree.Item
// TODO: Remove this once BuildDisplayTree is fully refactored
func noteToItem(note *models.Note) *tree.Item {
	if note == nil {
		return nil
	}

	item := &tree.Item{
		Path:     note.Path,
		Name:     filepath.Base(note.Path),
		IsDir:    false,
		ModTime:  note.ModifiedAt,
		Type:     tree.TypeNote,
		Metadata: make(map[string]interface{}),
	}

	// Populate metadata
	item.Metadata["Title"] = note.Title
	item.Metadata["Type"] = string(note.Type)
	item.Metadata["Workspace"] = note.Workspace
	item.Metadata["Group"] = note.Group
	item.Metadata["Branch"] = note.Branch
	item.Metadata["Tags"] = note.Tags
	item.Metadata["PlanRef"] = note.PlanRef
	item.Metadata["Priority"] = note.Priority
	item.Metadata["Created"] = note.CreatedAt

	if note.IsArtifact {
		item.Type = tree.TypeArtifact
	}

	return item
}

// ApplyLinks iterates through the display nodes to find and link notes with their corresponding plans.
func (m *Model) ApplyLinks() {
	notesWithPlanRef := make(map[string]*DisplayNode)
	planNodes := make(map[string]*DisplayNode)

	// First pass: collect all notes with plan references and all plan nodes.
	for _, node := range m.displayNodes {
		// Reset any previous links
		node.LinkedNode = nil

		// Skip separator nodes, which have a nil Item.
		if node.Item == nil {
			continue
		}

		if !node.Item.IsDir && node.Item.Type == tree.TypeNote {
			// Check if this note has a plan_ref
			if planRef, ok := node.Item.Metadata["PlanRef"].(string); ok && planRef != "" {
				// Extract workspace name
				workspace := ""
				if ws, ok := node.Item.Metadata["Workspace"].(string); ok {
					workspace = ws
				}
				// Key by workspace + plan_ref for uniqueness
				key := fmt.Sprintf("%s:%s", workspace, planRef)
				notesWithPlanRef[key] = node
			}
		} else if node.Item.IsDir && node.Item.Type == tree.TypePlan {
			// This is a plan directory
			workspace := ""
			if ws, ok := node.Item.Metadata["Workspace"].(string); ok {
				workspace = ws
			}
			// Extract the group name (e.g., "plans/my-feature")
			group := ""
			if g, ok := node.Item.Metadata["Group"].(string); ok {
				group = g
			}
			// Key by workspace + group_name
			key := fmt.Sprintf("%s:%s", workspace, group)
			planNodes[key] = node
		}
	}

	// Second pass: connect the notes and plans.
	for key, noteNode := range notesWithPlanRef {
		if planNode, ok := planNodes[key]; ok {
			// Found a match, create the bidirectional link.
			noteNode.LinkedNode = planNode
			planNode.LinkedNode = noteNode
		}
	}
}
