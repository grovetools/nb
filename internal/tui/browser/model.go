package browser

import (
	"os"
	"os/exec"
	"strings"

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
	isSeparator bool

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
	cursor         int
	scrollOffset   int
	keys           KeyMap
	help           help.Model
	width          int
	height         int
	filterInput    textinput.Model
	viewMode       viewMode
	sortAscending  bool
	jumpMap        map[rune]int
	lastKey        string // For detecting 'gg' and 'z' sequences
	collapsedNodes map[string]bool // Tracks collapsed workspaces and groups
	showArchives   bool            // Whether to show .archive directories
	hideGlobal     bool            // Whether to hide the global workspace node

	// Focus mode state
	ecosystemPickerMode bool
	focusedWorkspace    *workspace.WorkspaceNode
	focusChanged        bool // Tracks if focus just changed (to reset collapse state)

	// Selection and archiving state
	selected          map[string]struct{} // Tracks selected note paths
	selectedGroups    map[string]struct{} // Tracks selected groups (workspace:groupName)
	statusMessage     string
	confirmingArchive bool

	// File to edit when running in Neovim plugin mode
	fileToEdit string

	// Grep mode state
	isGrepping bool // True when in content search mode
}

// FileToEdit returns the file path that should be edited (for Neovim integration)
func (m Model) FileToEdit() string {
	return m.fileToEdit
}

// New creates a new TUI model.
func New(svc *service.Service, initialFocus *workspace.WorkspaceNode) Model {
	helpModel := help.NewBuilder().
		WithKeys(keys).
		WithTitle("Notebook Browser - Help").
		Build()

	ti := textinput.New()
	ti.Placeholder = "Search notes..."
	ti.CharLimit = 100

	return Model{
		service:          svc,
		keys:             keys,
		help:             helpModel,
		viewMode:         treeView, // Default to tree view
		filterInput:      ti,
		sortAscending:    false, // Descending by default
		jumpMap:          make(map[rune]int),
		collapsedNodes: map[string]bool{
			"ws:::global": true, // Start with global workspace collapsed
		},
		selected:         make(map[string]struct{}), // Initialize selection map
		selectedGroups:   make(map[string]struct{}), // Initialize group selection map
		focusedWorkspace: initialFocus,
	}
}

// Init initializes the TUI.
func (m Model) Init() tea.Cmd {
	var notesCmd tea.Cmd
	if m.focusedWorkspace != nil {
		notesCmd = fetchFocusedNotesCmd(m.service, m.focusedWorkspace)
	} else {
		notesCmd = fetchAllNotesCmd(m.service)
	}
	return tea.Batch(
		fetchWorkspacesCmd(m.service.GetWorkspaceProvider()),
		notesCmd,
	)
}

// editorFinishedMsg is sent when the editor closes
type editorFinishedMsg struct{ err error }

// editFileAndQuitMsg signals to quit and let neovim plugin handle opening
type editFileAndQuitMsg struct{ filePath string }

// previewFileMsg signals to preview a file in neovim (without focusing)
type previewFileMsg struct{ filePath string }

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
