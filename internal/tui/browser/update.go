package browser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/util/pathutil"
	"github.com/mattsolo1/grove-notebook/internal/tui/browser/components/confirm"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case confirm.ConfirmedMsg:
		// User confirmed the action in the dialog
		// We need to know which action was confirmed. A simple way is to check the prompt.
		if strings.Contains(m.confirmDialog.Prompt, "archive") {
			m.statusMessage = ""
			return m, m.archiveSelectedNotesCmd()
		}
		if strings.Contains(m.confirmDialog.Prompt, "delete") {
			m.statusMessage = ""
			return m, m.deleteSelectedNotesCmd()
		}
	case confirm.CancelledMsg:
		// User cancelled, just clear the status message
		m.statusMessage = ""
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
		m.columnList.SetSize(40, 8) // Set a reasonable size for the column picker
		return m, nil

	case workspacesLoadedMsg:
		m.workspaces = msg.workspaces
		// Ensure focused workspace is expanded when initially loaded
		if m.focusedWorkspace != nil {
			wsNodeID := "ws:" + m.focusedWorkspace.Path
			delete(m.collapsedNodes, wsNodeID)
		}
		m.buildDisplayTree()
		m.applyFilterAndSort()
		return m, nil

	case notesLoadedMsg:
		m.allNotes = msg.notes
		// Only reset collapse state if focus just changed
		if m.focusChanged {
			m.setCollapseStateForFocus()
			m.focusChanged = false
		}
		m.buildDisplayTree()
		m.applyFilterAndSort()
		return m, nil

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
		m.selected = make(map[string]struct{})
		m.selectedGroups = make(map[string]struct{})
		// Rebuild display
		m.buildDisplayTree()
		m.applyFilterAndSort()
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
			m.cutPaths = make(map[string]struct{})
		}
		m.statusMessage = fmt.Sprintf("Pasted %d note(s) successfully", msg.pastedCount)
		// Refresh notes to show the new locations
		if m.focusedWorkspace != nil {
			return m, fetchFocusedNotesCmd(m.service, m.focusedWorkspace)
		}
		return m, fetchAllNotesCmd(m.service)

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
		m.selected = make(map[string]struct{})
		m.selectedGroups = make(map[string]struct{})

		// Rebuild the display
		m.buildDisplayTree()
		m.applyFilterAndSort()

		if msg.archivedPlans > 0 {
			m.statusMessage = fmt.Sprintf("Archived %d note(s) and %d plan(s)", len(msg.archivedPaths), msg.archivedPlans)
		} else {
			m.statusMessage = fmt.Sprintf("Archived %d note(s)", len(msg.archivedPaths))
		}

		// Refresh notes to show the updated archive structure
		if m.focusedWorkspace != nil {
			return m, fetchFocusedNotesCmd(m.service, m.focusedWorkspace)
		}
		return m, fetchAllNotesCmd(m.service)

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
		if m.focusedWorkspace != nil {
			return m, fetchFocusedNotesCmd(m.service, m.focusedWorkspace)
		}
		return m, fetchAllNotesCmd(m.service)

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
		if m.focusedWorkspace != nil {
			return m, fetchFocusedNotesCmd(m.service, m.focusedWorkspace)
		}
		return m, fetchAllNotesCmd(m.service)

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
				m.applyFilterAndSort()
				return m, nil
			case key.Matches(msg, m.keys.Confirm): // Enter
				m.filterInput.Blur()
				return m, nil
			default:
				m.filterInput, cmd = m.filterInput.Update(msg)
				if m.isGrepping {
					m.applyGrepFilter()
				} else {
					m.applyFilterAndSort()
				}
				return m, cmd
			}
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.help.Toggle()
			return m, nil
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
				m.adjustScroll()
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.displayNodes)-1 {
				m.cursor++
				m.adjustScroll()
			}
		case key.Matches(msg, m.keys.PageUp):
			pageSize := m.getViewportHeight() / 2
			if pageSize < 1 {
				pageSize = 1
			}
			m.cursor -= pageSize
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.adjustScroll()
		case key.Matches(msg, m.keys.PageDown):
			pageSize := m.getViewportHeight() / 2
			if pageSize < 1 {
				pageSize = 1
			}
			m.cursor += pageSize
			if m.cursor >= len(m.displayNodes) {
				m.cursor = len(m.displayNodes) - 1
			}
			m.adjustScroll()
		case key.Matches(msg, m.keys.GoToTop):
			// Handle 'gg' - go to top when g is pressed twice
			if m.lastKey == "g" {
				m.cursor = 0
				m.adjustScroll()
				m.lastKey = ""
			} else {
				m.lastKey = "g"
			}
		case key.Matches(msg, m.keys.GoToBottom):
			if len(m.displayNodes) > 0 {
				m.cursor = len(m.displayNodes) - 1
				m.adjustScroll()
			}
		case key.Matches(msg, m.keys.Fold):
			m.closeFold()
		case key.Matches(msg, m.keys.Unfold):
			m.openFold()
		case key.Matches(msg, m.keys.FoldPrefix):
			// Handle 'z' prefix for fold commands
			m.lastKey = "z"
		case msg.String() == "a" && m.lastKey == "z":
			// za - toggle fold
			m.toggleFold()
			m.lastKey = ""
		case msg.String() == "A" && m.lastKey == "z":
			// zA - toggle fold recursively
			m.toggleFoldRecursive(m.cursor)
			m.lastKey = ""
		case msg.String() == "o" && m.lastKey == "z":
			// zo - open fold
			m.openFold()
			m.lastKey = ""
		case msg.String() == "O" && m.lastKey == "z":
			// zO - open fold recursively
			m.openFoldRecursive(m.cursor)
			m.lastKey = ""
		case msg.String() == "c" && m.lastKey == "z":
			// zc - close fold
			m.closeFold()
			m.lastKey = ""
		case msg.String() == "C" && m.lastKey == "z":
			// zC - close fold recursively
			m.closeFoldRecursive(m.cursor)
			m.lastKey = ""
		case msg.String() == "M" && m.lastKey == "z":
			// zM - close all folds
			m.closeAllFolds()
			m.lastKey = ""
		case msg.String() == "R" && m.lastKey == "z":
			// zR - open all folds
			m.openAllFolds()
			m.lastKey = ""
		case key.Matches(msg, m.keys.FocusEcosystem):
			if !m.ecosystemPickerMode {
				m.ecosystemPickerMode = true
				m.buildDisplayTree()
				m.applyFilterAndSort()
				m.cursor = 0
			}
		case key.Matches(msg, m.keys.ClearFocus):
			if m.focusedWorkspace != nil || m.ecosystemPickerMode {
				m.focusedWorkspace = nil
				m.ecosystemPickerMode = false
				m.focusChanged = true
				m.cursor = 0
				m.scrollOffset = 0
				// Re-fetch all notes for the global view
				return m, fetchAllNotesCmd(m.service)
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

				m.focusedWorkspace = parent // This can be nil if no parent is found, effectively clearing focus
				m.focusChanged = true
				m.cursor = 0
				// Re-fetch notes for the new focus level
				if m.focusedWorkspace != nil {
					return m, fetchFocusedNotesCmd(m.service, m.focusedWorkspace)
				} else {
					return m, fetchAllNotesCmd(m.service)
				}
			}
		case key.Matches(msg, m.keys.FocusSelected):
			if m.cursor < len(m.displayNodes) {
				node := m.displayNodes[m.cursor]
				if node.isWorkspace {
					m.focusedWorkspace = node.workspace
					m.ecosystemPickerMode = false // Focusing on a workspace exits picker mode
					m.focusChanged = true
					m.cursor = 0
					// Re-fetch notes for the newly focused workspace
					return m, fetchFocusedNotesCmd(m.service, m.focusedWorkspace)
				}
			}
		case key.Matches(msg, m.keys.ToggleView):
			if m.viewMode == treeView {
				m.viewMode = tableView
			} else {
				m.viewMode = treeView
			}
			m.cursor = 0
		case key.Matches(msg, m.keys.Search):
			m.isGrepping = false
			m.filterInput.SetValue("")
			m.filterInput.Placeholder = "Search notes..."
			m.filterInput.Focus()
			return m, textinput.Blink
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
			m.sortAscending = !m.sortAscending
			m.applyFilterAndSort()
		case key.Matches(msg, m.keys.ToggleArchives):
			m.showArchives = !m.showArchives
			m.statusMessage = fmt.Sprintf("Archives: %v (Found %d notes)", m.showArchives, len(m.allNotes))
			m.applyFilterAndSort()
		case key.Matches(msg, m.keys.ToggleGlobal):
			m.hideGlobal = !m.hideGlobal
			m.buildDisplayTree()
		case key.Matches(msg, m.keys.ToggleSelect):
			// Toggle selection for the current note or plan group
			if m.cursor < len(m.displayNodes) {
				node := m.displayNodes[m.cursor]
				if node.isNote {
					if _, ok := m.selected[node.note.Path]; ok {
						delete(m.selected, node.note.Path)
					} else {
						m.selected[node.note.Path] = struct{}{}
					}
				} else if node.isPlan() {
					// Allow selection of plan groups
					groupKey := m.getGroupKey(node)
					if _, ok := m.selectedGroups[groupKey]; ok {
						delete(m.selectedGroups, groupKey)
					} else {
						m.selectedGroups[groupKey] = struct{}{}
					}
				}
			}
		case key.Matches(msg, m.keys.SelectAll):
			// Select all visible notes
			for _, node := range m.displayNodes {
				if node.isNote {
					m.selected[node.note.Path] = struct{}{}
				}
			}
		case key.Matches(msg, m.keys.SelectNone):
			// Clear all selections
			m.selected = make(map[string]struct{})
			m.selectedGroups = make(map[string]struct{})
		case key.Matches(msg, m.keys.Delete):
			if m.lastKey == "d" { // This is the second 'd'
				pathsToDelete := m.getTargetedNotePaths()
				if len(pathsToDelete) > 0 {
					prompt := fmt.Sprintf("Permanently delete %d note(s)? This cannot be undone.", len(pathsToDelete))
					m.confirmDialog.Activate(prompt)
				}
				m.lastKey = "" // Reset sequence
			} else {
				m.lastKey = "d" // This is the first 'd'
			}
		case key.Matches(msg, m.keys.Cut):
			paths := m.getTargetedNotePaths()
			if len(paths) > 0 {
				m.clipboard = paths
				m.clipboardMode = "cut"
				m.cutPaths = make(map[string]struct{})
				for _, p := range paths {
					m.cutPaths[p] = struct{}{}
				}
				m.statusMessage = fmt.Sprintf("Cut %d note(s) to clipboard", len(paths))
			}
		case key.Matches(msg, m.keys.Copy):
			paths := m.getTargetedNotePaths()
			if len(paths) > 0 {
				m.clipboard = paths
				m.clipboardMode = "copy"
				m.cutPaths = make(map[string]struct{}) // Clear cut visual
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
			m.noteCreationCursor = m.cursor
			m.noteTitleInput.SetValue("")
			m.noteTitleInput.Focus()
			return m, textinput.Blink
		case key.Matches(msg, m.keys.CreateNoteInbox):
			// Inbox-style creation: show type picker, create in focused workspace or global
			m.isCreatingNote = true
			m.noteCreationMode = "inbox"
			m.noteCreationStep = 0 // Start with type picker
			m.noteCreationCursor = m.cursor
			m.noteTitleInput.SetValue("")
			return m, nil
		case key.Matches(msg, m.keys.CreateNoteGlobal):
			// Global note creation: show type picker, always create in global
			m.isCreatingNote = true
			m.noteCreationMode = "global"
			m.noteCreationStep = 0 // Start with type picker
			m.noteCreationCursor = m.cursor
			m.noteTitleInput.SetValue("")
			return m, nil
		case key.Matches(msg, m.keys.Rename):
			// Rename note: only works when cursor is on a note
			if m.cursor < len(m.displayNodes) {
				node := m.displayNodes[m.cursor]
				if node.isNote {
					m.isRenamingNote = true
					m.noteToRename = node.note
					m.renameInput.SetValue(node.note.Title)
					m.renameInput.Focus()
					return m, textinput.Blink
				}
			}
		case key.Matches(msg, m.keys.Archive):
			// Archive selected notes and/or plan groups
			totalNotes := len(m.selected)
			totalPlans := len(m.selectedGroups)
			if totalNotes > 0 || totalPlans > 0 {
				var prompt string
				if totalNotes > 0 && totalPlans > 0 {
					prompt = fmt.Sprintf("Archive %d notes and %d plans?", totalNotes, totalPlans)
				} else if totalPlans > 0 {
					prompt = fmt.Sprintf("Archive %d plans?", totalPlans)
				} else {
					prompt = fmt.Sprintf("Archive %d notes?", totalNotes)
				}
				m.confirmDialog.Activate(prompt)
			}
		case key.Matches(msg, m.keys.Confirm):
			if m.ecosystemPickerMode {
				if m.cursor < len(m.displayNodes) {
					selected := m.displayNodes[m.cursor]
					if selected.isWorkspace && selected.workspace.IsEcosystem() {
						m.focusedWorkspace = selected.workspace
						m.ecosystemPickerMode = false
						m.focusChanged = true
						m.cursor = 0
						// Re-fetch notes for the selected ecosystem
						return m, fetchFocusedNotesCmd(m.service, m.focusedWorkspace)
					}
				}
			} else {
				var noteToOpen *models.Note
				if m.cursor < len(m.displayNodes) {
					node := m.displayNodes[m.cursor]
					if node.isNote {
						noteToOpen = node.note
					} else if node.isFoldable() {
						// Toggle fold on workspaces and groups
						m.toggleFold()
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
		case key.Matches(msg, m.keys.Preview):
			if !m.ecosystemPickerMode {
				var noteToPreview *models.Note
				if m.cursor < len(m.displayNodes) {
					node := m.displayNodes[m.cursor]
					if node.isNote {
						noteToPreview = node.note
					}
				}
				if noteToPreview != nil {
					if os.Getenv("GROVE_NVIM_PLUGIN") == "true" {
						return m, func() tea.Msg {
							return previewFileMsg{filePath: noteToPreview.Path}
						}
					}

					// If in a tmux session, preview in split without switching focus
					if os.Getenv("TMUX") != "" {
						return m, m.previewInTmuxSplitCmd(noteToPreview.Path)
					}
				}
			}
		case key.Matches(msg, m.keys.Back):
			if m.ecosystemPickerMode {
				m.ecosystemPickerMode = false
				m.buildDisplayTree()
				return m, nil
			}
			// If in cut mode, escape cancels the cut operation
			if m.clipboardMode == "cut" {
				m.clipboard = nil
				m.clipboardMode = ""
				m.cutPaths = make(map[string]struct{})
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

func (m *Model) buildDisplayTree() {
	var nodes []*displayNode
	var workspacesToShow []*workspace.WorkspaceNode

	// Check if we should ignore collapsed state (when searching)
	hasSearchFilter := m.filterInput.Value() != "" && !m.isGrepping

	// 1. Filter workspaces based on focus mode
	var showUngroupedSection bool
	var ungroupedWorkspaces []*workspace.WorkspaceNode

	if m.ecosystemPickerMode {
		// In picker mode, show only top-level ecosystems and their eco-worktrees
		for _, ws := range m.workspaces {
			if ws.IsEcosystem() && ws.Depth == 0 {
				workspacesToShow = append(workspacesToShow, ws)
				// Also add eco-worktrees that are children of this ecosystem
				for _, child := range m.workspaces {
					if child.IsWorktree() && child.IsEcosystem() && strings.HasPrefix(child.Path, ws.Path+"/") && child.Depth == ws.Depth+1 {
						workspacesToShow = append(workspacesToShow, child)
					}
				}
			}
		}
	} else if m.focusedWorkspace != nil {
		var globalNode *workspace.WorkspaceNode
		for _, ws := range m.workspaces {
			// Save global separately
			if ws.Name == "global" {
				globalNode = ws
				continue
			}
			// Use case-insensitive path comparison
			isSame, _ := pathutil.ComparePaths(ws.Path, m.focusedWorkspace.Path)
			if isSame {
				workspacesToShow = append(workspacesToShow, ws)
				continue
			}
			// Check if it's a child workspace
			normFocused, _ := pathutil.NormalizeForLookup(m.focusedWorkspace.Path)
			normWs, _ := pathutil.NormalizeForLookup(ws.Path)
			if strings.HasPrefix(normWs, normFocused+string(filepath.Separator)) {
				workspacesToShow = append(workspacesToShow, ws)
			}
		}

		// Clear tree prefix on focused workspace (first item) to make it a sibling of global
		if len(workspacesToShow) > 0 {
			focusedCopy := *workspacesToShow[0]
			focusedCopy.TreePrefix = ""
			workspacesToShow[0] = &focusedCopy
		}

		// Prepend global at the front (unless hidden)
		if globalNode != nil && !m.hideGlobal {
			workspacesToShow = append([]*workspace.WorkspaceNode{globalNode}, workspacesToShow...)
		}
	} else {
		// Global view: partition into ecosystem workspaces and standalone workspaces
		for _, ws := range m.workspaces {
			// Skip global if hidden
			if ws.Name == "global" && m.hideGlobal {
				continue
			}
			// Check if this is a standalone (non-ecosystem) top-level workspace
			// Ungrouped workspaces are top-level, not ecosystems, and not our special "global" node.
			if ws.Depth == 0 && !ws.IsEcosystem() && ws.Name != "global" {
				ungroupedWorkspaces = append(ungroupedWorkspaces, ws)
			} else if ws.Depth == 0 || ws.IsEcosystem() {
				// Top-level ecosystems and their children, and the global node
				workspacesToShow = append(workspacesToShow, ws)
			} else {
				// Check if this workspace belongs to a standalone project
				belongsToStandalone := false
				for _, standalone := range ungroupedWorkspaces {
					if strings.HasPrefix(ws.Path, standalone.Path+"/") {
						ungroupedWorkspaces = append(ungroupedWorkspaces, ws)
						belongsToStandalone = true
						break
					}
				}
				if !belongsToStandalone {
					workspacesToShow = append(workspacesToShow, ws)
				}
			}
		}
		showUngroupedSection = len(ungroupedWorkspaces) > 0
	}

	// 2. Group notes by workspace path, then by group (directory)
	notesByWorkspace := make(map[string]map[string][]*models.Note)

	// Create a map of workspace names to their paths for relative path calculation
	workspacePathMap := make(map[string]string)
	for _, ws := range m.workspaces {
		workspacePathMap[ws.Name] = ws.Path
	}

	for _, note := range m.allNotes {
		if _, ok := notesByWorkspace[note.Workspace]; !ok {
			notesByWorkspace[note.Workspace] = make(map[string][]*models.Note)
		}
		notesByWorkspace[note.Workspace][note.Group] = append(notesByWorkspace[note.Workspace][note.Group], note)
	}

	// 3. Build the display node list and jump map
	m.jumpMap = make(map[rune]int)
	jumpCounter := '1'
	needsSeparator := false // Track if we need to add a separator before the next workspace

	for _, ws := range workspacesToShow {
		// Add separator between ecosystem's own notes and child workspaces
		if needsSeparator && m.focusedWorkspace != nil && m.focusedWorkspace.IsEcosystem() {
			isSame, _ := pathutil.ComparePaths(ws.Path, m.focusedWorkspace.Path)
			if !isSame {
				// This is a child workspace, add separator
				nodes = append(nodes, &displayNode{
					isSeparator: true,
					prefix:      "  ",
					depth:       0,
				})
				needsSeparator = false // Only add separator once
			}
		}
		// Skip worktrees - they never have their own notes
		if ws.IsWorktree() {
			continue
		}

		hasNotes := len(notesByWorkspace[ws.Name]) > 0
		// Always show ecosystem nodes at depth 0, even if they have no direct notes
		// (their children may have notes)
		// Also always show the global workspace
		if !hasNotes && m.focusedWorkspace == nil && ws.Depth > 0 && ws.Name != "global" {
			// In global view, only skip non-ecosystem workspaces that have no notes
			continue
		}

		// Add workspace node
		node := &displayNode{
			isWorkspace: true,
			workspace:   ws,
			prefix:      ws.TreePrefix,
			depth:       ws.Depth,
		}

		// Assign jump key for workspaces at depth <= 1
		if ws.Depth <= 1 && jumpCounter <= '9' {
			node.jumpKey = jumpCounter
			m.jumpMap[jumpCounter] = len(nodes)
			jumpCounter++
		}
		nodes = append(nodes, node)

		// In ecosystem picker mode, don't show notes - just workspaces
		if m.ecosystemPickerMode {
			continue
		}

		// Skip children if workspace is collapsed (unless searching)
		wsNodeID := node.nodeID()
		wsCollapsed := m.collapsedNodes[wsNodeID]
		if wsCollapsed && !hasSearchFilter {
			continue
		}

		if noteGroups, ok := notesByWorkspace[ws.Name]; ok {
			// Separate regular groups and archived subgroups
			// archiveSubgroups maps "parent" -> "child" -> notes
			// e.g., "plans" -> "test-plan" -> [notes in plans/.archive/test-plan]
			var regularGroups []string
			planGroups := make(map[string][]*models.Note)
			archiveSubgroups := make(map[string]map[string][]*models.Note)

			for name, notes := range noteGroups {
				// Check if this is an archived group - skip if archives are hidden
				isArchived := strings.Contains(name, "/.archive")
				if isArchived && !m.showArchives {
					continue
				}

				// Check if this matches pattern "<parent>/.archive/<child>"
				if strings.Contains(name, "/.archive/") {
					parts := strings.Split(name, "/.archive/")
					if len(parts) == 2 {
						parent := parts[0]
						child := parts[1]
						if archiveSubgroups[parent] == nil {
							archiveSubgroups[parent] = make(map[string][]*models.Note)
						}
						archiveSubgroups[parent][child] = notes
						continue
					}
				}

				// Check if this matches pattern "<parent>/.archive" (notes directly in .archive folder)
				if strings.HasSuffix(name, "/.archive") {
					parent := strings.TrimSuffix(name, "/.archive")
					if archiveSubgroups[parent] == nil {
						archiveSubgroups[parent] = make(map[string][]*models.Note)
					}
					// Use empty string as key to indicate notes directly in .archive
					archiveSubgroups[parent][""] = notes
					continue
				}

				// Handle plans grouping
				if strings.HasPrefix(name, "plans/") {
					planName := strings.TrimPrefix(name, "plans/")
					planGroups[planName] = notes
				} else {
					regularGroups = append(regularGroups, name)
				}
			}
			sort.Strings(regularGroups)

			// Check if we have plans to add a "plans" parent group
			hasPlans := len(planGroups) > 0 || len(archiveSubgroups["plans"]) > 0
			totalGroups := len(regularGroups)
			if hasPlans {
				totalGroups++
			}

			for i, groupName := range regularGroups {
				isLastGroup := i == len(regularGroups)-1 && !hasPlans
				notesInGroup := noteGroups[groupName]

				// Sort notes within the group
				sort.SliceStable(notesInGroup, func(i, j int) bool {
					if m.sortAscending {
						return notesInGroup[i].CreatedAt.Before(notesInGroup[j].CreatedAt)
					}
					return notesInGroup[i].CreatedAt.After(notesInGroup[j].CreatedAt)
				})

				// Calculate group prefix
				var groupPrefix strings.Builder
				indentPrefix := strings.ReplaceAll(ws.TreePrefix, "├─", "│ ")
				indentPrefix = strings.ReplaceAll(indentPrefix, "└─", "  ")
				groupPrefix.WriteString(indentPrefix)
				if ws.Depth > 0 || ws.TreePrefix != "" {
					groupPrefix.WriteString("  ")
				}
				if isLastGroup {
					groupPrefix.WriteString("└─ ")
				} else {
					groupPrefix.WriteString("├─ ")
				}

				// Add group node
				groupNode := &displayNode{
					isGroup:       true,
					groupName:     groupName,
					workspaceName: ws.Name,
					prefix:        groupPrefix.String(),
					depth:         ws.Depth + 1,
					childCount:    len(notesInGroup),
				}
				nodes = append(nodes, groupNode)

				// Skip notes if group is collapsed (unless searching)
				groupNodeID := groupNode.nodeID()
				if m.collapsedNodes[groupNodeID] && !hasSearchFilter {
					continue
				}

				// Check if this group has archived children
				hasArchives := len(archiveSubgroups[groupName]) > 0 && m.showArchives

				// Add note nodes
				for j, note := range notesInGroup {
					isLastNote := j == len(notesInGroup)-1 && !hasArchives
					var notePrefix strings.Builder
					noteIndent := strings.ReplaceAll(groupPrefix.String(), "├─", "│ ")
					noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
					notePrefix.WriteString(noteIndent)
					if isLastNote {
						notePrefix.WriteString("└─ ")
					} else {
						notePrefix.WriteString("├─ ")
					}
					nodes = append(nodes, &displayNode{
						isNote:       true,
						note:         note,
						prefix:       notePrefix.String(),
						depth:        ws.Depth + 2,
						relativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
					})
				}

				// Add .archive subgroup if this group has archived children
				if hasArchives {
					// Sort archived child names
					var archivedNames []string
					for name := range archiveSubgroups[groupName] {
						archivedNames = append(archivedNames, name)
					}
					sort.Strings(archivedNames)

					// Count total archived notes
					totalArchivedNotes := 0
					for _, notes := range archiveSubgroups[groupName] {
						totalArchivedNotes += len(notes)
					}

					// Calculate .archive prefix (last child under this group)
					var archivePrefix strings.Builder
					archiveIndent := strings.ReplaceAll(groupPrefix.String(), "├─", "│ ")
					archiveIndent = strings.ReplaceAll(archiveIndent, "└─", "  ")
					archivePrefix.WriteString(archiveIndent)
					archivePrefix.WriteString("└─ ")

					// Add .archive parent node
					archiveParentNode := &displayNode{
						isGroup:       true,
						groupName:     groupName + "/.archive",
						workspaceName: ws.Name,
						prefix:        archivePrefix.String(),
						depth:         ws.Depth + 2,
						childCount:    totalArchivedNotes,
					}
					nodes = append(nodes, archiveParentNode)

					// Check if .archive parent is collapsed
					archiveParentNodeID := archiveParentNode.nodeID()
					if !m.collapsedNodes[archiveParentNodeID] || hasSearchFilter {
						// Add individual archived children
						for pi, archivedName := range archivedNames {
							isLastArchived := pi == len(archivedNames)-1
							archivedNotes := archiveSubgroups[groupName][archivedName]

							// Sort notes within the archived child
							sort.SliceStable(archivedNotes, func(i, j int) bool {
								if m.sortAscending {
									return archivedNotes[i].CreatedAt.Before(archivedNotes[j].CreatedAt)
								}
								return archivedNotes[i].CreatedAt.After(archivedNotes[j].CreatedAt)
							})

							// If archivedName is empty, these are notes directly in .archive folder
							if archivedName == "" {
								// Add notes directly under .archive parent
								for ni, note := range archivedNotes {
									isLastNote := ni == len(archivedNotes)-1 && isLastArchived
									var notePrefix strings.Builder
									noteIndent := strings.ReplaceAll(archivePrefix.String(), "├─", "│ ")
									noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
									notePrefix.WriteString(noteIndent)
									if isLastNote {
										notePrefix.WriteString("└─ ")
									} else {
										notePrefix.WriteString("├─ ")
									}
									nodes = append(nodes, &displayNode{
										isNote:       true,
										note:         note,
										prefix:       notePrefix.String(),
										depth:        ws.Depth + 3,
										relativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
									})
								}
								continue
							}

							// Calculate archived child prefix
							var archivedPrefix strings.Builder
							archivedIndent := strings.ReplaceAll(archivePrefix.String(), "├─", "│ ")
							archivedIndent = strings.ReplaceAll(archivedIndent, "└─", "  ")
							archivedPrefix.WriteString(archivedIndent)
							if isLastArchived {
								archivedPrefix.WriteString("└─ ")
							} else {
								archivedPrefix.WriteString("├─ ")
							}

							// Add archived child node
							archivedChildNode := &displayNode{
								isGroup:       true,
								groupName:     groupName + "/.archive/" + archivedName,
								workspaceName: ws.Name,
								prefix:        archivedPrefix.String(),
								depth:         ws.Depth + 3,
								childCount:    len(archivedNotes),
							}
							nodes = append(nodes, archivedChildNode)

							// Check if archived child is collapsed
							archivedChildNodeID := archivedChildNode.nodeID()
							if !m.collapsedNodes[archivedChildNodeID] || hasSearchFilter {
								// Add notes within the archived child
								for ni, note := range archivedNotes {
									isLastArchivedNote := ni == len(archivedNotes)-1
									var archivedNotePrefix strings.Builder
									archivedNoteIndent := strings.ReplaceAll(archivedPrefix.String(), "├─", "│ ")
									archivedNoteIndent = strings.ReplaceAll(archivedIndent, "└─", "  ")
									archivedNotePrefix.WriteString(archivedNoteIndent)
									if isLastArchivedNote {
										archivedNotePrefix.WriteString("└─ ")
									} else {
										archivedNotePrefix.WriteString("├─ ")
									}
									nodes = append(nodes, &displayNode{
										isNote:       true,
										note:         note,
										prefix:       archivedNotePrefix.String(),
										depth:        ws.Depth + 4,
										relativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
									})
								}
							}
						}
					}
				}
			}

			// Add "plans" parent group if there are any plans
			if hasPlans {
				// Calculate plans parent prefix
				var plansPrefix strings.Builder
				indentPrefix := strings.ReplaceAll(ws.TreePrefix, "├─", "│ ")
				indentPrefix = strings.ReplaceAll(indentPrefix, "└─", "  ")
				plansPrefix.WriteString(indentPrefix)
				if ws.Depth > 0 || ws.TreePrefix != "" {
					plansPrefix.WriteString("  ")
				}
				plansPrefix.WriteString("└─ ") // Plans is always last

				// Add "plans" parent node
				plansParentNode := &displayNode{
					isGroup:       true,
					groupName:     "plans",
					workspaceName: ws.Name,
					prefix:        plansPrefix.String(),
					depth:         ws.Depth + 1,
					childCount:    len(planGroups), // Count of plans, not notes
				}
				nodes = append(nodes, plansParentNode)

				// Check if plans parent is collapsed (unless searching)
				plansParentNodeID := plansParentNode.nodeID()
				if !m.collapsedNodes[plansParentNodeID] || hasSearchFilter {
					// Sort plan names
					var planNames []string
					for planName := range planGroups {
						planNames = append(planNames, planName)
					}
					sort.Strings(planNames)

					// Add individual plan nodes
					for pi, planName := range planNames {
						isLastPlan := pi == len(planNames)-1
						planNotes := planGroups[planName]

						// Sort notes within the plan
						sort.SliceStable(planNotes, func(i, j int) bool {
							if m.sortAscending {
								return planNotes[i].CreatedAt.Before(planNotes[j].CreatedAt)
							}
							return planNotes[i].CreatedAt.After(planNotes[j].CreatedAt)
						})

						// Calculate plan prefix
						var planPrefix strings.Builder
						planIndent := strings.ReplaceAll(plansPrefix.String(), "├─", "│ ")
						planIndent = strings.ReplaceAll(planIndent, "└─", "  ")
						planPrefix.WriteString(planIndent)
						if isLastPlan {
							planPrefix.WriteString("└─ ")
						} else {
							planPrefix.WriteString("├─ ")
						}

						// Add plan node with status icon
						planNode := &displayNode{
							isGroup:       true,
							groupName:     "plans/" + planName, // Keep full path for isPlan() check
							workspaceName: ws.Name,
							prefix:        planPrefix.String(),
							depth:         ws.Depth + 2,
							childCount:    len(planNotes),
						}
						nodes = append(nodes, planNode)

						// Check if this plan is collapsed (unless searching)
						planNodeID := planNode.nodeID()
						if !m.collapsedNodes[planNodeID] || hasSearchFilter {
							// Add notes in this plan
							for ni, note := range planNotes {
								isLastNote := ni == len(planNotes)-1
								var notePrefix strings.Builder
								noteIndent := strings.ReplaceAll(planPrefix.String(), "├─", "│ ")
								noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
								notePrefix.WriteString(noteIndent)
								if isLastNote {
									notePrefix.WriteString("└─ ")
								} else {
									notePrefix.WriteString("├─ ")
								}
								nodes = append(nodes, &displayNode{
									isNote:       true,
									note:         note,
									prefix:       notePrefix.String(),
									depth:        ws.Depth + 3,
									relativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
								})
							}
						}
					}

					// Add .archive parent group if there are archived children
					if len(archiveSubgroups["plans"]) > 0 && m.showArchives {
						// Sort archived child names
						var archivedNames []string
						for name := range archiveSubgroups["plans"] {
							archivedNames = append(archivedNames, name)
						}
						sort.Strings(archivedNames)

						// Count total archived notes
						totalArchivedNotes := 0
						for _, notes := range archiveSubgroups["plans"] {
							totalArchivedNotes += len(notes)
						}

						// Calculate .archive prefix (last child under plans)
						var archivePrefix strings.Builder
						archiveIndent := strings.ReplaceAll(plansPrefix.String(), "├─", "│ ")
						archiveIndent = strings.ReplaceAll(archiveIndent, "└─", "  ")
						archivePrefix.WriteString(archiveIndent)
						archivePrefix.WriteString("└─ ")

						// Add .archive parent node
						archiveParentNode := &displayNode{
							isGroup:       true,
							groupName:     "plans/.archive",
							workspaceName: ws.Name,
							prefix:        archivePrefix.String(),
							depth:         ws.Depth + 2,
							childCount:    totalArchivedNotes,
						}
						nodes = append(nodes, archiveParentNode)

						// Check if .archive parent is collapsed
						archiveParentNodeID := archiveParentNode.nodeID()
						if !m.collapsedNodes[archiveParentNodeID] || hasSearchFilter {
							// Add individual archived children
							for pi, archivedName := range archivedNames {
								isLastArchived := pi == len(archivedNames)-1
								archivedNotes := archiveSubgroups["plans"][archivedName]

								// Sort notes within the archived child
								sort.SliceStable(archivedNotes, func(i, j int) bool {
									if m.sortAscending {
										return archivedNotes[i].CreatedAt.Before(archivedNotes[j].CreatedAt)
									}
									return archivedNotes[i].CreatedAt.After(archivedNotes[j].CreatedAt)
								})

								// If archivedName is empty, these are plans directly in .archive folder
								if archivedName == "" {
									// Add notes directly under .archive parent
									for ni, note := range archivedNotes {
										isLastNote := ni == len(archivedNotes)-1 && isLastArchived
										var notePrefix strings.Builder
										noteIndent := strings.ReplaceAll(archivePrefix.String(), "├─", "│ ")
										noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
										notePrefix.WriteString(noteIndent)
										if isLastNote {
											notePrefix.WriteString("└─ ")
										} else {
											notePrefix.WriteString("├─ ")
										}
										nodes = append(nodes, &displayNode{
											isNote:       true,
											note:         note,
											prefix:       notePrefix.String(),
											depth:        ws.Depth + 3,
											relativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
										})
									}
									continue
								}

								// Calculate archived child prefix
								var archivedPrefix strings.Builder
								archivedIndent := strings.ReplaceAll(archivePrefix.String(), "├─", "│ ")
								archivedIndent = strings.ReplaceAll(archivedIndent, "└─", "  ")
								archivedPrefix.WriteString(archivedIndent)
								if isLastArchived {
									archivedPrefix.WriteString("└─ ")
								} else {
									archivedPrefix.WriteString("├─ ")
								}

								// Add archived child node
								archivedNode := &displayNode{
									isGroup:       true,
									groupName:     "plans/.archive/" + archivedName,
									workspaceName: ws.Name,
									prefix:        archivedPrefix.String(),
									depth:         ws.Depth + 3,
									childCount:    len(archivedNotes),
								}
								nodes = append(nodes, archivedNode)

								// Check if this archived child is collapsed
								archivedNodeID := archivedNode.nodeID()
								if !m.collapsedNodes[archivedNodeID] || hasSearchFilter {
									// Add notes in this archived child
									for ni, note := range archivedNotes {
										isLastNote := ni == len(archivedNotes)-1
										var notePrefix strings.Builder
										noteIndent := strings.ReplaceAll(archivedPrefix.String(), "├─", "│ ")
										noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
										notePrefix.WriteString(noteIndent)
										if isLastNote {
											notePrefix.WriteString("└─ ")
										} else {
											notePrefix.WriteString("├─ ")
										}
										nodes = append(nodes, &displayNode{
											isNote:       true,
											note:         note,
											prefix:       notePrefix.String(),
											depth:        ws.Depth + 4,
											relativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
										})
									}
								}
							}
						}
					}
				}
			}
		}

		// Mark that we need a separator before child workspaces
		// This is set after rendering the focused ecosystem's own note groups
		// Only show separator if the ecosystem is expanded
		if m.focusedWorkspace != nil && m.focusedWorkspace.IsEcosystem() && !wsCollapsed {
			isSame, _ := pathutil.ComparePaths(ws.Path, m.focusedWorkspace.Path)
			if isSame {
				needsSeparator = true
			}
		}
	}

	// 4. Add "ungrouped" section if there are standalone workspaces
	if showUngroupedSection {
		ungroupedNode := &displayNode{
			isGroup:   true,
			groupName: "ungrouped",
			prefix:    "",
			depth:     0,
		}
		nodes = append(nodes, ungroupedNode)

		// Check if ungrouped section is collapsed (unless searching)
		if !m.collapsedNodes[ungroupedNode.nodeID()] || hasSearchFilter {
			// Render each ungrouped workspace
			for i, ws := range ungroupedWorkspaces {
				// Skip worktrees
				if ws.IsWorktree() {
					continue
				}

				hasNotes := len(notesByWorkspace[ws.Name]) > 0
				if !hasNotes && ws.Depth > 0 {
					continue
				}

				// Adjust tree prefix to be indented under "Ungrouped"
				adjustedPrefix := "  " + ws.TreePrefix
				// If this is a depth-0 workspace, give it proper indentation
				if ws.Depth == 0 {
					// Check if this is the last ungrouped workspace
					isLast := i == len(ungroupedWorkspaces)-1
					if isLast {
						adjustedPrefix = "  └─ "
					} else {
						adjustedPrefix = "  ├─ "
					}
				}

				// Add workspace node
				node := &displayNode{
					isWorkspace: true,
					workspace:   ws,
					prefix:      adjustedPrefix,
					depth:       ws.Depth + 1, // Increase depth since it's under "Ungrouped"
				}

				// Assign jump key for ungrouped workspaces
				if jumpCounter <= '9' {
					node.jumpKey = jumpCounter
					m.jumpMap[jumpCounter] = len(nodes)
					jumpCounter++
				}
				nodes = append(nodes, node)

				// Skip children if workspace is collapsed (unless searching)
				wsNodeID := node.nodeID()
				if m.collapsedNodes[wsNodeID] && !hasSearchFilter {
					continue
				}

				// Render notes for this ungrouped workspace
				if noteGroups, ok := notesByWorkspace[ws.Name]; ok {
					// Separate regular groups and archived subgroups
					var regularGroups []string
					planGroups := make(map[string][]*models.Note)
					archiveSubgroups := make(map[string]map[string][]*models.Note)

					for name, notes := range noteGroups {
						// Check if this is an archived group - skip if archives are hidden
						isArchived := strings.Contains(name, "/.archive")
						if isArchived && !m.showArchives {
							continue
						}

						// Check if this matches pattern "<parent>/.archive/<child>"
						if strings.Contains(name, "/.archive/") {
							parts := strings.Split(name, "/.archive/")
							if len(parts) == 2 {
								parent := parts[0]
								child := parts[1]
								if archiveSubgroups[parent] == nil {
									archiveSubgroups[parent] = make(map[string][]*models.Note)
								}
								archiveSubgroups[parent][child] = notes
								continue
							}
						}

						// Handle plans grouping
						if strings.HasPrefix(name, "plans/") {
							planName := strings.TrimPrefix(name, "plans/")
							planGroups[planName] = notes
						} else {
							regularGroups = append(regularGroups, name)
						}
					}
					sort.Strings(regularGroups)

					hasPlans := len(planGroups) > 0 || len(archiveSubgroups["plans"]) > 0

					for i, groupName := range regularGroups {
						isLastGroup := i == len(regularGroups)-1 && !hasPlans
						notesInGroup := noteGroups[groupName]

						// Sort notes in group
						sort.SliceStable(notesInGroup, func(i, j int) bool {
							if m.sortAscending {
								return notesInGroup[i].CreatedAt.Before(notesInGroup[j].CreatedAt)
							}
							return notesInGroup[i].CreatedAt.After(notesInGroup[j].CreatedAt)
						})

						// Calculate group prefix (with extra indentation for ungrouped)
						var groupPrefix strings.Builder
						indentPrefix := strings.ReplaceAll(adjustedPrefix, "├─", "│ ")
						indentPrefix = strings.ReplaceAll(indentPrefix, "└─", "  ")
						groupPrefix.WriteString(indentPrefix)
						groupPrefix.WriteString("  ")
						if isLastGroup {
							groupPrefix.WriteString("└─ ")
						} else {
							groupPrefix.WriteString("├─ ")
						}

						// Add group node
						groupNode := &displayNode{
							isGroup:       true,
							groupName:     groupName,
							workspaceName: ws.Name,
							prefix:        groupPrefix.String(),
							depth:         ws.Depth + 2,
						}
						nodes = append(nodes, groupNode)

						// Skip notes if group is collapsed (unless searching)
						groupNodeID := groupNode.nodeID()
						if m.collapsedNodes[groupNodeID] && !hasSearchFilter {
							continue
						}

						// Add note nodes
						for j, note := range notesInGroup {
							isLastNote := j == len(notesInGroup)-1
							var notePrefix strings.Builder
							noteIndent := strings.ReplaceAll(groupPrefix.String(), "├─", "│ ")
							noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
							notePrefix.WriteString(noteIndent)
							if isLastNote {
								notePrefix.WriteString("└─ ")
							} else {
								notePrefix.WriteString("├─ ")
							}
							nodes = append(nodes, &displayNode{
								isNote:       true,
								note:         note,
								prefix:       notePrefix.String(),
								depth:        ws.Depth + 3,
								relativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
							})
						}
					}

					// Add "plans" parent group if there are any plans
					if hasPlans {
						var plansPrefix strings.Builder
						indentPrefix := strings.ReplaceAll(adjustedPrefix, "├─", "│ ")
						indentPrefix = strings.ReplaceAll(indentPrefix, "└─", "  ")
						plansPrefix.WriteString(indentPrefix)
						plansPrefix.WriteString("  ")
						plansPrefix.WriteString("└─ ")

						plansParentNode := &displayNode{
							isGroup:       true,
							groupName:     "plans",
							workspaceName: ws.Name,
							prefix:        plansPrefix.String(),
							depth:         ws.Depth + 2,
						}
						nodes = append(nodes, plansParentNode)

						plansParentNodeID := plansParentNode.nodeID()
						if !m.collapsedNodes[plansParentNodeID] || hasSearchFilter {
							var planNames []string
							for planName := range planGroups {
								planNames = append(planNames, planName)
							}
							sort.Strings(planNames)

							for pi, planName := range planNames {
								isLastPlan := pi == len(planNames)-1
								planNotes := planGroups[planName]

								// Sort notes in plan
								sort.SliceStable(planNotes, func(i, j int) bool {
									if m.sortAscending {
										return planNotes[i].CreatedAt.Before(planNotes[j].CreatedAt)
									}
									return planNotes[i].CreatedAt.After(planNotes[j].CreatedAt)
								})

								var planPrefix strings.Builder
								planIndent := strings.ReplaceAll(plansPrefix.String(), "├─", "│ ")
								planIndent = strings.ReplaceAll(planIndent, "└─", "  ")
								planPrefix.WriteString(planIndent)
								if isLastPlan {
									planPrefix.WriteString("└─ ")
								} else {
									planPrefix.WriteString("├─ ")
								}

								planNode := &displayNode{
									isGroup:       true,
									groupName:     "plans/" + planName,
									workspaceName: ws.Name,
									prefix:        planPrefix.String(),
									depth:         ws.Depth + 3,
								}
								nodes = append(nodes, planNode)

								planNodeID := planNode.nodeID()
								if !m.collapsedNodes[planNodeID] || hasSearchFilter {
									for ni, note := range planNotes {
										isLastNote := ni == len(planNotes)-1
										var notePrefix strings.Builder
										noteIndent := strings.ReplaceAll(planPrefix.String(), "├─", "│ ")
										noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
										notePrefix.WriteString(noteIndent)
										if isLastNote {
											notePrefix.WriteString("└─ ")
										} else {
											notePrefix.WriteString("├─ ")
										}
										nodes = append(nodes, &displayNode{
											isNote:       true,
											note:         note,
											prefix:       notePrefix.String(),
											depth:        ws.Depth + 4,
											relativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
										})
									}
								}
							}

							// Add .archive parent group if there are archived children
							if len(archiveSubgroups["plans"]) > 0 && m.showArchives {
								// Sort archived child names
								var archivedNames []string
								for name := range archiveSubgroups["plans"] {
									archivedNames = append(archivedNames, name)
								}
								sort.Strings(archivedNames)

								// Count total archived notes
								totalArchivedNotes := 0
								for _, notes := range archiveSubgroups["plans"] {
									totalArchivedNotes += len(notes)
								}

								// Calculate .archive prefix (last child under plans)
								var archivePrefix strings.Builder
								archiveIndent := strings.ReplaceAll(plansPrefix.String(), "├─", "│ ")
								archiveIndent = strings.ReplaceAll(archiveIndent, "└─", "  ")
								archivePrefix.WriteString(archiveIndent)
								archivePrefix.WriteString("└─ ")

								// Add .archive parent node
								archiveParentNode := &displayNode{
									isGroup:       true,
									groupName:     "plans/.archive",
									workspaceName: ws.Name,
									prefix:        archivePrefix.String(),
									depth:         ws.Depth + 3,
									childCount:    totalArchivedNotes,
								}
								nodes = append(nodes, archiveParentNode)

								// Check if .archive parent is collapsed
								archiveParentNodeID := archiveParentNode.nodeID()
								if !m.collapsedNodes[archiveParentNodeID] || hasSearchFilter {
									// Add individual archived children
									for pi, archivedName := range archivedNames {
										isLastArchived := pi == len(archivedNames)-1
										archivedNotes := archiveSubgroups["plans"][archivedName]

										// Sort notes within the archived child
										sort.SliceStable(archivedNotes, func(i, j int) bool {
											if m.sortAscending {
												return archivedNotes[i].CreatedAt.Before(archivedNotes[j].CreatedAt)
											}
											return archivedNotes[i].CreatedAt.After(archivedNotes[j].CreatedAt)
										})

										// If archivedName is empty, these are plans directly in .archive folder
										if archivedName == "" {
											// Add notes directly under .archive parent
											for ni, note := range archivedNotes {
												isLastNote := ni == len(archivedNotes)-1 && isLastArchived
												var notePrefix strings.Builder
												noteIndent := strings.ReplaceAll(archivePrefix.String(), "├─", "│ ")
												noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
												notePrefix.WriteString(noteIndent)
												if isLastNote {
													notePrefix.WriteString("└─ ")
												} else {
													notePrefix.WriteString("├─ ")
												}
												nodes = append(nodes, &displayNode{
													isNote:       true,
													note:         note,
													prefix:       notePrefix.String(),
													depth:        ws.Depth + 4,
													relativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
												})
											}
											continue
										}

										// Calculate archived child prefix
										var archivedPrefix strings.Builder
										archivedIndent := strings.ReplaceAll(archivePrefix.String(), "├─", "│ ")
										archivedIndent = strings.ReplaceAll(archivedIndent, "└─", "  ")
										archivedPrefix.WriteString(archivedIndent)
										if isLastArchived {
											archivedPrefix.WriteString("└─ ")
										} else {
											archivedPrefix.WriteString("├─ ")
										}

										// Add archived child node
										archivedChildNode := &displayNode{
											isGroup:       true,
											groupName:     "plans/.archive/" + archivedName,
											workspaceName: ws.Name,
											prefix:        archivedPrefix.String(),
											depth:         ws.Depth + 4,
											childCount:    len(archivedNotes),
										}
										nodes = append(nodes, archivedChildNode)

										// Check if archived child is collapsed
										archivedChildNodeID := archivedChildNode.nodeID()
										if !m.collapsedNodes[archivedChildNodeID] || hasSearchFilter {
											// Add notes within the archived child
											for ni, note := range archivedNotes {
												isLastNote := ni == len(archivedNotes)-1
												var notePrefix strings.Builder
												noteIndent := strings.ReplaceAll(archivedPrefix.String(), "├─", "│ ")
												noteIndent = strings.ReplaceAll(noteIndent, "└─", "  ")
												notePrefix.WriteString(noteIndent)
												if isLastNote {
													notePrefix.WriteString("└─ ")
												} else {
													notePrefix.WriteString("├─ ")
												}
												nodes = append(nodes, &displayNode{
													isNote:       true,
													note:         note,
													prefix:       notePrefix.String(),
													depth:        ws.Depth + 5,
													relativePath: calculateRelativePath(note, workspacePathMap, m.focusedWorkspace),
												})
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	m.displayNodes = nodes
	m.clampCursor()
}

// applyGrepFilter performs a content search and filters the tree view.
func (m *Model) applyGrepFilter() {
	query := m.filterInput.Value()

	if query == "" {
		// Restore the full tree with original collapsed state
		m.buildDisplayTree()
		m.statusMessage = ""
		return
	}

	// Use ripgrep to search actual file content
	// Build a map of all note paths for quick lookup
	notePathsMap := make(map[string]bool)
	for _, note := range m.allNotes {
		notePathsMap[note.Path] = true
	}

	// Use NotebookLocator to properly get notebook directories for each workspace
	// Build a map of workspace name -> workspace node for quick lookup
	workspaceMap := make(map[string]*workspace.WorkspaceNode)
	for _, ws := range m.workspaces {
		workspaceMap[ws.Name] = ws
	}

	// Get unique workspace names from notes
	uniqueWorkspaces := make(map[string]bool)
	for _, note := range m.allNotes {
		uniqueWorkspaces[note.Workspace] = true
	}

	// For each workspace, get its notebook directory using NotebookLocator
	searchDirs := make(map[string]bool)
	locator := m.service.GetNotebookLocator()

	for wsName := range uniqueWorkspaces {
		ws, ok := workspaceMap[wsName]
		if !ok || ws == nil {
			continue
		}

		// Get the notes directory for this workspace (we'll use "inbox" as a sample)
		// Then take the parent directory to get the workspace root
		notesDir, err := locator.GetNotesDir(ws, "inbox")
		if err != nil || notesDir == "" {
			continue
		}

		// The notes directory is something like /path/to/nb/repos/workspace/branch/current
		// We want to search at the branch level: /path/to/nb/repos/workspace/branch
		workspaceRoot := filepath.Dir(notesDir)
		searchDirs[workspaceRoot] = true
	}

	resultPaths := make(map[string]bool)
	totalRgFiles := 0

	// Run ripgrep once with all directories (much faster than multiple invocations)
	if len(searchDirs) > 0 {
		// Build args: rg -l --type md -i query dir1 dir2 dir3...
		args := []string{
			"-l",           // files-with-matches
			"--type", "md", // markdown only
			"-i", // case-insensitive
			query,
		}
		for dir := range searchDirs {
			args = append(args, dir)
		}

		cmd := exec.Command("rg", args...)
		output, err := cmd.Output()

		if err != nil {
			// rg returns exit code 1 when no matches found, which is not an error
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 1 {
				// Some other error occurred
				m.statusMessage = fmt.Sprintf("ripgrep error: %v", err)
			}
		}

		if len(output) > 0 {
			// Parse output - one file path per line
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" {
					totalRgFiles++
					// Only include files that are in our note list
					if notePathsMap[line] {
						resultPaths[line] = true
					}
				}
			}
		}
	}

	m.statusMessage = fmt.Sprintf("Found %d matching notes", len(resultPaths))

	// Temporarily expand all nodes to show grep results
	savedCollapsed := m.collapsedNodes
	m.collapsedNodes = make(map[string]bool)

	// Filter the display tree to show only matches and their parents
	m.filterDisplayTreeByPaths(resultPaths)

	// Keep the tree expanded for grep results
	// (savedCollapsed will be restored when exiting grep mode)
	_ = savedCollapsed
}

// filterDisplayTreeByPaths filters the tree view to show only nodes whose paths are in the provided map,
// along with their parent nodes.
func (m *Model) filterDisplayTreeByPaths(pathsToKeep map[string]bool) {
	// Rebuild the full tree (already expanded by caller)
	m.buildDisplayTree()
	fullTree := m.displayNodes

	// Normalize all paths to keep for case-insensitive comparison
	normalizedPaths := make(map[string]bool)
	for path := range pathsToKeep {
		normalized, err := pathutil.NormalizeForLookup(path)
		if err == nil {
			normalizedPaths[normalized] = true
		}
	}

	nodesToKeep := make(map[int]bool)
	parentMap := make(map[int]int)
	lastNodeAtDepth := make(map[int]int)

	// First pass: build parent map
	for i, node := range fullTree {
		if node.depth > 0 {
			if parentIndex, ok := lastNodeAtDepth[node.depth-1]; ok {
				parentMap[i] = parentIndex
			}
		}
		lastNodeAtDepth[node.depth] = i
	}

	// Second pass: mark nodes to keep
	for i, node := range fullTree {
		if node.isNote {
			// Try normalized path comparison
			normalizedNotePath, err := pathutil.NormalizeForLookup(node.note.Path)
			if err == nil && normalizedPaths[normalizedNotePath] {
				// Mark this note and all its parents to be kept
				curr := i
				for {
					nodesToKeep[curr] = true
					parentIndex, ok := parentMap[curr]
					if !ok {
						break // No more parents
					}
					curr = parentIndex
				}
			}
		}
	}

	// Third pass: build the filtered tree
	var filteredTree []*displayNode
	for i, node := range fullTree {
		if nodesToKeep[i] {
			filteredTree = append(filteredTree, node)
		}
	}

	m.displayNodes = filteredTree
	m.clampCursor()

	// Don't overwrite status message - let the caller handle it
}

// clampCursor ensures the cursor is within the valid range of display nodes.
func (m *Model) clampCursor() {
	if m.cursor >= len(m.displayNodes) {
		if len(m.displayNodes) > 0 {
			m.cursor = len(m.displayNodes) - 1
		} else {
			m.cursor = 0
		}
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

// adjustScroll ensures the cursor is visible in the viewport.
func (m *Model) adjustScroll() {
	viewportHeight := m.getViewportHeight()
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	} else if m.cursor >= m.scrollOffset+viewportHeight {
		m.scrollOffset = m.cursor - viewportHeight + 1
	}
	// Ensure scrollOffset never goes negative
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// applyFilterAndSort filters and sorts notes for both table and tree views.
func (m *Model) applyFilterAndSort() {
	// Rebuild the tree which now includes sorting logic
	m.buildDisplayTree()

	// Apply text filter if present
	if m.filterInput.Value() != "" {
		m.filterDisplayTree()
	}
}

// filterDisplayTree filters the tree view, preserving parent nodes of matches.
func (m *Model) filterDisplayTree() {
	filter := strings.ToLower(m.filterInput.Value())
	if filter == "" {
		return // No filter to apply
	}

	fullTree := m.displayNodes
	nodesToKeep := make(map[int]bool)
	parentMap := make(map[int]int)
	lastNodeAtDepth := make(map[int]int)

	// First pass: build parent map
	for i, node := range fullTree {
		if node.depth > 0 {
			if parentIndex, ok := lastNodeAtDepth[node.depth-1]; ok {
				parentMap[i] = parentIndex
			}
		}
		lastNodeAtDepth[node.depth] = i
	}

	// Second pass: mark nodes to keep
	for i, node := range fullTree {
		match := false

		if node.isNote {
			// Search only in note title and type
			match = strings.Contains(strings.ToLower(node.note.Title), filter) ||
				strings.Contains(strings.ToLower(string(node.note.Type)), filter)
		} else if node.isGroup {
			// Search in group/plan names (strip "plans/" prefix for matching)
			displayName := node.groupName
			if node.isPlan() {
				displayName = strings.TrimPrefix(displayName, "plans/")
			}
			match = strings.Contains(strings.ToLower(displayName), filter)
		}

		if match {
			// Mark this node and all its parents to be kept
			curr := i
			for {
				nodesToKeep[curr] = true
				parentIndex, ok := parentMap[curr]
				if !ok {
					break // No more parents
				}
				curr = parentIndex
			}
		}
	}

	// Third pass: build the filtered tree
	var filteredTree []*displayNode
	for i, node := range fullTree {
		if nodesToKeep[i] {
			filteredTree = append(filteredTree, node)
		}
	}

	m.displayNodes = filteredTree
	m.clampCursor()
}

// toggleFold toggles the fold state of the node under the cursor
func (m *Model) toggleFold() {
	if m.cursor >= len(m.displayNodes) {
		return
	}
	node := m.displayNodes[m.cursor]
	if !node.isFoldable() {
		return
	}
	nodeID := node.nodeID()
	if m.collapsedNodes[nodeID] {
		delete(m.collapsedNodes, nodeID)
	} else {
		m.collapsedNodes[nodeID] = true
	}
	m.buildDisplayTree()
}

// openFold opens the fold of the node under the cursor
func (m *Model) openFold() {
	if m.cursor >= len(m.displayNodes) {
		return
	}
	node := m.displayNodes[m.cursor]
	if !node.isFoldable() {
		return
	}
	delete(m.collapsedNodes, node.nodeID())
	m.buildDisplayTree()
}

// closeFold closes the fold of the node under the cursor
func (m *Model) closeFold() {
	if m.cursor >= len(m.displayNodes) {
		return
	}
	node := m.displayNodes[m.cursor]
	if !node.isFoldable() {
		return
	}
	m.collapsedNodes[node.nodeID()] = true
	m.buildDisplayTree()
}

// closeAllFolds closes all foldable nodes
func (m *Model) closeAllFolds() {
	for _, node := range m.displayNodes {
		if node.isFoldable() {
			m.collapsedNodes[node.nodeID()] = true
		}
	}
	m.buildDisplayTree()
}

// openAllFolds opens all foldable nodes
func (m *Model) openAllFolds() {
	m.collapsedNodes = make(map[string]bool)
	m.buildDisplayTree()
}

// closeFoldRecursive recursively closes the fold under the cursor and all nested folds within it
func (m *Model) closeFoldRecursive(cursorIndex int) {
	if cursorIndex >= len(m.displayNodes) {
		return
	}
	node := m.displayNodes[cursorIndex]
	if !node.isFoldable() {
		return
	}

	startDepth := node.depth
	m.collapsedNodes[node.nodeID()] = true

	// Iterate through subsequent nodes to find and collapse all children
	for i := cursorIndex + 1; i < len(m.displayNodes); i++ {
		childNode := m.displayNodes[i]
		if childNode.depth <= startDepth {
			// We've exited the current node's subtree
			break
		}
		if childNode.isFoldable() {
			m.collapsedNodes[childNode.nodeID()] = true
		}
	}
	m.buildDisplayTree()
}

// openFoldRecursive recursively opens the fold under the cursor and all nested folds within it
func (m *Model) openFoldRecursive(cursorIndex int) {
	if cursorIndex >= len(m.displayNodes) {
		return
	}
	node := m.displayNodes[cursorIndex]
	if !node.isFoldable() {
		return
	}

	// Un-collapse the target node itself
	delete(m.collapsedNodes, node.nodeID())

	if node.isWorkspace {
		// Un-collapse all descendant workspaces and their note groups
		wsPath := node.workspace.Path
		wsName := node.workspace.Name

		for _, ws := range m.workspaces {
			if strings.HasPrefix(ws.Path, wsPath) && ws.Path != wsPath {
				delete(m.collapsedNodes, "ws:"+ws.Path)
			}
		}
		for _, n := range m.allNotes {
			if n.Workspace == wsName {
				delete(m.collapsedNodes, "grp:"+n.Group)
				if strings.HasPrefix(n.Group, "plans/") {
					delete(m.collapsedNodes, "grp:plans")
				}
			}
		}
	} else if node.isGroup {
		// Un-collapse child groups (e.g., 'plans' contains 'plans/sub-plan')
		groupNamePrefix := node.groupName + "/"
		for _, n := range m.allNotes {
			if n.Workspace == node.workspaceName && strings.HasPrefix(n.Group, groupNamePrefix) {
				delete(m.collapsedNodes, "grp:"+n.Group)
			}
		}
	}

	m.buildDisplayTree()
}

// toggleFoldRecursive recursively toggles the fold state under the cursor
func (m *Model) toggleFoldRecursive(cursorIndex int) {
	if cursorIndex >= len(m.displayNodes) {
		return
	}
	node := m.displayNodes[cursorIndex]
	if !node.isFoldable() {
		return
	}

	// If the node is currently collapsed, open it recursively. Otherwise, close it recursively.
	if m.collapsedNodes[node.nodeID()] {
		m.openFoldRecursive(cursorIndex)
	} else {
		m.closeFoldRecursive(cursorIndex)
	}
}

// collapseFocusedWorkspaceGroups collapses individual plans and child workspaces
func (m *Model) collapseFocusedWorkspaceGroups() {
	if m.focusedWorkspace == nil {
		return
	}

	// If focusing on an ecosystem, collapse all child workspaces
	if m.focusedWorkspace.IsEcosystem() {
		for _, ws := range m.workspaces {
			// Collapse child workspaces (those under this ecosystem)
			isSame, _ := pathutil.ComparePaths(ws.Path, m.focusedWorkspace.Path)
			if !isSame && strings.HasPrefix(strings.ToLower(ws.Path), strings.ToLower(m.focusedWorkspace.Path)+string(filepath.Separator)) {
				wsNodeID := "ws:" + ws.Path
				m.collapsedNodes[wsNodeID] = true
			}
		}
	}

	// Collapse only individual plans (plans/*) for this workspace
	// This keeps "inbox", "issues", etc. expanded while showing plan names collapsed
	for _, note := range m.allNotes {
		if note.Workspace == m.focusedWorkspace.Name {
			// Only collapse individual plans (anything starting with "plans/")
			if strings.HasPrefix(note.Group, "plans/") {
				groupNodeID := "grp:" + note.Group
				m.collapsedNodes[groupNodeID] = true
			}
		}
	}
}

// collapseChildWorkspaces collapses all child workspaces under the given workspace
func (m *Model) collapseChildWorkspaces(parent *workspace.WorkspaceNode) {
	if parent == nil {
		return
	}

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
			m.collapsedNodes[wsNodeID] = true
		}
	}
}

// collapseAllWorkspaces collapses all top-level workspaces
func (m *Model) collapseAllWorkspaces() {
	for _, ws := range m.workspaces {
		wsNodeID := "ws:" + ws.Path
		m.collapsedNodes[wsNodeID] = true
	}
}

// setCollapseStateForFocus systematically sets the collapse state based on the current focus level
func (m *Model) setCollapseStateForFocus() {
	if m.focusedWorkspace == nil {
		// Global/top level view: collapse all workspaces for a clean overview
		m.collapseAllWorkspaces()
	} else if m.focusedWorkspace.IsEcosystem() {
		// Ecosystem focus: collapse ALL note groups and child workspaces
		// to show a clean view of the ecosystem structure
		// First, ensure the focused ecosystem itself is expanded
		wsNodeID := "ws:" + m.focusedWorkspace.Path
		delete(m.collapsedNodes, wsNodeID)

		// Collapse all child workspaces under this ecosystem
		m.collapseChildWorkspaces(m.focusedWorkspace)

		// Collapse ALL note groups within the focused ecosystem
		// (current, learn, quick, etc.) BUT keep "plans" parent expanded
		groupsSeen := make(map[string]bool)
		for _, note := range m.allNotes {
			if note.Workspace == m.focusedWorkspace.Name && !groupsSeen[note.Group] {
				groupNodeID := "grp:" + note.Group
				// Collapse individual plan directories but not other top-level groups
				if strings.HasPrefix(note.Group, "plans/") {
					m.collapsedNodes[groupNodeID] = true
				} else if note.Group != "plans" {
					// Collapse regular groups (current, learn, quick, etc.)
					m.collapsedNodes[groupNodeID] = true
				}
				groupsSeen[note.Group] = true
			}
		}
		// Ensure the "plans" parent group is expanded
		plansParentID := "grp:plans"
		delete(m.collapsedNodes, plansParentID)
	} else {
		// Leaf workspace focus: expand the workspace, collapse child workspaces and individual plans
		// Ensure the focused workspace itself is expanded
		wsNodeID := "ws:" + m.focusedWorkspace.Path
		delete(m.collapsedNodes, wsNodeID)

		// Collapse any child workspaces (if this workspace has children)
		m.collapseChildWorkspaces(m.focusedWorkspace)

		// Collapse individual plan directories within the focused workspace
		// but keep the "plans" parent group expanded
		for _, note := range m.allNotes {
			if note.Workspace == m.focusedWorkspace.Name {
				if strings.HasPrefix(note.Group, "plans/") {
					groupNodeID := "grp:" + note.Group
					m.collapsedNodes[groupNodeID] = true
				}
			}
		}
		// Ensure the "plans" parent group is expanded
		plansParentID := "grp:plans"
		delete(m.collapsedNodes, plansParentID)
	}
}

// getGroupKey returns a unique key for a group node (workspace:groupName)
func (m *Model) getGroupKey(node *displayNode) string {
	return node.workspaceName + ":" + node.groupName
}

// getTargetedNotePaths returns a slice of note paths to be operated on,
// based on selection or the current cursor position.
func (m *Model) getTargetedNotePaths() []string {
	if len(m.selected) > 0 {
		paths := make([]string, 0, len(m.selected))
		for path := range m.selected {
			paths = append(paths, path)
		}
		return paths
	}

	if m.cursor < len(m.displayNodes) {
		node := m.displayNodes[m.cursor]
		if node.isNote {
			return []string{node.note.Path}
		}
		// If on a group, collect all notes within that group
		if node.isGroup {
			var paths []string
			// Find all notes belonging to this group
			for _, n := range m.allNotes {
				if n.Workspace == node.workspaceName && n.Group == node.groupName {
					paths = append(paths, n.Path)
				}
			}
			return paths
		}
	}
	return nil
}

// deleteSelectedNotesCmd creates a command to delete the selected notes.
func (m *Model) deleteSelectedNotesCmd() tea.Cmd {
	pathsToDelete := m.getTargetedNotePaths()
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

	if m.cursor < len(m.displayNodes) {
		node := m.displayNodes[m.cursor]
		if node.isWorkspace {
			destWorkspace = node.workspace
			destGroup = "inbox" // Default group when pasting on a workspace
		} else if node.isGroup {
			destWorkspace, _ = m.findWorkspaceNodeByName(node.workspaceName)
			destGroup = node.groupName
		} else if node.isNote {
			destWorkspace, _ = m.findWorkspaceNodeByName(node.note.Workspace)
			destGroup = node.note.Group
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
		if m.noteCreationCursor < len(m.displayNodes) {
			node := m.displayNodes[m.noteCreationCursor]
			var wsPath string

			if node.isWorkspace {
				wsPath = node.workspace.Path
				noteType = "inbox" // Default to inbox for workspace
			} else if node.isGroup {
				// Find workspace by name to get its path
				ws, found := m.findWorkspaceNodeByName(node.workspaceName)
				if found {
					wsPath = ws.Path
				}
				noteType = models.NoteType(node.groupName)
			} else if node.isNote {
				// Find workspace by name to get its path
				ws, found := m.findWorkspaceNodeByName(node.note.Workspace)
				if found {
					wsPath = ws.Path
				}
				noteType = models.NoteType(node.note.Group)
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
		for _, note := range m.allNotes {
			if _, ok := m.selected[note.Path]; ok {
				notesByWorkspace[note.Workspace] = append(notesByWorkspace[note.Workspace], note)
			}
		}

		var archivedPaths []string
		var archivedPlans int
		var archiveErr error

		// Archive notes workspace by workspace
		for workspaceName, notes := range notesByWorkspace {
			// Get workspace context
			wsCtx, err := m.service.GetWorkspaceContext(workspaceName)
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
		if archiveErr == nil && len(m.selectedGroups) > 0 {
			// Group plans by workspace
			plansByWorkspace := make(map[string][]string)
			for groupKey := range m.selectedGroups {
				parts := strings.SplitN(groupKey, ":", 2)
				if len(parts) == 2 {
					workspaceName := parts[0]
					groupName := parts[1]
					plansByWorkspace[workspaceName] = append(plansByWorkspace[workspaceName], groupName)
				}
			}

			// Archive plans workspace by workspace
			for workspaceName, planNames := range plansByWorkspace {
				// Find the workspace node by name
				var wsNode *workspace.WorkspaceNode
				for _, node := range m.service.GetWorkspaceProvider().All() {
					if node.Name == workspaceName {
						wsNode = node
						break
					}
				}
				if wsNode == nil {
					archiveErr = fmt.Errorf("workspace not found: %s", workspaceName)
					break
				}

				wsCtx, err := m.service.GetWorkspaceContext(wsNode.Path)
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
