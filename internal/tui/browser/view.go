package browser

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/tui/theme"
)

func (m Model) View() string {
	if len(m.workspaces) == 0 && len(m.allNotes) == 0 {
		return "Loading..."
	}

	if m.help.ShowAll {
		return m.help.View()
	}

	var viewContent string
	if m.viewMode == treeView {
		viewContent = m.renderTreeView()
	} else {
		viewContent = m.renderTableView()
	}

	// Header
	var header string
	if m.ecosystemPickerMode {
		header = theme.DefaultTheme.Info.Render("[Select Ecosystem to Focus]")
	} else if m.focusedWorkspace != nil {
		focusIndicator := theme.DefaultTheme.Info.Render(
			fmt.Sprintf("[Focus: %s]", m.focusedWorkspace.Name))
		header = focusIndicator
	} else {
		header = theme.DefaultTheme.Header.Render("Notebook Browser")
	}

	footer := m.help.View()

	// Combine components vertically
	fullView := lipgloss.JoinVertical(lipgloss.Left,
		header,
		"", // This adds a blank line for spacing
		viewContent,
		"", // Another blank line for spacing
		footer,
	)

	// Add top margin to prevent border cutoff
	return "\n" + fullView
}

func (m Model) renderTreeView() string {
	var b strings.Builder

	// Viewport calculation
	viewportHeight := m.getViewportHeight()
	start := m.scrollOffset
	end := m.scrollOffset + viewportHeight
	if end > len(m.displayNodes) {
		end = len(m.displayNodes)
	}

	// Render visible nodes
	for i := start; i < end; i++ {
		node := m.displayNodes[i]
		cursor := "  "
		if i == m.cursor {
			cursor = theme.DefaultTheme.Highlight.Render("▶ ")
		}

		var line string
		if node.isWorkspace {
			// Add fold indicator
			foldIndicator := ""
			if node.isFoldable() {
				nodeID := node.nodeID()
				if m.collapsedNodes[nodeID] {
					foldIndicator = "▶ "
				} else {
					foldIndicator = "▼ "
				}
			}

			wsName := node.workspace.Name
			line = fmt.Sprintf("%s%s%s%s", cursor, node.prefix, foldIndicator, wsName)
			if i == m.cursor {
				line = lipgloss.NewStyle().Bold(true).Render(line)
			}
		} else if node.isGroup {
			// Add fold indicator
			foldIndicator := ""
			if node.isFoldable() {
				nodeID := node.nodeID()
				if m.collapsedNodes[nodeID] {
					foldIndicator = "▶ "
				} else {
					foldIndicator = "▼ "
				}
			}
			line = fmt.Sprintf("%s%s%s%s", cursor, node.prefix, foldIndicator, node.groupName)
			if i == m.cursor {
				line = lipgloss.NewStyle().Bold(true).Render(line)
			}
		} else if node.isNote {
			line = fmt.Sprintf("%s%s▢ %s", cursor, node.prefix, node.note.Title)
			if i == m.cursor {
				line = theme.DefaultTheme.Selected.Render(line)
			}
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Scroll indicator
	if len(m.displayNodes) > viewportHeight {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(" (%d-%d of %d)", start+1, end, len(m.displayNodes))))
	}

	return b.String()
}

func (m Model) renderTableView() string {
	var b strings.Builder
	b.WriteString(m.filterInput.View() + "\n\n")

	if len(m.filteredNotes) == 0 {
		b.WriteString(theme.DefaultTheme.Muted.Render("No matching notes found."))
		return b.String()
	}

	// Render table using bubbles table
	b.WriteString(m.table.View())

	return b.String()
}

func formatRelativeTime(t time.Time) string {
	diff := time.Since(t)
	if diff < time.Minute {
		return "just now"
	}
	if diff < time.Hour {
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	}
	if diff < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	}
	if diff < 7*24*time.Hour {
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	}
	return t.Format("2006-01-02")
}
