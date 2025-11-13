package confirm

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/tui/theme"
)

// --- Messages ---

// ConfirmedMsg is sent when the user confirms the action.
type ConfirmedMsg struct{}

// CancelledMsg is sent when the user cancels the action.
type CancelledMsg struct{}

// --- Model ---

// Model represents a confirmation dialog.
type Model struct {
	Active bool
	Prompt string
	keys   keyMap
}

// New creates a new confirmation dialog model.
func New() Model {
	return Model{
		keys: defaultKeyMap,
	}
}

// Activate prepares the dialog for display with a given prompt.
func (m *Model) Activate(prompt string) {
	m.Prompt = prompt
	m.Active = true
}

// --- Update ---

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.Active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Confirm):
			m.Active = false
			return m, func() tea.Msg { return ConfirmedMsg{} }
		case key.Matches(msg, m.keys.Cancel):
			m.Active = false
			return m, func() tea.Msg { return CancelledMsg{} }
		}
	}

	return m, nil
}

// --- View ---

func (m Model) View() string {
	if !m.Active {
		return ""
	}

	dialogBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.DefaultTheme.Colors.Orange).
		Padding(1, 2).
		Render(m.Prompt)

	helpText := lipgloss.NewStyle().
		Faint(true).
		Width(lipgloss.Width(dialogBox)).
		Align(lipgloss.Center).
		Render("\n\n(y/n)")

	return lipgloss.JoinVertical(lipgloss.Left, dialogBox, helpText)
}

// --- KeyMap ---

type keyMap struct {
	Confirm key.Binding
	Cancel  key.Binding
}

var defaultKeyMap = keyMap{
	Confirm: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "confirm"),
	),
	Cancel: key.NewBinding(
		key.WithKeys("n", "esc"),
		key.WithHelp("n/esc", "cancel"),
	),
}
