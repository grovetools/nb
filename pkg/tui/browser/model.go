package browser

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/components/help"
	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/core/tui/theme"
	"github.com/grovetools/nb/pkg/tui/browser/components/confirm"
	"github.com/grovetools/nb/pkg/tui/browser/views"
	"github.com/grovetools/nb/pkg/models"
	"github.com/grovetools/nb/pkg/service"
	"github.com/grovetools/nb/pkg/sync"
	"github.com/grovetools/nb/pkg/tree"
)

// Model is the main model for the notebook browser TUI
type Model struct {
	service    *service.Service
	workspaces []*workspace.WorkspaceNode
	allItems   []*tree.Item
	keys       KeyMap
	help       help.Model
	width      int
	height     int
	filterInput textinput.Model
	sequence    *keymap.SequenceState // For detecting multi-key sequences (gg, dd, z*)
	showArchives        bool   // Whether to show .archive and .closed directories
	showArtifacts       bool   // Whether to show .artifacts directories
	hideGlobal          bool   // Whether to hide the global workspace node
	showOnHold          bool   // Whether to show on-hold plans
	showGitModifiedOnly bool   // Whether to show only notes with git changes
	spinner             spinner.Model
	loadingCount        int
	recentNotesMode     bool // Whether to show only recent notes
	savedViewMode views.ViewMode // View mode to restore when exiting recent notes mode
	savedModVisibility bool      // Saved visibility state for MODIFIED column
	savedWsVisibility  bool      // Saved visibility state for WORKSPACE column

	// Focus mode state
	ecosystemPickerMode bool
	focusedWorkspace    *workspace.WorkspaceNode
	focusChanged        bool // Tracks if focus just changed (to reset collapse state)

	// Selection and archiving state
	statusMessage string
	confirmDialog confirm.Model

	// Clipboard state
	clipboard     []string // Paths of notes to be cut/copied
	clipboardMode string   // "cut" or "copy"

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

	// Note promotion state
	isPromotingToPlan bool // True when confirming worktree creation for a new plan
	noteToPromote     *models.Note

	// Column Visibility
	columnVisibility map[string]bool
	columnSelectMode bool
	columnList       list.Model
	availableColumns []string

	// Grep mode state
	isGrepping bool // True when in content search mode

	// Tag filter mode state
	isFilteringByTag bool   // True when in tag filter mode
	selectedTag      string // The tag being filtered on
	tagPickerMode    bool   // True when showing tag picker
	tagPicker        list.Model

	// View component
	views views.Model

	// Preview Pane
	preview        viewport.Model
	previewFocused bool
	previewVisible bool   // Whether the preview pane is shown
	previewContent string
	previewFile    string // Path of the file currently in preview

	// Git status state
	gitFileStatus    map[string]string // Key: normalized absolute path, Value: git status code
	gitDeletedFiles  []string          // Paths of deleted files (don't exist on disk)
	scannedGitRepos  map[string]bool   // Key: git root path

	// Commit dialog state
	isCommitting bool
	commitInput  textinput.Model
}

// refreshMsg signals that a full data refresh is required.
type refreshMsg struct{}

// Config configures a browser Model. It is the single entry point used by both
// the standalone CLI and embedding hosts (e.g. grove terminal).
type Config struct {
	Service      *service.Service
	InitialFocus *workspace.WorkspaceNode
	Context      *service.WorkspaceContext
}

// New creates a new browser TUI model from a Config.
func New(cfg Config) Model {
	svc := cfg.Service
	initialFocus := cfg.InitialFocus
	ctx := cfg.Context
	// Load user-configurable keybindings
	keys := NewKeyMap(svc.CoreConfig)

	helpModel := help.NewBuilder().
		WithKeys(keys).
		WithTitle("Notebook Browser - Help").
		Build()

	ti := textinput.New()
	ti.Placeholder = "Search notes..."
	ti.CharLimit = 100

	confirmDialog := confirm.New()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(theme.DefaultTheme.Colors.Orange)

	// Note creation setup
	noteTitleInput := textinput.New()
	noteTitleInput.Placeholder = "Enter note title..."
	noteTitleInput.CharLimit = 200
	noteTitleInput.Width = 60

	// Get note types dynamically from the service
	configuredTypes, err := svc.ListNoteTypes(ctx.NotebookContextWorkspace)
	if err != nil {
		// Fallback on error
		configuredTypes = []models.NoteType{"inbox", "quick", "learn"}
	}
	var noteTypes []list.Item
	for _, t := range configuredTypes {
		noteTypes = append(noteTypes, noteTypeItem(t))
	}
	// Calculate height based on number of types + title + padding (min 10, max 25)
	pickerHeight := len(noteTypes) + 4
	if pickerHeight < 10 {
		pickerHeight = 10
	}
	if pickerHeight > 25 {
		pickerHeight = 25
	}
	noteTypePicker := list.New(noteTypes, noteTypeDelegate{}, 40, pickerHeight)
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

	// Commit dialog setup
	commitInput := textinput.New()
	commitInput.Placeholder = "Update notes"
	commitInput.CharLimit = 200
	commitInput.Width = 60

	// Column Visibility Setup - load from state
	availableColumns := []string{"TYPE", "STATUS", "TAGS", "WORKSPACE", "CREATED", "MODIFIED", "PATH"}

	// Load saved state
	state, err := loadState()
	if err != nil {
		// On error, use defaults
		state = &tuiState{
			ColumnVisibility: map[string]bool{
				"TYPE":      true,
				"STATUS":    true,
				"TAGS":      true,
				"WORKSPACE": false,
				"CREATED":   true,
				"MODIFIED":  false,
				"PATH":      true,
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

	// Initialize tag picker (will be populated when opened)
	// Start with a reasonable default height, will be adjusted in populateTagPicker
	tagPicker := list.New([]list.Item{}, tagDelegate{}, 40, 20)
	tagPicker.Title = "Select Tag"
	tagPicker.SetShowHelp(false)
	tagPicker.SetFilteringEnabled(false)
	tagPicker.SetShowStatusBar(false)
	tagPicker.SetShowPagination(false)

	// Initialize views with KeyMap converted to views.KeyMap
	// Use Base fields for standard navigation/fold bindings
	viewsKeys := views.KeyMap{
		Up:           keys.Up,
		Down:         keys.Down,
		Left:         keys.Left,
		Right:        keys.Right,
		PageUp:       keys.PageUp,
		PageDown:     keys.PageDown,
		Top:          keys.Top,
		Bottom:       keys.Bottom,
		FoldOpen:     keys.FoldOpen,
		FoldClose:    keys.FoldClose,
		FoldToggle:   keys.FoldToggle,
		FoldOpenAll:  keys.FoldOpenAll,
		FoldCloseAll: keys.FoldCloseAll,
		Select: keys.Select,
		SelectNone:   keys.SelectNone,
	}
	viewsModel := views.New(viewsKeys, columnVisibility)

	// Initialize preview viewport
	preview := viewport.New(80, 20) // Initial size, will be updated on WindowSizeMsg
	preview.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.DefaultTheme.Colors.MutedText)

	return Model{
		service:           svc,
		keys:              keys,
		help:              helpModel,
		filterInput:       ti,
		sequence:          keymap.NewSequenceState(),
		spinner:           s,
		loadingCount:      2, // For initial workspaces + notes load
		showArchives:      false, // Default to hiding archives
		showArtifacts:     false, // Default to hiding artifacts
		focusedWorkspace:  initialFocus,
		focusChanged:      initialFocus != nil, // Trigger initial collapse state setup
		noteTitleInput:    noteTitleInput,
		noteTypePicker:    noteTypePicker,
		renameInput:       renameInput,
		columnVisibility:  columnVisibility,
		columnSelectMode:  false,
		columnList:        columnList,
		availableColumns:  availableColumns,
		confirmDialog:     confirmDialog,
		clipboard:         []string{},
		tagPicker:         tagPicker,
		views:             viewsModel,
		preview:           preview,
		previewFocused:    false,
		previewVisible:    false, // Preview hidden by default
		recentNotesMode:   false,
		gitFileStatus:     make(map[string]string),
		scannedGitRepos:   make(map[string]bool),
		commitInput:       commitInput,
	}
}

// populateTagPicker collects all unique tags with counts and populates the tag picker, sorted by count descending
func (m *Model) populateTagPicker() {
	tagCounts := make(map[string]int)

	// Count occurrences of each tag
	for _, item := range m.allItems {
		// Skip archived and closed items unless showArchives is true
		if !m.showArchives && filepath.Dir(item.Path) != "" {
			if strings.Contains(item.Path, string(filepath.Separator)+".archive"+string(filepath.Separator)) ||
				strings.Contains(item.Path, string(filepath.Separator)+".closed"+string(filepath.Separator)) {
				continue
			}
		}

		// Extract tags from metadata
		if tags, ok := item.Metadata["Tags"].([]string); ok {
			for _, tag := range tags {
				tagCounts[tag]++
			}
		}
	}

	// Convert to tagItem slice
	type tagCount struct {
		tag   string
		count int
	}
	var tags []tagCount
	for tag, count := range tagCounts {
		tags = append(tags, tagCount{tag: tag, count: count})
	}

	// Sort by count descending
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].count > tags[j].count
	})

	// Convert to list items
	var items []list.Item
	for _, tc := range tags {
		items = append(items, tagItem{tag: tc.tag, count: tc.count})
	}

	m.tagPicker.SetItems(items)

	// Adjust height based on number of tags (min 10, max 25)
	pickerHeight := len(items) + 4
	if pickerHeight < 10 {
		pickerHeight = 10
	}
	if pickerHeight > 25 {
		pickerHeight = 25
	}
	m.tagPicker.SetSize(40, pickerHeight)
}

// Init initializes the TUI.
func (m Model) Init() tea.Cmd {
	var notesCmd tea.Cmd
	if m.focusedWorkspace != nil {
		notesCmd = fetchFocusedItemsCmd(m.service, m.focusedWorkspace, m.showArtifacts)
	} else {
		notesCmd = fetchAllItemsCmd(m.service, m.showArtifacts)
	}
	return tea.Batch(
		fetchWorkspacesCmd(m.service.GetWorkspaceProvider()),
		notesCmd,
		m.updatePreviewContent(),
		m.spinner.Tick,
	)
}

// updatePreviewContent checks if the preview needs to be updated and returns a command to load the file.
// When preview is visible, it also emits an embed.PreviewRequestMsg so the terminal host can
// open/update a PTY-based preview split (nvim -R in the VDrawer).
func (m *Model) updatePreviewContent() tea.Cmd {
	node := m.views.GetCurrentNode()
	if node != nil && node.IsNote() {
		// If the file in preview is already the selected one, do nothing.
		if m.previewFile == node.Item.Path {
			return nil
		}
		// Otherwise, load the new file.
		if m.previewVisible {
			m.statusMessage = fmt.Sprintf("Loading %s...", filepath.Base(node.Item.Path))
		}
		path := node.Item.Path
		cmds := []tea.Cmd{loadFileContentCmd(path)}
		// Emit preview request for the terminal host (VDrawer with nvim -R).
		if m.previewVisible {
			cmds = append(cmds, func() tea.Msg {
				return embed.PreviewRequestMsg{Path: path}
			})
		}
		return tea.Batch(cmds...)
	}

	// If not on a note, clear the preview.
	if m.previewFile != "" {
		m.previewFile = ""
		m.previewContent = ""
		m.preview.SetContent("Select a note to preview its content.")
	}
	return nil
}

// fileContentReadyMsg is sent when a file's content has been read from disk.
type fileContentReadyMsg struct {
	path    string
	content string
	err     error
}

// editorFinishedMsg is sent when the editor closes
type editorFinishedMsg struct{ err error }

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

// syncFinishedMsg is sent when a sync operation completes
type syncFinishedMsg struct {
	reports []*sync.Report
	err     error
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

// noteTypeItem implements the list.Item interface for the note type picker.
type noteTypeItem string

func (i noteTypeItem) FilterValue() string { return string(i) }
func (i noteTypeItem) Title() string       { return string(i) }
func (i noteTypeItem) Description() string { return "" }

// tagItem implements the list.Item interface for the tag picker.
type tagItem struct {
	tag   string
	count int
}

func (i tagItem) FilterValue() string { return i.tag }
func (i tagItem) Title() string       { return i.tag }
func (i tagItem) Description() string { return fmt.Sprintf("%d notes", i.count) }

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

// tagDelegate is a custom delegate with minimal spacing for the tag picker
type tagDelegate struct{}

func (d tagDelegate) Height() int                             { return 1 }
func (d tagDelegate) Spacing() int                            { return 0 }
func (d tagDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d tagDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(tagItem)
	if !ok {
		return
	}

	// Format with count
	str := fmt.Sprintf("%s (%d)", i.tag, i.count)
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
	stateDir := filepath.Join(paths.StateDir(), "nb")
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
					"TYPE":      true,
					"STATUS":    true,
					"TAGS":      true,
					"WORKSPACE": false,
					"CREATED":   true,
					"PATH":      true,
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
