package browser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/pkg/workspace"
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

	if m.columnSelectMode {
		listView := m.columnList.View()
		styledView := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.DefaultTheme.Colors.Cyan).
			Padding(1, 2).
			Render(listView)
		helpText := lipgloss.NewStyle().
			Faint(true).
			Width(lipgloss.Width(styledView)).
			Align(lipgloss.Center).
			Render("\n\nPress space to toggle • Enter/Esc/V to close")
		content := lipgloss.JoinVertical(lipgloss.Left, styledView, helpText)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
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
			cursor = lipgloss.NewStyle().Foreground(theme.DefaultTheme.Colors.Orange).Render("▶ ")
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

			if i == m.cursor {
				// For selected rows, use underline instead of background
				prefix := theme.DefaultTheme.Muted.Render(node.prefix + foldIndicator)
				styledName := lipgloss.NewStyle().Underline(true).Render(wsName)
				line = cursor + prefix + styledName
			} else {
				// For non-selected rows, apply muted to prefix and color to name
				prefix := theme.DefaultTheme.Muted.Render(node.prefix + foldIndicator)
				var styledName string
				if node.workspace.Name == "global" {
					styledName = lipgloss.NewStyle().Bold(true).Foreground(theme.DefaultTheme.Colors.Green).Render(wsName)
				} else if node.workspace.IsEcosystem() {
					styledName = lipgloss.NewStyle().Bold(true).Foreground(theme.DefaultTheme.Colors.Cyan).Render(wsName)
				} else {
					styledName = lipgloss.NewStyle().Bold(true).Foreground(theme.DefaultTheme.Colors.Violet).Render(wsName)
				}
				line = cursor + prefix + styledName
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

			// Add count if available
			countSuffix := ""
			if node.childCount > 0 {
				countSuffix = fmt.Sprintf(" (%d)", node.childCount)
			}

			if i == m.cursor {
				// For selected rows, use underline instead of background
				prefix := theme.DefaultTheme.Muted.Render(node.prefix + foldIndicator)
				content := lipgloss.NewStyle().Underline(true).Render(selIndicator + displayName)
				count := theme.DefaultTheme.Muted.Render(countSuffix)
				line = cursor + prefix + content + count
			} else {
				// For non-selected rows, apply muted to prefix and count, color to name
				prefix := theme.DefaultTheme.Muted.Render(node.prefix + foldIndicator)
				count := theme.DefaultTheme.Muted.Render(countSuffix)

				var styledName string
				if isArchived {
					styledName = lipgloss.NewStyle().Faint(true).Render(selIndicator + displayName)
				} else if node.isPlan() {
					styledName = lipgloss.NewStyle().Foreground(theme.DefaultTheme.Colors.Yellow).Render(selIndicator + displayName)
				} else {
					styledName = selIndicator + displayName
				}
				line = cursor + prefix + styledName + count
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

			if i == m.cursor {
				// For selected rows, use underline instead of background
				prefix := theme.DefaultTheme.Muted.Render(node.prefix)
				content := lipgloss.NewStyle().Underline(true).Render(fmt.Sprintf("%s %s", selIndicator, title))
				line = cursor + prefix + content
			} else {
				// For non-selected rows, apply muted to prefix
				prefix := theme.DefaultTheme.Muted.Render(node.prefix)
				var styledContent string
				if isArchived {
					styledContent = lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf("%s %s", selIndicator, title))
				} else {
					styledContent = fmt.Sprintf("%s %s", selIndicator, title)
				}
				line = cursor + prefix + styledContent
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

	separator := theme.DefaultTheme.Muted.Render(" │ ")
	const selectionWidth = 2

	// Calculate column widths based on content
	colWidths := m.calculateTableColumnWidths()
	nameWidth := colWidths[0]
	typeWidth := colWidths[1]
	statusWidth := colWidths[2]
	tagsWidth := colWidths[3]
	createdWidth := colWidths[4]

	// Header
	var headerParts []string
	headerParts = append(headerParts, padOrTruncate("", selectionWidth))
	headerParts = append(headerParts, padOrTruncate("WORKSPACE / NOTE", nameWidth))
	if m.columnVisibility["TYPE"] {
		headerParts = append(headerParts, separator, padOrTruncate("TYPE", typeWidth))
	}
	if m.columnVisibility["STATUS"] {
		headerParts = append(headerParts, separator, padOrTruncate("STATUS", statusWidth))
	}
	if m.columnVisibility["TAGS"] {
		headerParts = append(headerParts, separator, padOrTruncate("TAGS", tagsWidth))
	}
	if m.columnVisibility["CREATED"] {
		headerParts = append(headerParts, separator, padOrTruncate("CREATED", createdWidth))
	}
	header := strings.Join(headerParts, "")

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
			selIndicator = lipgloss.NewStyle().Foreground(theme.DefaultTheme.Colors.Orange).Render("▶")
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
			nameBuilder.WriteString(fmt.Sprintf("%s%s", node.prefix+foldIndicator, node.workspace.Name))
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
			// Add count if available
			if node.childCount > 0 {
				displayName = fmt.Sprintf("%s (%d)", displayName, node.childCount)
			}
			nameBuilder.WriteString(fmt.Sprintf("%s%s", node.prefix+foldIndicator, displayName))
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

		// Build row with proper styling
		var row string

		if isSelected {
			// For selected rows, only underline the name column content (not prefix, not other columns)
			// Build the name column with muted prefix and underlined content
			var styledNameCol string
			if node.isSeparator {
				styledNameCol = theme.DefaultTheme.Muted.Render(padOrTruncate(nameCol, nameWidth))
			} else {
				// Parse out the actual prefix and content from the raw name parts
				var prefix, content string

				if node.isWorkspace {
					foldIndicator := " "
					if node.isFoldable() {
						if m.collapsedNodes[node.nodeID()] {
							foldIndicator = "▶ "
						} else {
							foldIndicator = "▼ "
						}
					}
					prefix = theme.DefaultTheme.Muted.Render(node.prefix + foldIndicator)
					content = lipgloss.NewStyle().Underline(true).Render(node.workspace.Name)
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
					if node.childCount > 0 {
						displayName = fmt.Sprintf("%s (%d)", displayName, node.childCount)
					}
					prefix = theme.DefaultTheme.Muted.Render(node.prefix + foldIndicator)
					content = lipgloss.NewStyle().Underline(true).Render(displayName)
				} else if node.isNote {
					prefix = theme.DefaultTheme.Muted.Render(node.prefix)
					content = lipgloss.NewStyle().Underline(true).Render(fmt.Sprintf("%s %s", getNoteIcon(string(node.note.Type)), node.note.Title))
				}

				styledNameCol = padOrTruncate(prefix+content, nameWidth)
			}

			// Other columns just padded, no styling
			typeCol = padOrTruncate(typeCol, typeWidth)
			statusCol = padOrTruncate(statusCol, statusWidth)
			tagsCol = padOrTruncate(tagsCol, tagsWidth)
			createdCol = padOrTruncate(createdCol, createdWidth)

			var rowParts []string
			rowParts = append(rowParts, padOrTruncate(selCol, selectionWidth))
			rowParts = append(rowParts, styledNameCol)
			if m.columnVisibility["TYPE"] {
				rowParts = append(rowParts, separator, typeCol)
			}
			if m.columnVisibility["STATUS"] {
				rowParts = append(rowParts, separator, statusCol)
			}
			if m.columnVisibility["TAGS"] {
				rowParts = append(rowParts, separator, tagsCol)
			}
			if m.columnVisibility["CREATED"] {
				rowParts = append(rowParts, separator, createdCol)
			}
			row = strings.Join(rowParts, "")
		} else {
			// For non-selected rows, rebuild name column with proper styling
			var styledNameCol string
			if node.isSeparator {
				styledNameCol = theme.DefaultTheme.Muted.Render(padOrTruncate(nameCol, nameWidth))
			} else {
				// Rebuild from node components to properly style prefix vs content
				var prefix, content string

				if node.isWorkspace {
					foldIndicator := " "
					if node.isFoldable() {
						if m.collapsedNodes[node.nodeID()] {
							foldIndicator = "▶ "
						} else {
							foldIndicator = "▼ "
						}
					}
					prefix = theme.DefaultTheme.Muted.Render(node.prefix + foldIndicator)

					// Apply workspace color
					if node.workspace.Name == "global" {
						content = lipgloss.NewStyle().Bold(true).Foreground(theme.DefaultTheme.Colors.Green).Render(node.workspace.Name)
					} else if node.workspace.IsEcosystem() {
						content = lipgloss.NewStyle().Bold(true).Foreground(theme.DefaultTheme.Colors.Cyan).Render(node.workspace.Name)
					} else {
						content = lipgloss.NewStyle().Bold(true).Foreground(theme.DefaultTheme.Colors.Violet).Render(node.workspace.Name)
					}
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
					if node.childCount > 0 {
						displayName = fmt.Sprintf("%s (%d)", displayName, node.childCount)
					}
					prefix = theme.DefaultTheme.Muted.Render(node.prefix + foldIndicator)

					// Apply group styling
					isArchived := strings.Contains(node.groupName, "/.archive/") || node.groupName == ".archive"
					if isArchived {
						content = lipgloss.NewStyle().Faint(true).Render(displayName)
					} else if node.isPlan() {
						content = lipgloss.NewStyle().Foreground(theme.DefaultTheme.Colors.Yellow).Render(displayName)
					} else {
						content = displayName
					}
				} else if node.isNote {
					prefix = theme.DefaultTheme.Muted.Render(node.prefix)
					noteContent := fmt.Sprintf("%s %s", getNoteIcon(string(node.note.Type)), node.note.Title)

					// Apply note styling
					isArchived := strings.Contains(node.note.Path, "/.archive/")
					if isArchived {
						content = lipgloss.NewStyle().Faint(true).Render(noteContent)
					} else {
						content = noteContent
					}
				}

				styledNameCol = padOrTruncate(prefix+content, nameWidth)
			}

			typeCol = padOrTruncate(typeCol, typeWidth)
			statusCol = padOrTruncate(statusCol, statusWidth)
			tagsCol = padOrTruncate(tagsCol, tagsWidth)
			createdCol = padOrTruncate(createdCol, createdWidth)

			var rowParts []string
			rowParts = append(rowParts, padOrTruncate(selCol, selectionWidth))
			rowParts = append(rowParts, styledNameCol)
			if m.columnVisibility["TYPE"] {
				rowParts = append(rowParts, separator, typeCol)
			}
			if m.columnVisibility["STATUS"] {
				rowParts = append(rowParts, separator, statusCol)
			}
			if m.columnVisibility["TAGS"] {
				rowParts = append(rowParts, separator, tagsCol)
			}
			if m.columnVisibility["CREATED"] {
				rowParts = append(rowParts, separator, createdCol)
			}
			row = strings.Join(rowParts, "")
		}

		b.WriteString(row)
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

	// Apply visibility constraints
	if !m.columnVisibility["TYPE"] {
		maxType = 0
	}
	if !m.columnVisibility["STATUS"] {
		maxStatus = 0
	}
	if !m.columnVisibility["TAGS"] {
		maxTags = 0
	}
	if !m.columnVisibility["CREATED"] {
		maxCreated = 0
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

// getPlanStatus reads the plan status from the .grove-plan.yml file
func (m *Model) getPlanStatus(workspaceName, planGroup string) string {
	// Find workspace to get the node
	var wsNode *workspace.WorkspaceNode
	for _, ws := range m.workspaces {
		if ws.Name == workspaceName {
			wsNode = ws
			break
		}
	}
	if wsNode == nil {
		return "unknown"
	}

	// Get the plans base directory for this workspace using the locator
	plansBaseDir, err := m.service.GetNotebookLocator().GetPlansDir(wsNode)
	if err != nil {
		return "unknown"
	}

	// Extract plan name (e.g., "plans/my-plan" -> "my-plan")
	planName := strings.TrimPrefix(planGroup, "plans/")

	// Construct path to .grove-plan.yml file
	planFile := filepath.Join(plansBaseDir, planName, ".grove-plan.yml")

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
			if status != "" {
				return status
			}
		}
	}

	return "pending" // Default to pending if status field is empty or not found
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
