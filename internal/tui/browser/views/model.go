package views

import (
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-notebook/pkg/service"
	"github.com/mattsolo1/grove-notebook/pkg/tree"
)

// ViewMode defines the different ways to display notes.
type ViewMode int

const (
	TreeView ViewMode = iota
	TableView
)

// DisplayNode represents a single line in the hierarchical TUI view.
type DisplayNode struct {
	Item *tree.Item

	// Pre-calculated for rendering
	Prefix       string
	Depth        int
	JumpKey      rune
	ChildCount   int           // For groups: number of child items
	RelativePath string        // Shortened display path for notes
	LinkedNode   *DisplayNode  // For notes, points to the plan node; for plans, points to the note node
}

// NodeID returns a unique identifier for this node (for tracking collapsed state).
func (n *DisplayNode) NodeID() string {
	if n.Item == nil {
		return "separator"
	}
	if n.Item.IsDir {
		return "dir:" + n.Item.Path
	}
	return "file:" + n.Item.Path
}

// IsFoldable returns true if this node can be collapsed/expanded.
func (n *DisplayNode) IsFoldable() bool {
	return n.Item != nil && n.Item.IsDir
}

// GroupKey returns a unique key for this group (for selection tracking).
func (n *DisplayNode) GroupKey() string {
	if n.Item != nil && (n.Item.Type == tree.TypeGroup || n.Item.Type == tree.TypePlan) {
		if wsName, ok := n.Item.Metadata["Workspace"].(string); ok {
			return wsName + ":" + n.Item.Name
		}
	}
	return ""
}

// IsPlan returns true if this item represents a plan directory.
func (n *DisplayNode) IsPlan() bool {
	return n.Item != nil && n.Item.Type == tree.TypePlan
}

// Helper methods for backward compatibility during refactoring

// IsNote returns true if this is a note (file, not directory).
func (n *DisplayNode) IsNote() bool {
	return n.Item != nil && !n.Item.IsDir
}

// IsGroup returns true if this is a group or plan directory.
func (n *DisplayNode) IsGroup() bool {
	return n.Item != nil && n.Item.IsDir && (n.Item.Type == tree.TypeGroup || n.Item.Type == tree.TypePlan)
}

// IsWorkspace returns true if this is a workspace node.
func (n *DisplayNode) IsWorkspace() bool {
	return n.Item != nil && n.Item.Type == tree.TypeWorkspace
}

// IsSeparator returns true if this is a separator node (no Item).
func (n *DisplayNode) IsSeparator() bool {
	return n.Item == nil
}

// Model encapsulates the state for rendering and navigating the note list.
type Model struct {
	keys             KeyMap
	displayNodes     []*DisplayNode
	cursor           int
	scrollOffset     int
	viewMode         ViewMode
	sortAscending    bool
	jumpMap          map[rune]int
	collapsedNodes   map[string]bool
	selected         map[string]struct{}
	selectedGroups   map[string]struct{}
	cutPaths         map[string]struct{}
	columnVisibility map[string]bool
	width            int
	height           int
	lastKey          string // For detecting 'gg' and 'z' sequences

	// References to parent (browser) state for rendering
	service             *service.Service
	allItems            []*tree.Item
	workspaces          []*workspace.WorkspaceNode
	focusedWorkspace    *workspace.WorkspaceNode
	ecosystemPickerMode bool
	hideGlobal          bool
	showArchives        bool
	showArtifacts       bool
	showOnHold          bool
	filterValue         string
	isGrepping          bool
	isFilteringByTag    bool
	selectedTag         string
	recentNotesMode     bool
}

// New creates a new view model.
func New(keys KeyMap, columnVisibility map[string]bool) Model {
	return Model{
		keys:             keys,
		viewMode:         TreeView,
		sortAscending:    false,
		jumpMap:          make(map[rune]int),
		collapsedNodes:   make(map[string]bool), // Start with all expanded
		selected:         make(map[string]struct{}),
		selectedGroups:   make(map[string]struct{}),
		cutPaths:         make(map[string]struct{}),
		columnVisibility: columnVisibility,
	}
}

// SetSize sets the dimensions of the view.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// SetParentState provides the view with necessary data from the parent controller.
func (m *Model) SetParentState(
	service *service.Service,
	allItems []*tree.Item,
	workspaces []*workspace.WorkspaceNode,
	focused *workspace.WorkspaceNode,
	filterValue string,
	isGrepping bool,
	isFilteringByTag bool,
	selectedTag string,
	ecoPickerMode, hideGlobal, showArchives, showArtifacts, showOnHold, recentNotesMode bool,
) {
	m.service = service
	m.allItems = allItems
	m.workspaces = workspaces
	m.focusedWorkspace = focused
	m.filterValue = filterValue
	m.isGrepping = isGrepping
	m.isFilteringByTag = isFilteringByTag
	m.selectedTag = selectedTag
	m.ecosystemPickerMode = ecoPickerMode
	m.hideGlobal = hideGlobal
	m.showArchives = showArchives
	m.showArtifacts = showArtifacts
	m.showOnHold = showOnHold
	m.recentNotesMode = recentNotesMode
}

// ToggleViewMode switches between tree and table views.
func (m *Model) ToggleViewMode() {
	if m.viewMode == TreeView {
		m.viewMode = TableView
	} else {
		m.viewMode = TreeView
	}
	m.cursor = 0
}

// SetViewMode forces a specific view mode.
func (m *Model) SetViewMode(mode ViewMode) {
	m.viewMode = mode
}

// ToggleSortOrder switches between ascending and descending sort.
func (m *Model) ToggleSortOrder() {
	m.sortAscending = !m.sortAscending
	m.BuildDisplayTree()
}

// SetColumnVisibility updates the column visibility settings.
func (m *Model) SetColumnVisibility(visibility map[string]bool) {
	m.columnVisibility = visibility
}

// GetCurrentNode returns the node currently under the cursor.
func (m *Model) GetCurrentNode() *DisplayNode {
	if m.cursor >= 0 && m.cursor < len(m.displayNodes) {
		return m.displayNodes[m.cursor]
	}
	return nil
}

// GetTargetedNotePaths returns the paths of all selected notes.
func (m *Model) GetTargetedNotePaths() []string {
	if len(m.selected) > 0 {
		paths := make([]string, 0, len(m.selected))
		for path := range m.selected {
			paths = append(paths, path)
		}
		return paths
	}

	if m.cursor < len(m.displayNodes) {
		node := m.displayNodes[m.cursor]
		// Check if it's a note (file, not directory)
		if !node.Item.IsDir {
			return []string{node.Item.Path}
		}
		// If on a group, collect all notes within that group
		if node.Item.Type == tree.TypeGroup || node.Item.Type == tree.TypePlan {
			var paths []string
			wsName, _ := node.Item.Metadata["Workspace"].(string)
			groupName := node.Item.Name
			// Find all notes belonging to this group
			for _, item := range m.allItems {
				if !item.IsDir {
					itemWs, _ := item.Metadata["Workspace"].(string)
					itemGroup, _ := item.Metadata["Group"].(string)
					if itemWs == wsName && itemGroup == groupName {
						paths = append(paths, item.Path)
					}
				}
			}
			return paths
		}
	}
	return nil
}

// SetDisplayNodes updates the nodes to be displayed.
func (m *Model) SetDisplayNodes(nodes []*DisplayNode, jumpMap map[rune]int) {
	m.displayNodes = nodes
	m.jumpMap = jumpMap
	m.clampCursor()
}

// ClearSelections clears all selected notes and groups.
func (m *Model) ClearSelections() {
	m.selected = make(map[string]struct{})
	m.selectedGroups = make(map[string]struct{})
}

// SetCutPaths updates the cut paths for visual indication.
func (m *Model) SetCutPaths(paths map[string]struct{}) {
	m.cutPaths = paths
}

// ToggleFold toggles the fold state of the node under the cursor.
func (m *Model) ToggleFold() {
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

// GetViewMode returns the current view mode.
func (m *Model) GetViewMode() ViewMode {
	return m.viewMode
}

// GetCursor returns the current cursor position.
func (m *Model) GetCursor() int {
	return m.cursor
}

// GetDisplayNodes returns the current display nodes (for operations that need direct access).
func (m *Model) GetDisplayNodes() []*DisplayNode {
	return m.displayNodes
}

// SetCollapseState sets the collapsed state for all nodes.
func (m *Model) SetCollapseState(collapsed map[string]bool) {
	m.collapsedNodes = collapsed
}

// GetCollapseState returns the current collapse state.
func (m *Model) GetCollapseState() map[string]bool {
	return m.collapsedNodes
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

// GetCounts returns note count and selection counts for display.
func (m *Model) GetCounts() (noteCount, selectedNotes, selectedPlans int) {
	// Count notes in display nodes (files, not directories)
	for _, node := range m.displayNodes {
		if node.Item != nil && !node.Item.IsDir {
			noteCount++
		}
	}
	selectedNotes = len(m.selected)
	selectedPlans = len(m.selectedGroups)
	return
}

// GetSelected returns the set of selected note paths.
func (m *Model) GetSelected() map[string]struct{} {
	return m.selected
}

// GetSelectedGroups returns the set of selected group keys.
func (m *Model) GetSelectedGroups() map[string]struct{} {
	return m.selectedGroups
}
