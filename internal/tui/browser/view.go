package browser

import (
	"fmt"
	"os"
	"path/filepath"
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

	// Build status bar
	var status string
	if m.statusMessage != "" {
		status = m.statusMessage
	} else {
		var noteCount int
		if m.viewMode == treeView {
			// Count notes in display nodes
			for _, node := range m.displayNodes {
				if node.isNote {
					noteCount++
				}
			}
		} else {
			noteCount = len(m.filteredNotes)
		}

		selectionInfo := ""
		if len(m.selected) > 0 && len(m.selectedGroups) > 0 {
			selectionInfo = fmt.Sprintf(" | %d notes + %d plans selected", len(m.selected), len(m.selectedGroups))
		} else if len(m.selected) > 0 {
			selectionInfo = fmt.Sprintf(" | %d notes selected", len(m.selected))
		} else if len(m.selectedGroups) > 0 {
			selectionInfo = fmt.Sprintf(" | %d plans selected", len(m.selectedGroups))
		} else {
			selectionInfo = " | 0 selected"
		}
		status = fmt.Sprintf("%d notes shown%s", noteCount, selectionInfo)
	}

	// Search bar (if active)
	var searchBar string
	if m.filterInput.Focused() || m.filterInput.Value() != "" {
		searchBar = "Search: " + m.filterInput.View()
	}

	// Combine components vertically
	mainContent := lipgloss.JoinVertical(lipgloss.Left,
		header,
		searchBar,
	)

	// Ensure there is spacing between header/search and content
	if mainContent != "" {
		mainContent = lipgloss.JoinVertical(lipgloss.Left, mainContent, "")
	}

	fullView := lipgloss.JoinVertical(lipgloss.Left,
		mainContent,
		viewContent,
		"", // Another blank line for spacing
		theme.DefaultTheme.Muted.Render(status),
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
				line = theme.DefaultTheme.Highlight.Render(line)
			} else if node.workspace.Name == "global" {
				// Global notes workspace: green + bold
				line = lipgloss.NewStyle().Bold(true).Foreground(theme.DefaultTheme.Colors.Green).Render(line)
			} else if node.workspace.IsEcosystem() {
				// Ecosystems: cyan + bold
				line = lipgloss.NewStyle().Bold(true).Foreground(theme.DefaultTheme.Colors.Cyan).Render(line)
			} else {
				// Regular workspaces: violet + bold
				line = lipgloss.NewStyle().Bold(true).Foreground(theme.DefaultTheme.Colors.Violet).Render(line)
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

			// Add selection and plan icon for plan groups
			selIndicator := ""
			if node.isPlan() {
				groupKey := m.getGroupKey(node)
				if _, ok := m.selectedGroups[groupKey]; ok {
					selIndicator = "■ " // Selected indicator
				} else {
					// Get plan status icon
					planStatus := m.getPlanStatus(node.workspaceName, node.groupName)
					selIndicator = getPlanStatusIcon(planStatus) + " "
				}
			}

			// Check if this is an archived item
			isArchived := strings.Contains(node.groupName, "/.archive/") || node.groupName == ".archive"

			// Display name - strip "plans/" prefix for plan nodes
			displayName := node.groupName
			if node.isPlan() {
				displayName = strings.TrimPrefix(displayName, "plans/")
			}

			// Handle search highlighting for group/plan names
			filterValue := m.filterInput.Value()
			if filterValue != "" {
				lowerName := strings.ToLower(displayName)
				lowerFilter := strings.ToLower(filterValue)
				if idx := strings.Index(lowerName, lowerFilter); idx != -1 {
					pre := displayName[:idx]
					match := displayName[idx : idx+len(filterValue)]
					post := displayName[idx+len(filterValue):]
					highlightStyle := theme.DefaultTheme.Highlight.Copy().Reverse(true)
					displayName = fmt.Sprintf("%s%s%s", pre, highlightStyle.Render(match), post)
				}
			}

			line = fmt.Sprintf("%s%s%s%s%s", cursor, node.prefix, foldIndicator, selIndicator, displayName)
			if i == m.cursor {
				line = theme.DefaultTheme.Highlight.Render(line)
			} else if isArchived {
				line = lipgloss.NewStyle().Faint(true).Render(line)
			} else if node.isPlan() {
				// Individual plan nodes: yellow (not bold)
				line = lipgloss.NewStyle().Foreground(theme.DefaultTheme.Colors.Yellow).Render(line)
			}
		} else if node.isNote {
			// Get type-specific icon
			noteType := string(node.note.Type)
			noteIcon := getNoteIcon(noteType)

			selIndicator := noteIcon
			if _, ok := m.selected[node.note.Path]; ok {
				selIndicator = "■" // Selected indicator
			}

			// Check if this note is in an archived directory
			isArchived := strings.Contains(node.note.Path, "/.archive/")

			// Handle search highlighting
			title := node.note.Title
			filterValue := m.filterInput.Value()
			if filterValue != "" {
				lowerTitle := strings.ToLower(title)
				lowerFilter := strings.ToLower(filterValue)
				if idx := strings.Index(lowerTitle, lowerFilter); idx != -1 {
					pre := title[:idx]
					match := title[idx : idx+len(filterValue)]
					post := title[idx+len(filterValue):]
					highlightStyle := theme.DefaultTheme.Highlight.Copy().Reverse(true)
					title = fmt.Sprintf("%s%s%s", pre, highlightStyle.Render(match), post)
				}
			}

			line = fmt.Sprintf("%s%s%s %s", cursor, node.prefix, selIndicator, title)

			if i == m.cursor {
				line = theme.DefaultTheme.Highlight.Render(line)
			} else if isArchived {
				line = lipgloss.NewStyle().Faint(true).Render(line)
			}
		} else if node.isSeparator {
			// Render a visual separator line
			line = lipgloss.NewStyle().Faint(true).Render("  ─────")
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

// getNoteIcon returns the appropriate icon for a note type
func getNoteIcon(noteType string) string {
	switch noteType {
	case "chat":
		return theme.IconChat // ★
	case "interactive_agent":
		return theme.IconInteractiveAgent // ⚙
	case "oneshot":
		return theme.IconOneshot // ●
	case "headless_agent":
		return theme.IconHeadlessAgent // ◆
	case "shell":
		return theme.IconShell // ▶
	default:
		return "▢" // Default fallback
	}
}

// getPlanStatus reads the plan status from the .grove-plan yaml file
func (m *Model) getPlanStatus(workspaceName, planGroup string) string {
	// Find workspace to get path
	var wsPath string
	for _, ws := range m.workspaces {
		if ws.Name == workspaceName {
			wsPath = ws.Path
			break
		}
	}
	if wsPath == "" {
		return "unknown"
	}

	// Extract plan name (e.g., "plans/my-plan" -> "my-plan")
	planName := strings.TrimPrefix(planGroup, "plans/")

	// Construct path to .grove-plan file
	planFile := filepath.Join(wsPath, "plans", planName, ".grove-plan")

	// Read and parse the file
	data, err := os.ReadFile(planFile)
	if err != nil {
		return "unknown"
	}

	// Simple parsing - look for "status:" line
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "status:") {
			status := strings.TrimSpace(strings.TrimPrefix(line, "status:"))
			status = strings.Trim(status, `"'`)
			return status
		}
	}

	return "unknown"
}

// getPlanStatusIcon returns the appropriate icon for a plan status
func getPlanStatusIcon(status string) string {
	switch status {
	case "completed", "finished", "done":
		return theme.IconStatusCompleted // ●
	case "running", "active", "in_progress":
		return theme.IconStatusRunning // ◐
	case "failed", "error":
		return theme.IconStatusFailed // ✗
	case "abandoned", "cancelled":
		return theme.IconStatusAbandoned // ⊗
	case "pending", "todo", "unknown":
		fallthrough
	default:
		return theme.IconStatusTodo // ○
	}
}
