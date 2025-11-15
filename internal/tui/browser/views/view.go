package views

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
	count      string // "(d)"
	suffix     string
	isArchived bool
	isPlan      bool
	isGroup     bool
	isWorkspace bool
	isSeparator bool
	workspace   *workspace.WorkspaceNode // a reference to the workspace node if applicable
	note        *models.Note             // a reference to the note if applicable
}

// View renders the main content area (tree or table view).
func (m *Model) View() string {
	if m.viewMode == TreeView {
		return m.renderTreeView()
	}
	return m.renderTableView()
}

// renderTreeView renders the hierarchical tree view.
func (m *Model) renderTreeView() string {
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

// renderTableView renders the table view with columns.
func (m *Model) renderTableView() string {
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
	pathWidth := colWidths[5]

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
	if m.columnVisibility["PATH"] {
		headerParts = append(headerParts, separator, padOrTruncate("PATH", pathWidth))
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

		var typeCol, statusCol, tagsCol, createdCol, pathCol string
		if node.IsNote {
			typeCol = string(node.Note.Type)
			statusCol = getNoteStatus(node.Note)
			tagsCol = strings.Join(node.Note.Tags, ", ")
			createdCol = node.Note.CreatedAt.Format("2006-01-02 15:04")
			pathCol = node.RelativePath
		} else if info.isPlan {
			statusCol = m.GetPlanStatus(node.WorkspaceName, node.GroupName)
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
		if m.columnVisibility["PATH"] {
			rowParts = append(rowParts, separator, theme.DefaultTheme.Muted.Render(padOrTruncate(pathCol, pathWidth)))
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

// getNodeRenderInfo populates a nodeRenderInfo struct with unstyled data from a DisplayNode.
func (m *Model) getNodeRenderInfo(node *DisplayNode) nodeRenderInfo {
	info := nodeRenderInfo{
		prefix: node.Prefix,
	}

	if node.IsSeparator {
		info.isSeparator = true
		return info
	}

	// Set fold indicator for foldable nodes
	if node.IsFoldable() {
		if m.collapsedNodes[node.NodeID()] {
			info.fold = "▶ "
		} else {
			info.fold = "▼ "
		}
	}

	if node.IsWorkspace {
		info.isWorkspace = true
		info.workspace = node.Workspace
		info.name = node.Workspace.Name
	} else if node.IsGroup {
		info.isGroup = true
		info.name = node.GroupName
		info.isArchived = strings.Contains(node.GroupName, "/.archive")

		// For archive parent nodes (e.g., "current/.archive" or "plans/.archive"), display just ".archive"
		if strings.HasSuffix(node.GroupName, "/.archive") {
			info.name = ".archive"
		} else if node.IsPlan() {
			// Handle plan nodes (but not archive nodes that start with "plans/")
			info.isPlan = true
			info.name = strings.TrimPrefix(node.GroupName, "plans/") // Display plan name without "plans/"
			info.name = strings.TrimPrefix(info.name, ".archive/")   // Also remove ".archive/" prefix for archived plans
			groupKey := m.getGroupKey(node)
			if _, ok := m.selectedGroups[groupKey]; ok {
				info.indicator = "■ " // Selected indicator
			} else {
				planStatus := m.GetPlanStatus(node.WorkspaceName, node.GroupName)
				info.indicator = getPlanStatusIcon(planStatus) + " "
			}
		}
		if node.ChildCount > 0 {
			info.count = fmt.Sprintf(" (%d)", node.ChildCount)
		}
	} else if node.IsNote {
		info.note = node.Note
		info.name = node.Note.Title
		info.isArchived = strings.Contains(node.Note.Path, "/.archive/")
		if _, ok := m.selected[node.Note.Path]; ok {
			info.indicator = "■" // Selected indicator
		} else {
			info.indicator = getNoteIcon(string(node.Note.Type))
		}
	}

	// Add link suffix if the node is linked
	if node.LinkedNode != nil {
		if node.IsNote {
			// Note -> Plan: [plan: → plan-name]
			planName := strings.TrimPrefix(node.LinkedNode.GroupName, "plans/")
			italicStyle := lipgloss.NewStyle().Italic(true)
			prefix := italicStyle.Render("plan:")
			info.suffix = fmt.Sprintf(" [%s → %s]", prefix, planName)
		} else if node.IsPlan() {
			// Plan -> Note: [note: ← Note Title]
			italicStyle := lipgloss.NewStyle().Italic(true)
			prefix := italicStyle.Render("note:")
			info.suffix = fmt.Sprintf(" [%s ← %s]", prefix, node.LinkedNode.Note.Title)
		}
	}

	return info
}

// styleNodeContent applies styling to the main content (name/title) of a node.
func (m *Model) styleNodeContent(info nodeRenderInfo, isSelected bool) string {
	// Build base content string
	content := info.indicator
	if info.indicator != "" && !strings.HasSuffix(info.indicator, " ") {
		content += " "
	}

	// Get search match indices before applying styles to the name
	matchStart, matchEnd := m.getSearchHighlightIndices(info.name)
	hasMatch := matchStart != -1

	// --- Hierarchical Styling ---
	// 1. Start with a base style
	style := lipgloss.NewStyle()

	// 2. Apply type-specific base styling
	if info.isWorkspace {
		ws := info.workspace
		style = style.Bold(true)
		if ws.Name == "global" {
			style = style.Foreground(theme.DefaultTheme.Colors.Green)
		} else if ws.IsEcosystem() {
			style = style.Foreground(theme.DefaultTheme.Colors.Cyan)
		} else {
			style = style.Foreground(theme.DefaultTheme.Colors.Violet)
		}
	} else if info.isPlan {
		// Individual plans are blue
		style = style.Foreground(theme.DefaultTheme.Colors.Blue)
	} else if info.isGroup && info.name == "plans" {
		// Plans heading is bold blue
		style = style.Bold(true).Foreground(theme.DefaultTheme.Colors.Blue)
	}

	// 3. Apply special group colors
	if info.isGroup && info.name == "in_progress" {
		// In progress heading is bold yellow
		style = style.Bold(true).Foreground(theme.DefaultTheme.Colors.Yellow)
	} else if info.note != nil && info.note.Group == "in_progress" {
		// Notes in the in_progress group should also be yellow
		style = style.Foreground(theme.DefaultTheme.Colors.Yellow)
	} else if info.isGroup && info.name == "completed" {
		style = style.Foreground(theme.DefaultTheme.Colors.Green)
	} else if info.note != nil && info.note.Group == "completed" {
		// Notes in the completed group should also be green
		style = style.Foreground(theme.DefaultTheme.Colors.Green)
	} else if info.isGroup && info.name == "review" {
		// Review heading is bold magenta
		style = style.Bold(true).Foreground(theme.DefaultTheme.Colors.Pink)
	} else if info.note != nil && info.note.Group == "review" {
		// Notes in the review group should also be pink (magenta)
		style = style.Foreground(theme.DefaultTheme.Colors.Pink)
	}

	// 4. Apply state modifiers
	if info.note != nil {
		if _, isCut := m.cutPaths[info.note.Path]; isCut {
			style = style.Faint(true).Strikethrough(true)
		} else if info.isArchived {
			style = style.Faint(true)
		}
	} else if info.isArchived {
		style = style.Faint(true)
	}

	// 5. Apply selection modifier (final visual state)
	if isSelected {
		style = style.Underline(true)
	}

	// --- Render Content ---
	// Prepare suffix styling
	suffix := theme.DefaultTheme.Muted.Render(info.suffix)

	var styledName string
	if hasMatch {
		pre := info.name[:matchStart]
		match := info.name[matchStart:matchEnd]
		post := info.name[matchEnd:]
		highlightStyle := theme.DefaultTheme.Highlight.Copy().Reverse(true)
		styledName = style.Render(pre) + highlightStyle.Render(match) + style.Render(post)
	} else {
		styledName = style.Render(info.name)
	}

	return content + styledName + suffix
}

// getSearchHighlightIndices returns the start and end indices of the search match, or -1, -1 if no match
func (m *Model) getSearchHighlightIndices(text string) (int, int) {
	if m.filterValue == "" || m.isGrepping {
		return -1, -1
	}

	lowerText := strings.ToLower(text)
	lowerFilter := strings.ToLower(m.filterValue)
	if idx := strings.Index(lowerText, lowerFilter); idx != -1 {
		return idx, idx + len(m.filterValue)
	}
	return -1, -1
}

// calculateTableColumnWidths calculates optimal column widths based on content
func (m *Model) calculateTableColumnWidths() [6]int {
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
	const minPathWidth = 20
	const maxPathWidth = 120

	// Start with header widths
	maxName := len("WORKSPACE / NOTE")
	maxType := len("TYPE")
	maxStatus := len("STATUS")
	maxTags := len("TAGS")
	maxCreated := len("CREATED")
	maxPath := len("PATH")

	// Scan through all display nodes to find max widths
	for _, node := range m.displayNodes {
		if node.IsWorkspace {
			nameLen := lipgloss.Width(fmt.Sprintf("%s%s", node.Prefix, node.Workspace.Name)) + 4 // +4 for fold indicator
			if nameLen > maxName {
				maxName = nameLen
			}
		} else if node.IsGroup {
			displayName := node.GroupName
			if node.IsPlan() {
				displayName = strings.TrimPrefix(displayName, "plans/")
			}
			nameLen := lipgloss.Width(fmt.Sprintf("%s%s", node.Prefix, displayName)) + 4
			if nameLen > maxName {
				maxName = nameLen
			}
			if node.IsPlan() {
				status := m.GetPlanStatus(node.WorkspaceName, node.GroupName)
				if len(status) > maxStatus {
					maxStatus = len(status)
				}
			}
		} else if node.IsNote {
			nameLen := lipgloss.Width(fmt.Sprintf("%s%s %s", node.Prefix, getNoteIcon(string(node.Note.Type)), node.Note.Title))
			if nameLen > maxName {
				maxName = nameLen
			}
			typeLen := len(string(node.Note.Type))
			if typeLen > maxType {
				maxType = typeLen
			}
			status := getNoteStatus(node.Note)
			if len(status) > maxStatus {
				maxStatus = len(status)
			}
			tagsLen := len(strings.Join(node.Note.Tags, ", "))
			if tagsLen > maxTags {
				maxTags = tagsLen
			}
			if len(node.RelativePath) > maxPath {
				maxPath = len(node.RelativePath)
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
	if maxPath < minPathWidth {
		maxPath = minPathWidth
	}
	if maxPath > maxPathWidth {
		maxPath = maxPathWidth
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
	if !m.columnVisibility["PATH"] {
		maxPath = 0
	}

	return [6]int{maxName, maxType, maxStatus, maxTags, maxCreated, maxPath}
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
func (m *Model) GetPlanStatus(workspaceName, planGroup string) string {
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

// getViewportHeight calculates how many lines are available for the list.
func (m *Model) getViewportHeight() int {
	// Account for:
	// - Top margin: 2 lines
	// - Header: 1 line
	// - Blank line before footer: 1 line
	// - Status bar: 1 line
	// - Footer (help): 1 line
	// - Scroll indicator (when shown): 2 lines (blank + indicator)
	const fixedLines = 15
	availableHeight := m.height - fixedLines
	if availableHeight < 1 {
		return 1
	}
	return availableHeight
}

// getGroupKey returns a unique key for a group node (workspace:groupName)
func (m *Model) getGroupKey(node *DisplayNode) string {
	return node.WorkspaceName + ":" + node.GroupName
}

// formatRelativeTime formats a time as a relative string
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
