package browser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/tui/theme"
	"github.com/mattsolo1/grove-notebook/pkg/models"
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
	if m.isGrepping {
		header = theme.DefaultTheme.Warning.Render("[Grep Mode]")
	} else if m.ecosystemPickerMode {
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
		// Count notes in display nodes
		var noteCount int
		for _, node := range m.displayNodes {
			if node.isNote {
				noteCount++
			}
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
		prefix := "Search: "
		if m.isGrepping {
			prefix = "Grep: "
		}
		searchBar = prefix + m.filterInput.View()
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
			if filterValue != "" && !m.isGrepping { // Only highlight title in normal filter mode
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

	const separator = " │ "
	const selectionWidth = 2

	// Calculate column widths based on content
	colWidths := m.calculateTableColumnWidths()
	nameWidth := colWidths[0]
	typeWidth := colWidths[1]
	statusWidth := colWidths[2]
	tagsWidth := colWidths[3]
	createdWidth := colWidths[4]

	// Header
	header := padOrTruncate("", selectionWidth) +
		padOrTruncate("WORKSPACE / NOTE", nameWidth) + separator +
		padOrTruncate("TYPE", typeWidth) + separator +
		padOrTruncate("STATUS", statusWidth) + separator +
		padOrTruncate("TAGS", tagsWidth) + separator +
		padOrTruncate("CREATED", createdWidth)

	b.WriteString(theme.DefaultTheme.TableHeader.Render(header))
	b.WriteString("\n")

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
		isSelected := i == m.cursor

		// --- Build Columns ---
		var selCol, nameCol, typeCol, statusCol, tagsCol, createdCol string

		// Selection and Name Column (Hierarchical)
		selIndicator := " "
		if isSelected {
			selIndicator = theme.DefaultTheme.Highlight.Render("▶")
		}
		selCol = selIndicator

		var nameBuilder strings.Builder
		if node.isWorkspace {
			foldIndicator := " "
			if node.isFoldable() {
				if m.collapsedNodes[node.nodeID()] {
					foldIndicator = "▶ "
				} else {
					foldIndicator = "▼ "
				}
			}
			nameBuilder.WriteString(fmt.Sprintf("%s%s%s", node.prefix, foldIndicator, node.workspace.Name))
		} else if node.isGroup {
			foldIndicator := " "
			if node.isFoldable() {
				if m.collapsedNodes[node.nodeID()] {
					foldIndicator = "▶ "
				} else {
					foldIndicator = "▼ "
				}
			}
			displayName := node.groupName
			if node.isPlan() {
				displayName = strings.TrimPrefix(displayName, "plans/")
			}
			nameBuilder.WriteString(fmt.Sprintf("%s%s%s", node.prefix, foldIndicator, displayName))
		} else if node.isNote {
			nameBuilder.WriteString(fmt.Sprintf("%s%s %s", node.prefix, getNoteIcon(string(node.note.Type)), node.note.Title))
		} else if node.isSeparator {
			nameBuilder.WriteString("  ─────")
		}
		nameCol = nameBuilder.String()

		// Other Columns
		if node.isNote {
			typeCol = string(node.note.Type)
			statusCol = getNoteStatus(node.note)
			tagsCol = strings.Join(node.note.Tags, ", ")
			createdCol = node.note.CreatedAt.Format("2006-01-02 15:04")
		} else if node.isPlan() {
			statusCol = m.getPlanStatus(node.workspaceName, node.groupName)
		}

		// Determine the style to apply
		var rowStyle lipgloss.Style
		hasStyle := false

		if node.isWorkspace {
			hasStyle = true
			if node.workspace.Name == "global" {
				rowStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.DefaultTheme.Colors.Green)
			} else if node.workspace.IsEcosystem() {
				rowStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.DefaultTheme.Colors.Cyan)
			} else {
				rowStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.DefaultTheme.Colors.Violet)
			}
		} else if node.isGroup {
			isArchived := strings.Contains(node.groupName, "/.archive/") || node.groupName == ".archive"
			if isArchived {
				hasStyle = true
				rowStyle = lipgloss.NewStyle().Faint(true)
			} else if node.isPlan() {
				hasStyle = true
				rowStyle = lipgloss.NewStyle().Foreground(theme.DefaultTheme.Colors.Yellow)
			}
		} else if node.isNote {
			isArchived := strings.Contains(node.note.Path, "/.archive/")
			if isArchived {
				hasStyle = true
				rowStyle = lipgloss.NewStyle().Faint(true)
			}
		}

		// Add reverse video for selection
		if isSelected {
			if !hasStyle {
				rowStyle = lipgloss.NewStyle()
			}
			rowStyle = rowStyle.Reverse(true)
			hasStyle = true
		}

		// Build row by concatenating plain padded columns with separators
		row := padOrTruncate(selCol, selectionWidth) +
			padOrTruncate(nameCol, nameWidth) + separator +
			padOrTruncate(typeCol, typeWidth) + separator +
			padOrTruncate(statusCol, statusWidth) + separator +
			padOrTruncate(tagsCol, tagsWidth) + separator +
			padOrTruncate(createdCol, createdWidth)

		// Apply style if any, ensuring no width limit
		if hasStyle {
			b.WriteString(rowStyle.Inline(true).Render(row))
		} else {
			b.WriteString(row)
		}
		b.WriteString("\n")
	}

	// Scroll indicator
	if len(m.displayNodes) > viewportHeight {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(" (%d-%d of %d)", start+1, end, len(m.displayNodes))))
	}

	return b.String()
}

// calculateTableColumnWidths calculates optimal column widths based on content
func (m Model) calculateTableColumnWidths() [5]int {
	// Min and max constraints for each column
	const minNameWidth = 30
	const maxNameWidth = 60
	const minTypeWidth = 10
	const maxTypeWidth = 25
	const minStatusWidth = 10
	const maxStatusWidth = 20
	const minTagsWidth = 15
	const maxTagsWidth = 50
	const minCreatedWidth = 17
	const maxCreatedWidth = 17 // Fixed width for dates

	// Start with header widths
	maxName := len("WORKSPACE / NOTE")
	maxType := len("TYPE")
	maxStatus := len("STATUS")
	maxTags := len("TAGS")
	maxCreated := len("CREATED")

	// Scan through all display nodes to find max widths
	for _, node := range m.displayNodes {
		if node.isWorkspace {
			nameLen := lipgloss.Width(fmt.Sprintf("%s%s", node.prefix, node.workspace.Name)) + 4 // +4 for fold indicator
			if nameLen > maxName {
				maxName = nameLen
			}
		} else if node.isGroup {
			displayName := node.groupName
			if node.isPlan() {
				displayName = strings.TrimPrefix(displayName, "plans/")
			}
			nameLen := lipgloss.Width(fmt.Sprintf("%s%s", node.prefix, displayName)) + 4
			if nameLen > maxName {
				maxName = nameLen
			}
			if node.isPlan() {
				status := m.getPlanStatus(node.workspaceName, node.groupName)
				if len(status) > maxStatus {
					maxStatus = len(status)
				}
			}
		} else if node.isNote {
			nameLen := lipgloss.Width(fmt.Sprintf("%s%s %s", node.prefix, getNoteIcon(string(node.note.Type)), node.note.Title))
			if nameLen > maxName {
				maxName = nameLen
			}
			typeLen := len(string(node.note.Type))
			if typeLen > maxType {
				maxType = typeLen
			}
			status := getNoteStatus(node.note)
			if len(status) > maxStatus {
				maxStatus = len(status)
			}
			tagsLen := len(strings.Join(node.note.Tags, ", "))
			if tagsLen > maxTags {
				maxTags = tagsLen
			}
		}
	}

	// Apply constraints
	if maxName < minNameWidth {
		maxName = minNameWidth
	}
	if maxName > maxNameWidth {
		maxName = maxNameWidth
	}
	if maxType < minTypeWidth {
		maxType = minTypeWidth
	}
	if maxType > maxTypeWidth {
		maxType = maxTypeWidth
	}
	if maxStatus < minStatusWidth {
		maxStatus = minStatusWidth
	}
	if maxStatus > maxStatusWidth {
		maxStatus = maxStatusWidth
	}
	if maxTags < minTagsWidth {
		maxTags = minTagsWidth
	}
	if maxTags > maxTagsWidth {
		maxTags = maxTagsWidth
	}
	if maxCreated < minCreatedWidth {
		maxCreated = minCreatedWidth
	}
	if maxCreated > maxCreatedWidth {
		maxCreated = maxCreatedWidth
	}

	return [5]int{maxName, maxType, maxStatus, maxTags, maxCreated}
}

// getNoteStatus determines the status of a note (e.g., pending if it has todos)
func getNoteStatus(note *models.Note) string {
	if note.HasTodos {
		// A more sophisticated check could see if all are checked off
		return "pending"
	}
	return ""
}

// padOrTruncate ensures a string fits a specific width, handling ANSI codes properly
func padOrTruncate(s string, width int) string {
	if width <= 0 {
		return ""
	}

	// Use lipgloss.Width to get visual width (ignoring ANSI codes)
	visualWidth := lipgloss.Width(s)

	if visualWidth > width {
		// Truncate - need to use runes but account for ANSI codes
		// Strip ANSI codes, truncate, then pad
		plain := stripAnsi(s)
		runes := []rune(plain)
		if len(runes) > width {
			return string(runes[:width])
		}
		return plain + strings.Repeat(" ", width-len(runes))
	}

	// Pad to exact width
	return s + strings.Repeat(" ", width-visualWidth)
}

// stripAnsi removes ANSI escape codes from a string
func stripAnsi(s string) string {
	// Manual stripping for ANSI codes
	result := s
	for {
		start := strings.Index(result, "\x1b[")
		if start == -1 {
			break
		}
		end := start + 2
		for end < len(result) && (result[end] >= '0' && result[end] <= '9' || result[end] == ';') {
			end++
		}
		if end < len(result) {
			end++ // Include the final letter
		}
		result = result[:start] + result[end:]
	}
	return result
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
