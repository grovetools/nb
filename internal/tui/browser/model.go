package browser

import (
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/components/help"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

type viewMode int

const (
	treeView viewMode = iota
	tableView
)

// displayNode represents a single line in the hierarchical TUI view.
type displayNode struct {
	isWorkspace bool
	isGroup     bool
	isNote      bool

	workspace     *workspace.WorkspaceNode
	groupName     string
	workspaceName string // For groups, tracks which workspace they belong to
	note          *models.Note

	// Pre-calculated for rendering
	prefix  string
	depth   int
	jumpKey rune
}

// nodeID returns a unique identifier for this node (for tracking collapsed state)
func (n *displayNode) nodeID() string {
	if n.isWorkspace {
		return "ws:" + n.workspace.Path
	} else if n.isGroup {
		return "grp:" + n.groupName
	}
	return ""
}

// isFoldable returns true if this node can be collapsed/expanded
func (n *displayNode) isFoldable() bool {
	return n.isWorkspace || n.isGroup
}

// groupKey returns a unique key for this group (for selection tracking)
func (n *displayNode) groupKey() string {
	if n.isGroup {
		return n.workspaceName + ":" + n.groupName
	}
	return ""
}

// isPlan returns true if this group represents a plan directory
func (n *displayNode) isPlan() bool {
	return n.isGroup && strings.HasPrefix(n.groupName, "plans/")
}

// Model is the main model for the notebook browser TUI
type Model struct {
	service        *service.Service
	workspaces     []*workspace.WorkspaceNode
	allNotes       []*models.Note
	displayNodes   []*displayNode
	filteredNotes  []*models.Note // For table view
	cursor         int
	scrollOffset   int
	keys           KeyMap
	help           help.Model
	width          int
	height         int
	filterInput    textinput.Model
	table          table.Model
	viewMode       viewMode
	sortColumn     int
	sortAsc        bool
	jumpMap        map[rune]int
	lastKey        string // For detecting 'gg' and 'z' sequences
	collapsedNodes map[string]bool // Tracks collapsed workspaces and groups
	showArchives   bool            // Whether to show .archive directories

	// Focus mode state
	ecosystemPickerMode bool
	focusedWorkspace    *workspace.WorkspaceNode

	// Selection and archiving state
	selected          map[string]struct{} // Tracks selected note paths
	selectedGroups    map[string]struct{} // Tracks selected groups (workspace:groupName)
	statusMessage     string
	confirmingArchive bool

	// File to edit when running in Neovim plugin mode
	fileToEdit string
}

// FileToEdit returns the file path that should be edited (for Neovim integration)
func (m Model) FileToEdit() string {
	return m.fileToEdit
}

// New creates a new TUI model.
func New(svc *service.Service) Model {
	helpModel := help.NewBuilder().
		WithKeys(keys).
		WithTitle("Notebook Browser - Help").
		Build()

	// Define table columns
	columns := []table.Column{
		{Title: " ", Width: 3}, // Selection indicator
		{Title: "WORKSPACE", Width: 20},
		{Title: "TYPE", Width: 15},
		{Title: "TITLE", Width: 40},
		{Title: "MODIFIED", Width: 20},
	}

	tbl := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(15), // Will be resized
	)

	ti := textinput.New()
	ti.Placeholder = "Search notes..."
	ti.CharLimit = 100

	return Model{
		service:        svc,
		keys:           keys,
		help:           helpModel,
		viewMode:       treeView, // Default to tree view
		table:          tbl,
		filterInput:    ti,
		sortColumn:     4,                           // Default sort by modified date (adjusted for new column)
		sortAsc:        false,                       // Descending
		jumpMap:        make(map[rune]int),
		collapsedNodes: make(map[string]bool),
		selected:       make(map[string]struct{}),  // Initialize selection map
		selectedGroups: make(map[string]struct{}),  // Initialize group selection map
	}
}

// Init initializes the TUI.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		fetchWorkspacesCmd(m.service.GetWorkspaceProvider()),
		fetchAllNotesCmd(m.service),
	)
}

// editorFinishedMsg is sent when the editor closes
type editorFinishedMsg struct{ err error }

// editFileAndQuitMsg signals to quit and let neovim plugin handle opening
type editFileAndQuitMsg struct{ filePath string }

// notesArchivedMsg is sent when notes and/or plans have been archived
type notesArchivedMsg struct {
	archivedPaths []string
	archivedPlans int
	err           error
}

// openInEditor opens a note in the configured editor
func (m Model) openInEditor(path string) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim" // fallback
	}
	cmd := exec.Command(editor, path)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorFinishedMsg{err: err}
	})
}
