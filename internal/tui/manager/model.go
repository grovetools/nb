package manager

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/tui/components/help"
	"github.com/mattsolo1/grove-core/tui/keymap"
	"github.com/mattsolo1/grove-core/tui/theme"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

// Keymap for the note manager TUI
type managerKeyMap struct {
	keymap.Base
}

func (k managerKeyMap) ShortHelp() []key.Binding {
	// Return empty to show no help in footer - all help goes in popup
	return []key.Binding{}
}

func (k managerKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Navigation")),
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓, j/k", "Move cursor")),
			key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "Page up")),
			key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "Page down")),
			key.NewBinding(key.WithKeys("g"), key.WithHelp("gg", "Go to top")),
			key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "Go to bottom")),
			key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "Toggle sort order")),
			key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "Search notes")),
			key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "Filter by type")),
			key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "Clear all filters")),
		},
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Actions")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "Open note in editor")),
			key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "Toggle selection")),
			key.NewBinding(key.WithKeys("shift+up", "shift+down"), key.WithHelp("shift+↑/↓", "Select range")),
			key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "Select all")),
			key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "Deselect all")),
			key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "Archive selected")),
			key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "Quit")),
		},
	}
}

var managerKeys = managerKeyMap{Base: keymap.NewBase()}

// Model represents the state of the note manager TUI
type Model struct {
	table         table.Model
	allNotes      []*models.Note      // Master unfiltered list
	notes         []*models.Note      // Currently visible (potentially filtered) list
	selected      map[string]struct{} // Set of selected note paths
	service       *service.Service
	context       *service.WorkspaceContext
	quitting      bool
	confirming    bool // Confirmation mode for archiving
	message       string
	width         int
	height        int
	lastCursor    int  // For shift+click selection
	sortNewest    bool // true = newest first, false = oldest first
	filtering     bool // Whether filter mode is active
	filterInput   textinput.Model
	selectingType bool       // Whether type selection mode is active
	typeList      list.Model // List for type selection
	activeFilter  string     // Currently active type filter
	help          help.Model
}

// Styles
var (
	baseStyle = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(theme.DefaultColors.Border)

	headerStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.DefaultColors.LightText).
		Background(theme.DefaultColors.SelectedBackground)

	selectedStyle = lipgloss.NewStyle().
		Foreground(theme.DefaultColors.LightText).
		Background(theme.DefaultColors.SelectedBackground)

	dimStyle = lipgloss.NewStyle().
		Foreground(theme.DefaultColors.MutedText)

	helpStyle = lipgloss.NewStyle().
		Foreground(theme.DefaultColors.MutedText)

	messageStyle = lipgloss.NewStyle().
		Foreground(theme.DefaultColors.Green)

	warningStyle = lipgloss.NewStyle().
		Foreground(theme.DefaultColors.Yellow)
)

// typeItem implements list.Item for the type selection list
type typeItem struct {
	name  string
	count int
}

func (i typeItem) FilterValue() string { return i.name }
func (i typeItem) Title() string       { return i.name }
func (i typeItem) Description() string { return fmt.Sprintf("%d notes", i.count) }

// getUniqueTypes extracts unique types from notes with counts
func getUniqueTypes(notes []*models.Note) []list.Item {
	typeCounts := make(map[string]int)
	for _, note := range notes {
		typeCounts[string(note.Type)]++
	}
	
	// Add "All" option at the beginning
	items := []list.Item{
		typeItem{name: "All", count: len(notes)},
	}
	
	// Sort types alphabetically
	var types []string
	for t := range typeCounts {
		types = append(types, t)
	}
	sort.Strings(types)
	
	// Create list items
	for _, t := range types {
		items = append(items, typeItem{name: t, count: typeCounts[t]})
	}
	
	return items
}

// New creates a new Model instance
func New(notes []*models.Note, svc *service.Service, ctx *service.WorkspaceContext) Model {
	// Sort notes by modified date (newest first)
	sort.Slice(notes, func(i, j int) bool {
		return notes[i].ModifiedAt.After(notes[j].ModifiedAt)
	})

	// Calculate dynamic width for TYPE column
	maxTypeWidth := 10 // Minimum width
	for _, note := range notes {
		if len(string(note.Type)) > maxTypeWidth {
			maxTypeWidth = len(string(note.Type))
		}
	}
	if maxTypeWidth > 25 { // Cap the width
		maxTypeWidth = 25
	}
	maxTypeWidth += 2 // Add padding

	columns := []table.Column{
		{Title: "", Width: 4},              // Hidden SEL header, slightly wider for spacing
		{Title: "TYPE", Width: maxTypeWidth},
		{Title: "TITLE", Width: 50},
		{Title: "MODIFIED", Width: 16},
	}

	rows := make([]table.Row, len(notes))
	for i, note := range notes {
		modified := formatTime(note.ModifiedAt)
		rows[i] = table.Row{
			" ",
			string(note.Type),
			truncate(note.Title, 50),
			modified,
		}
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(15),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(theme.DefaultColors.Border).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(theme.DefaultColors.LightText).
		Background(theme.DefaultColors.SelectedBackground).
		Bold(false)
	t.SetStyles(s)

	// Initialize text input for filtering
	ti := textinput.New()
	ti.Placeholder = "Search notes..."
	ti.CharLimit = 100

	// Initialize type selection list
	typeItems := getUniqueTypes(notes)
	typeListDelegate := list.NewDefaultDelegate()
	typeListDelegate.ShowDescription = true
	typeList := list.New(typeItems, typeListDelegate, 0, 0)
	typeList.Title = "Select Type to Filter"
	typeList.SetShowHelp(false)
	typeList.SetFilteringEnabled(false)
	
	// Style the list
	typeList.Styles.Title = headerStyle

	helpModel := help.NewBuilder().
		WithKeys(managerKeys).
		WithTitle("📝 Note Manager - Help").
		Build()

	return Model{
		table:         t,
		allNotes:      notes,
		notes:         notes, // Initially, visible list is the full list
		selected:      make(map[string]struct{}),
		service:       svc,
		context:       ctx,
		lastCursor:    0,
		sortNewest:    true,
		filtering:     false,
		filterInput:   ti,
		selectingType: false,
		typeList:      typeList,
		activeFilter:  "",
		help:          helpModel,
	}
}

// truncate shortens a string to fit within a given width
func truncate(s string, width int) string {
	if len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}

// formatTime formats a time for display
func formatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("2006-01-02")
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetHeight(m.height - 8) // Leave space for header and help
		// Also resize the type list
		m.typeList.SetWidth(m.width / 2)
		m.typeList.SetHeight(m.height / 2)
		return m, nil

	case editorFinishedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Error opening editor: %v", msg.err)
		}
		return m, nil

	case editFileAndQuitMsg:
		// Print protocol string and quit - Neovim plugin will handle the file opening
		fmt.Printf("EDIT_FILE:%s\n", msg.filePath)
		return m, tea.Quit

	case tea.KeyMsg:
		if m.help.ShowAll {
			m.help.Toggle() // Any key closes help
			return m, nil
		}

		if m.confirming {
			return m.handleConfirmation(msg)
		}

		// Handle type selection mode
		if m.selectingType {
			switch msg.String() {
			case "enter":
				// Apply selected type filter
				if selectedItem, ok := m.typeList.SelectedItem().(typeItem); ok {
					if selectedItem.name == "All" {
						m.activeFilter = ""
					} else {
						m.activeFilter = selectedItem.name
					}
					m.applyTypeFilter()
					m.message = fmt.Sprintf("Filter: %s", selectedItem.name)
				}
				m.selectingType = false
				return m, nil
			case "esc", "q":
				// Cancel type selection
				m.selectingType = false
				return m, nil
			default:
				// Pass to type list
				m.typeList, cmd = m.typeList.Update(msg)
				return m, cmd
			}
		}

		// Handle filtering mode
		if m.filtering {
			switch msg.String() {
			case "enter":
				// Exit filter mode
				m.filtering = false
				m.filterInput.Blur()
				return m, nil
			case "esc":
				// Clear filter and exit filter mode
				m.filterInput.SetValue("")
				m.applyFilter()
				m.filtering = false
				m.filterInput.Blur()
				return m, nil
			default:
				// Update filter input
				var filterCmd tea.Cmd
				m.filterInput, filterCmd = m.filterInput.Update(msg)
				m.applyFilter()
				return m, filterCmd
			}
		}

		switch msg.String() {
		case "?":
			m.help.Toggle()
			return m, nil

		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		
		case "t":
			// Enter type selection mode
			m.selectingType = true
			// Update the type list with current notes
			m.typeList.SetItems(getUniqueTypes(m.allNotes))
			return m, nil
		
		case "c":
			// Clear all filters
			m.activeFilter = ""
			m.filterInput.SetValue("")
			m.notes = m.allNotes
			m.sortNotes()
			m.message = "Filters cleared"
			return m, nil
		
		case "/":
			// Enter filter mode
			m.filtering = true
			m.filterInput.Focus()
			return m, textinput.Blink

		case "enter":
			// Open the note under cursor in editor
			cursor := m.table.Cursor()
			if cursor < len(m.notes) {
				note := m.notes[cursor]
				// Check if running inside Neovim plugin
				if os.Getenv("GROVE_NVIM_PLUGIN") == "true" {
					return m, func() tea.Msg {
						return editFileAndQuitMsg{filePath: note.Path}
					}
				}
				return m, m.openInEditor(note.Path)
			}

		case " ":
			// Toggle selection
			cursor := m.table.Cursor()
			if cursor < len(m.notes) {
				note := m.notes[cursor]
				if _, ok := m.selected[note.Path]; ok {
					delete(m.selected, note.Path)
				} else {
					m.selected[note.Path] = struct{}{}
				}
				m.lastCursor = cursor
				// Rebuild the table with updated selection
				m.updateTableRows()
			}

		case "shift+down":
			// Select range downward
			cursor := m.table.Cursor()
			if cursor < len(m.notes)-1 {
				newCursor := cursor + 1
				m.selectRange(m.lastCursor, newCursor)
				m.table.SetCursor(newCursor)
			}

		case "shift+up":
			// Select range upward
			cursor := m.table.Cursor()
			if cursor > 0 {
				newCursor := cursor - 1
				m.selectRange(m.lastCursor, newCursor)
				m.table.SetCursor(newCursor)
			}

		case "a":
			// Select all
			for _, note := range m.notes {
				m.selected[note.Path] = struct{}{}
			}
			m.updateTableRows()
			m.message = fmt.Sprintf("Selected all %d notes", len(m.notes))

		case "n":
			// Deselect all
			m.selected = make(map[string]struct{})
			m.updateTableRows()
			m.message = "Deselected all notes"

		case "x":
			// Archive selected
			if len(m.selected) > 0 {
				m.confirming = true
				m.message = fmt.Sprintf("Archive %d notes? (y/n)", len(m.selected))
			} else {
				m.message = "No notes selected"
			}

		case "s":
			// Toggle sort order
			m.sortNewest = !m.sortNewest
			m.sortNotes()
			if m.sortNewest {
				m.message = "Sorted by newest first"
			} else {
				m.message = "Sorted by oldest first"
			}

		default:
			m.table, cmd = m.table.Update(msg)
		}

	default:
		m.table, cmd = m.table.Update(msg)
	}

	return m, cmd
}

// handleConfirmation handles key presses during confirmation mode
func (m Model) handleConfirmation(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		// Perform archive
		count := len(m.selected)
		if err := m.archiveSelected(); err != nil {
			m.message = fmt.Sprintf("Error archiving: %v", err)
		} else {
			m.message = fmt.Sprintf("Archived %d notes", count)
			// Remove archived notes from the display
			m.removeArchivedNotes()
		}
		m.confirming = false
		m.selected = make(map[string]struct{})

	case "n", "esc":
		m.confirming = false
		m.message = "Archive cancelled"
	}

	return m, nil
}

// makeRow creates a table row for a note
func (m Model) makeRow(note *models.Note, selected bool) table.Row {
	sel := " "
	if selected {
		sel = "✓"
	}
	return table.Row{
		sel,
		string(note.Type),
		truncate(note.Title, 50),
		formatTime(note.ModifiedAt),
	}
}

// archiveSelected archives all selected notes
func (m *Model) archiveSelected() error {
	// Collect paths of selected notes
	paths := make([]string, 0, len(m.selected))
	for path := range m.selected {
		paths = append(paths, path)
	}
	
	// Archive the notes using the current context
	if err := m.service.ArchiveNotes(m.context, paths); err != nil {
		return fmt.Errorf("failed to archive notes: %w", err)
	}
	
	return nil
}

// removeArchivedNotes removes archived notes from the display
func (m *Model) removeArchivedNotes() {
	// Create new slices without archived notes
	var remainingAllNotes []*models.Note
	for _, note := range m.allNotes {
		if _, archived := m.selected[note.Path]; !archived {
			remainingAllNotes = append(remainingAllNotes, note)
		}
	}
	
	var remainingNotes []*models.Note
	for _, note := range m.notes {
		if _, archived := m.selected[note.Path]; !archived {
			remainingNotes = append(remainingNotes, note)
		}
	}
	
	m.allNotes = remainingAllNotes
	m.notes = remainingNotes
	m.updateTableRows()
	
	// Adjust cursor if needed
	if m.table.Cursor() >= len(m.notes) && len(m.notes) > 0 {
		m.table.SetCursor(len(m.notes) - 1)
	}
}

// updateTableRows rebuilds the table rows based on current state
func (m *Model) updateTableRows() {
	rows := make([]table.Row, len(m.notes))
	for i, note := range m.notes {
		_, isSelected := m.selected[note.Path]
		rows[i] = m.makeRow(note, isSelected)
	}
	m.table.SetRows(rows)
}

// View renders the UI
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	// If help is visible, show it and return
	if m.help.ShowAll {
		m.help.SetSize(m.width, m.height)
		return m.help.View()
	}

	// If in type selection mode, show only the type list
	if m.selectingType {
		return m.typeList.View()
	}

	var s strings.Builder

	// Header with workspace context
	workspace := m.context.Workspace.Name
	branch := m.context.Branch
	contextStr := workspace
	if branch != "" {
		contextStr = fmt.Sprintf("%s (%s)", workspace, branch)
	}
	
	// Add sort indicator
	sortIndicator := "↓"
	if !m.sortNewest {
		sortIndicator = "↑"
	}
	
	header := headerStyle.Render(fmt.Sprintf("  Notes Manager - %s %s  ", contextStr, sortIndicator))
	s.WriteString(header + "\n\n")

	// Show active type filter if any
	if m.activeFilter != "" {
		s.WriteString(messageStyle.Render(fmt.Sprintf("Type Filter: %s", m.activeFilter)) + "\n\n")
	}

	// Filter input (if in filter mode)
	if m.filtering {
		s.WriteString("Search: " + m.filterInput.View() + "\n\n")
	} else if m.filterInput.Value() != "" {
		s.WriteString(dimStyle.Render(fmt.Sprintf("Search: %s", m.filterInput.Value())) + "\n\n")
	}

	// Message
	if m.message != "" && !m.selectingType {
		if m.confirming {
			s.WriteString(warningStyle.Render(m.message) + "\n\n")
		} else {
			s.WriteString(messageStyle.Render(m.message) + "\n\n")
		}
	}

	// Table
	s.WriteString(m.table.View() + "\n")

	// Status line
	status := fmt.Sprintf("%d notes, %d selected", len(m.notes), len(m.selected))
	s.WriteString(dimStyle.Render(status) + "\n\n")

	// Help
	s.WriteString(helpStyle.Render("Press ? for help"))

	return s.String()
}

// openInEditor opens a note in the configured editor
func (m Model) openInEditor(path string) tea.Cmd {
	// Get editor from environment or use default
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Create command
	cmd := exec.Command(editor, path)
	
	// Use tea.ExecProcess to properly handle terminal
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return editorFinishedMsg{err: err}
		}
		return editorFinishedMsg{}
	})
}

// editorFinishedMsg is sent when the editor closes
type editorFinishedMsg struct {
	err error
}

// editFileAndQuitMsg signals to quit and let neovim plugin handle opening
type editFileAndQuitMsg struct {
	filePath string
}

// selectRange selects all notes between start and end indices
func (m *Model) selectRange(start, end int) {
	// Ensure start is less than end
	if start > end {
		start, end = end, start
	}

	// Select all notes in range (without clearing existing selections)
	for i := start; i <= end && i < len(m.notes); i++ {
		m.selected[m.notes[i].Path] = struct{}{}
	}

	m.updateTableRows()
}

// sortNotes sorts the notes based on current sort order
func (m *Model) sortNotes() {
	sort.Slice(m.notes, func(i, j int) bool {
		if m.sortNewest {
			return m.notes[i].ModifiedAt.After(m.notes[j].ModifiedAt)
		}
		return m.notes[i].ModifiedAt.Before(m.notes[j].ModifiedAt)
	})
	
	// Update table rows after sorting
	m.updateTableRows()
	
	// Reset cursor to top
	m.table.SetCursor(0)
	m.lastCursor = 0
}

// applyFilter filters the notes based on the current filter input
func (m *Model) applyFilter() {
	filterValue := m.filterInput.Value()
	
	if filterValue == "" {
		// No filter, show all notes
		m.notes = m.allNotes
	} else {
		// Filter notes by searching across multiple fields (case-insensitive)
		filterLower := strings.ToLower(filterValue)
		var filtered []*models.Note
		for _, note := range m.allNotes {
			// Search in type, title, and path
			typeMatch := strings.Contains(strings.ToLower(string(note.Type)), filterLower)
			titleMatch := strings.Contains(strings.ToLower(note.Title), filterLower)
			pathMatch := strings.Contains(strings.ToLower(note.Path), filterLower)
			
			// Include note if any field matches
			if typeMatch || titleMatch || pathMatch {
				filtered = append(filtered, note)
			}
		}
		m.notes = filtered
	}
	
	// Resort after filtering
	m.sortNotes()
}

// applyTypeFilter filters the notes based on the selected type
func (m *Model) applyTypeFilter() {
	if m.activeFilter == "" {
		// No filter, show all notes
		m.notes = m.allNotes
	} else {
		// Filter notes by exact type match
		var filtered []*models.Note
		for _, note := range m.allNotes {
			if string(note.Type) == m.activeFilter {
				filtered = append(filtered, note)
			}
		}
		m.notes = filtered
	}
	
	// Resort after filtering
	m.sortNotes()
}