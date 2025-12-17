package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/components/help"
	"github.com/mattsolo1/grove-core/tui/theme"
	"github.com/mattsolo1/grove-notebook/internal/tui/browser/components/confirm"
	"github.com/mattsolo1/grove-notebook/internal/tui/browser/views"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/service"
	"github.com/mattsolo1/grove-notebook/pkg/sync"
)

// Model is the main model for the notebook browser TUI
type Model struct {
	service   *service.Service
	workspaces []*workspace.WorkspaceNode
	allNotes   []*models.Note
	keys       KeyMap
	help       help.Model
	width      int
	height     int
	filterInput textinput.Model
	lastKey      string // For detecting 'gg' and 'z' sequences
	showArchives bool   // Whether to show .archive and .closed directories
	showArtifacts bool  // Whether to show .artifacts directories
	hideGlobal   bool   // Whether to hide the global workspace node
	showOnHold   bool   // Whether to show on-hold plans
	spinner      spinner.Model
	loadingCount int
	recentNotesMode bool  // Whether to show only recent notes
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

	// File to edit when running in Neovim plugin mode
	fileToEdit string

	// Grep mode state
	isGrepping bool // True when in content search mode

	// Tag filter mode state
	isFilteringByTag bool   // True when in tag filter mode
	selectedTag      string // The tag being filtered on
	tagPickerMode    bool   // True when showing tag picker
	tagPicker        list.Model

	// Tmux split state
	tmuxSplitPaneID string // ID of the tmux pane created for editing
	tmuxTUIPaneID   string // ID of the pane running the TUI

	// View component
	views views.Model

	// Preview Pane
	preview        viewport.Model
	previewFocused bool
	previewVisible bool   // Whether the preview pane is shown
	previewContent string
	previewFile    string // Path of the file currently in preview
}

// FileToEdit returns the file path that should be edited (for Neovim integration)
func (m Model) FileToEdit() string {
	return m.fileToEdit
}

// refreshMsg signals that a full data refresh is required.
type refreshMsg struct{}

// quitPopupMsg signals that the TUI should exit, causing the tmux popup to close.
type quitPopupMsg struct{}

// New creates a new TUI model.
func New(svc *service.Service, initialFocus *workspace.WorkspaceNode, ctx *service.WorkspaceContext) Model {
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
	viewsKeys := views.KeyMap{
		Up:           keys.Up,
		Down:         keys.Down,
		PageUp:       keys.PageUp,
		PageDown:     keys.PageDown,
		GoToTop:      keys.GoToTop,
		GoToBottom:   keys.GoToBottom,
		Fold:         keys.Fold,
		Unfold:       keys.Unfold,
		FoldPrefix:   keys.FoldPrefix,
		ToggleSelect: keys.ToggleSelect,
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
	}
}

// populateTagPicker collects all unique tags with counts and populates the tag picker, sorted by count descending
func (m *Model) populateTagPicker() {
	tagCounts := make(map[string]int)

	// Count occurrences of each tag
	for _, note := range m.allNotes {
		// Skip archived and closed notes unless showArchives is true
		if !m.showArchives && filepath.Dir(note.Path) != "" {
			if strings.Contains(note.Path, string(filepath.Separator)+".archive"+string(filepath.Separator)) ||
				strings.Contains(note.Path, string(filepath.Separator)+".closed"+string(filepath.Separator)) {
				continue
			}
		}

		for _, tag := range note.Tags {
			tagCounts[tag]++
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
		notesCmd = fetchFocusedNotesCmd(m.service, m.focusedWorkspace, m.showArtifacts)
	} else {
		notesCmd = fetchAllNotesCmd(m.service, m.showArtifacts)
	}
	return tea.Batch(
		fetchWorkspacesCmd(m.service.GetWorkspaceProvider()),
		notesCmd,
		m.updatePreviewContent(),
		m.spinner.Tick,
	)
}

// updatePreviewContent checks if the preview needs to be updated and returns a command to load the file.
func (m *Model) updatePreviewContent() tea.Cmd {
	node := m.views.GetCurrentNode()
	if node != nil && node.IsNote {
		// If the file in preview is already the selected one, do nothing.
		if m.previewFile == node.Note.Path {
			return nil
		}
		// Otherwise, load the new file.
		if m.previewVisible {
			m.statusMessage = fmt.Sprintf("Loading %s...", filepath.Base(node.Note.Path))
		}
		return loadFileContentCmd(node.Note.Path)
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

// tmuxSplitFinishedMsg is sent when a tmux split operation completes
type tmuxSplitFinishedMsg struct {
	paneID     string
	tuiPaneID  string
	err        error
	clearPanes bool // If true, clear the stored pane IDs (pane was closed)
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

// openInTmuxCmd intelligently opens a file in tmux.
// If in a popup, it opens in the main session and signals to quit.
// If not in a popup, it opens in a split and stays open.
func (m Model) openInTmuxCmd(path string) tea.Cmd {
	return func() tea.Msg {
		client, err := tmux.NewClient()
		if err != nil {
			return tmuxSplitFinishedMsg{err: fmt.Errorf("tmux client not found: %w", err)}
		}

		isPopup, err := client.IsPopup(context.Background())
		if err != nil {
			// Not in tmux or error, fall back to split behavior
			return tmuxSplitFinishedMsg{err: fmt.Errorf("IsPopup error: %w", err)}
		}

		if isPopup {
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "nvim"
			}
			ctx := context.Background()
			err := client.OpenInEditorWindow(ctx, editor, path, "notebook", 2, false)
			if err != nil {
				return tmuxSplitFinishedMsg{err: fmt.Errorf("popup mode - failed to open in editor: %w", err)}
			}
			// Close the popup explicitly before quitting
			if err := client.ClosePopup(ctx); err != nil {
				// Log error but continue - the file was opened successfully
				return tmuxSplitFinishedMsg{err: fmt.Errorf("failed to close popup: %w", err)}
			}
			return quitPopupMsg{}
		}

		// Not in a popup, use existing split behavior
		return m.openInTmuxSplitCmd(path)()
	}
}

// openInTmuxSplitCmd opens a note in a tmux split pane or reuses an existing split
func (m Model) openInTmuxSplitCmd(path string) tea.Cmd {
	return m.openInTmuxSplitCmdImpl(path, false)
}

// previewInTmuxSplitCmd opens a note in a tmux split without switching focus
func (m Model) previewInTmuxSplitCmd(path string) tea.Cmd {
	return m.openInTmuxSplitCmdImpl(path, true)
}

// openInTmuxSplitCmdImpl is the shared implementation for opening files in tmux
func (m Model) openInTmuxSplitCmdImpl(path string, switchBack bool) tea.Cmd {
	return func() tea.Msg {
		client, err := tmux.NewClient()
		if err != nil {
			return tmuxSplitFinishedMsg{err: fmt.Errorf("tmux client not found: %w", err)}
		}

		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vim" // fallback
		}

		// If we already have a split pane, verify it still exists and try to use it
		paneStillExists := false
		if m.tmuxSplitPaneID != "" {
			// Check if the pane still exists (user may have closed it)
			if client.PaneExists(context.Background(), m.tmuxSplitPaneID) {
				// Send keys to open the file in the existing editor
				// Using :e command for vim/nvim
				err = client.SendKeys(context.Background(), m.tmuxSplitPaneID, fmt.Sprintf(":e %s", path), "Enter")
				if err == nil {
					// Success! Handle focus based on mode
					if switchBack {
						// Preview mode: switch back to TUI pane
						if m.tmuxTUIPaneID != "" {
							err = client.SelectPane(context.Background(), m.tmuxTUIPaneID)
							if err != nil {
								return tmuxSplitFinishedMsg{err: fmt.Errorf("failed to switch back to TUI: %w", err)}
							}
						}
					} else {
						// Open mode: switch to editor pane
						err = client.SelectPane(context.Background(), m.tmuxSplitPaneID)
						if err != nil {
							return tmuxSplitFinishedMsg{err: fmt.Errorf("failed to switch to editor: %w", err)}
						}
					}

					// Return success with the existing pane ID
					return tmuxSplitFinishedMsg{paneID: m.tmuxSplitPaneID, err: nil}
				}
				// SendKeys failed, pane might have been closed between check and now
			}
			// Pane doesn't exist or SendKeys failed - fall through to create new split
		}

		// At this point, either no split exists or the old one is gone
		// Track if we need to clear old pane IDs
		shouldClearOldPanes := m.tmuxSplitPaneID != "" && !paneStillExists

		// No existing split, create a new one
		// First get the current pane ID (the TUI pane)
		tuiPaneID, err := client.GetCurrentPaneID(context.Background())
		if err != nil {
			return tmuxSplitFinishedMsg{err: fmt.Errorf("failed to get current pane ID: %w", err)}
		}

		// Get current pane width to calculate optimal split
		currentWidth, err := client.GetPaneWidth(context.Background(), "")
		if err != nil {
			return tmuxSplitFinishedMsg{err: fmt.Errorf("failed to get pane width: %w", err)}
		}

		// Calculate optimal TUI width based on view mode
		var tuiWidth int
		if m.views.GetViewMode() == views.TableView {
			// Table view needs more space for columns: 40% with min 70, max 120
			tuiWidth = currentWidth * 40 / 100
			if tuiWidth < 70 {
				tuiWidth = 70
			}
			if tuiWidth > 120 {
				tuiWidth = 120
			}
		} else {
			// Tree view: 30% of screen with min 40, max 80 columns
			tuiWidth = currentWidth * 30 / 100
			if tuiWidth < 40 {
				tuiWidth = 40
			}
			if tuiWidth > 80 {
				tuiWidth = 80
			}
		}
		// If screen is too narrow, just split 50/50
		if currentWidth < 120 {
			tuiWidth = 0 // 0 means default split
		}

		// Calculate editor width (remaining space after TUI and border)
		editorWidth := 0
		if tuiWidth > 0 {
			editorWidth = currentWidth - tuiWidth - 1 // -1 for tmux border
			if editorWidth < 40 {
				editorWidth = 0 // Fall back to default split if too small
			}
		}

		// The command to run in the new pane. Quote the path to handle spaces.
		commandToRun := fmt.Sprintf("%s %q", editor, path)

		// Split current pane horizontally (creating a vertical split) and run the editor.
		// The target is empty, which means the currently active pane.
		// editorWidth > 0 means we give the editor that much space, TUI keeps the rest
		paneID, err := client.SplitWindow(context.Background(), "", true, editorWidth, commandToRun)
		if err != nil {
			return tmuxSplitFinishedMsg{err: fmt.Errorf("failed to split tmux window: %w", err)}
		}

		// If preview mode, switch back to TUI pane
		if switchBack {
			err = client.SelectPane(context.Background(), tuiPaneID)
			if err != nil {
				return tmuxSplitFinishedMsg{err: fmt.Errorf("failed to switch back to TUI: %w", err)}
			}
		}

		// On success, return the pane ID and TUI pane ID
		// If we're replacing a closed pane, signal to clear the old IDs first
		return tmuxSplitFinishedMsg{paneID: paneID, tuiPaneID: tuiPaneID, clearPanes: shouldClearOldPanes, err: nil}
	}
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
