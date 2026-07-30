package browser

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/core/tui/theme"
)

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
	node := m.views.GetCurrentNode()
	if node == nil {
		return "global/inbox"
	}

	if node.IsWorkspace() {
		if ws, ok := node.Item.Metadata["Workspace"].(*workspace.WorkspaceNode); ok {
			return fmt.Sprintf("%s/inbox", ws.Name)
		}
	} else if node.IsGroup() {
		if wsName, ok := node.Item.Metadata["Workspace"].(string); ok {
			return fmt.Sprintf("%s/%s", wsName, node.Item.Name)
		}
	} else if node.IsNote() {
		wsName, _ := node.Item.Metadata["Workspace"].(string)
		groupName, _ := node.Item.Metadata["Group"].(string)
		return fmt.Sprintf("%s/%s", wsName, groupName)
	}

	return "global/inbox"
}

// searchBarVisible reports whether View() will render the search bar. Kept
// next to chromeRows so the row reservation and the render can never disagree
// about it.
func (m Model) searchBarVisible() bool {
	return m.filterInput.Focused() || m.filterInput.Value() != ""
}

// chromeRows is the number of rows View() spends on everything that is not
// the list, derived term by term from what it emits:
//
//	top margin        1  the leading "\n" — standalone only (see below)
//	header            1
//	header spacer     1  theme Header's MarginBottom(1)
//	search bar        1  only while searchBarVisible()
//	search spacer     1  the explicit "" joined under the search bar
//	status spacer     1  the explicit "" joined above the status row
//	status            1  now also carries the scroll indicator, flush right
//
// The help footer is deliberately absent: standalone renders none at all, and
// when hosted the pager reserves it (pager.Config.FooterHeight in
// tui/view) before handing this model its height. The top margin is likewise
// hosted-only in reverse — the pager's page adapter strips the leading "\n",
// and with HideTabBar the pager spends no rows of its own above the body.
func (m Model) chromeRows() int {
	rows := 2 // header + its bottom spacer
	if !m.hosted {
		rows++ // leading "\n"
	}
	if m.searchBarVisible() {
		rows += 2 // search bar + spacer
	}
	rows += 2 // status spacer + status row
	return rows
}

// chromeCols is the horizontal space View() spends on its own frame: the
// PaddingLeft(2) applied to the whole layout. The list renders flush inside
// that inset, so nothing else is deducted.
func (m Model) chromeCols() int { return 2 }

// syncViewsSize hands the list component the space View() will actually leave
// it. Bounds are clamped to >= 1 so a pane too short for the chrome still
// renders a usable single row instead of a negative viewport.
func (m *Model) syncViewsSize() {
	w := m.width - m.chromeCols()
	h := m.height - m.chromeRows()
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	m.views.SetSize(w, h)
}

func (m Model) View() string {
	if m.loadingCount > 0 && len(m.workspaces) == 0 {
		return "\n" + lipgloss.NewStyle().PaddingLeft(2).Render(m.spinner.View()+" Loading notebook...")
	}

	if m.help.ShowAll {
		return m.help.View()
	}

	// If a component is active, render it as an overlay
	if m.confirmDialog.Active {
		dialog := m.confirmDialog.View()
		return "\n" + lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
	}

	// Render tag picker if active
	if m.tagPickerMode {
		content := m.tagPicker.View()

		dialogBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.DefaultTheme.Colors.Cyan).
			Padding(1, 2).
			Render(content)

		helpText := lipgloss.NewStyle().
			Faint(true).
			Width(lipgloss.Width(dialogBox)).
			Align(lipgloss.Center).
			Render("\n\nEnter to select • Esc to cancel")

		overlay := lipgloss.JoinVertical(lipgloss.Left, dialogBox, helpText)

		// Add padding from top and left
		paddedOverlay := lipgloss.NewStyle().
			Padding(2, 0, 0, 4).
			Render(overlay)

		return "\n" + lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, paddedOverlay)
	}

	// Render plan picker if active (promote to job)
	if m.isPromotingToJob {
		content := m.planPicker.View()

		dialogBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.DefaultTheme.Colors.Cyan).
			Padding(1, 2).
			Render(content)

		helpText := lipgloss.NewStyle().
			Faint(true).
			Width(lipgloss.Width(dialogBox)).
			Align(lipgloss.Center).
			Render("\n\nEnter to select • Esc to cancel")

		overlay := lipgloss.JoinVertical(lipgloss.Left, dialogBox, helpText)

		// Add padding from top and left
		paddedOverlay := lipgloss.NewStyle().
			Padding(2, 0, 0, 4).
			Render(overlay)

		return "\n" + lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, paddedOverlay)
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

		// Add padding from top and left
		paddedOverlay := lipgloss.NewStyle().
			Padding(2, 0, 0, 4).
			Render(overlay)

		return "\n" + lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, paddedOverlay)
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
		return "\n" + lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
	}

	// Render git commit dialog if active
	if m.isCommitting {
		contextLine := lipgloss.NewStyle().
			Faint(true).
			Render("Git Commit")

		content := contextLine + "\n\nCommit Message:\n" + m.commitInput.View()

		dialogBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.DefaultTheme.Colors.Green).
			Padding(1, 2).
			Render(content)

		helpText := lipgloss.NewStyle().
			Faint(true).
			Width(lipgloss.Width(dialogBox)).
			Align(lipgloss.Center).
			Render("\n\nEnter to commit • Esc to cancel")

		overlay := lipgloss.JoinVertical(lipgloss.Left, dialogBox, helpText)
		return "\n" + lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
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
		return "\n" + lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}

	// --- Single-pane layout (preview is handled by the terminal host VDrawer) ---
	// The list is rendered flush against the frame's own PaddingLeft(2) rather
	// than inside a second pane inset: an extra Padding(0, 1) here pushed the
	// rows one column right of the header directly above them and clipped a
	// column off the right edge, which costs real table columns in a
	// half-width treemux panel.
	viewContent := m.views.View()

	// Header - breadcrumb style
	// Get notebook title from config
	notebookTitle := "Notebook Browser"
	if m.service.CoreConfig != nil && m.service.CoreConfig.Notebooks != nil && m.service.CoreConfig.Notebooks.Definitions != nil && len(m.service.CoreConfig.Notebooks.Definitions) > 0 {
		// Use the default notebook name from rules if available
		if m.service.CoreConfig.Notebooks.Rules != nil && m.service.CoreConfig.Notebooks.Rules.Default != "" {
			notebookTitle = m.service.CoreConfig.Notebooks.Rules.Default
		} else if _, ok := m.service.CoreConfig.Notebooks.Definitions["default"]; ok {
			// Fall back to "default" if it exists
			notebookTitle = "default"
		} else {
			// Use the first notebook name
			for name := range m.service.CoreConfig.Notebooks.Definitions {
				notebookTitle = name
				break
			}
		}
	}

	// Build header string with inline styling for tag indicator
	headerParts := []string{notebookTitle}
	if m.focusedWorkspace != nil {
		headerParts = append(headerParts, " > ", m.focusedWorkspace.Name)
	}
	if m.recentNotesMode {
		headerParts = append(headerParts, " [Recent]")
	}
	if m.showGitModifiedOnly {
		headerParts = append(headerParts, " [Git Modified]")
	}

	// Add group-by indicator when an axis other than "none" is active.
	if m.groupBy != "" && m.groupBy != "none" {
		groupStyled := lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.DefaultTheme.Colors.Orange).
			Render(fmt.Sprintf(" [Group: %s]", m.groupBy))
		headerParts = append(headerParts, groupStyled)
	}

	// Add tag indicator inline with special styling
	if m.isFilteringByTag {
		tagStyled := lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.DefaultTheme.Colors.Orange).
			Render(fmt.Sprintf(" [Tag: %s]", m.selectedTag))
		headerParts = append(headerParts, tagStyled)
	}

	// Add mode indicators
	if m.isGrepping {
		headerParts = append(headerParts, " [Grep Mode]")
	} else if m.filterInput.Focused() && !m.isGrepping && !m.isFilteringByTag {
		headerParts = append(headerParts, " [Find Mode]")
	} else if m.filterInput.Focused() && m.isFilteringByTag {
		headerParts = append(headerParts, " [Search]")
	} else if m.ecosystemPickerMode {
		headerParts = append(headerParts, " [Select Ecosystem]")
	}

	// Join all parts and apply theme styling. The theme's Header carries a
	// MarginTop(1) worth a row we have better uses for: standalone it would
	// land directly under the frame's own leading "\n", double-spacing the
	// title, and hosted there is nothing above the body to separate from
	// (the page adapter strips that "\n" and the tab bar is hidden), so the
	// title belongs flush against the top of the pane.
	headerText := lipgloss.JoinHorizontal(lipgloss.Left, headerParts...)
	header := theme.DefaultTheme.Header.MarginTop(0).Render(headerText)

	// Build status bar
	var status string
	if m.loadingCount > 0 {
		status = m.spinner.View() + " Loading..."
	} else if m.statusMessage != "" {
		// Truncate status message to fit terminal width
		maxStatusWidth := m.width - 10 // Leave some margin
		if maxStatusWidth < 50 {
			maxStatusWidth = 50
		}
		if len(m.statusMessage) > maxStatusWidth {
			status = m.statusMessage[:maxStatusWidth-3] + "..."
		} else {
			status = m.statusMessage
		}
	} else {
		// Get note count and selection info from views
		noteCount, selectedNotes, selectedPlans := m.views.GetCounts()

		selectionInfo := ""
		if selectedNotes > 0 && selectedPlans > 0 {
			selectionInfo = fmt.Sprintf(" | %d notes + %d plans selected", selectedNotes, selectedPlans)
		} else if selectedNotes > 0 {
			selectionInfo = fmt.Sprintf(" | %d notes selected", selectedNotes)
		} else if selectedPlans > 0 {
			selectionInfo = fmt.Sprintf(" | %d plans selected", selectedPlans)
		} else {
			selectionInfo = " | 0 selected"
		}
		status = fmt.Sprintf("%d notes shown%s", noteCount, selectionInfo)
	}

	// Immediate flat-chord footer hint (gg/dd/yy) so single-key arming is not
	// invisible. Only shown when NO namespace prefix is armed — a t…/g… prefix
	// renders the which-key popup (below) instead, so the two never double up.
	if !m.whichKey.Armed() {
		hintBindings := append(keymap.CommonSequenceBindings(m.keys.Base), m.keys.Copy)
		if hint := m.whichKey.FooterHint(hintBindings...); hint != "" {
			if status == "" {
				status = hint
			} else {
				status = status + "  " + hint
			}
		}
	}

	// Search bar (if active). Mirror nav's manual cursor rendering
	// (nav/pkg/tui/sessionizer/view.go) instead of bubbletea's
	// m.filterInput.View(): show the search icon, the value, and a thin "▏"
	// caret when focused / a fat "█" block when blurred-but-active. The fat
	// block makes an active-but-unfocused filter always visible.
	//
	// The single input carries the mode via its leading prefix ("?" grep,
	// "#tag"); the raw value (including the prefix) is shown verbatim so the
	// caret position stays truthful, while the label reflects the parsed mode.
	var searchBar string
	if m.searchBarVisible() {
		label := "Search: "
		if m.isGrepping {
			label = "Grep: "
		} else if m.isFilteringByTag {
			label = "Tag: "
		}
		val := m.filterInput.Value()
		caret := "█"
		if m.filterInput.Focused() {
			caret = "▏"
		}
		searchBar = label + theme.IconSearch + " " + val + caret
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
		m.alignScrollIndicator(theme.DefaultTheme.Muted.Render(status)),
	)

	// Apply global left padding, top margin, and width clamping
	styledView := lipgloss.NewStyle().PaddingLeft(2).MaxWidth(m.width).Render(fullView)
	frame := "\n" + styledView
	// Composite the bottom-anchored which-key popup onto the final frame while a
	// t…/c…/g… namespace prefix is armed (past the show-delay). Returns frame
	// unchanged otherwise; the delayed keymap.WhichKeyShowMsg tick forces the
	// re-render that reveals it.
	//
	// The vertical budget is passed explicitly (RenderOverlayAvail): the frame
	// is assembled from a header + a viewport-sized list + a status row and can
	// be shorter than the terminal, and plain RenderOverlay clamps the popup to
	// the frame's own line count — which truncates the nine-member t… namespace
	// despite ample room on screen.
	return m.whichKey.RenderOverlayAvail(frame, lipgloss.Width(frame), m.height, *theme.DefaultTheme)
}

// alignScrollIndicator pins the list's "(1-17 of 40)" position token to the
// right edge of the status row. The status row is mostly empty space and the
// indicator is one short token, so sharing the row hands the list back the two
// rows the indicator used to occupy under it (its own row plus the blank
// separating it from the last node).
//
// The row must never wrap — a wrapped status line would immediately cost back
// a reclaimed row — so when the pane is too narrow to hold both, the status
// text is truncated to make room, and if even that leaves nothing the
// indicator is dropped (the cursor row still conveys position).
func (m Model) alignScrollIndicator(status string) string {
	indicator := m.views.ScrollIndicator()
	// contentWidth mirrors the frame's PaddingLeft(2) below: the status row
	// is laid out inside that inset, not against the raw pane width.
	contentWidth := m.width - 2
	if indicator == "" || contentWidth <= 0 {
		return status
	}
	// One column of separation between the status text and the indicator.
	statusWidth := contentWidth - lipgloss.Width(indicator) - 1
	if statusWidth < 1 {
		return status
	}
	if lipgloss.Width(status) > statusWidth {
		status = ansi.Truncate(status, statusWidth, "…")
	}
	// Width() pads the (now guaranteed short enough) status text out so the
	// indicator lands flush right without a manual space run.
	return lipgloss.NewStyle().Width(statusWidth).Render(status) + " " +
		theme.DefaultTheme.Muted.Render(indicator)
}

// FooterView returns the help text for use as the pager footer.
// The host (view/model.go) wires this into pager.SetFooter so the
// pager can pin it below the scrollable body.
func (m Model) FooterView() string {
	return m.help.View()
}
