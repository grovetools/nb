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

// nodeRenderInfo holds the unstyled components of a display node for rendering.
// This decouples data extraction from styling logic.
type nodeRenderInfo struct {
	prefix      string
	fold        string // "▶ ", "▼ ", or ""
	indicator   string // "■", note icon, or plan status icon
	name        string
	count       string // "(d)"
	isArchived  bool
	isPlan      bool
	isWorkspace bool
	isSeparator bool
	workspace   *workspace.WorkspaceNode // a reference to the workspace node if applicable
	note        *models.Note             // a reference to the note if applicable
}

// getNodeRenderInfo populates a nodeRenderInfo struct with unstyled data from a displayNode.
func (m *Model) getNodeRenderInfo(node *displayNode) nodeRenderInfo {
	info := nodeRenderInfo{
		prefix: node.prefix,
	}

	if node.isSeparator {
		info.isSeparator = true
		return info
	}

	// Set fold indicator for foldable nodes
	if node.isFoldable() {
		if m.collapsedNodes[node.nodeID()] {
			info.fold = "▶ "
		} else {
			info.fold = "▼ "
		}
	}

	if node.isWorkspace {
		info.isWorkspace = true
		info.workspace = node.workspace
		info.name = node.workspace.Name
	} else if node.isGroup {
		info.name = node.groupName
		info.isArchived = strings.Contains(node.groupName, "/.archive/") || node.groupName == ".archive"
		if node.isPlan() {
			info.isPlan = true
			info.name = strings.TrimPrefix(node.groupName, "plans/") // Display plan name without "plans/"
			groupKey := m.getGroupKey(node)
			if _, ok := m.selectedGroups[groupKey]; ok {
				info.indicator = "■ " // Selected indicator
			} else {
				planStatus := m.getPlanStatus(node.workspaceName, node.groupName)
				info.indicator = getPlanStatusIcon(planStatus) + " "
			}
		}
		if node.childCount > 0 {
			info.count = fmt.Sprintf(" (%d)", node.childCount)
		}
	} else if node.isNote {
		info.note = node.note
		info.name = node.note.Title
		info.isArchived = strings.Contains(node.note.Path, "/.archive/")
		if _, ok := m.selected[node.note.Path]; ok {
			info.indicator = "■" // Selected indicator
		} else {
			info.indicator = getNoteIcon(string(node.note.Type))
		}
	}

	return info
}

// getSearchHighlightIndices returns the start and end indices of the search match, or -1, -1 if no match
func (m *Model) getSearchHighlightIndices(text string) (int, int) {
	filterValue := m.filterInput.Value()
	if filterValue == "" || m.isGrepping {
		return -1, -1
	}

	lowerText := strings.ToLower(text)
	lowerFilter := strings.ToLower(filterValue)
	if idx := strings.Index(lowerText, lowerFilter); idx != -1 {
		return idx, idx + len(filterValue)
	}
	return -1, -1
}

// styleNodeContent applies styling to the main content (name/title) of a node.
func (m *Model) styleNodeContent(info nodeRenderInfo, isSelected bool) string {
	// Build base content
	content := info.indicator
	if info.indicator != "" && !strings.HasSuffix(info.indicator, " ") {
		content += " "
	}

	// Get search match indices before building content
	matchStart, matchEnd := m.getSearchHighlightIndices(info.name)
	hasMatch := matchStart != -1

	// Determine the base style for the node type
	var baseStyle lipgloss.Style
	hasStyle := false

	if isSelected {
		baseStyle = lipgloss.NewStyle().Underline(true)
		hasStyle = true
	} else if info.note != nil {
		if _, isCut := m.cutPaths[info.note.Path]; isCut {
			baseStyle = lipgloss.NewStyle().Faint(true).Strikethrough(true)
			hasStyle = true
		} else if info.isArchived {
			baseStyle = lipgloss.NewStyle().Faint(true)
			hasStyle = true
		}
	} else if info.isArchived {
		baseStyle = lipgloss.NewStyle().Faint(true)
		hasStyle = true
	} else if info.isWorkspace {
		ws := info.workspace
		if ws.Name == "global" {
			baseStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.DefaultTheme.Colors.Green)
		} else if ws.IsEcosystem() {
			baseStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.DefaultTheme.Colors.Cyan)
		} else {
			baseStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.DefaultTheme.Colors.Violet)
		}
		hasStyle = true
	} else if info.isPlan {
		baseStyle = lipgloss.NewStyle().Foreground(theme.DefaultTheme.Colors.Yellow)
		hasStyle = true
	}

	// If there's a search match, build the string with highlighted portion
	if hasMatch {
		pre := info.name[:matchStart]
		match := info.name[matchStart:matchEnd]
		post := info.name[matchEnd:]

		highlightStyle := theme.DefaultTheme.Highlight.Copy().Reverse(true)

		// Render each part separately and concatenate
		if hasStyle {
			return content + baseStyle.Render(pre) + highlightStyle.Render(match) + baseStyle.Render(post)
		}
		return content + pre + highlightStyle.Render(match) + post
	}

	// No search match - render normally
	content += info.name
	if hasStyle {
		return baseStyle.Render(content)
	}
	return content
}

// getNoteCreationContext returns a description of where the note will be created
func (m Model) getNoteCreationContext() string {
	if m.noteCreationMode == "inbox" {
		// Inbox mode: goes to focused workspace or global
		if m.focusedWorkspace != nil {
			return fmt.Sprintf("%s (inbox)", m.focusedWorkspace.Name)
		}
		return "global (inbox)"
	}

	if m.noteCreationMode == "global" {
		// Global mode: always creates in global
		return "global"
	}

	// Context mode: use the cursor position when creation started
	if m.noteCreationCursor >= len(m.displayNodes) {
		return "global/current"
	}

	node := m.displayNodes[m.noteCreationCursor]
	if node.isWorkspace {
		return fmt.Sprintf("%s/current", node.workspace.Name)
	} else if node.isGroup {
		return fmt.Sprintf("%s/%s", node.workspaceName, node.groupName)
	} else if node.isNote {
		return fmt.Sprintf("%s/%s", node.note.Workspace, node.note.Group)
	}

	return "global/current"
}

func (m Model) View() string {
	if len(m.workspaces) == 0 && len(m.allNotes) == 0 {
		return "Loading..."
	}

	if m.help.ShowAll {
		return m.help.View()
	}

	// Render note creation UI if active
	if m.isCreatingNote {
		// Get context information
		contextInfo := m.getNoteCreationContext()
		contextLine := lipgloss.NewStyle().
			Faint(true).
			Render(fmt.Sprintf("Creating in: %s", contextInfo))

		var content string
		if m.noteCreationStep == 0 { // Type picker
			content = contextLine + "\n\n" + m.noteTypePicker.View()
		} else { // Title input
			content = contextLine + "\n\nEnter Title:\n" + m.noteTitleInput.View()
		}

		dialogBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.DefaultTheme.Colors.Cyan).
			Padding(1, 2).
			Render(content)

		helpText := lipgloss.NewStyle().
			Faint(true).
			Width(lipgloss.Width(dialogBox)).
			Align(lipgloss.Center).
			Render("\n\nPress Enter to confirm • Esc to cancel")

		overlay := lipgloss.JoinVertical(lipgloss.Left, dialogBox, helpText)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
	}

	// Render note rename UI if active
	if m.isRenamingNote && m.noteToRename != nil {
		oldTitle := m.noteToRename.Title
		contextLine := lipgloss.NewStyle().
			Faint(true).
			Render(fmt.Sprintf("Renaming: %s", oldTitle))

		content := contextLine + "\n\nNew Title:\n" + m.renameInput.View()

		dialogBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.DefaultTheme.Colors.Cyan).
			Padding(1, 2).
			Render(content)

		helpText := lipgloss.NewStyle().
			Faint(true).
			Width(lipgloss.Width(dialogBox)).
			Align(lipgloss.Center).
			Render("\n\nPress Enter to confirm • Esc to cancel")

		overlay := lipgloss.JoinVertical(lipgloss.Left, dialogBox, helpText)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
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

	// Header - breadcrumb style
	// Get notebook title from config
	notebookTitle := "Notebook Browser"
	if m.service.CoreConfig != nil && m.service.CoreConfig.Notebooks != nil && len(m.service.CoreConfig.Notebooks) > 0 {
		// Prefer "default" notebook if it exists, otherwise use the first one
		if _, ok := m.service.CoreConfig.Notebooks["default"]; ok {
			notebookTitle = "default"
		} else {
			// Use the first notebook name
			for name := range m.service.CoreConfig.Notebooks {
				notebookTitle = name
				break
			}
		}
	}

	// Build header string without styling first
	headerText := notebookTitle
	if m.focusedWorkspace != nil {
		headerText += " > " + m.focusedWorkspace.Name
	}
	if m.isGrepping {
		headerText += " [Grep Mode]"
	} else if m.filterInput.Focused() && !m.isGrepping {
		headerText += " [Find Mode]"
	} else if m.ecosystemPickerMode {
		headerText += " [Select Ecosystem]"
	}

	// Apply styling to complete header
	header := theme.DefaultTheme.Header.Render(headerText)

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
	var mainContent string
	if searchBar != "" {
		mainContent = lipgloss.JoinVertical(lipgloss.Left, header, searchBar, "")
	} else {
		mainContent = header
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
		isSelected := i == m.cursor

		cursor := "  "
		if isSelected {
			cursor = lipgloss.NewStyle().Foreground(theme.DefaultTheme.Colors.Orange).Render("▶ ")
		}

		info := m.getNodeRenderInfo(node)

		var line string
		if info.isSeparator {
			line = lipgloss.NewStyle().Faint(true).Render("  ─────")
		} else {
			prefix := theme.DefaultTheme.Muted.Render(info.prefix + info.fold)
			content := m.styleNodeContent(info, isSelected)
			count := theme.DefaultTheme.Muted.Render(info.count)
			line = cursor + prefix + content + count
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

		info := m.getNodeRenderInfo(node)

		// --- Build Columns ---
		selCol := " "
		if isSelected {
			selCol = lipgloss.NewStyle().Foreground(theme.DefaultTheme.Colors.Orange).Render("▶")
		}

		var typeCol, statusCol, tagsCol, createdCol string
		if node.isNote {
			typeCol = string(node.note.Type)
			statusCol = getNoteStatus(node.note)
			tagsCol = strings.Join(node.note.Tags, ", ")
			createdCol = node.note.CreatedAt.Format("2006-01-02 15:04")
		} else if info.isPlan {
			statusCol = m.getPlanStatus(node.workspaceName, node.groupName)
		}

		// --- Build Name Column using new helpers ---
		var styledNameCol string
		if info.isSeparator {
			styledNameCol = theme.DefaultTheme.Muted.Render(padOrTruncate("  ─────", nameWidth))
		} else {
			prefix := theme.DefaultTheme.Muted.Render(info.prefix + info.fold)
			content := m.styleNodeContent(info, isSelected)
			count := theme.DefaultTheme.Muted.Render(info.count)
			styledNameCol = padOrTruncate(prefix+content+count, nameWidth)
		}

		// --- Assemble Row ---
		var rowParts []string
		rowParts = append(rowParts, padOrTruncate(selCol, selectionWidth))
		rowParts = append(rowParts, styledNameCol)
		if m.columnVisibility["TYPE"] {
			rowParts = append(rowParts, separator, padOrTruncate(typeCol, typeWidth))
		}
		if m.columnVisibility["STATUS"] {
			rowParts = append(rowParts, separator, padOrTruncate(statusCol, statusWidth))
		}
		if m.columnVisibility["TAGS"] {
			rowParts = append(rowParts, separator, padOrTruncate(tagsCol, tagsWidth))
		}
		if m.columnVisibility["CREATED"] {
			rowParts = append(rowParts, separator, padOrTruncate(createdCol, createdWidth))
		}
		row := strings.Join(rowParts, "")

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
