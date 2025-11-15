package browser

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/tui/theme"
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

	if node.IsWorkspace {
		return fmt.Sprintf("%s/inbox", node.Workspace.Name)
	} else if node.IsGroup {
		return fmt.Sprintf("%s/%s", node.WorkspaceName, node.GroupName)
	} else if node.IsNote {
		return fmt.Sprintf("%s/%s", node.Note.Workspace, node.Note.Group)
	}

	return "global/inbox"
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
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
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

		return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, paddedOverlay)
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

		return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, paddedOverlay)
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

	// --- Main two-pane layout (or single pane if preview hidden) ---
	browserContent := m.views.View()

	var viewContent string
	if m.previewVisible {
		// Update preview pane border based on focus
		if m.previewFocused {
			m.preview.Style = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(theme.DefaultTheme.Colors.Orange) // Highlight when focused
		} else {
			m.preview.Style = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(theme.DefaultTheme.Colors.MutedText)
		}

		// Create the two panes
		browserPaneStyle := lipgloss.NewStyle().Padding(0, 1)
		browserPane := browserPaneStyle.Render(browserContent)
		previewPane := m.preview.View()

		// Join them horizontally
		viewContent = lipgloss.JoinHorizontal(lipgloss.Top, browserPane, previewPane)
	} else {
		// Preview hidden - only show browser pane
		browserPaneStyle := lipgloss.NewStyle().Padding(0, 1)
		viewContent = browserPaneStyle.Render(browserContent)
	}

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

	// Join all parts and apply theme styling
	headerText := lipgloss.JoinHorizontal(lipgloss.Left, headerParts...)
	header := theme.DefaultTheme.Header.Render(headerText)

	footer := m.help.View()

	// Build status bar
	var status string
	if m.loadingCount > 0 {
		status = m.spinner.View() + " Loading..."
	} else if m.statusMessage != "" {
		status = m.statusMessage
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

	// Apply global left padding and top margin
	styledView := lipgloss.NewStyle().PaddingLeft(2).Render(fullView)
	return "\n" + styledView
}
