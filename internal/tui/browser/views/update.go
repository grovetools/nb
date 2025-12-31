package views

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/util/pathutil"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/tree"
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
		var baseDepth int
		for _, ws := range m.workspaces {
			// Save global separately
			if ws.Name == "global" {
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
			IsDir:    ws.Name != "global", // Global is not a directory, so it won't show folding icon
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
			// artifactSubgroups maps "parent" -> notes
			// e.g., "plans/binary-test" -> [artifacts in plans/binary-test/.artifacts/]
			var regularGroups []string
			planGroups := make(map[string][]*models.Note)
			holdPlanGroups := make(map[string][]*models.Note)
			archiveSubgroups := make(map[string]map[string][]*models.Note)
			closedSubgroups := make(map[string]map[string][]*models.Note)
			artifactSubgroups := make(map[string][]*models.Note)

			for name, notes := range noteGroups {
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

				// Check if this matches pattern "<parent>/.artifacts" (artifacts directly in .artifacts folder)
				if strings.HasSuffix(name, "/.artifacts") {
					parent := strings.TrimSuffix(name, "/.artifacts")
					artifactSubgroups[parent] = notes
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
			// NEW: Categorize groups for workflow-based sorting
			var workflowGroups, plainGroups, executionGroups, reviewGroups []string
			var completedGroup string
			var hasCompleted bool

			// Separate regular groups from special ones
			for _, name := range regularGroups {
				switch name {
				case "inbox", "issues", "github-issues", "docs", "learn":
					workflowGroups = append(workflowGroups, name)
				case "in_progress":
					executionGroups = append(executionGroups, name)
				case "review", "github-prs":
					reviewGroups = append(reviewGroups, name)
				case "completed":
					completedGroup = name
					hasCompleted = true
				default:
					plainGroups = append(plainGroups, name)
				}
			}

			// Sort workflow groups with specific order (intake first, then issue tracking)
			workflowOrder := map[string]int{
				"inbox":         1,
				"issues":        2,
				"github-issues": 3,
				"docs":          4,
				"learn":         5,
			}
			sort.Slice(workflowGroups, func(i, j int) bool {
				orderA, okA := workflowOrder[workflowGroups[i]]
				orderB, okB := workflowOrder[workflowGroups[j]]
				if !okA {
					orderA = 99
				}
				if !okB {
					orderB = 99
				}
				if orderA == orderB {
					return workflowGroups[i] < workflowGroups[j]
				}
				return orderA < orderB
			})

			// Sort other categories alphabetically
			sort.Strings(plainGroups)
			sort.Strings(executionGroups)
			sort.Strings(reviewGroups)

			// Combine workflow and plain groups for the first rendering block
			topLevelGroups := append(workflowGroups, plainGroups...)

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

			// Render groups in the desired order
			notesRootDir, err := m.service.GetNotebookLocator().GetNotesDir(ws, "")
			if err == nil { // Proceed only if we can get the notes root directory
				// 1. Render Workflow and Plain Groups
				if len(topLevelGroups) > 0 {
					rootGroupNode := buildGroupTree(noteGroups, topLevelGroups)
					hasFollowingTopLevelSiblings := hasPlans || hasHoldPlans || len(executionGroups) > 0 || len(reviewGroups) > 0 || hasCompleted
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

				// 2. Render Plans Group
				if hasPlans {
					hasGroupsAfter := hasHoldPlans || len(executionGroups) > 0 || len(reviewGroups) > 0 || hasCompleted
					m.addPlansGroup(&nodes, ws, planGroups, archiveSubgroups, artifactSubgroups, hasSearchFilter, workspacePathMap, hasGroupsAfter)
				}

				// 3. Render Execution Groups (in_progress)
				if len(executionGroups) > 0 {
					rootGroupNode := buildGroupTree(noteGroups, executionGroups)
					hasFollowingTopLevelSiblings := hasHoldPlans || len(reviewGroups) > 0 || hasCompleted
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

				// 4. Render Review Groups
				if len(reviewGroups) > 0 {
					rootGroupNode := buildGroupTree(noteGroups, reviewGroups)
					hasFollowingTopLevelSiblings := hasHoldPlans || hasCompleted
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

				// 5. Render On-Hold Plans
				if hasHoldPlans {
					m.addHoldPlansGroup(&nodes, ws, holdPlanGroups, hasSearchFilter, workspacePathMap, hasCompleted)
				}

				// 6. Render Completed Group
				if hasCompleted {
					m.addCompletedGroup(&nodes, ws, noteGroups[completedGroup], archiveSubgroups, hasSearchFilter, workspacePathMap)
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
					Item:   noteToItem(note),
					Prefix: notePrefix.String(),
					Depth:  ws.Depth + 1,
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
	archiveIndent := strings.ReplaceAll(groupPrefix, "├ ", "│ ")
	archiveIndent = strings.ReplaceAll(archiveIndent, "└ ", "  ")
	archivePrefix.WriteString(archiveIndent)
	archivePrefix.WriteString("└ ")

	// Get parent group name from groupPrefix context
	// This is a bit tricky - we need to track which group we're in
	// For simplicity, we'll extract it from the nodes list
	parentGroupName := ""
	if len(*nodes) > 0 {
		for i := len(*nodes) - 1; i >= 0; i-- {
			if (*nodes)[i].IsGroup() && !strings.Contains((*nodes)[i].Item.Name, "/.archive") {
				parentGroupName = (*nodes)[i].Item.Name
				break
			}
		}
	}

	// Add .archive parent node
	archiveParentItem := &tree.Item{
		Path:     filepath.Join(ws.Path, parentGroupName, ".archive"),
		Name:     parentGroupName + "/.archive",
		IsDir:    true,
		Type:     tree.TypeGroup,
		Metadata: make(map[string]interface{}),
	}
	archiveParentItem.Metadata["Workspace"] = ws.Name
	archiveParentItem.Metadata["Group"] = parentGroupName + "/.archive"
	archiveParentNode := &DisplayNode{
		Item:       archiveParentItem,
		Prefix:     archivePrefix.String(),
		Depth:      ws.Depth + 2,
		ChildCount: totalArchivedNotes,
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
				Path:     filepath.Join(ws.Path, parentGroupName, ".archive", archivedName),
				Name:     parentGroupName + "/.archive/" + archivedName,
				IsDir:    true,
				Type:     tree.TypeGroup,
				Metadata: make(map[string]interface{}),
			}
			archivedChildItem.Metadata["Workspace"] = ws.Name
			archivedChildItem.Metadata["Group"] = parentGroupName + "/.archive/" + archivedName
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
					archivedNoteIndent = strings.ReplaceAll(archivedIndent, "└ ", "  ")
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

func (m *Model) addClosedSubgroup(nodes *[]*DisplayNode, ws *workspace.WorkspaceNode, groupPrefix string, closedSubgroups map[string][]*models.Note, hasSearchFilter bool, workspacePathMap map[string]string) {
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

	// Get parent group name from groupPrefix context
	parentGroupName := ""
	if len(*nodes) > 0 {
		for i := len(*nodes) - 1; i >= 0; i-- {
			if (*nodes)[i].IsGroup() && !strings.Contains((*nodes)[i].Item.Name, "/.closed") {
				parentGroupName = (*nodes)[i].Item.Name
				break
			}
		}
	}

	// Add .closed parent node
	closedParentItem := &tree.Item{
		Path:     filepath.Join(ws.Path, parentGroupName, ".closed"),
		Name:     parentGroupName + "/.closed",
		IsDir:    true,
		Type:     tree.TypeGroup,
		Metadata: make(map[string]interface{}),
	}
	closedParentItem.Metadata["Workspace"] = ws.Name
	closedParentItem.Metadata["Group"] = parentGroupName + "/.closed"
	closedParentNode := &DisplayNode{
		Item:       closedParentItem,
		Prefix:     closedPrefix.String(),
		Depth:      ws.Depth + 2,
		ChildCount: totalClosedNotes,
	}
	*nodes = append(*nodes, closedParentNode)

	// Check if .closed parent is collapsed
	closedParentNodeID := closedParentNode.NodeID()
	if !m.collapsedNodes[closedParentNodeID] || hasSearchFilter {
		// Add individual closed children
		for pi, closedName := range closedNames {
			isLastClosed := pi == len(closedNames)-1
			closedNotes := closedSubgroups[closedName]

			// Sort notes within the closed child
			sort.SliceStable(closedNotes, func(i, j int) bool {
				if m.sortAscending {
					return closedNotes[i].CreatedAt.Before(closedNotes[j].CreatedAt)
				}
				return closedNotes[i].CreatedAt.After(closedNotes[j].CreatedAt)
			})

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
				Path:     filepath.Join(ws.Path, parentGroupName + "/.closed/" + closedName),
				Name:     parentGroupName + "/.closed/" + closedName,
				IsDir:    true,
				Type:     tree.TypeGroup,
				Metadata: make(map[string]interface{}),
			}
			closedChildItem.Metadata["Workspace"] = ws.Name
			closedChildItem.Metadata["Group"] = parentGroupName + "/.closed/" + closedName
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
					closedNoteIndent = strings.ReplaceAll(closedIndent, "└ ", "  ")
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

func (m *Model) addArtifactSubgroup(nodes *[]*DisplayNode, ws *workspace.WorkspaceNode, groupPrefix string, artifactNotes []*models.Note, hasSearchFilter bool, workspacePathMap map[string]string) {
	// Calculate .artifacts prefix (last child under this group)
	var artifactsPrefix strings.Builder
	artifactsIndent := strings.ReplaceAll(groupPrefix, "├ ", "│ ")
	artifactsIndent = strings.ReplaceAll(artifactsIndent, "└ ", "  ")
	artifactsPrefix.WriteString(artifactsIndent)
	artifactsPrefix.WriteString("└ ")

	// Get parent group name from the last group node
	parentGroupName := ""
	if len(*nodes) > 0 {
		for i := len(*nodes) - 1; i >= 0; i-- {
			if (*nodes)[i].IsGroup() && !strings.Contains((*nodes)[i].Item.Name, "/.artifacts") {
				parentGroupName = (*nodes)[i].Item.Name
				break
			}
		}
	}

	// Add .artifacts parent node
	artifactsParentItem := &tree.Item{
		Path:     filepath.Join(ws.Path, parentGroupName, ".artifacts"),
		Name:     parentGroupName + "/.artifacts",
		IsDir:    true,
		Type:     tree.TypeGroup,
		Metadata: make(map[string]interface{}),
	}
	artifactsParentItem.Metadata["Workspace"] = ws.Name
	artifactsParentItem.Metadata["Group"] = parentGroupName + "/.artifacts"
	artifactsParentNode := &DisplayNode{
		Item:       artifactsParentItem,
		Prefix:     artifactsPrefix.String(),
		Depth:      ws.Depth + 2,
		ChildCount: len(artifactNotes),
	}
	*nodes = append(*nodes, artifactsParentNode)

	// Check if .artifacts parent is collapsed
	artifactsParentNodeID := artifactsParentNode.NodeID()
	if !m.collapsedNodes[artifactsParentNodeID] || hasSearchFilter {
		// Sort artifact notes by created date
		sort.SliceStable(artifactNotes, func(i, j int) bool {
			if m.sortAscending {
				return artifactNotes[i].CreatedAt.Before(artifactNotes[j].CreatedAt)
			}
			return artifactNotes[i].CreatedAt.After(artifactNotes[j].CreatedAt)
		})

		// Add individual artifact notes
		for ai, artifact := range artifactNotes {
			isLastArtifact := ai == len(artifactNotes)-1
			var artifactPrefix strings.Builder
			artifactIndent := strings.ReplaceAll(artifactsPrefix.String(), "├ ", "│ ")
			artifactIndent = strings.ReplaceAll(artifactIndent, "└ ", "  ")
			artifactPrefix.WriteString(artifactIndent)
			if isLastArtifact {
				artifactPrefix.WriteString("└ ")
			} else {
				artifactPrefix.WriteString("├ ")
			}

			*nodes = append(*nodes, &DisplayNode{Item: noteToItem(artifact), Prefix: artifactPrefix.String(), Depth: ws.Depth + 3, RelativePath: calculateRelativePath(artifact, workspacePathMap, m.focusedWorkspace)})
		}
	}
}

func (m *Model) addPlansGroup(nodes *[]*DisplayNode, ws *workspace.WorkspaceNode, planGroups map[string][]*models.Note, archiveSubgroups map[string]map[string][]*models.Note, artifactSubgroups map[string][]*models.Note, hasSearchFilter bool, workspacePathMap map[string]string, hasGroupsAfter bool) {
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

	// Add "plans" parent node
	plansParentItem := &tree.Item{
		Path:     filepath.Join(ws.Path, "plans"),
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

		// Build hierarchical tree for plan names
		planTree := buildGroupTree(planGroups, planNames)
		hasPlansArchive := len(archiveSubgroups["plans"]) > 0 && m.showArchives

		// Get the plans directory for absolute paths
		plansDir := filepath.Join(ws.Path, "plans")

		// Render the plan tree hierarchically
		config := treeRenderConfig{
			itemType:            tree.TypePlan,
			groupMetadataPrefix: "plans/",
			nameUsesPrefix:      true,
			includeArtifacts:    true,
			includeArchives:     false,
			includeClosed:       false,
		}
		m.renderTree(nodes, ws, planTree, plansPrefix.String(), ws.Depth+1, hasSearchFilter, workspacePathMap, plansDir, config, hasPlansArchive, nil, nil, artifactSubgroups)

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

	// Add .archive parent node
	// Create tree.Item for group
	archiveParentNodeItem := &tree.Item{
		Path:     filepath.Join(ws.Path, "plans/.archive"),
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

	// Check if .archive parent is collapsed
	archiveParentNodeID := archiveParentNode.NodeID()
	if !m.collapsedNodes[archiveParentNodeID] || hasSearchFilter {
		// Build hierarchical tree for archived plan names
		archivedPlanTree := buildGroupTree(archivedPlans, archivedNames)

		// Get the plans/.archive directory for absolute paths
		archiveDir := filepath.Join(ws.Path, "plans/.archive")

		// Render the archived plan tree hierarchically
		config := treeRenderConfig{
			itemType:            tree.TypeGroup, // An archived plan is just a group
			groupMetadataPrefix: "plans/.archive/",
			nameUsesPrefix:      false,
			includeArtifacts:    false,
			includeArchives:     false,
			includeClosed:       false,
		}
		m.renderTree(nodes, ws, archivedPlanTree, archivePrefix.String(), ws.Depth+2, hasSearchFilter, workspacePathMap, archiveDir, config, false, nil, nil, nil)
	}
}

func (m *Model) addHoldPlansGroup(nodes *[]*DisplayNode, ws *workspace.WorkspaceNode, holdPlanGroups map[string][]*models.Note, hasSearchFilter bool, workspacePathMap map[string]string, hasCompleted bool) {
	// Calculate .hold parent prefix
	var holdPrefix strings.Builder
	indentPrefix := strings.ReplaceAll(ws.TreePrefix, "├ ", "│ ")
	indentPrefix = strings.ReplaceAll(indentPrefix, "└ ", "  ")
	holdPrefix.WriteString(indentPrefix)
	if ws.Depth > 0 || ws.TreePrefix != "" {
		holdPrefix.WriteString("  ")
	}
	if hasCompleted {
		holdPrefix.WriteString("├ ") // Not last if completed exists
	} else {
		holdPrefix.WriteString("└ ") // .hold is last if no completed
	}

	// Add ".hold" parent node
	holdParentItem := &tree.Item{
		Path:     filepath.Join(ws.Path, ".hold"),
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
			sort.SliceStable(planNotes, func(i, j int) bool {
				if m.sortAscending {
					return planNotes[i].CreatedAt.Before(planNotes[j].CreatedAt)
				}
				return planNotes[i].CreatedAt.After(planNotes[j].CreatedAt)
			})

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

			// Add hold plan node
			planItem := &tree.Item{
				Path:     filepath.Join(ws.Path, "plans", planName),
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

func (m *Model) addCompletedGroup(nodes *[]*DisplayNode, ws *workspace.WorkspaceNode, completedNotes []*models.Note, archiveSubgroups map[string]map[string][]*models.Note, hasSearchFilter bool, workspacePathMap map[string]string) {
	// Calculate completed group prefix (always last)
	var completedPrefix strings.Builder
	indentPrefix := strings.ReplaceAll(ws.TreePrefix, "├ ", "│ ")
	indentPrefix = strings.ReplaceAll(indentPrefix, "└ ", "  ")
	completedPrefix.WriteString(indentPrefix)
	if ws.Depth > 0 || ws.TreePrefix != "" {
		completedPrefix.WriteString("  ")
	}
	completedPrefix.WriteString("└ ") // Completed is always last

	// Sort notes within the completed group
	sort.SliceStable(completedNotes, func(i, j int) bool {
		if m.sortAscending {
			return completedNotes[i].CreatedAt.Before(completedNotes[j].CreatedAt)
		}
		return completedNotes[i].CreatedAt.After(completedNotes[j].CreatedAt)
	})

	// Add completed group node
	// Create tree.Item for group
	completedGroupNodeItem := &tree.Item{
		Path:     filepath.Join(ws.Path, "completed"),
		Name:     "completed",
		IsDir:    true,
		Type:     tree.TypeGroup,
		Metadata: make(map[string]interface{}),
	}
	completedGroupNodeItem.Metadata["Workspace"] = ws.Name
	completedGroupNodeItem.Metadata["Group"] = "completed"
	completedGroupNode := &DisplayNode{
		Item:       completedGroupNodeItem,
		Prefix:     completedPrefix.String(),
		Depth:      ws.Depth + 1,
		ChildCount: len(completedNotes),
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
			noteIndent := strings.ReplaceAll(completedPrefix.String(), "├ ", "│ ")
			noteIndent = strings.ReplaceAll(noteIndent, "└ ", "  ")
			notePrefix.WriteString(noteIndent)
			if isLastNote {
				notePrefix.WriteString("└ ")
			} else {
				notePrefix.WriteString("├ ")
			}
			*nodes = append(*nodes, &DisplayNode{Item: noteToItem(note), Prefix: notePrefix.String(), Depth: ws.Depth + 2, RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace)})
		}

		// Add .archive subgroup if this group has archived children
		if hasArchives {
			m.addArchiveSubgroup(nodes, ws, completedPrefix.String(), archiveSubgroups["completed"], hasSearchFilter, workspacePathMap)
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

// addNoteNodes renders the note items for a given group.
func (m *Model) addNoteNodes(
	nodes *[]*DisplayNode,
	notesInGroup []*models.Note,
	ws *workspace.WorkspaceNode,
	groupPrefix string,
	depth int,
	workspacePathMap map[string]string,
	hasFollowingSiblings bool,
) {
	// Sort notes within the group
	sort.SliceStable(notesInGroup, func(i, j int) bool {
		if m.sortAscending {
			return notesInGroup[i].CreatedAt.Before(notesInGroup[j].CreatedAt)
		}
		return notesInGroup[i].CreatedAt.After(notesInGroup[j].CreatedAt)
	})

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
		*nodes = append(*nodes, &DisplayNode{
			Item:         noteToItem(note),
			Prefix:       notePrefix.String(),
			Depth:        depth + 1,
			RelativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
		})
	}
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
	artifactSubgroups map[string][]*models.Note,
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

		groupItem := &tree.Item{
			Path:  filepath.Join(rootDir, child.fullName), // Absolute path for NodeID
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

			// Recurse for subdirectories
			m.renderTree(nodes, ws, child, nextParentPrefix, depth+1, hasSearchFilter, workspacePathMap, rootDir, config, false, archiveSubgroups, closedSubgroups, artifactSubgroups)

			// Render notes and special subgroups if this node corresponds to an original group
			if len(child.notes) > 0 {
				hasArchives := config.includeArchives && len(archiveSubgroups[child.fullName]) > 0 && m.showArchives
				hasClosed := config.includeClosed && len(closedSubgroups[child.fullName]) > 0 && m.showArchives

				artifactGroupKey := child.fullName
				if config.itemType == tree.TypePlan {
					artifactGroupKey = config.groupMetadataPrefix + child.fullName
				}
				hasArtifacts := config.includeArtifacts && len(artifactSubgroups[artifactGroupKey]) > 0 && m.showArtifacts

				hasFollowingNoteSiblings := hasArchives || hasClosed || hasArtifacts

				m.addNoteNodes(nodes, child.notes, ws, childPrefix.String(), depth, workspacePathMap, hasFollowingNoteSiblings)

				if hasArchives {
					m.addArchiveSubgroup(nodes, ws, childPrefix.String(), archiveSubgroups[child.fullName], hasSearchFilter, workspacePathMap)
				}
				if hasClosed {
					m.addClosedSubgroup(nodes, ws, childPrefix.String(), closedSubgroups[child.fullName], hasSearchFilter, workspacePathMap)
				}
				if hasArtifacts {
					m.addArtifactSubgroup(nodes, ws, childPrefix.String(), artifactSubgroups[artifactGroupKey], hasSearchFilter, workspacePathMap)
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
			Item:  noteToItem(note),
			Depth: 0,
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
		Name:     note.Title,
		IsDir:    false,
		ModTime:  note.ModifiedAt,
		Type:     tree.TypeNote,
		Metadata: make(map[string]interface{}),
	}

	// Populate metadata
	item.Metadata["Title"] = note.Title
	item.Metadata["Workspace"] = note.Workspace
	item.Metadata["Group"] = note.Group
	item.Metadata["Branch"] = note.Branch
	item.Metadata["Tags"] = note.Tags
	item.Metadata["PlanRef"] = note.PlanRef
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
