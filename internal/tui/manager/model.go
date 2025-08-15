package manager

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

// Model represents the state of the note manager TUI
type Model struct {
	table        table.Model
	notes        []*models.Note
	selected     map[string]struct{} // Set of selected note paths
	service      *service.Service
	context      *service.WorkspaceContext
	quitting     bool
	confirming   bool // Confirmation mode for archiving
	message      string
	width        int
	height       int
	lastCursor   int  // For shift+click selection
	sortNewest   bool // true = newest first, false = oldest first
}

// Styles
var (
	baseStyle = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240"))

	headerStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57"))

	selectedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57"))

	dimStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	helpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	messageStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("86"))

	warningStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("214"))
)

// New creates a new Model instance
func New(notes []*models.Note, svc *service.Service, ctx *service.WorkspaceContext) Model {
	// Sort notes by modified date (newest first)
	sort.Slice(notes, func(i, j int) bool {
		return notes[i].ModifiedAt.After(notes[j].ModifiedAt)
	})

	columns := []table.Column{
		{Title: "SEL", Width: 3},
		{Title: "TYPE", Width: 10},
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
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	return Model{
		table:      t,
		notes:      notes,
		selected:   make(map[string]struct{}),
		service:    svc,
		context:    ctx,
		lastCursor: 0,
		sortNewest: true,
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
		return m, nil

	case editorFinishedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Error opening editor: %v", msg.err)
		}
		return m, nil

	case tea.KeyMsg:
		if m.confirming {
			return m.handleConfirmation(msg)
		}

		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "enter":
			// Open the note under cursor in editor
			cursor := m.table.Cursor()
			if cursor < len(m.notes) {
				note := m.notes[cursor]
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
	// Create new slice without archived notes
	var remainingNotes []*models.Note
	for _, note := range m.notes {
		if _, archived := m.selected[note.Path]; !archived {
			remainingNotes = append(remainingNotes, note)
		}
	}
	
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

	// Message
	if m.message != "" {
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
	help := []string{
		"enter: open",
		"space: toggle",
		"shift+↑/↓: multi-select",
		"a: all",
		"n: none",
		"s: sort",
		"x: archive",
		"q: quit",
	}
	s.WriteString(helpStyle.Render(strings.Join(help, " • ")))

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