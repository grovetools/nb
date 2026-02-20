package browser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/core/util/delegation"
	"github.com/grovetools/core/util/pathutil"
	"github.com/grovetools/nb/internal/tui/browser/components/confirm"
	"github.com/grovetools/nb/internal/tui/browser/views"
	"github.com/grovetools/nb/pkg/models"
	"github.com/grovetools/nb/pkg/service"
	"github.com/grovetools/nb/pkg/sync"
	"github.com/grovetools/nb/pkg/sync/github"
	"github.com/grovetools/nb/pkg/tree"
	"github.com/sirupsen/logrus"
)

// updateViewsState synchronizes the view state with the browser model
func (m *Model) updateViewsState() {
	log := logging.NewLogger("tui.browser.update")
	log.Info("updateViewsState called")

	m.views.SetParentState(
		m.service,
		m.allItems,
		m.workspaces,
		m.focusedWorkspace,
		m.filterInput.Value(),
		m.isGrepping,
		m.isFilteringByTag,
		m.selectedTag,
		m.ecosystemPickerMode,
		m.hideGlobal,
		m.showArchives,
		m.showArtifacts,
		m.showOnHold,
		m.recentNotesMode,
		m.showGitModifiedOnly,
		m.gitFileStatus,
	)
	m.views.BuildDisplayTree()

	// Apply git status filter if active
	if m.showGitModifiedOnly {
		m.views.FilterDisplayTreeByGitStatus()
	}

	// Apply text filter if present (not grep mode and not tag filter mode)
	if m.filterInput.Value() != "" && !m.isGrepping && !m.isFilteringByTag {
		m.views.FilterDisplayTree()
	}

	// Apply text filter on top of tag filter if both are active
	if m.filterInput.Value() != "" && m.isFilteringByTag {
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

// createPlanCmd creates a command to launch the flow TUI for promoting a note to a plan.
func (m *Model) createPlanCmd(note *models.Note) tea.Cmd {
	if note == nil {
		return func() tea.Msg {
			return fmt.Errorf("no note selected for promotion")
		}
	}
	// Sanitize the note title to create a valid directory name for the plan
	var planName string
	if note.FrontmatterTitle != "" {
		planName = sanitizeForFilename(note.FrontmatterTitle)
	} else {
		planName = sanitizeForFilename(note.Title)
	}

	args := []string{
		"plan", "init", planName,
		"--tui",
		"--from-note", note.Path,
		"--recipe", "standard-feature",
		"--note-target-file", "02-spec.md",
		"--worktree", // Pre-selects the worktree option in the TUI
	}

	cmd := delegation.Command("flow", args...)

	// This command takes over the terminal. When it exits, we want to refresh our state.
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			// Return an error message to display in the status bar
			// Don't quit, just show the error and let user continue
			return fmt.Errorf("plan creation failed or was cancelled")
		}
		// On success, trigger a full refresh of the note browser
		return refreshMsg{}
	})
}

func (m *Model) syncWorkspaceCmd() tea.Cmd {
	return func() tea.Msg {
		// Get workspace context
		ctx, err := m.service.GetWorkspaceContext("")
		if err != nil {
			return syncFinishedMsg{err: err}
		}

		// Create syncer and register providers
		syncer := sync.NewSyncer(m.service)
		syncer.RegisterProvider("github", func() sync.Provider {
			return github.NewProvider()
		})

		// Run sync
		reports, err := syncer.SyncWorkspace(ctx)

		return syncFinishedMsg{reports: reports, err: err}
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
			_, selectedNotes, selectedPlans := m.views.GetCounts()
			m.service.Logger.WithFields(logrus.Fields{
				"notes_count": selectedNotes,
				"plans_count": selectedPlans,
				"source":      "tui",
			}).Info("Archiving items")
			m.statusMessage = "Archiving..."
			return m, m.archiveSelectedNotesCmd()
		}
		if strings.Contains(strings.ToLower(m.confirmDialog.Prompt), "delete") {
			pathsToDelete := m.views.GetTargetedNotePaths()
			m.service.Logger.WithFields(logrus.Fields{
				"count":  len(pathsToDelete),
				"source": "tui",
			}).Warn("Deleting items")
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
			m.previewContent = msg.content
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

		// If we have a focused workspace that's not in the provider's list,
		// add it so the tree builder can display its notes
		if m.focusedWorkspace != nil {
			found := false
			for _, ws := range m.workspaces {
				if ws.Name == m.focusedWorkspace.Name {
					found = true
					break
				}
			}
			if !found {
				m.workspaces = append(m.workspaces, m.focusedWorkspace)
			}
		}

		m.updateViewsState()
		return m, nil

	case itemsLoadedMsg:
		if m.loadingCount > 0 {
			m.loadingCount--
		}
		m.allItems = msg.items
		// Set collapse state on focus change OR on initial load
		if m.focusChanged || len(m.views.GetCollapseState()) == 0 {
			m.setCollapseStateForFocus()
			m.focusChanged = false
		}
		m.updateViewsState()

		// Trigger git status fetching for items in git repos
		var gitCmds []tea.Cmd
		for _, item := range msg.items {
			gitCmds = append(gitCmds, fetchGitStatusCmd(item.Path))
			// Only fetch status for first few items to find git roots
			// The fetch will scan the whole repo once found
			if len(gitCmds) > 5 {
				break
			}
		}
		if len(gitCmds) > 0 {
			return m, tea.Batch(append(gitCmds, m.updatePreviewContent())...)
		}
		return m, m.updatePreviewContent()

	case gitStatusLoadedMsg:
		if msg.err == nil && msg.repoPath != "" && msg.fileStatus != nil {
			// Only process if we haven't already scanned this repo
			if !m.scannedGitRepos[msg.repoPath] {
				m.scannedGitRepos[msg.repoPath] = true
				// Merge file status into our map
				for path, status := range msg.fileStatus {
					m.gitFileStatus[path] = status
				}
				// Merge deleted files (deduplicate)
				existingDeleted := make(map[string]bool)
				for _, p := range m.gitDeletedFiles {
					existingDeleted[p] = true
				}
				for _, p := range msg.deletedFiles {
					if !existingDeleted[p] {
						m.gitDeletedFiles = append(m.gitDeletedFiles, p)
						existingDeleted[p] = true
					}
				}
				// Pass git status and deleted files to views for rendering
				m.views.SetGitFileStatus(m.gitFileStatus)
				m.views.SetGitDeletedFiles(m.gitDeletedFiles)
				// Rebuild view to include deleted files and update status indicators
				if m.showGitModifiedOnly {
					m.updateViewsState()
				} else if len(msg.deletedFiles) > 0 {
					// Add deleted files to tree even in normal view
					m.views.AddDeletedFilesToTree()
				}
			}
		}
		return m, nil

	case commitFinishedMsg:
		m.isCommitting = false
		m.commitInput.Blur()
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Commit failed: %v", msg.err)
		} else if msg.success {
			m.statusMessage = fmt.Sprintf("Committed: %s", msg.message)
			m.clearGitStatus()
			// Trigger refresh to update git status
			return m, func() tea.Msg { return refreshMsg{} }
		} else {
			m.statusMessage = msg.message
		}
		return m, nil

	case stageFinishedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Stage failed: %v", msg.err)
		} else if msg.success && msg.count == -1 {
			// Stage all - need full refresh
			m.statusMessage = "Staged all changes"
			m.scannedGitRepos = make(map[string]bool)
			return m, func() tea.Msg { return refreshMsg{} }
		} else if msg.success && msg.count > 0 {
			m.statusMessage = fmt.Sprintf("Staged %d file(s)", msg.count)
			// Optimistically update git status without full refresh
			for path, status := range msg.updatedStatus {
				m.gitFileStatus[path] = status
			}
			m.views.SetGitFileStatus(m.gitFileStatus)
		} else {
			m.statusMessage = "No files to stage"
		}
		return m, nil

	case unstageFinishedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Unstage failed: %v", msg.err)
		} else if msg.success && msg.count == -1 {
			// -1 signals "all" were unstaged
			m.statusMessage = "Unstaged all changes"
			// Clear cached git repos to force full refresh
			m.scannedGitRepos = make(map[string]bool)
			return m, func() tea.Msg { return refreshMsg{} }
		} else if msg.success && msg.count > 0 {
			m.statusMessage = fmt.Sprintf("Unstaged %d file(s)", msg.count)
			// Optimistically update git status without full refresh
			for path, status := range msg.updatedStatus {
				m.gitFileStatus[path] = status
			}
			m.views.SetGitFileStatus(m.gitFileStatus)
		} else {
			m.statusMessage = "No files to unstage"
		}
		return m, nil

	case syncFinishedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Sync failed: %v", msg.err)
			return m, nil
		}
		var reportStrings []string
		var allErrors []string
		for _, report := range msg.reports {
			parts := []string{}
			if report.Created > 0 {
				parts = append(parts, fmt.Sprintf("%d created", report.Created))
			}
			if report.Updated > 0 {
				parts = append(parts, fmt.Sprintf("%d updated", report.Updated))
			}
			if report.Unchanged > 0 {
				parts = append(parts, fmt.Sprintf("%d unchanged", report.Unchanged))
			}
			if report.Failed > 0 {
				parts = append(parts, fmt.Sprintf("%d FAILED", report.Failed))
			}
			reportStrings = append(reportStrings, fmt.Sprintf("%s: %s", report.Provider, strings.Join(parts, ", ")))
			allErrors = append(allErrors, report.Errors...)
		}
		statusMsg := "Sync: " + strings.Join(reportStrings, ", ")
		// If there were errors, show count (run CLI for details)
		if len(allErrors) > 0 {
			if len(allErrors) == 1 {
				statusMsg += " (run 'nb remote sync' for error details)"
			} else {
				statusMsg += fmt.Sprintf(" (run 'nb remote sync' for %d error details)", len(allErrors))
			}
		}
		m.statusMessage = statusMsg
		return m, func() tea.Msg { return refreshMsg{} }

	case refreshMsg:
		m.loadingCount = 2 // for workspaces and notes
		m.clearGitStatus()

		var notesCmd tea.Cmd
		if m.focusedWorkspace != nil {
			notesCmd = fetchFocusedItemsCmd(m.service, m.focusedWorkspace, m.showArtifacts)
		} else {
			notesCmd = fetchAllItemsCmd(m.service, m.showArtifacts)
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
		// Filter out deleted items
		newAllItems := make([]*tree.Item, 0, len(m.allItems))
		for _, item := range m.allItems {
			if !deletedMap[item.Path] {
				newAllItems = append(newAllItems, item)
			}
		}
		m.allItems = newAllItems
		// Clear selections
		m.views.ClearSelections()
		// Rebuild display
		m.updateViewsState()
		m.clearGitStatus()
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
		m.clearGitStatus()
		// Refresh notes to show the new locations
		m.loadingCount++
		if m.focusedWorkspace != nil {
			return m, tea.Batch(fetchFocusedItemsCmd(m.service, m.focusedWorkspace, m.showArtifacts), m.spinner.Tick)
		}
		return m, tea.Batch(fetchAllItemsCmd(m.service, m.showArtifacts), m.spinner.Tick)

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

		// Filter out archived items from allItems
		newAllItems := make([]*tree.Item, 0)
		for _, item := range m.allItems {
			if !archivedMap[item.Path] {
				newAllItems = append(newAllItems, item)
			}
		}
		m.allItems = newAllItems

		// Clear selections
		m.views.ClearSelections()

		// Rebuild the display
		m.updateViewsState()

		if msg.archivedPlans > 0 {
			m.statusMessage = fmt.Sprintf("Archived %d note(s) and %d plan(s)", len(msg.archivedPaths), msg.archivedPlans)
		} else {
			m.statusMessage = fmt.Sprintf("Archived %d note(s)", len(msg.archivedPaths))
		}

		m.clearGitStatus()
		// Refresh notes to show the updated archive structure
		m.loadingCount++
		if m.focusedWorkspace != nil {
			return m, tea.Batch(fetchFocusedItemsCmd(m.service, m.focusedWorkspace, m.showArtifacts), m.spinner.Tick)
		}
		return m, tea.Batch(fetchAllItemsCmd(m.service, m.showArtifacts), m.spinner.Tick)

	case noteCreatedMsg:
		m.isCreatingNote = false
		m.noteTitleInput.Blur()
		m.noteTitleInput.SetValue("")
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error creating note: %v", msg.err)
			return m, nil
		}
		m.statusMessage = fmt.Sprintf("Created note: %s", msg.note.Title)
		m.clearGitStatus()
		// Refresh notes to show the new one
		m.loadingCount++
		if m.focusedWorkspace != nil {
			return m, tea.Batch(fetchFocusedItemsCmd(m.service, m.focusedWorkspace, m.showArtifacts), m.spinner.Tick)
		}
		return m, tea.Batch(fetchAllItemsCmd(m.service, m.showArtifacts), m.spinner.Tick)

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
		m.clearGitStatus()
		// Refresh notes to show the updated name
		m.loadingCount++
		if m.focusedWorkspace != nil {
			return m, tea.Batch(fetchFocusedItemsCmd(m.service, m.focusedWorkspace, m.showArtifacts), m.spinner.Tick)
		}
		return m, tea.Batch(fetchAllItemsCmd(m.service, m.showArtifacts), m.spinner.Tick)

	case tea.KeyMsg:
		if m.help.ShowAll {
			// Let help component handle keys (scrolling, close on ?/q/esc)
			var cmd tea.Cmd
			m.help, cmd = m.help.Update(msg)
			return m, cmd
		}

		// Handle active components first
		if m.confirmDialog.Active {
			m.confirmDialog, cmd = m.confirmDialog.Update(msg)
			return m, cmd
		}

		// Handle filtering mode early - before any other key bindings
		if m.filterInput.Focused() {
			// Only handle Esc and Enter specially, pass everything else to the input
			// Note: We check msg.String() directly instead of using key bindings
			// because m.keys.Confirm includes "y" which should be typed in the input
			if msg.String() == "esc" {
				m.filterInput.SetValue("")
				m.filterInput.Blur()
				// If we're in tag filter mode, only clear the search, not the tag filter
				if m.isFilteringByTag {
					m.updateViewsState()
					return m, nil
				}
				m.isGrepping = false // Exit grep mode
				m.isFilteringByTag = false // Exit tag filter mode
				m.selectedTag = ""
				m.updateViewsState()
				return m, nil
			}
			if msg.String() == "enter" {
				m.filterInput.Blur()
				return m, nil
			}
			// Pass all other keys to the input
			m.filterInput, cmd = m.filterInput.Update(msg)
			if m.isGrepping {
				m.applyGrepFilter()
			} else {
				m.updateViewsState()
			}
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

		// Handle commit dialog mode
		if m.isCommitting {
			return m.updateCommitDialog(msg)
		}

		// Handle tag picker mode
		if m.tagPickerMode {
			switch msg.String() {
			case "esc":
				m.tagPickerMode = false
				return m, nil
			case "enter":
				// Apply the selected tag filter
				if selectedItem, ok := m.tagPicker.SelectedItem().(tagItem); ok {
					m.tagPickerMode = false
					m.isFilteringByTag = true
					m.selectedTag = selectedItem.tag
					m.filterInput.SetValue("") // Clear filter input for additional search
					// Expand everything when tag filter is active
					m.views.SetCollapseState(make(map[string]bool))
					m.updateViewsState()
				}
				return m, nil
			default:
				m.tagPicker, cmd = m.tagPicker.Update(msg)
				return m, cmd
			}
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

		// Process sequence state for browser-level sequences (dd for delete)
		sequenceBindings := keymap.CommonSequenceBindings(m.keys.Base)
		seqResult, _ := m.sequence.Process(msg, sequenceBindings...)
		buffer := m.sequence.Buffer()

		// Try delegating to views for navigation, folding, and selection
		// Views.Update handles: Up, Down, Left, Right, PageUp, PageDown, Top, Bottom,
		// FoldOpen (zo), FoldClose (zc), FoldToggle (za), FoldOpenAll (zR), FoldCloseAll (zM),
		// ToggleSelect (space), SelectNone (N)
		isFoldPrefix := keymap.IsPrefixOfAny(buffer, m.keys.FoldOpen, m.keys.FoldClose, m.keys.FoldToggle, m.keys.FoldOpenAll, m.keys.FoldCloseAll)
		isFoldMatch := keymap.Matches(buffer, m.keys.FoldOpen) || keymap.Matches(buffer, m.keys.FoldClose) ||
			keymap.Matches(buffer, m.keys.FoldToggle) || keymap.Matches(buffer, m.keys.FoldOpenAll) ||
			keymap.Matches(buffer, m.keys.FoldCloseAll)
		isTopSequence := keymap.IsPrefixOfAny(buffer, m.keys.Top) || keymap.Matches(buffer, m.keys.Top)

		if key.Matches(msg, m.keys.Up) || key.Matches(msg, m.keys.Down) ||
			key.Matches(msg, m.keys.Left) || key.Matches(msg, m.keys.Right) ||
			key.Matches(msg, m.keys.PageUp) || key.Matches(msg, m.keys.PageDown) ||
			key.Matches(msg, m.keys.Bottom) ||
			isFoldPrefix || isFoldMatch || isTopSequence ||
			key.Matches(msg, m.keys.ToggleSelect) ||
			key.Matches(msg, m.keys.SelectNone) {
			// Clear browser sequence only when we've completed a sequence or it's a non-sequence key
			// Don't clear when we're in the middle of a fold/top sequence (prefix match)
			if !isFoldPrefix && !keymap.IsPrefixOfAny(buffer, m.keys.Top) && !key.Matches(msg, m.keys.Delete) {
				m.sequence.Clear()
			}
			m.views, cmd = m.views.Update(msg)
			// After any view update that could change the cursor, update the preview.
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
				return m, tea.Batch(fetchAllItemsCmd(m.service, m.showArtifacts), m.spinner.Tick)
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
					return m, tea.Batch(fetchFocusedItemsCmd(m.service, m.focusedWorkspace, m.showArtifacts), m.spinner.Tick)
				} else {
					return m, tea.Batch(fetchAllItemsCmd(m.service, m.showArtifacts), m.spinner.Tick)
				}
			}
		case key.Matches(msg, m.keys.FocusSelected):
			node := m.views.GetCurrentNode()
			if node != nil && node.IsWorkspace() {
				m.loadingCount++
				if ws, ok := node.Item.Metadata["Workspace"].(*workspace.WorkspaceNode); ok {
					m.focusedWorkspace = ws
				}
				m.ecosystemPickerMode = false // Focusing on a workspace exits picker mode
				m.focusChanged = true
				// Re-fetch notes for the newly focused workspace
				return m, tea.Batch(fetchFocusedItemsCmd(m.service, m.focusedWorkspace, m.showArtifacts), m.spinner.Tick)
			}
		case key.Matches(msg, m.keys.SwitchView):
			m.views.ToggleViewMode()
		case key.Matches(msg, m.keys.FocusRecent):
			m.recentNotesMode = !m.recentNotesMode
			if m.recentNotesMode {
				// Save current state and switch to recent notes mode
				m.savedViewMode = m.views.GetViewMode()
				m.views.SetViewMode(views.TableView)
				m.savedModVisibility = m.columnVisibility["MODIFIED"]
				m.savedWsVisibility = m.columnVisibility["WORKSPACE"]
				m.columnVisibility["MODIFIED"] = true
				m.columnVisibility["WORKSPACE"] = true
				m.views.SetColumnVisibility(m.columnVisibility)
				m.statusMessage = "Recent notes view"
			} else {
				// Restore previous state
				m.views.SetViewMode(m.savedViewMode)
				m.columnVisibility["MODIFIED"] = m.savedModVisibility
				m.columnVisibility["WORKSPACE"] = m.savedWsVisibility
				m.views.SetColumnVisibility(m.columnVisibility)
				m.statusMessage = "Default view restored"
			}
			m.updateViewsState()
		case key.Matches(msg, m.keys.Search):
			m.isGrepping = false
			// Don't clear tag filter when searching - allow search on top of tag filter
			if !m.isFilteringByTag {
				m.selectedTag = ""
			}
			m.filterInput.SetValue("")
			if m.isFilteringByTag {
				m.filterInput.Placeholder = fmt.Sprintf("Search in tag '%s'...", m.selectedTag)
			} else {
				m.filterInput.Placeholder = "Search notes..."
			}
			m.filterInput.Focus()
			return m, textinput.Blink
	case key.Matches(msg, m.keys.Refresh):
		return m, func() tea.Msg { return refreshMsg{} }
	case key.Matches(msg, m.keys.ToggleGitChanges):
		m.showGitModifiedOnly = !m.showGitModifiedOnly
		if m.showGitModifiedOnly {
			m.statusMessage = "Filtering for git changes"
		} else {
			m.statusMessage = "Cleared git changes filter"
		}
		m.updateViewsState()
		return m, nil
	case key.Matches(msg, m.keys.Sync):
		m.statusMessage = "Syncing with remotes..."
		return m, tea.Batch(m.syncWorkspaceCmd(), m.spinner.Tick)
	case key.Matches(msg, m.keys.FilterByTag):
			m.isGrepping = false
			// Always show the tag picker - allows switching between tags
			m.tagPickerMode = true
			m.populateTagPicker()
			return m, nil
		case key.Matches(msg, m.keys.Grep):
			m.isGrepping = true
			m.isFilteringByTag = false
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
			m.statusMessage = fmt.Sprintf("Archives: %v (Found %d notes)", m.showArchives, len(m.allItems))
			m.updateViewsState()
		case key.Matches(msg, m.keys.ToggleArtifacts):
			m.showArtifacts = !m.showArtifacts
			m.statusMessage = fmt.Sprintf("Artifacts: %v", m.showArtifacts)
			// Refetch notes with new artifact visibility setting
			var notesCmd tea.Cmd
			if m.focusedWorkspace != nil {
				notesCmd = fetchFocusedItemsCmd(m.service, m.focusedWorkspace, m.showArtifacts)
			} else {
				notesCmd = fetchAllItemsCmd(m.service, m.showArtifacts)
			}
			return m, tea.Batch(notesCmd, m.spinner.Tick)
		case key.Matches(msg, m.keys.ToggleHold):
			m.showOnHold = !m.showOnHold
			m.statusMessage = fmt.Sprintf("On-hold plans: %v", m.showOnHold)
			m.updateViewsState()
		case key.Matches(msg, m.keys.ToggleGlobal):
			m.hideGlobal = !m.hideGlobal
			m.updateViewsState()
		case seqResult == keymap.SequenceMatch && keymap.Matches(buffer, m.keys.Delete):
			// dd - delete selected notes
			pathsToDelete := m.views.GetTargetedNotePaths()
			if len(pathsToDelete) > 0 {
				prompt := fmt.Sprintf("Permanently delete %d note(s)? This cannot be undone.", len(pathsToDelete))
				m.confirmDialog.Activate(prompt)
			}
			m.sequence.Clear()
		case key.Matches(msg, m.keys.Delete):
			// First 'd' press - sequence state is tracking it, just wait
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
		case key.Matches(msg, m.keys.Yank):
			node := m.views.GetCurrentNode()
			if node != nil && node.Item != nil {
				path := node.Item.Path
				if err := clipboard.WriteAll(path); err != nil {
					m.statusMessage = fmt.Sprintf("Error yanking path: %v", err)
				} else {
					m.statusMessage = fmt.Sprintf("Yanked: %s", shortenPath(path))
				}
			}
			return m, nil
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
			if node != nil && node.IsNote() {
				m.isRenamingNote = true
				m.noteToRename = views.ItemToNote(node.Item)
				if title, ok := node.Item.Metadata["FrontmatterTitle"].(string); ok {
					m.renameInput.SetValue(title)
				}
				m.renameInput.Focus()
				return m, textinput.Blink
			}
		case key.Matches(msg, m.keys.CreatePlan):
			node := m.views.GetCurrentNode()
			if node != nil && node.IsNote() {
				note := views.ItemToNote(node.Item)
				title := node.Item.Name
				if t, ok := node.Item.Metadata["Title"].(string); ok {
					title = t
				}
				m.statusMessage = fmt.Sprintf("Promoting note '%s' to a new plan...", title)
				// This command will take over the terminal to launch the flow TUI
				return m, m.createPlanCmd(note)
			}
			return m, nil
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
				if node != nil && node.IsWorkspace() {
					if ws, ok := node.Item.Metadata["Workspace"].(*workspace.WorkspaceNode); ok && ws.IsEcosystem() {
						m.loadingCount++
						m.focusedWorkspace = ws
						m.ecosystemPickerMode = false
						m.focusChanged = true
						// Re-fetch notes for the selected ecosystem
						return m, tea.Batch(fetchFocusedItemsCmd(m.service, m.focusedWorkspace, m.showArtifacts), m.spinner.Tick)
					}
				}
			} else {
				var noteToOpen *models.Note
				node := m.views.GetCurrentNode()
				if node != nil {
					if node.IsNote() {
						noteToOpen = views.ItemToNote(node.Item)
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
		case key.Matches(msg, m.keys.GitCommit):
			// Start commit dialog
			m.isCommitting = true
			m.commitInput.SetValue("")
			m.commitInput.Focus()
			return m, textinput.Blink
		case key.Matches(msg, m.keys.GitStageToggle):
			// Toggle stage for selected files (or current file if none selected)
			paths := m.views.GetTargetedNotePaths()
			if len(paths) == 0 {
				m.statusMessage = "No files to stage/unstage"
				return m, nil
			}
			return m, toggleStageFilesCmd(paths, m.gitFileStatus)
		case key.Matches(msg, m.keys.GitStageAll):
			// Stage all changes
			return m, stageAllCmd(m.allItems)
		case key.Matches(msg, m.keys.GitUnstageAll):
			// Unstage all changes
			return m, unstageAllCmd(m.allItems)
		case key.Matches(msg, m.keys.Back):
			if m.ecosystemPickerMode {
				m.ecosystemPickerMode = false
				m.updateViewsState()
				return m, nil
			}
			// If in tag filter mode, clear it
			if m.isFilteringByTag {
				m.isFilteringByTag = false
				m.selectedTag = ""
				m.filterInput.SetValue("")
				m.updateViewsState()
				m.statusMessage = "Tag filter cleared"
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
			// Clear sequence buffer for keys that aren't part of sequences
			// unless we're in the middle of a potential sequence
			if seqResult != keymap.SequencePending {
				m.sequence.Clear()
			}
		}

	default:
		if m.filterInput.Focused() {
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			return m, cmd
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
		if node.IsWorkspace() {
			if ws, ok := node.Item.Metadata["Workspace"].(*workspace.WorkspaceNode); ok {
				destWorkspace = ws
			}
			destGroup = "inbox" // Default group when pasting on a workspace
		} else if node.IsGroup() {
			if wsName, ok := node.Item.Metadata["Workspace"].(string); ok {
				destWorkspace, _ = m.findWorkspaceNodeByName(wsName)
			}
			// Use the Group metadata which contains the full path (e.g., "plans/my-target-plan")
			// Fall back to Item.Name if metadata is not present
			if group, ok := node.Item.Metadata["Group"].(string); ok {
				destGroup = group
			} else {
				destGroup = node.Item.Name
			}
		} else if node.IsNote() {
			if wsName, ok := node.Item.Metadata["Workspace"].(string); ok {
				destWorkspace, _ = m.findWorkspaceNodeByName(wsName)
			}
			if group, ok := node.Item.Metadata["Group"].(string); ok {
				destGroup = group
			}
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

			if node.IsWorkspace() {
				if ws, ok := node.Item.Metadata["Workspace"].(*workspace.WorkspaceNode); ok {
					wsPath = ws.Path
				}
				noteType = "inbox" // Default to inbox for workspace
			} else if node.IsGroup() {
				// Find workspace by name to get its path
				if wsName, ok := node.Item.Metadata["Workspace"].(string); ok {
					ws, found := m.findWorkspaceNodeByName(wsName)
					if found {
						wsPath = ws.Path
					}
				}
				noteType = models.NoteType(node.Item.Name)
			} else if node.IsNote() {
				// Find workspace by name to get its path
				if wsName, ok := node.Item.Metadata["Workspace"].(string); ok {
					ws, found := m.findWorkspaceNodeByName(wsName)
					if found {
						wsPath = ws.Path
					}
				}
				if group, ok := node.Item.Metadata["Group"].(string); ok {
					noteType = models.NoteType(group)
				}
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
	for _, item := range m.allItems {
		if strings.HasPrefix(item.Path, sourcePath+string(filepath.Separator)) {
			notePaths = append(notePaths, item.Path)
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
		notesByWorkspace := make(map[string][]*tree.Item)
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
		for _, item := range m.allItems {
			if _, ok := selectedNotes[item.Path]; ok {
				if wsName, ok := item.Metadata["Workspace"].(string); ok {
					notesByWorkspace[wsName] = append(notesByWorkspace[wsName], item)
				}
			}
		}

		var archivedPaths []string
		var archivedPlans int
		var archiveErr error

		// Archive notes workspace by workspace
		for workspaceName, items := range notesByWorkspace {
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

			// Extract paths from items
			paths := make([]string, len(items))
			for i, item := range items {
				paths[i] = item.Path
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
			wsNodeID := "dir:" + ws.Path
			collapsedNodes[wsNodeID] = true
		}
	}
	m.views.SetCollapseState(collapsedNodes)
}

// collapseAllWorkspaces collapses all top-level workspaces
func (m *Model) collapseAllWorkspaces() {
	collapsedNodes := m.views.GetCollapseState()
	for _, ws := range m.workspaces {
		wsNodeID := "dir:" + ws.Path
		collapsedNodes[wsNodeID] = true
	}
	m.views.SetCollapseState(collapsedNodes)
}

// setCollapseStateForFocus systematically sets the collapse state based on the current focus level
func (m *Model) setCollapseStateForFocus() {
	collapsedNodes := make(map[string]bool) // Start fresh

	if m.focusedWorkspace == nil {
		// Global/top level view: collapse all workspaces for a clean overview
		m.collapseAllWorkspaces()
	} else if m.focusedWorkspace.IsEcosystem() {
		// Ecosystem focus: collapse ALL note groups and child workspaces
		// to show a clean view of the ecosystem structure
		// First, ensure the focused ecosystem itself is expanded
		wsNodeID := "dir:" + m.focusedWorkspace.Path
		delete(collapsedNodes, wsNodeID)

		// Collapse all child workspaces under this ecosystem
		m.collapseChildWorkspaces(m.focusedWorkspace)

		// Get workspace node from m.workspaces to ensure we use the same one as tree building
		var wsNode *workspace.WorkspaceNode
		for _, ws := range m.workspaces {
			if ws.Name == m.focusedWorkspace.Name {
				wsNode = ws
				break
			}
		}
		if wsNode == nil {
			wsNode = m.focusedWorkspace
		}

		// Use GetGroupDir for all path resolution to ensure consistency
		groupsSeen := make(map[string]bool)
		for _, item := range m.allItems {
			wsName, _ := item.Metadata["Workspace"].(string)
			groupName, _ := item.Metadata["Group"].(string)
			if wsName == m.focusedWorkspace.Name && !groupsSeen[groupName] {
				// Use GetGroupDir for centralized path resolution
				groupPath, err := m.service.GetNotebookLocator().GetGroupDir(wsNode, groupName)
				if err != nil {
					// Skip if we can't resolve path
					groupsSeen[groupName] = true
					continue
				}

				groupNodeID := "dir:" + groupPath

				// Use DefaultExpand from NoteTypes to determine if group should be expanded
				shouldExpand := false
				if typeConfig, ok := m.service.NoteTypes[groupName]; ok {
					shouldExpand = typeConfig.DefaultExpand
				}

				// Collapse groups that shouldn't be expanded by default
				if !shouldExpand {
					collapsedNodes[groupNodeID] = true
				}
				groupsSeen[groupName] = true
			}
		}

		// Ensure groups with DefaultExpand=true are expanded
		for groupName, typeConfig := range m.service.NoteTypes {
			if typeConfig.DefaultExpand {
				if groupPath, err := m.service.GetNotebookLocator().GetGroupDir(wsNode, groupName); err == nil {
					delete(collapsedNodes, "dir:"+groupPath)
				}
			}
		}
	} else {
		// Leaf workspace focus (e.g., repo or worktree)
		// This implements the requested default folding behavior.

		// 1. Collapse the global workspace
		collapsedNodes["dir:::global"] = true

		// 2. Ensure the focused workspace itself is expanded
		wsNodeID := "dir:" + m.focusedWorkspace.Path
		delete(collapsedNodes, wsNodeID)

		// 3. Collapse any child workspaces (if any)
		m.collapseChildWorkspaces(m.focusedWorkspace)

		// 4. Collapse/expand note groups according to rules
		// NOW USING GetGroupDir for ALL path resolution to ensure consistency
		var wsNode *workspace.WorkspaceNode
		for _, ws := range m.workspaces {
			if ws.Name == m.focusedWorkspace.Name {
				wsNode = ws
				break
			}
		}

		if wsNode == nil {
			wsNode = m.focusedWorkspace
		}

		groupsSeen := make(map[string]bool)
		for _, item := range m.allItems {
			wsName, _ := item.Metadata["Workspace"].(string)
			groupName, _ := item.Metadata["Group"].(string)

			if wsName == m.focusedWorkspace.Name && !groupsSeen[groupName] {
				// Use GetGroupDir for centralized path resolution
				groupPath, err := m.service.GetNotebookLocator().GetGroupDir(wsNode, groupName)
				if err != nil {
					// Skip if we can't resolve path
					groupsSeen[groupName] = true
					continue
				}

				groupNodeID := "dir:" + groupPath

				// Use DefaultExpand from NoteTypes to determine if group should be expanded
				shouldExpand := false
				if typeConfig, ok := m.service.NoteTypes[groupName]; ok {
					shouldExpand = typeConfig.DefaultExpand
				}

				// Collapse groups that shouldn't be expanded by default
				if !shouldExpand {
					collapsedNodes[groupNodeID] = true
				}
				groupsSeen[groupName] = true
			}
		}

		// Ensure groups with DefaultExpand=true are expanded
		for groupName, typeConfig := range m.service.NoteTypes {
			if typeConfig.DefaultExpand {
				if groupPath, err := m.service.GetNotebookLocator().GetGroupDir(wsNode, groupName); err == nil {
					delete(collapsedNodes, "dir:"+groupPath)
				}
			}
		}
	}

	m.views.SetCollapseState(collapsedNodes)
}

// applyGrepFilter applies the grep-based content filter to the display nodes
func (m *Model) applyGrepFilter() {
	m.views.ApplyGrepFilter()
}

// clearGitStatus resets the git status state to force a re-fetch
func (m *Model) clearGitStatus() {
	m.gitFileStatus = make(map[string]string)
	m.gitDeletedFiles = nil
	m.scannedGitRepos = make(map[string]bool)
	m.views.SetGitFileStatus(m.gitFileStatus)
	m.views.SetGitDeletedFiles(nil)
	// Rebuild view to reflect cleared status
	if m.showGitModifiedOnly {
		m.updateViewsState()
	}
}

// updateCommitDialog handles input when the commit dialog is active.
func (m *Model) updateCommitDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.isCommitting = false
			m.commitInput.Blur()
			return m, nil
		case "enter":
			return m, m.executeCommitCmd()
		}
	}

	m.commitInput, cmd = m.commitInput.Update(msg)
	return m, cmd
}

// executeCommitCmd creates a command to execute the git commit.
func (m *Model) executeCommitCmd() tea.Cmd {
	message := m.commitInput.Value()
	if message == "" {
		message = "Update notes"
	}

	return func() tea.Msg {
		// Find a git repo from the current items
		var gitRoot string
		for _, item := range m.allItems {
			root, err := findGitRoot(item.Path)
			if err == nil && root != "" {
				gitRoot = root
				break
			}
		}

		if gitRoot == "" {
			return commitFinishedMsg{success: false, message: "No git repository found", err: nil}
		}

		// Commit only what's already staged (no auto-staging)
		gitCommitCmd := exec.Command("git", "commit", "-m", message)
		gitCommitCmd.Dir = gitRoot
		output, err := gitCommitCmd.CombinedOutput()
		if err != nil {
			// Check if there's nothing to commit
			if strings.Contains(string(output), "nothing to commit") {
				return commitFinishedMsg{success: false, message: "Nothing to commit", err: nil}
			}
			return commitFinishedMsg{success: false, message: "", err: fmt.Errorf("git commit failed: %w\n%s", err, string(output))}
		}

		return commitFinishedMsg{success: true, message: message, err: nil}
	}
}

// findGitRoot finds the git root directory for a given path
func findGitRoot(path string) (string, error) {
	dir := filepath.Dir(path)
	for {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not a git repository")
		}
		dir = parent
	}
}


