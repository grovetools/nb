package views

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	coreconfig "github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/theme"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/tree"
)

// nodeRenderInfo holds the unstyled components of a display node for rendering.
// This decouples data extraction from styling logic.
type nodeRenderInfo struct {
	prefix      string
	indicator   string // "■", note icon, or plan status icon
	name        string
	count      string // "(d)"
	suffix     string
	isArchived bool
	isArtifact bool
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

// recomputePrefixes recalculates tree prefixes to match the neotree style.
func (m *Model) recomputePrefixes(nodes []*DisplayNode) {
	// A map to track if the node at a given depth is the last in its peer group.
	lastNodeAtDepth := make(map[int]bool)

	for i, node := range nodes {
		if node.Item == nil { // Skip separators.
			continue
		}

		var prefixBuilder strings.Builder
		depth := node.Depth

		// Determine if the current node is the last among its direct siblings.
		isLast := true
		for j := i + 1; j < len(nodes); j++ {
			if nodes[j].Item == nil {
				continue
			}
			if nodes[j].Depth < depth {
				break // We've moved to a shallower depth, so this was the last.
			}
			if nodes[j].Depth == depth {
				isLast = false // Found a sibling at the same depth.
				break
			}
		}

		// Build the prefix.
		for d := 0; d < depth; d++ {
			if d == depth-1 { // This is the node's own connector level.
				if isLast {
					prefixBuilder.WriteString("└ ")
				} else {
					prefixBuilder.WriteString("│ ")
				}
			} else { // This is for an ancestor's vertical line.
				isLastAncestor := lastNodeAtDepth[d]

				// For children of root nodes (where ancestor depth d is 0),
				// always behave as if the root is not the last item. This ensures
				// the vertical line is always drawn for the children of every
				// top-level workspace, creating a consistent look.
				if d == 0 {
					isLastAncestor = false
				}

				if isLastAncestor {
					// The ancestor at this level was a "last" child, so we draw a space instead of a line.
					prefixBuilder.WriteString("  ")
				} else {
					// The ancestor was not a "last" child, so we continue the vertical line.
					prefixBuilder.WriteString("│ ")
				}
			}
		}

		node.Prefix = prefixBuilder.String()

		// Update the status for the current depth.
		lastNodeAtDepth[depth] = isLast
		// Invalidate deeper levels.
		for d := depth + 1; d < len(lastNodeAtDepth); d++ {
			delete(lastNodeAtDepth, d)
		}
	}
}

// renderTreeView renders the hierarchical tree view.
func (m *Model) renderTreeView() string {
	var b strings.Builder

	// Recompute prefixes for the current view
	m.recomputePrefixes(m.displayNodes)

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
			prefix := theme.DefaultTheme.Muted.Render(info.prefix)
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
	modifiedWidth := colWidths[5]
	workspaceWidth := colWidths[6]
	pathWidth := colWidths[7]

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
	if m.columnVisibility["MODIFIED"] {
		headerParts = append(headerParts, separator, padOrTruncate("MODIFIED", modifiedWidth))
	}
	if m.columnVisibility["WORKSPACE"] {
		headerParts = append(headerParts, separator, padOrTruncate("WORKSPACE", workspaceWidth))
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

		var typeCol, statusCol, tagsCol, createdCol, modifiedCol, workspaceCol, pathCol string
		if node.IsNote() {
			// Extract type from metadata
			if noteType, ok := node.Item.Metadata["Type"].(string); ok {
				typeCol = noteType
			}
			statusCol = getNoteStatus(ItemToNote(node.Item))
			if tags, ok := node.Item.Metadata["Tags"].([]string); ok {
				tagsCol = strings.Join(tags, ", ")
			}
			if ws, ok := node.Item.Metadata["Workspace"].(string); ok {
				workspaceCol = ws
			}
			if created, ok := node.Item.Metadata["CreatedAt"].(time.Time); ok {
				createdCol = created.Format("2006-01-02 15:04")
			}
			modifiedCol = node.Item.ModTime.Format("2006-01-02 15:04")
			pathCol = node.RelativePath
		} else if info.isPlan {
			wsName, _ := node.Item.Metadata["Workspace"].(string)
			statusCol = m.GetPlanStatus(wsName, node.Item.Name)
		}

		// --- Build Name Column using new helpers ---
		var styledNameCol string
		if info.isSeparator {
			styledNameCol = theme.DefaultTheme.Muted.Render(padOrTruncate("  ─────", nameWidth))
		} else {
			prefix := theme.DefaultTheme.Muted.Render(info.prefix)
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
		if m.columnVisibility["MODIFIED"] {
			rowParts = append(rowParts, separator, padOrTruncate(modifiedCol, modifiedWidth))
		}
		if m.columnVisibility["WORKSPACE"] {
			rowParts = append(rowParts, separator, padOrTruncate(workspaceCol, workspaceWidth))
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

	if node.IsSeparator() {
		info.isSeparator = true
		return info
	}

	if node.IsWorkspace() {
		info.isWorkspace = true
		if ws, ok := node.Item.Metadata["Workspace"].(*workspace.WorkspaceNode); ok {
			info.workspace = ws
			info.name = ws.Name
			if ws.Name == "global" {
				info.indicator = theme.IconEarth
			} else if ws.IsEcosystem() {
				info.indicator = theme.IconFolderTree
			} else { // It's a project/repo
				info.indicator = theme.IconRepo
			}
		}
	} else if node.IsGroup() {
		info.isGroup = true
		groupName := node.Item.Name
		info.name = groupName
		info.isArchived = strings.Contains(groupName, "/.archive") || strings.Contains(groupName, "/.closed")
		info.isArtifact = strings.Contains(groupName, "/.artifacts")

		// For archive parent nodes (e.g., "current/.archive" or "plans/.archive"), display just ".archive"
		if strings.HasSuffix(groupName, "/.archive") {
			info.name = ".archive"
			icon := getGroupIcon(".archive", m.service.NoteTypes)
			if icon != "" {
				info.indicator = icon
			}
		} else if strings.HasSuffix(groupName, "/.closed") {
			// For closed parent nodes (e.g., "github-issues/.closed"), display just ".closed"
			info.name = ".closed"
			icon := getGroupIcon(".closed", m.service.NoteTypes)
			if icon != "" {
				info.indicator = icon
			}
		} else if strings.HasSuffix(groupName, "/.artifacts") {
			// For artifact parent nodes, display just ".artifacts"
			info.name = ".artifacts"
			icon := getGroupIcon(".artifacts", m.service.NoteTypes)
			if icon != "" {
				info.indicator = icon
			}
		} else if node.IsPlan() {
			// Handle plan nodes (but not archive nodes that start with "plans/")
			info.isPlan = true
			info.name = strings.TrimPrefix(groupName, "plans/") // Display plan name without "plans/"
			info.name = strings.TrimPrefix(info.name, ".archive/")   // Also remove ".archive/" prefix for archived plans
			groupKey := m.getGroupKey(node)
			if _, ok := m.selectedGroups[groupKey]; ok {
				info.indicator = "■ " // Selected indicator
			} else {
				wsName, _ := node.Item.Metadata["Workspace"].(string)
				planStatus := m.GetPlanStatus(wsName, groupName)
				info.indicator = getPlanStatusIcon(planStatus) + " "
			}
		} else {
			// Regular note groups - add their semantic icons
			icon := getGroupIcon(groupName, m.service.NoteTypes)
			if icon != "" {
				info.indicator = icon
			}
		}
		if node.ChildCount > 0 {
			info.count = fmt.Sprintf(" (%d)", node.ChildCount)
		}
	} else if node.IsNote() {
		// Convert Item to Note for compatibility
		note := ItemToNote(node.Item)
		info.note = note
		info.name = node.Item.Name
		info.isArchived = strings.Contains(node.Item.Path, "/.archive/") || strings.Contains(node.Item.Path, "/.closed/")
		info.isArtifact = node.Item.Type == tree.TypeArtifact
		if _, ok := m.selected[node.Item.Path]; ok {
			info.indicator = "■" // Selected indicator
		} else {
			noteType := ""
			if nt, ok := node.Item.Metadata["Type"].(string); ok {
				noteType = nt
			}
			info.indicator = getNoteIcon(noteType)
		}
	}

	// Add link suffix if the node is linked
	if node.LinkedNode != nil {
		if node.IsNote() {
			// Note -> Plan: [plan: → plan-name]
			linkedGroup := ""
			if g, ok := node.LinkedNode.Item.Metadata["Group"].(string); ok {
				linkedGroup = g
			}
			planName := strings.TrimPrefix(linkedGroup, "plans/")
			italicStyle := lipgloss.NewStyle().Italic(true)
			prefix := italicStyle.Render("plan:")
			info.suffix = fmt.Sprintf(" [%s → %s]", prefix, planName)
		} else if node.IsPlan() {
			// Plan -> Note: [note: ← Note Title]
			linkedTitle := ""
			if t, ok := node.LinkedNode.Item.Metadata["Title"].(string); ok {
				linkedTitle = t
			}
			italicStyle := lipgloss.NewStyle().Italic(true)
			prefix := italicStyle.Render("note:")
			info.suffix = fmt.Sprintf(" [%s ← %s]", prefix, linkedTitle)
		}
	} else if node.IsNote() || node.IsPlan() {
		// Debug: log when a note/plan has no link
		// (remove this after debugging)
		_ = node // suppress unused warning
	}

	return info
}

// styleNodeContent applies styling to the main content (name/title) of a node.
func (m *Model) styleNodeContent(info nodeRenderInfo, isSelected bool) string {
	// Style the indicator (icon) separately with semantic colors
	styledIndicator := info.indicator

	// Color icons for groups and plans
	if (info.isGroup || info.isPlan) && !info.isArchived && !info.isArtifact && info.indicator != "" {
		var iconColor lipgloss.TerminalColor
		applyColor := false

		// Look up the icon color from NoteTypes registry
		if typeConfig, ok := m.service.NoteTypes[info.name]; ok && typeConfig.IconColor != "" {
			iconColor = m.mapColorString(typeConfig.IconColor)
			applyColor = true
		} else if info.isPlan {
			// Individual plans use blue (fallback if not in registry)
			iconColor = theme.DefaultTheme.Colors.Blue
			applyColor = true
		} else if info.indicator == theme.IconFolder {
			// Generic directory icons use cyan
			iconColor = theme.DefaultTheme.Colors.Cyan
			applyColor = true
		}

		if applyColor {
			styledIndicator = lipgloss.NewStyle().Foreground(iconColor).Render(info.indicator)
		}
	} else if info.note != nil && info.indicator != "" && info.indicator != "■" {
		// Color note icons based on their group
		var iconColor lipgloss.TerminalColor
		applyColor := false

		// Notes in in_progress that are linked to plans get blue icons
		if info.note.Group == "in_progress" && info.note.PlanRef != "" {
			iconColor = theme.DefaultTheme.Colors.Blue
			applyColor = true
		}

		if applyColor {
			styledIndicator = lipgloss.NewStyle().Foreground(iconColor).Render(info.indicator)
		}
	}

	// Build base content string with styled indicator
	content := styledIndicator
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
		style = style.Italic(true)
	} else if info.isPlan {
		// Individual plans are italic with default text color
		style = style.Italic(true)
	} else if info.isGroup && info.name == "plans" {
		// Plans heading is italic with default text color
		style = style.Italic(true)
	} else if info.isGroup && info.name == "in_progress" {
		// In progress heading is italic with default text color
		style = style.Italic(true)
	}

	// 3. Apply special group colors (only for text, icons are colored separately)
	// Note: Icons are already colored in the icon styling section above
	// Here we only color the group names themselves if needed for special emphasis
	if info.note != nil && info.note.Group == "review" {
		// Notes in the review group use pink text
		style = style.Foreground(theme.DefaultTheme.Colors.Pink)
	}
	// Notes in completed group use default text color (only icon is green)

	// 4. Apply state modifiers
	if info.note != nil {
		if _, isCut := m.cutPaths[info.note.Path]; isCut {
			style = style.Faint(true).Strikethrough(true)
		} else if info.note.Remote != nil && (info.note.Remote.State == "closed" || info.note.Remote.State == "merged") {
			style = style.Faint(true)
		} else if info.isArchived || info.isArtifact {
			style = style.Faint(true)
		}
	} else if info.isArchived || info.isArtifact {
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

// mapColorString maps a color string from config to a lipgloss.TerminalColor
func (m *Model) mapColorString(colorStr string) lipgloss.TerminalColor {
	switch colorStr {
	case "red":
		return theme.DefaultTheme.Colors.Red
	case "orange":
		return theme.DefaultTheme.Colors.Orange
	case "blue":
		return theme.DefaultTheme.Colors.Blue
	case "pink":
		return theme.DefaultTheme.Colors.Pink
	case "green":
		return theme.DefaultTheme.Colors.Green
	case "cyan":
		return theme.DefaultTheme.Colors.Cyan
	default:
		// Treat any other value as a direct lipgloss.Color value
		return lipgloss.Color(colorStr)
	}
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
func (m *Model) calculateTableColumnWidths() [8]int {
	// Min and max constraints for each column
	const minNameWidth = 30
	const maxNameWidth = 60
	const minTypeWidth = 10
	const maxTypeWidth = 25
	const minStatusWidth = 10
	const maxStatusWidth = 20
	const minTagsWidth = 15
	const maxTagsWidth = 50
	const minWorkspaceWidth = 10
	const maxWorkspaceWidth = 30
	const minCreatedWidth = 17
	const maxCreatedWidth = 17 // Fixed width for dates
	const minModifiedWidth = 17
	const maxModifiedWidth = 17 // Fixed width for dates
	const minPathWidth = 20
	const maxPathWidth = 120

	// Start with header widths
	maxName := len("WORKSPACE / NOTE")
	maxType := len("TYPE")
	maxStatus := len("STATUS")
	maxTags := len("TAGS")
	maxWorkspace := len("WORKSPACE")
	maxCreated := len("CREATED")
	maxModified := len("MODIFIED")
	maxPath := len("PATH")

	// Scan through all display nodes to find max widths
	for _, node := range m.displayNodes {
		if node.IsWorkspace() {
			wsName := ""
			if ws, ok := node.Item.Metadata["Workspace"].(*workspace.WorkspaceNode); ok {
				wsName = ws.Name
			}
			nameLen := lipgloss.Width(fmt.Sprintf("%s%s", node.Prefix, wsName)) + 2 // +2 for icon
			if nameLen > maxName {
				maxName = nameLen
			}
		} else if node.IsGroup() {
			displayName := node.Item.Name
			if node.IsPlan() {
				displayName = strings.TrimPrefix(displayName, "plans/")
			}
			nameLen := lipgloss.Width(fmt.Sprintf("%s%s", node.Prefix, displayName)) + 2
			if nameLen > maxName {
				maxName = nameLen
			}
			if node.IsPlan() {
				wsName, _ := node.Item.Metadata["Workspace"].(string)
				status := m.GetPlanStatus(wsName, node.Item.Name)
				if len(status) > maxStatus {
					maxStatus = len(status)
				}
			}
		} else if node.IsNote() {
			noteType := ""
			title := ""
			if nt, ok := node.Item.Metadata["Type"].(string); ok {
				noteType = nt
			}
			if t, ok := node.Item.Metadata["Title"].(string); ok {
				title = t
			}
			nameLen := lipgloss.Width(fmt.Sprintf("%s%s %s", node.Prefix, getNoteIcon(noteType), title))
			if nameLen > maxName {
				maxName = nameLen
			}
			typeLen := len(noteType)
			if typeLen > maxType {
				maxType = typeLen
			}
			note := ItemToNote(node.Item)
			status := getNoteStatus(note)
			if len(status) > maxStatus {
				maxStatus = len(status)
			}
			tagsLen := len(strings.Join(note.Tags, ", "))
			if tagsLen > maxTags {
				maxTags = tagsLen
			}
			if len(note.Workspace) > maxWorkspace {
				maxWorkspace = len(note.Workspace)
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
	if maxWorkspace < minWorkspaceWidth {
		maxWorkspace = minWorkspaceWidth
	}
	if maxWorkspace > maxWorkspaceWidth {
		maxWorkspace = maxWorkspaceWidth
	}
	if maxCreated < minCreatedWidth {
		maxCreated = minCreatedWidth
	}
	if maxCreated > maxCreatedWidth {
		maxCreated = maxCreatedWidth
	}
	if maxModified < minModifiedWidth {
		maxModified = minModifiedWidth
	}
	if maxModified > maxModifiedWidth {
		maxModified = maxModifiedWidth
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
	if !m.columnVisibility["WORKSPACE"] {
		maxWorkspace = 0
	}
	if !m.columnVisibility["CREATED"] {
		maxCreated = 0
	}
	if !m.columnVisibility["MODIFIED"] {
		maxModified = 0
	}
	if !m.columnVisibility["PATH"] {
		maxPath = 0
	}

	return [8]int{maxName, maxType, maxStatus, maxTags, maxCreated, maxModified, maxWorkspace, maxPath}
}

// getNoteStatus determines the status of a note (e.g., pending if it has todos)
func getNoteStatus(note *models.Note) string {
	if note.Remote != nil && note.Remote.State != "" {
		return note.Remote.State
	}
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
	case "note":
		return theme.IconNote
	case "plan":
		return theme.IconPlan
	case "chat":
		return theme.IconChat
	case "claude_session":
		return theme.IconInteractiveAgent
	case "oneshot":
		return theme.IconOneshot
	case "interactive_agent":
		return theme.IconInteractiveAgent
	case "headless_agent":
		return theme.IconHeadlessAgent
	case "shell":
		return theme.IconShell
	case "artifact":
		return theme.IconDocs
	default:
		return theme.IconDocs // Default to a generic file icon
	}
}

// getGroupIcon returns the appropriate icon for a note group
func getGroupIcon(groupName string, noteTypes map[string]*coreconfig.NoteTypeConfig) string {
	// Look up the icon from the NoteTypes registry
	if typeConfig, ok := noteTypes[groupName]; ok && typeConfig.Icon != "" {
		return typeConfig.Icon
	}
	// Fallback to a generic folder icon
	return theme.IconFolder
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
	case "completed":
		return theme.IconStatusCompleted
	case "running":
		return theme.IconStatusRunning
	case "failed":
		return theme.IconStatusFailed
	case "blocked":
		return theme.IconStatusBlocked
	case "needs_review":
		return theme.IconStatusNeedsReview
	case "pending_user":
		return theme.IconStatusPendingUser
	case "pending_llm":
		return theme.IconInteractiveAgent
	case "interrupted":
		return theme.IconStatusInterrupted
	case "todo":
		return theme.IconStatusTodo
	case "hold":
		return theme.IconStatusHold
	case "abandoned":
		return theme.IconStatusAbandoned
	default:
		return theme.IconPending // pending
	}
}

// getViewportHeight calculates how many lines are available for the list content,
// accounting for chrome rendered within this view component.
func (m *Model) getViewportHeight() int {
	var fixedLines int
	if m.viewMode == TableView {
		// Account for table header (2 lines), scroll indicator (2 lines), and bottom margin (1 line).
		fixedLines = 5
	} else {
		// Account for scroll indicator (2 lines) and bottom margin (1 line).
		fixedLines = 3
	}

	availableHeight := m.height - fixedLines
	if availableHeight < 1 {
		return 1
	}
	return availableHeight
}

// getGroupKey returns a unique key for a group node (workspace:groupName)
func (m *Model) getGroupKey(node *DisplayNode) string {
	if node.Item.Type == tree.TypeGroup || node.Item.Type == tree.TypePlan {
		if wsName, ok := node.Item.Metadata["Workspace"].(string); ok {
			if group, ok := node.Item.Metadata["Group"].(string); ok {
				return wsName + ":" + group
			}
			return wsName + ":" + node.Item.Name
		}
	}
	return ""
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
