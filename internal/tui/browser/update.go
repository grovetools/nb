package browser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/theme"
	"github.com/mattsolo1/grove-core/util/pathutil"
	"github.com/mattsolo1/grove-notebook/internal/tui/browser/components/confirm"
	"github.com/mattsolo1/grove-notebook/internal/tui/browser/views"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

// updateViewsState synchronizes the view state with the browser model
func (m *Model) updateViewsState() {
	m.views.SetParentState(
		m.service,
		m.allNotes,
		m.workspaces,
		m.focusedWorkspace,
		m.filterInput.Value(),
		m.isGrepping,
		m.ecosystemPickerMode,
		m.hideGlobal,
		m.showArchives,
	)
	m.views.BuildDisplayTree()

	// Apply text filter if present (not grep mode)
	if m.filterInput.Value() != "" && !m.isGrepping {
		m.views.FilterDisplayTree()
	}
}

// loadFileContentCmd is a command that reads a file and returns its content.
func loadFileContentCmd(path string) tea.Cmd {
	return func() tea.Msg {
		content, err := os.ReadFile(path)
		if err != nil {
			return fileContentReadyMsg{path: path, err: err}
		}
		return fileContentReadyMsg{path: path, content: string(content)}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case confirm.ConfirmedMsg:
		// User confirmed the action in the dialog
		// We need to know which action was confirmed. A simple way is to check the prompt.
		if strings.Contains(strings.ToLower(m.confirmDialog.Prompt), "archive") {
			m.statusMessage = "Archiving..."
			return m, m.archiveSelectedNotesCmd()
		}
		if strings.Contains(strings.ToLower(m.confirmDialog.Prompt), "delete") {
			m.statusMessage = "Deleting..."
			return m, m.deleteSelectedNotesCmd()
		}
	case confirm.CancelledMsg:
		// User cancelled, just clear the status message
		m.statusMessage = ""
		return m, nil
	case fileContentReadyMsg:
		if msg.err != nil {
			m.previewContent = fmt.Sprintf("Error loading file:\n%v", msg.err)
			if m.previewVisible {
				m.statusMessage = fmt.Sprintf("Error loading %s", filepath.Base(msg.path))
			}
		} else {
			// Check for a linked item to enhance the preview
			var infoBox string
			node := m.views.GetCurrentNode()
			if node != nil && node.LinkedNode != nil {
				style := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1).BorderForeground(theme.DefaultTheme.Colors.MutedText)
				var title, body string
				if node.IsNote {
					title = "🔗 Linked Plan"
					planStatus := m.views.GetPlanStatus(node.LinkedNode.WorkspaceName, node.LinkedNode.GroupName)
					planName := strings.TrimPrefix(node.LinkedNode.GroupName, "plans/")
					body = fmt.Sprintf("%s\nStatus: %s", planName, planStatus)
				} else if node.IsPlan() {
					title = "🔗 Linked Note"
					body = node.LinkedNode.Note.Title
				}
				infoBox = style.Render(fmt.Sprintf("%s\n%s", title, body)) + "\n\n"
			}
			m.previewContent = infoBox + msg.content
			if m.previewVisible {
				m.statusMessage = fmt.Sprintf("Previewing %s", filepath.Base(msg.path))
			}
		}
		m.previewFile = msg.path
		m.preview.SetContent(m.previewContent)
		m.preview.GotoTop() // Reset scroll on new file
		return m, nil
	case quitPopupMsg:
		return m, tea.Quit
	case editFileAndQuitMsg:
		// Write file path to temp file for Neovim to read
		// Use session ID from environment if available, otherwise fall back to PID
		sessionID := os.Getenv("GROVE_NVIM_SESSION_ID")
		if sessionID == "" {
			sessionID = fmt.Sprintf("%d", os.Getpid())
		}
		tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("grove-nb-edit-%s", sessionID))
		err := os.WriteFile(tempFile, []byte("OPEN:"+msg.filePath+"\n"), 0644)
		if err != nil {
			m.statusMessage = fmt.Sprintf("Error writing temp file: %v", err)
		} else {
			m.statusMessage = ""
		}
		// Don't quit - stay open
		return m, nil

	case previewFileMsg:
		// Write file path to temp file for Neovim to preview
		sessionID := os.Getenv("GROVE_NVIM_SESSION_ID")
		if sessionID == "" {
			sessionID = fmt.Sprintf("%d", os.Getpid())
		}
		tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("grove-nb-edit-%s", sessionID))
		err := os.WriteFile(tempFile, []byte("PREVIEW:"+msg.filePath+"\n"), 0644)
		if err != nil {
			m.statusMessage = fmt.Sprintf("Error writing temp file: %v", err)
		} else {
			m.statusMessage = ""
		}
		return m, nil

	case tmuxSplitFinishedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("tmux error: %v", msg.err)
			return m, nil // Stay in TUI to show the error
		}
		// Split was successful, store the pane IDs and stay in TUI
		if msg.clearPanes {
			// Old pane was closed, clear stored IDs
			m.tmuxSplitPaneID = ""
			m.tmuxTUIPaneID = ""
		}
		if msg.paneID != "" {
			m.tmuxSplitPaneID = msg.paneID
		}
		if msg.tuiPaneID != "" {
			m.tmuxTUIPaneID = msg.tuiPaneID
		}
		m.statusMessage = ""
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetSize(msg.Width, msg.Height)

		// Calculate pane sizes
		// header(1) + search(1) + blank(1) + view + blank(1) + status(1) + footer(1) + top_margin(1)
		const mainContentHeight = 7
		availableHeight := m.height - mainContentHeight

		browserWidth := m.width / 2
		previewWidth := m.width - browserWidth

		m.views.SetSize(browserWidth, availableHeight)
		m.preview.Width = previewWidth
		m.preview.Height = availableHeight

		m.columnList.SetSize(40, 8)
		return m, nil

	case workspacesLoadedMsg:
		if m.loadingCount > 0 {
			m.loadingCount--
		}
		m.workspaces = msg.workspaces
		m.updateViewsState()
		return m, nil

	case notesLoadedMsg:
		if m.loadingCount > 0 {
			m.loadingCount--
		}
		m.allNotes = msg.notes
		// Only reset collapse state if focus just changed
		if m.focusChanged {
			m.setCollapseStateForFocus()
			m.focusChanged = false
		}
		m.updateViewsState()
		m.findAndApplyLinks() // Detect and apply links between visible nodes
		return m, m.updatePreviewContent()

	case refreshMsg:
		m.loadingCount = 2 // for workspaces and notes

		var notesCmd tea.Cmd
		if m.focusedWorkspace != nil {
			notesCmd = fetchFocusedNotesCmd(m.service, m.focusedWorkspace)
		} else {
			notesCmd = fetchAllNotesCmd(m.service)
		}

		return m, tea.Batch(
			fetchWorkspacesCmd(m.service.GetWorkspaceProvider()),
			notesCmd,
			m.spinner.Tick,
		)

	case notesDeletedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error deleting notes: %v", msg.err)
			return m, nil
		}
		// Create a lookup map of deleted paths
		deletedMap := make(map[string]bool)
		for _, path := range msg.deletedPaths {
			deletedMap[path] = true
		}
		// Filter out deleted notes
		newAllNotes := make([]*models.Note, 0, len(m.allNotes))
		for _, note := range m.allNotes {
			if !deletedMap[note.Path] {
				newAllNotes = append(newAllNotes, note)
			}
		}
		m.allNotes = newAllNotes
		// Clear selections
		m.views.ClearSelections()
		// Rebuild display
		m.updateViewsState()
		m.statusMessage = fmt.Sprintf("Deleted %d note(s)", len(msg.deletedPaths))
		return m, nil

	case notesPastedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error pasting notes: %v", msg.err)
			return m, nil
		}
		// If it was a cut operation, clear the clipboard and cut paths
		if m.clipboardMode == "cut" {
			m.clipboard = nil
			m.clipboardMode = ""
			m.views.SetCutPaths(make(map[string]struct{}))
		}
		m.statusMessage = fmt.Sprintf("Pasted %d note(s) successfully", msg.pastedCount)
		// Refresh notes to show the new locations
		m.loadingCount++
		if m.focusedWorkspace != nil {
			return m, tea.Batch(fetchFocusedNotesCmd(m.service, m.focusedWorkspace), m.spinner.Tick)
		}
		return m, tea.Batch(fetchAllNotesCmd(m.service), m.spinner.Tick)

	case notesArchivedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error archiving notes: %v", msg.err)
			return m, nil
		}

		// Create a lookup map of archived paths
		archivedMap := make(map[string]bool)
		for _, path := range msg.archivedPaths {
			archivedMap[path] = true
		}

		// Filter out archived notes from allNotes
		newAllNotes := make([]*models.Note, 0)
		for _, note := range m.allNotes {
			if !archivedMap[note.Path] {
				newAllNotes = append(newAllNotes, note)
			}
		}
		m.allNotes = newAllNotes

		// Clear selections
		m.views.ClearSelections()

		// Rebuild the display
		m.updateViewsState()

		if msg.archivedPlans > 0 {
			m.statusMessage = fmt.Sprintf("Archived %d note(s) and %d plan(s)", len(msg.archivedPaths), msg.archivedPlans)
		} else {
			m.statusMessage = fmt.Sprintf("Archived %d note(s)", len(msg.archivedPaths))
		}

		// Refresh notes to show the updated archive structure
		m.loadingCount++
		if m.focusedWorkspace != nil {
			return m, tea.Batch(fetchFocusedNotesCmd(m.service, m.focusedWorkspace), m.spinner.Tick)
		}
		return m, tea.Batch(fetchAllNotesCmd(m.service), m.spinner.Tick)

	case noteCreatedMsg:
		m.isCreatingNote = false
		m.noteTitleInput.Blur()
		m.noteTitleInput.SetValue("")
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error creating note: %v", msg.err)
			return m, nil
		}
		m.statusMessage = fmt.Sprintf("Created note: %s", msg.note.Title)
		// Refresh notes to show the new one
		m.loadingCount++
		if m.focusedWorkspace != nil {
			return m, tea.Batch(fetchFocusedNotesCmd(m.service, m.focusedWorkspace), m.spinner.Tick)
		}
		return m, tea.Batch(fetchAllNotesCmd(m.service), m.spinner.Tick)

	case noteRenamedMsg:
		m.isRenamingNote = false
		m.renameInput.Blur()
		m.renameInput.SetValue("")
		m.noteToRename = nil
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error renaming note: %v", msg.err)
			return m, nil
		}
		m.statusMessage = "Note renamed successfully"
		// Refresh notes to show the updated name
		m.loadingCount++
		if m.focusedWorkspace != nil {
			return m, tea.Batch(fetchFocusedNotesCmd(m.service, m.focusedWorkspace), m.spinner.Tick)
		}
		return m, tea.Batch(fetchAllNotesCmd(m.service), m.spinner.Tick)

	case tea.KeyMsg:
		if m.help.ShowAll {
			m.help.Toggle()
			return m, nil
		}

		// Handle active components first
		if m.confirmDialog.Active {
			m.confirmDialog, cmd = m.confirmDialog.Update(msg)
			return m, cmd
		}

		// If preview is focused, it gets priority for key events.
		if m.previewFocused {
			switch {
			case key.Matches(msg, m.keys.Preview): // Tab still switches focus back
				m.previewFocused = false
				return m, nil
			case key.Matches(msg, m.keys.Quit): // Allow quitting from preview
				return m, tea.Quit
			case key.Matches(msg, m.keys.Back): // Esc to switch focus back
				m.previewFocused = false
				return m, nil
			default:
				// Pass all other keys to the viewport for scrolling
				var cmd tea.Cmd
				m.preview, cmd = m.preview.Update(msg)
				return m, cmd
			}
		}

		// Handle note creation mode
		if m.isCreatingNote {
			return m.updateNoteCreation(msg)
		}

		// Handle note rename mode
		if m.isRenamingNote {
			return m.updateNoteRename(msg)
		}

		// Handle column selection mode
		if m.columnSelectMode {
			switch msg.String() {
			case "enter", "esc", "V":
				m.columnSelectMode = false
				return m, nil
			case " ":
				// Toggle selection
				if i, ok := m.columnList.SelectedItem().(columnSelectItem); ok {
					i.selected = !i.selected
					m.columnVisibility[i.name] = i.selected
					m.columnList.SetItem(m.columnList.Index(), i)
					// Save state to disk
					_ = m.saveState()
				}
				return m, nil
			default:
				m.columnList, cmd = m.columnList.Update(msg)
				return m, cmd
			}
		}

		// Handle filtering mode
		if m.filterInput.Focused() {
			switch {
			case key.Matches(msg, m.keys.Back): // Esc
				m.filterInput.SetValue("")
				m.filterInput.Blur()
				m.isGrepping = false // Exit grep mode
				m.updateViewsState()
				return m, nil
			case key.Matches(msg, m.keys.Confirm): // Enter
				m.filterInput.Blur()
				return m, nil
			default:
				m.filterInput, cmd = m.filterInput.Update(msg)
				if m.isGrepping {
					m.applyGrepFilter()
				} else {
					m.updateViewsState()
				}
				return m, cmd
			}
		}

		// Try delegating to views for navigation, folding, and selection
		// Views.Update handles: Up, Down, PageUp, PageDown, GoToTop, GoToBottom,
		// Fold (h), Unfold (l), FoldPrefix (z), all z* fold commands,
		// ToggleSelect (space), SelectNone (N)
		if key.Matches(msg, m.keys.Up) || key.Matches(msg, m.keys.Down) ||
			key.Matches(msg, m.keys.PageUp) || key.Matches(msg, m.keys.PageDown) ||
			key.Matches(msg, m.keys.GoToTop) || key.Matches(msg, m.keys.GoToBottom) ||
			key.Matches(msg, m.keys.Fold) || key.Matches(msg, m.keys.Unfold) ||
			key.Matches(msg, m.keys.FoldPrefix) ||
			msg.String() == "a" ||
			msg.String() == "o" || msg.String() == "O" ||
			msg.String() == "c" || msg.String() == "C" ||
			msg.String() == "M" || msg.String() == "R" ||
			(msg.String() == "A" && m.lastKey == "z") || // zA fold command
			key.Matches(msg, m.keys.ToggleSelect) ||
			key.Matches(msg, m.keys.SelectNone) {
			// Track 'z' for fold sequences
			if key.Matches(msg, m.keys.FoldPrefix) {
				m.lastKey = "z"
			} else if msg.String() == "A" && m.lastKey == "z" {
				m.lastKey = "" // Reset after zA sequence
			}
			m.views, cmd = m.views.Update(msg)
			// After any view update that could change the cursor, reapply links and update the preview.
			m.findAndApplyLinks()
			return m, tea.Batch(cmd, m.updatePreviewContent())
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.help.Toggle()
			return m, nil
		case key.Matches(msg, m.keys.FocusEcosystem):
			if !m.ecosystemPickerMode {
				m.ecosystemPickerMode = true
				m.updateViewsState()
			}
		case key.Matches(msg, m.keys.ClearFocus):
			if m.focusedWorkspace != nil || m.ecosystemPickerMode {
				m.loadingCount++
				m.focusedWorkspace = nil
				m.ecosystemPickerMode = false
				m.focusChanged = true
				// Re-fetch all notes for the global view
				return m, tea.Batch(fetchAllNotesCmd(m.service), m.spinner.Tick)
			}
		case key.Matches(msg, m.keys.FocusParent):
			if m.focusedWorkspace != nil {
				var parent *workspace.WorkspaceNode

				// Try to find parent in this order:
				// 1. ParentEcosystemPath (immediate parent ecosystem)
				// 2. RootEcosystemPath (if not already at root)
				// 3. ParentProjectPath (parent project)
				// 4. nil (go to global view)

				var parentPath string
				if m.focusedWorkspace.ParentEcosystemPath != "" {
					parentPath = m.focusedWorkspace.ParentEcosystemPath
				} else if m.focusedWorkspace.RootEcosystemPath != "" &&
					m.focusedWorkspace.RootEcosystemPath != m.focusedWorkspace.Path {
					// Not at root yet, go to root ecosystem
					parentPath = m.focusedWorkspace.RootEcosystemPath
				} else if m.focusedWorkspace.ParentProjectPath != "" {
					parentPath = m.focusedWorkspace.ParentProjectPath
				}

				if parentPath != "" {
					// Find the parent workspace node
					for _, ws := range m.workspaces {
						isSame, _ := pathutil.ComparePaths(ws.Path, parentPath)
						if isSame {
							parent = ws
							break
						}
					}
				}

				m.loadingCount++
				m.focusedWorkspace = parent // This can be nil if no parent is found, effectively clearing focus
				m.focusChanged = true
				// Re-fetch notes for the new focus level
				if m.focusedWorkspace != nil {
					return m, tea.Batch(fetchFocusedNotesCmd(m.service, m.focusedWorkspace), m.spinner.Tick)
				} else {
					return m, tea.Batch(fetchAllNotesCmd(m.service), m.spinner.Tick)
				}
			}
		case key.Matches(msg, m.keys.FocusSelected):
			node := m.views.GetCurrentNode()
			if node != nil && node.IsWorkspace {
				m.loadingCount++
				m.focusedWorkspace = node.Workspace
				m.ecosystemPickerMode = false // Focusing on a workspace exits picker mode
				m.focusChanged = true
				// Re-fetch notes for the newly focused workspace
				return m, tea.Batch(fetchFocusedNotesCmd(m.service, m.focusedWorkspace), m.spinner.Tick)
			}
		case key.Matches(msg, m.keys.ToggleView):
			m.views.ToggleViewMode()
		case key.Matches(msg, m.keys.Search):
			m.isGrepping = false
			m.filterInput.SetValue("")
			m.filterInput.Placeholder = "Search notes..."
			m.filterInput.Focus()
			return m, textinput.Blink
		case key.Matches(msg, m.keys.Refresh):
			return m, func() tea.Msg { return refreshMsg{} }
		case key.Matches(msg, m.keys.Grep):
			m.isGrepping = true
			m.filterInput.SetValue("")
			m.filterInput.Placeholder = "Grep content..."
			m.filterInput.Focus()
			return m, textinput.Blink
		case key.Matches(msg, m.keys.ToggleColumns):
			m.columnSelectMode = true
			m.columnList.SetItems(m.getColumnListItems())
			m.columnList.SetSize(40, 8)
			return m, nil
		case key.Matches(msg, m.keys.Sort):
			m.views.ToggleSortOrder()
		case key.Matches(msg, m.keys.ToggleArchives):
			m.showArchives = !m.showArchives
			m.statusMessage = fmt.Sprintf("Archives: %v (Found %d notes)", m.showArchives, len(m.allNotes))
			m.updateViewsState()
		case key.Matches(msg, m.keys.ToggleGlobal):
			m.hideGlobal = !m.hideGlobal
			m.updateViewsState()
		case key.Matches(msg, m.keys.Delete):
			if m.lastKey == "d" { // This is the second 'd'
				pathsToDelete := m.views.GetTargetedNotePaths()
				if len(pathsToDelete) > 0 {
					prompt := fmt.Sprintf("Permanently delete %d note(s)? This cannot be undone.", len(pathsToDelete))
					m.confirmDialog.Activate(prompt)
				}
				m.lastKey = "" // Reset sequence
			} else {
				m.lastKey = "d" // This is the first 'd'
			}
		case key.Matches(msg, m.keys.Cut):
			paths := m.views.GetTargetedNotePaths()
			if len(paths) > 0 {
				m.clipboard = paths
				m.clipboardMode = "cut"
				cutPaths := make(map[string]struct{})
				for _, p := range paths {
					cutPaths[p] = struct{}{}
				}
				m.views.SetCutPaths(cutPaths)
				m.statusMessage = fmt.Sprintf("Cut %d note(s) to clipboard", len(paths))
			}
		case key.Matches(msg, m.keys.Copy):
			paths := m.views.GetTargetedNotePaths()
			if len(paths) > 0 {
				m.clipboard = paths
				m.clipboardMode = "copy"
				m.views.SetCutPaths(make(map[string]struct{})) // Clear cut visual
				m.statusMessage = fmt.Sprintf("Copied %d note(s) to clipboard", len(paths))
			}
		case key.Matches(msg, m.keys.Paste):
			if len(m.clipboard) > 0 {
				m.statusMessage = fmt.Sprintf("Pasting %d note(s)...", len(m.clipboard))
				return m, m.pasteNotesCmd()
			}
		case key.Matches(msg, m.keys.CreateNote):
			// Context-based creation: create at cursor, skip type picker
			m.isCreatingNote = true
			m.noteCreationMode = "context"
			m.noteCreationStep = 1 // Skip type picker, go straight to title
			m.noteCreationCursor = m.views.GetCursor()
			m.noteTitleInput.SetValue("")
			m.noteTitleInput.Focus()
			return m, textinput.Blink
		case key.Matches(msg, m.keys.CreateNoteInbox):
			// Inbox-style creation: show type picker, create in focused workspace or global
			m.isCreatingNote = true
			m.noteCreationMode = "inbox"
			m.noteCreationStep = 0 // Start with type picker
			m.noteCreationCursor = m.views.GetCursor()
			m.noteTitleInput.SetValue("")
			return m, nil
		case key.Matches(msg, m.keys.CreateNoteGlobal):
			// Global note creation: show type picker, always create in global
			m.isCreatingNote = true
			m.noteCreationMode = "global"
			m.noteCreationStep = 0 // Start with type picker
			m.noteCreationCursor = m.views.GetCursor()
			m.noteTitleInput.SetValue("")
			return m, nil
		case key.Matches(msg, m.keys.Rename):
			// Rename note: only works when cursor is on a note
			node := m.views.GetCurrentNode()
			if node != nil && node.IsNote {
				m.isRenamingNote = true
				m.noteToRename = node.Note
				m.renameInput.SetValue(node.Note.Title)
				m.renameInput.Focus()
				return m, textinput.Blink
			}
		case key.Matches(msg, m.keys.CreatePlan):
			node := m.views.GetCurrentNode()
			if node != nil && node.IsNote {
				// Sanitize the note title for the plan name
				planName := sanitizeForFilename(node.Note.Title)
				notePath := node.Note.Path

				// Show status message to user
				m.statusMessage = fmt.Sprintf("Promoting '%s' to plan '%s'...", node.Note.Title, planName)

				// Construct the flow plan init command
				cmd := exec.Command("flow", "plan", "init", planName,
					"--recipe", "chat",
					"--worktree",
					"--extract-all-from", notePath,
					"--note-ref", notePath,
					"--open-session",
				)

				// Execute the command - this will take over the terminal and open a tmux session
				// When complete, we'll quit the TUI
				return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
					if err != nil {
						// If there was an error, show it before quitting
						fmt.Fprintf(os.Stderr, "\nError promoting note to plan: %v\n", err)
					} else {
						// Success message
						fmt.Fprintf(os.Stderr, "\nSuccessfully promoted note to plan '%s'\n", planName)
					}
					return tea.Quit()
				})
			}
		case key.Matches(msg, m.keys.Archive):
			// Archive selected notes and/or plan groups
			noteCount, selectedNotes, selectedPlans := m.views.GetCounts()
			_ = noteCount // unused
			if selectedNotes > 0 || selectedPlans > 0 {
				var prompt string
				if selectedNotes > 0 && selectedPlans > 0 {
					prompt = fmt.Sprintf("Archive %d notes and %d plans?", selectedNotes, selectedPlans)
				} else if selectedPlans > 0 {
					prompt = fmt.Sprintf("Archive %d plans?", selectedPlans)
				} else {
					prompt = fmt.Sprintf("Archive %d notes?", selectedNotes)
				}
				m.confirmDialog.Activate(prompt)
			}
		case key.Matches(msg, m.keys.Confirm):
			if m.ecosystemPickerMode {
				node := m.views.GetCurrentNode()
				if node != nil && node.IsWorkspace && node.Workspace.IsEcosystem() {
					m.loadingCount++
					m.focusedWorkspace = node.Workspace
					m.ecosystemPickerMode = false
					m.focusChanged = true
					// Re-fetch notes for the selected ecosystem
					return m, tea.Batch(fetchFocusedNotesCmd(m.service, m.focusedWorkspace), m.spinner.Tick)
				}
			} else {
				var noteToOpen *models.Note
				node := m.views.GetCurrentNode()
				if node != nil {
					if node.IsNote {
						noteToOpen = node.Note
					} else if node.IsFoldable() {
						// Toggle fold on workspaces and groups
						m.views.ToggleFold()
						return m, nil
					}
				}
				if noteToOpen != nil {
					if os.Getenv("GROVE_NVIM_PLUGIN") == "true" {
						return m, func() tea.Msg {
							return editFileAndQuitMsg{filePath: noteToOpen.Path}
						}
					}

					// If in a tmux session, intelligently open based on context (popup vs normal)
					if os.Getenv("TMUX") != "" {
						return m, m.openInTmuxCmd(noteToOpen.Path)
					}

					return m, m.openInEditor(noteToOpen.Path)
				}
			}
		case key.Matches(msg, m.keys.Preview): // Tab
			m.previewFocused = !m.previewFocused
			return m, nil
		case key.Matches(msg, m.keys.TogglePreview): // v - toggle preview visibility
			m.previewVisible = !m.previewVisible
			if !m.previewVisible {
				m.previewFocused = false // Can't focus a hidden preview
				// Clear preview-related status message
				if strings.Contains(m.statusMessage, "Previewing") || strings.Contains(m.statusMessage, "Loading") {
					m.statusMessage = ""
				}
			}
			return m, nil
		case key.Matches(msg, m.keys.Back):
			if m.ecosystemPickerMode {
				m.ecosystemPickerMode = false
				m.updateViewsState()
				return m, nil
			}
			// If in cut mode, escape cancels the cut operation
			if m.clipboardMode == "cut" {
				m.clipboard = nil
				m.clipboardMode = ""
				m.views.SetCutPaths(make(map[string]struct{}))
				m.statusMessage = "Cut operation cancelled"
				return m, nil
			}
		default:
			// Reset lastKey for any other key press (for gg, dd and z* detection)
			if !key.Matches(msg, m.keys.GoToTop) && !key.Matches(msg, m.keys.FoldPrefix) && !key.Matches(msg, m.keys.Delete) {
				m.lastKey = ""
			}
		}
	}
	return m, nil
}

// buildDisplayTree constructs the hierarchical list of nodes for rendering.
// calculateRelativePath returns the absolute path for a note (shortened with ~ for home)
func calculateRelativePath(note *models.Note, workspacePathMap map[string]string, focusedWorkspace *workspace.WorkspaceNode) string {
	// Always use absolute path with ~ for home
	return shortenPath(note.Path)
}


// deleteSelectedNotesCmd creates a command to delete the selected notes.
func (m *Model) deleteSelectedNotesCmd() tea.Cmd {
	pathsToDelete := m.views.GetTargetedNotePaths()
	if len(pathsToDelete) == 0 {
		return nil
	}
	return func() tea.Msg {
		err := m.service.DeleteNotes(pathsToDelete)
		return notesDeletedMsg{
			deletedPaths: pathsToDelete,
			err:          err,
		}
	}
}

// pasteNotesCmd creates a command to paste notes from the clipboard.
func (m *Model) pasteNotesCmd() tea.Cmd {
	if len(m.clipboard) == 0 {
		return nil
	}

	// Determine destination from cursor
	var destWorkspace *workspace.WorkspaceNode
	var destGroup string

	node := m.views.GetCurrentNode()
	if node != nil {
		if node.IsWorkspace {
			destWorkspace = node.Workspace
			destGroup = "inbox" // Default group when pasting on a workspace
		} else if node.IsGroup {
			destWorkspace, _ = m.findWorkspaceNodeByName(node.WorkspaceName)
			destGroup = node.GroupName
		} else if node.IsNote {
			destWorkspace, _ = m.findWorkspaceNodeByName(node.Note.Workspace)
			destGroup = node.Note.Group
		}
	}

	if destWorkspace == nil {
		// Fallback to global if no context can be determined
		destWorkspace, _ = m.findWorkspaceNodeByName("global")
		destGroup = "inbox"
	}

	mode := m.clipboardMode
	paths := m.clipboard

	return func() tea.Msg {
		var newPaths []string
		var err error
		if mode == "copy" {
			newPaths, err = m.service.CopyNotes(paths, destWorkspace, destGroup)
		} else {
			newPaths, err = m.service.MoveNotes(paths, destWorkspace, destGroup)
		}
		return notesPastedMsg{
			pastedCount: len(paths),
			newPaths:    newPaths,
			err:         err,
		}
	}
}

func (m *Model) findWorkspaceNodeByName(name string) (*workspace.WorkspaceNode, bool) {
	for _, ws := range m.workspaces {
		if ws.Name == name {
			return ws, true
		}
	}
	return nil, false
}

// updateNoteCreation handles input when the note creation UI is active.
func (m *Model) updateNoteCreation(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.isCreatingNote = false
			m.noteTitleInput.Blur()
			return m, nil
		case "enter":
			if m.noteCreationStep == 0 { // From type picker to title
				m.noteCreationStep = 1
				m.noteTitleInput.Focus()
				return m, textinput.Blink
			} else { // From title to creating note
				return m, m.createNoteCmd()
			}
		}
	}

	if m.noteCreationStep == 0 {
		m.noteTypePicker, cmd = m.noteTypePicker.Update(msg)
	} else {
		m.noteTitleInput, cmd = m.noteTitleInput.Update(msg)
	}
	return m, cmd
}

// createNoteCmd creates a command to create a new note.
func (m *Model) createNoteCmd() tea.Cmd {
	title := m.noteTitleInput.Value()
	if title == "" {
		title = "Untitled Note"
	}

	var wsCtx *service.WorkspaceContext
	var noteType models.NoteType
	var err error

	if m.noteCreationMode == "context" {
		// Context-based: Create at the cursor position stored when creation started
		displayNodes := m.views.GetDisplayNodes()
		if m.noteCreationCursor < len(displayNodes) {
			node := displayNodes[m.noteCreationCursor]
			var wsPath string

			if node.IsWorkspace {
				wsPath = node.Workspace.Path
				noteType = "inbox" // Default to inbox for workspace
			} else if node.IsGroup {
				// Find workspace by name to get its path
				ws, found := m.findWorkspaceNodeByName(node.WorkspaceName)
				if found {
					wsPath = ws.Path
				}
				noteType = models.NoteType(node.GroupName)
			} else if node.IsNote {
				// Find workspace by name to get its path
				ws, found := m.findWorkspaceNodeByName(node.Note.Workspace)
				if found {
					wsPath = ws.Path
				}
				noteType = models.NoteType(node.Note.Group)
			}

			if wsPath != "" {
				wsCtx, err = m.service.GetWorkspaceContext(wsPath)
			}
		}

		if wsCtx == nil || err != nil {
			// Default to focused workspace or global
			if m.focusedWorkspace != nil {
				wsCtx, _ = m.service.GetWorkspaceContext(m.focusedWorkspace.Path)
				noteType = "inbox"
			} else {
				wsCtx, _ = m.service.GetWorkspaceContext("global")
				noteType = "inbox"
			}
		}
	} else if m.noteCreationMode == "inbox" {
		// Inbox mode: Use selected type from picker, create in focused workspace or global
		selectedType := m.noteTypePicker.SelectedItem().(noteTypeItem)
		noteType = models.NoteType(selectedType)

		if m.focusedWorkspace != nil {
			wsCtx, _ = m.service.GetWorkspaceContext(m.focusedWorkspace.Path)
		} else {
			wsCtx, _ = m.service.GetWorkspaceContext("global")
		}
	} else if m.noteCreationMode == "global" {
		// Global mode: Use selected type from picker, always create in global
		selectedType := m.noteTypePicker.SelectedItem().(noteTypeItem)
		noteType = models.NoteType(selectedType)
		wsCtx, _ = m.service.GetWorkspaceContext("global")
	}

	return func() tea.Msg {
		note, err := m.service.CreateNote(wsCtx, noteType, title, service.WithoutEditor())
		return noteCreatedMsg{note: note, err: err}
	}
}

// updateNoteRename handles input when the note rename UI is active.
func (m *Model) updateNoteRename(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.isRenamingNote = false
			m.renameInput.Blur()
			m.noteToRename = nil
			return m, nil
		case "enter":
			return m, m.renameNoteCmd()
		}
	}

	m.renameInput, cmd = m.renameInput.Update(msg)
	return m, cmd
}

// renameNoteCmd creates a command to rename a note.
func (m *Model) renameNoteCmd() tea.Cmd {
	if m.noteToRename == nil {
		return nil
	}

	newTitle := m.renameInput.Value()
	if newTitle == "" || newTitle == m.noteToRename.Title {
		// No change, just cancel
		return func() tea.Msg {
			return noteRenamedMsg{
				oldPath: m.noteToRename.Path,
				newPath: m.noteToRename.Path,
				err:     nil,
			}
		}
	}

	oldPath := m.noteToRename.Path

	return func() tea.Msg {
		newPath, err := m.service.RenameNote(oldPath, newTitle)
		return noteRenamedMsg{
			oldPath: oldPath,
			newPath: newPath,
			err:     err,
		}
	}
}

// archivePlanDirectory moves a plan directory to the .archive subdirectory
// and returns the paths of all notes that were in the plan directory
func (m *Model) archivePlanDirectory(ctx *service.WorkspaceContext, planGroup string) ([]string, error) {
	// planGroup is like "plans/my-plan"
	// We need to move it to "plans/.archive/my-plan"

	// Get the plans base directory
	plansBaseDir, err := m.service.GetNotebookLocator().GetPlansDir(ctx.NotebookContextWorkspace)
	if err != nil {
		return nil, fmt.Errorf("get plans directory: %w", err)
	}

	// Extract plan name from group (remove "plans/" prefix)
	planName := strings.TrimPrefix(planGroup, "plans/")

	// Source path: plans/<planName>
	sourcePath := filepath.Join(plansBaseDir, planName)

	// Collect paths of notes in this plan directory
	var notePaths []string
	for _, note := range m.allNotes {
		if strings.HasPrefix(note.Path, sourcePath+string(filepath.Separator)) {
			notePaths = append(notePaths, note.Path)
		}
	}

	// Create archive directory if it doesn't exist
	archiveDir := filepath.Join(plansBaseDir, ".archive")
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return nil, fmt.Errorf("create archive directory: %w", err)
	}

	// Destination path: plans/.archive/<planName>
	destPath := filepath.Join(archiveDir, planName)

	// Check if destination already exists, if so append timestamp
	if _, err := os.Stat(destPath); err == nil {
		// Destination exists, create unique name with timestamp
		timestamp := time.Now().Format("20060102150405")
		destPath = filepath.Join(archiveDir, fmt.Sprintf("%s-%s", planName, timestamp))
	}

	// Move the plan directory
	if err := os.Rename(sourcePath, destPath); err != nil {
		return nil, fmt.Errorf("move plan directory: %w", err)
	}

	return notePaths, nil
}

// archiveSelectedNotesCmd creates a command to archive the selected notes and plan groups
func (m *Model) archiveSelectedNotesCmd() tea.Cmd {
	return func() tea.Msg {
		// Group notes by workspace
		notesByWorkspace := make(map[string][]*models.Note)
		selectedNotes := m.views.GetSelected()
		selectedGroups := m.views.GetSelectedGroups()

		// Check if BOTH are empty
		if len(selectedNotes) == 0 && len(selectedGroups) == 0 {
			return notesArchivedMsg{
				archivedPaths: nil,
				archivedPlans: 0,
				err:           fmt.Errorf("no notes or plans selected"),
			}
		}

		// Populate notesByWorkspace from selected notes
		for _, note := range m.allNotes {
			if _, ok := selectedNotes[note.Path]; ok {
				notesByWorkspace[note.Workspace] = append(notesByWorkspace[note.Workspace], note)
			}
		}

		var archivedPaths []string
		var archivedPlans int
		var archiveErr error

		// Archive notes workspace by workspace
		for workspaceName, notes := range notesByWorkspace {
			// Get workspace context - need to resolve name to path
			var wsCtx *service.WorkspaceContext
			var err error
			if workspaceName == "global" {
				wsCtx, err = m.service.GetWorkspaceContext("global")
			} else {
				// Find the workspace node by name to get its path
				wsNode, found := m.findWorkspaceNodeByName(workspaceName)
				if !found {
					archiveErr = fmt.Errorf("workspace not found: %s", workspaceName)
					break
				}
				wsCtx, err = m.service.GetWorkspaceContext(wsNode.Path)
			}

			if err != nil {
				archiveErr = fmt.Errorf("failed to get workspace context for %s: %w", workspaceName, err)
				break
			}

			// Extract paths from notes
			paths := make([]string, len(notes))
			for i, note := range notes {
				paths[i] = note.Path
			}

			// Archive the notes
			if err := m.service.ArchiveNotes(wsCtx, paths); err != nil {
				archiveErr = fmt.Errorf("failed to archive notes in workspace %s: %w", workspaceName, err)
				break
			}

			archivedPaths = append(archivedPaths, paths...)
		}

		// Archive selected plan groups
		if archiveErr == nil && len(selectedGroups) > 0 {
			// Group plans by workspace
			plansByWorkspace := make(map[string][]string)
			for groupKey := range selectedGroups {
				parts := strings.SplitN(groupKey, ":", 2)
				if len(parts) == 2 {
					workspaceName := parts[0]
					groupName := parts[1]
					// Only archive plan groups (those starting with "plans/")
					if strings.HasPrefix(groupName, "plans/") {
						plansByWorkspace[workspaceName] = append(plansByWorkspace[workspaceName], groupName)
					}
				}
			}

			// Archive plans workspace by workspace
			if len(plansByWorkspace) == 0 && len(selectedGroups) > 0 {
				// Groups were selected but none were plans
				archiveErr = fmt.Errorf("selected groups are not plans (must start with 'plans/')")
			}

			for workspaceName, planNames := range plansByWorkspace {
				// Get workspace context - need to resolve name to path
				var wsCtx *service.WorkspaceContext
				var err error
				if workspaceName == "global" {
					wsCtx, err = m.service.GetWorkspaceContext("global")
				} else {
					// Find the workspace node by name to get its path
					wsNode, found := m.findWorkspaceNodeByName(workspaceName)
					if !found {
						archiveErr = fmt.Errorf("workspace not found: %s", workspaceName)
						break
					}
					wsCtx, err = m.service.GetWorkspaceContext(wsNode.Path)
				}

				if err != nil {
					archiveErr = fmt.Errorf("failed to get workspace context for %s: %w", workspaceName, err)
					break
				}

				for _, planName := range planNames {
					// Archive the plan directory and collect archived note paths
					planNotePaths, err := m.archivePlanDirectory(wsCtx, planName)
					if err != nil {
						archiveErr = fmt.Errorf("failed to archive plan %s in workspace %s: %w", planName, workspaceName, err)
						break
					}
					archivedPaths = append(archivedPaths, planNotePaths...)
					archivedPlans++
				}

				if archiveErr != nil {
					break
				}
			}
		}

		return notesArchivedMsg{
			archivedPaths: archivedPaths,
			archivedPlans: archivedPlans,
			err:           archiveErr,
		}
	}
}

// collapseChildWorkspaces collapses all child workspaces of the given parent
func (m *Model) collapseChildWorkspaces(parent *workspace.WorkspaceNode) {
	if parent == nil {
		return
	}

	collapsedNodes := m.views.GetCollapseState()
	normParent, _ := pathutil.NormalizeForLookup(parent.Path)
	for _, ws := range m.workspaces {
		// Skip the parent workspace itself
		isSame, _ := pathutil.ComparePaths(ws.Path, parent.Path)
		if isSame {
			continue
		}

		// Check if this workspace is a child of parent
		normWs, _ := pathutil.NormalizeForLookup(ws.Path)
		if strings.HasPrefix(normWs, normParent+string(filepath.Separator)) {
			wsNodeID := "ws:" + ws.Path
			collapsedNodes[wsNodeID] = true
		}
	}
	m.views.SetCollapseState(collapsedNodes)
}

// collapseAllWorkspaces collapses all top-level workspaces
func (m *Model) collapseAllWorkspaces() {
	collapsedNodes := m.views.GetCollapseState()
	for _, ws := range m.workspaces {
		wsNodeID := "ws:" + ws.Path
		collapsedNodes[wsNodeID] = true
	}
	m.views.SetCollapseState(collapsedNodes)
}

// setCollapseStateForFocus systematically sets the collapse state based on the current focus level
func (m *Model) setCollapseStateForFocus() {
	collapsedNodes := m.views.GetCollapseState()

	if m.focusedWorkspace == nil {
		// Global/top level view: collapse all workspaces for a clean overview
		m.collapseAllWorkspaces()
	} else if m.focusedWorkspace.IsEcosystem() {
		// Ecosystem focus: collapse ALL note groups and child workspaces
		// to show a clean view of the ecosystem structure
		// First, ensure the focused ecosystem itself is expanded
		wsNodeID := "ws:" + m.focusedWorkspace.Path
		delete(collapsedNodes, wsNodeID)

		// Collapse all child workspaces under this ecosystem
		m.collapseChildWorkspaces(m.focusedWorkspace)

		// Collapse ALL note groups within the focused ecosystem
		// (inbox, learn, quick, etc.) BUT keep "plans" parent expanded
		groupsSeen := make(map[string]bool)
		for _, note := range m.allNotes {
			if note.Workspace == m.focusedWorkspace.Name && !groupsSeen[note.Group] {
				groupNodeID := "grp:" + note.Group
				// Collapse individual plan directories but not other top-level groups
				if strings.HasPrefix(note.Group, "plans/") {
					collapsedNodes[groupNodeID] = true
				} else if note.Group != "plans" {
					// Collapse regular groups (inbox, learn, quick, etc.)
					collapsedNodes[groupNodeID] = true
				}
				groupsSeen[note.Group] = true
			}
		}
		// Ensure the "plans" parent group is expanded
		plansParentID := "grp:plans"
		delete(collapsedNodes, plansParentID)
	} else {
		// Leaf workspace focus: expand the workspace, collapse child workspaces and individual plans
		// Ensure the focused workspace itself is expanded
		wsNodeID := "ws:" + m.focusedWorkspace.Path
		delete(collapsedNodes, wsNodeID)

		// Collapse any child workspaces (if this workspace has children)
		m.collapseChildWorkspaces(m.focusedWorkspace)

		// Collapse individual plan directories within the focused workspace
		// but keep the "plans" parent group expanded
		for _, note := range m.allNotes {
			if note.Workspace == m.focusedWorkspace.Name {
				if strings.HasPrefix(note.Group, "plans/") {
					groupNodeID := "grp:" + note.Group
					collapsedNodes[groupNodeID] = true
				}
			}
		}
		// Ensure the "plans" parent group is expanded
		plansParentID := "grp:plans"
		delete(collapsedNodes, plansParentID)
	}

	m.views.SetCollapseState(collapsedNodes)
}

// applyGrepFilter applies the grep-based content filter to the display nodes
func (m *Model) applyGrepFilter() {
	m.views.ApplyGrepFilter()
}

// findAndApplyLinks iterates through the visible display nodes to find and link notes with their corresponding plans.
func (m *Model) findAndApplyLinks() {
	notesWithPlanRef := make(map[string]*views.DisplayNode)
	planNodes := make(map[string]*views.DisplayNode)
	displayNodes := m.views.GetDisplayNodes()

	// First pass: collect all notes with plan references and all plan nodes.
	for _, node := range displayNodes {
		// Reset any previous links
		node.LinkedNode = nil
		if node.IsNote && node.Note.PlanRef != "" {
			// Key by workspace + plan_ref for uniqueness
			key := fmt.Sprintf("%s:%s", node.Note.Workspace, node.Note.PlanRef)
			notesWithPlanRef[key] = node
		} else if node.IsPlan() {
			// Key by workspace + group_name
			key := fmt.Sprintf("%s:%s", node.WorkspaceName, node.GroupName)
			planNodes[key] = node
		}
	}

	// Second pass: connect the notes and plans.
	for key, noteNode := range notesWithPlanRef {
		if planNode, ok := planNodes[key]; ok {
			// Found a match, create the bidirectional link.
			noteNode.LinkedNode = planNode
			planNode.LinkedNode = noteNode
		}
	}
}
