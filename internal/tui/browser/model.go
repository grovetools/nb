package browser

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/components/help"
	"github.com/mattsolo1/grove-core/tui/theme"
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
	prefix     string
	depth      int
	jumpKey    rune
	childCount int // For groups: number of notes/plans in the group
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
	confirmingDelete  bool

	// Clipboard state
	clipboard     []string            // Paths of notes to be cut/copied
	clipboardMode string              // "cut" or "copy"
	cutPaths      map[string]struct{} // For visual indication of cut items

	// Note creation state
	isCreatingNote     bool   // True when in the note creation flow
	noteCreationMode   string // "context" or "inbox"
	noteCreationStep   int    // 0: type picker, 1: title input
	noteTypePicker     list.Model
	noteTitleInput     textinput.Model
	noteCreationCursor int // Cursor position when creation started

	// Note rename state
	isRenamingNote bool
	renameInput    textinput.Model
	noteToRename   *models.Note

	// Column Visibility
	columnVisibility map[string]bool
	columnSelectMode bool
	columnList       list.Model
	availableColumns []string

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

	// Note creation setup
	noteTitleInput := textinput.New()
	noteTitleInput.Placeholder = "Enter note title..."
	noteTitleInput.CharLimit = 200
	noteTitleInput.Width = 60

	noteTypes := []list.Item{
		noteTypeItem("current"), noteTypeItem("llm"), noteTypeItem("learn"),
		noteTypeItem("daily"), noteTypeItem("issues"), noteTypeItem("architecture"),
		noteTypeItem("todos"), noteTypeItem("blog"), noteTypeItem("prompts"),
		noteTypeItem("quick"),
	}
	noteTypePicker := list.New(noteTypes, noteTypeDelegate{}, 40, 12)
	noteTypePicker.Title = "Select Note Type"
	noteTypePicker.SetShowHelp(false)
	noteTypePicker.SetShowStatusBar(false)
	noteTypePicker.SetShowPagination(false)
	noteTypePicker.SetFilteringEnabled(false)

	// Note rename setup
	renameInput := textinput.New()
	renameInput.Placeholder = "Enter new title..."
	renameInput.CharLimit = 200
	renameInput.Width = 60

	// Column Visibility Setup - load from state
	availableColumns := []string{"TYPE", "STATUS", "TAGS", "CREATED"}

	// Load saved state
	state, err := loadState()
	if err != nil {
		// On error, use defaults
		state = &tuiState{
			ColumnVisibility: map[string]bool{
				"TYPE":    true,
				"STATUS":  true,
				"TAGS":    true,
				"CREATED": true,
			},
		}
	}

	columnVisibility := state.ColumnVisibility
	var columnItems []list.Item
	for _, col := range availableColumns {
		columnItems = append(columnItems, columnSelectItem{name: col, selected: columnVisibility[col]})
	}

	columnList := list.New(columnItems, columnSelectDelegate{}, 40, 8)
	columnList.Title = "Toggle Column Visibility"
	columnList.SetShowHelp(false)
	columnList.SetFilteringEnabled(false)
	columnList.SetShowStatusBar(false)
	columnList.SetShowPagination(false)

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
		selected:          make(map[string]struct{}), // Initialize selection map
		selectedGroups:    make(map[string]struct{}), // Initialize group selection map
		cutPaths:          make(map[string]struct{}), // Initialize cut paths map
		focusedWorkspace:  initialFocus,
		noteTitleInput:    noteTitleInput,
		noteTypePicker:    noteTypePicker,
		renameInput:       renameInput,
		columnVisibility:  columnVisibility,
		columnSelectMode:  false,
		columnList:        columnList,
		availableColumns:  availableColumns,
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

// notesDeletedMsg is sent when notes have been deleted
type notesDeletedMsg struct {
	deletedPaths []string
	err          error
}

// notesPastedMsg is sent after a paste operation
type notesPastedMsg struct {
	pastedCount int
	newPaths    []string
	err         error
}

// noteCreatedMsg is sent after a note is created
type noteCreatedMsg struct {
	note *models.Note
	err  error
}

// noteRenamedMsg is sent after a note is renamed
type noteRenamedMsg struct {
	oldPath string
	newPath string
	err     error
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

// noteTypeItem implements the list.Item interface for the note type picker.
type noteTypeItem string

func (i noteTypeItem) FilterValue() string { return string(i) }
func (i noteTypeItem) Title() string       { return string(i) }
func (i noteTypeItem) Description() string { return "" }

// noteTypeDelegate is a custom delegate with minimal spacing for the note type picker
type noteTypeDelegate struct{}

func (d noteTypeDelegate) Height() int                             { return 1 }
func (d noteTypeDelegate) Spacing() int                            { return 0 }
func (d noteTypeDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d noteTypeDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(noteTypeItem)
	if !ok {
		return
	}

	str := string(i)
	if index == m.Index() {
		str = lipgloss.NewStyle().Foreground(theme.DefaultTheme.Colors.Orange).Render("│ " + str)
	} else {
		str = "  " + str
	}

	fmt.Fprint(w, str)
}

// columnSelectItem represents an item in the column visibility list
type columnSelectItem struct {
	name     string
	selected bool
}

func (i columnSelectItem) FilterValue() string { return i.name }
func (i columnSelectItem) Title() string {
	if i.selected {
		return "[x] " + i.name
	}
	return "[ ] " + i.name
}
func (i columnSelectItem) Description() string { return "" }

// columnSelectDelegate is a custom delegate with minimal spacing
type columnSelectDelegate struct{}

func (d columnSelectDelegate) Height() int                             { return 1 }
func (d columnSelectDelegate) Spacing() int                            { return 0 }
func (d columnSelectDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d columnSelectDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(columnSelectItem)
	if !ok {
		return
	}

	str := i.Title()
	if index == m.Index() {
		str = lipgloss.NewStyle().Foreground(theme.DefaultTheme.Colors.Orange).Render("│ " + str)
	} else {
		str = "  " + str
	}

	fmt.Fprint(w, str)
}

// getColumnListItems returns the current list items for the column selector
func (m *Model) getColumnListItems() []list.Item {
	var items []list.Item
	for _, col := range m.availableColumns {
		items = append(items, columnSelectItem{
			name:     col,
			selected: m.columnVisibility[col],
		})
	}
	return items
}

// tuiState holds persistent TUI settings
type tuiState struct {
	ColumnVisibility map[string]bool `json:"column_visibility"`
}

// getStateFilePath returns the path to the TUI state file
func getStateFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	stateDir := filepath.Join(home, ".grove", "nb")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(stateDir, "tui-state.json"), nil
}

// loadState loads the TUI state from disk
func loadState() (*tuiState, error) {
	path, err := getStateFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default state if file doesn't exist
			return &tuiState{
				ColumnVisibility: map[string]bool{
					"TYPE":    true,
					"STATUS":  true,
					"TAGS":    true,
					"CREATED": true,
				},
			}, nil
		}
		return nil, err
	}

	var state tuiState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// saveState saves the TUI state to disk
func (m *Model) saveState() error {
	path, err := getStateFilePath()
	if err != nil {
		return err
	}

	state := tuiState{
		ColumnVisibility: m.columnVisibility,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
