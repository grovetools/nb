package browser

import (
	"fmt"
	"os"
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
	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/core/util/delegation"
	"github.com/grovetools/core/util/pathutil"
	"github.com/sirupsen/logrus"

	"github.com/grovetools/nb/pkg/models"
	"github.com/grovetools/nb/pkg/service"
	"github.com/grovetools/nb/pkg/sync"
	"github.com/grovetools/nb/pkg/sync/github"
	"github.com/grovetools/nb/pkg/tree"
	"github.com/grovetools/nb/pkg/tui/browser/components/confirm"
	"github.com/grovetools/nb/pkg/tui/browser/views"
)

// parseSearchInput interprets the single search input's leading prefix to pick a
// search mode, mirroring nav's unified text-input model. A leading '?' routes to
// content grep, a leading '#' routes to tag filtering, and anything else is a
// plain case-insensitive substring filter. For tag mode the token after '#' is
// the (exact) tag name and any text after the first space is an additional
// substring search within that tag. The returned query is the prefix-stripped
// filter value that the views layer should match against.
//
// This is the single place the prefix is parsed — the views-layer matchers
// (FilterDisplayTree / ApplyGrepFilter / BuildDisplayTree) stay prefix-agnostic
// and just consume the derived flags + query.
func parseSearchInput(raw string) (query string, tag string, isGrep, isTag bool) {
	switch {
	case strings.HasPrefix(raw, "?"):
		return strings.TrimPrefix(raw, "?"), "", true, false
	case strings.HasPrefix(raw, "#"):
		rest := strings.TrimPrefix(raw, "#")
		tagName := rest
		extra := ""
		if i := strings.IndexByte(rest, ' '); i >= 0 {
			tagName = rest[:i]
			extra = strings.TrimSpace(rest[i+1:])
		}
		return extra, tagName, false, true
	default:
		return raw, "", false, false
	}
}

// updateViewsState synchronizes the view state with the browser model. It parses
// the search input's prefix (see parseSearchInput) to derive the grep/tag/plain
// mode rather than relying on standalone mode booleans, then pushes the stripped
// query + derived flags into the views layer.
func (m *Model) updateViewsState() {
	log := logging.NewLogger("tui.browser.update")
	log.Debug("updateViewsState called")

	query, tag, isGrep, isTag := parseSearchInput(m.filterInput.Value())
	// Keep the model's mode flags in sync with the parsed input so other call
	// sites (status bar, view header, second-Esc clear) observe a single source
	// of truth.
	m.isGrepping = isGrep
	m.isFilteringByTag = isTag
	if isTag {
		m.selectedTag = tag
	} else {
		m.selectedTag = ""
	}

	m.views.SetParentState(
		m.service,
		m.allItems,
		m.workspaces,
		m.focusedWorkspace,
		query,
		isGrep,
		isTag,
		m.selectedTag,
		m.ecosystemPickerMode,
		m.hideGlobal,
		m.showArchives,
		m.showArtifacts,
		m.showOnHold,
		m.recentNotesMode,
		m.showGitModifiedOnly,
		m.archiveViewMode,
		m.gitFileStatus,
		m.jobs,
	)
	m.views.BuildDisplayTree()

	// Grep matches file content, not the tree, so route to the grep filter and
	// skip the substring passes below.
	if isGrep {
		if query != "" {
			m.applyGrepFilter()
		}
		return
	}

	// Apply git status filter if active
	if m.showGitModifiedOnly {
		m.views.FilterDisplayTreeByGitStatus()
	}

	// Apply substring filter if present. In tag mode `query` is the additional
	// within-tag search (empty unless the user typed "#tag extra"); the tag
	// itself is already applied during BuildDisplayTree.
	if query != "" {
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

// bumpSelectedPriority adjusts the priority of the note under the cursor one
// step more (moreCritical=true) or less critical and writes it to disk. It is a
// no-op when the cursor is not on a note or the bump hits a ladder end.
//
// The update is OPTIMISTIC: after the disk write succeeds we mutate the
// in-memory item and rebuild the display tree LOCALLY instead of returning
// refreshMsg{}. A refresh would re-fetch items from the daemon note index, which
// re-indexes the just-written file asynchronously (file-watch latency) and so
// almost always returns the OLD priority — making the bump appear not to stick
// and corrupting the base value the next bump computes from. The disk write is
// synchronous (os.WriteFile), so the daemon catches up in the background and a
// later natural refresh stays consistent. Returns nil (no command) — the rebuild
// is done in place.
func (m *Model) bumpSelectedPriority(moreCritical bool) tea.Cmd {
	node := m.views.GetCurrentNode()
	if node == nil || !node.IsNote() {
		return nil
	}
	note := views.ItemToNote(node.Item)
	newPriority := views.BumpPriority(note.Priority, moreCritical)
	if newPriority == note.Priority {
		// Already at the most/least critical end.
		return nil
	}
	if err := m.service.UpdateNotePriority(note.Path, newPriority); err != nil {
		m.statusMessage = fmt.Sprintf("Failed to set priority: %s", err)
		return nil
	}

	// Update the backing source item so the local rebuild reflects the new value.
	// BuildDisplayTree reconstructs every display node fresh from m.allItems (via
	// ItemToNote -> noteToItem), so the display node's Item is a DIFFERENT pointer
	// from the allItems entry — mutating the allItems entry is what actually
	// survives the rebuild. We also touch the current node's Item for correctness
	// in case anything reads it before the rebuild completes.
	path := note.Path
	for _, item := range m.allItems {
		if item.Path == path {
			if item.Metadata == nil {
				item.Metadata = make(map[string]interface{})
			}
			item.Metadata["Priority"] = newPriority
		}
	}
	if node.Item != nil {
		if node.Item.Metadata == nil {
			node.Item.Metadata = make(map[string]interface{})
		}
		node.Item.Metadata["Priority"] = newPriority
	}

	// Rebuild locally (preserves the active filter via updateViewsState) and pin
	// the cursor to the bumped note by path. When grouped by priority the note
	// moves to its new bucket on rebuild; pinning by path keeps the cursor on it
	// instead of stranding it at a now-unrelated index.
	m.updateViewsState()
	m.views.SetCursorToPath(path)

	if newPriority == "" {
		m.statusMessage = "Cleared priority"
	} else {
		m.statusMessage = "Priority set to " + newPriority
	}
	return nil
}

// notePromotedToJobMsg is sent after a note has been promoted to a job in a plan.
type notePromotedToJobMsg struct {
	planName string
	jobFile  string
	err      error
}

// promoteToJobCmd creates a job in the selected plan from the current note.
func (m *Model) promoteToJobCmd() tea.Cmd {
	note := m.noteToPromote
	if note == nil {
		return func() tea.Msg {
			return notePromotedToJobMsg{err: fmt.Errorf("no note selected for promotion")}
		}
	}

	// Get selected plan from picker
	selected := m.planPicker.SelectedItem()
	if selected == nil {
		return func() tea.Msg {
			return notePromotedToJobMsg{err: fmt.Errorf("no plan selected")}
		}
	}
	pi := selected.(planItem)
	planPath := pi.path
	planName := pi.name

	return func() tea.Msg {
		jobFilename, err := m.service.PromoteNoteToJob(note.Path, planPath, service.PromoteOptions{})
		if err != nil {
			return notePromotedToJobMsg{err: err}
		}
		return notePromotedToJobMsg{planName: planName, jobFile: jobFilename}
	}
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

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:gocyclo
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case confirm.ConfirmedMsg:
		// User confirmed the action in the dialog
		// We need to know which action was confirmed. A simple way is to check the prompt.
		if strings.Contains(strings.ToLower(m.confirmDialog.Prompt), "auto-archive") {
			m.service.Logger.WithField("count", len(m.autoArchivePaths)).Info("Auto-archiving stale notes")
			m.statusMessage = "Auto-archiving..."
			return m, m.autoArchiveStaleNotesCmd()
		}
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
		// Track the file path for dedup in updatePreviewContent.
		// The actual preview rendering is handled by the terminal
		// host's VDrawer (nvim -R), not the internal viewport.
		m.previewFile = msg.path
		return m, nil
	case embed.EditFinishedMsg:
		// External editor closed — refresh the tree to pick up any
		// changes (modified time, title, new files).
		return m, func() tea.Msg { return refreshMsg{} }
	case embed.SplitEditorClosedMsg:
		// BSP split editor closed — refresh to pick up edits.
		return m, func() tea.Msg { return refreshMsg{} }
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetSize(msg.Width, msg.Height)

		// Calculate pane sizes — preview is handled by the terminal host
		// VDrawer, so nb always uses full width.
		// header(1) + search(1) + blank(1) + view + blank(1) + status(1) + footer(1) + top_margin(1)
		const mainContentHeight = 7
		availableHeight := m.height - mainContentHeight
		m.views.SetSize(m.width-4, availableHeight)

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
		m.jobs = msg.jobs
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
		if m.clipboardMode == "cut" { //nolint:goconst
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

		// Clear any staged auto-archive paths now that they've been processed.
		m.autoArchivePaths = nil

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

	case notePromotedToJobMsg:
		m.noteToPromote = nil
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error promoting note: %v", msg.err)
			return m, nil
		}
		m.statusMessage = fmt.Sprintf("Promoted to job in %s/%s", msg.planName, msg.jobFile)
		// Refresh notes to reflect the archived note
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
			if msg.String() == "esc" { //nolint:goconst
				// Vim-style (mirrors nav/pkg/tui/sessionizer/update.go KeyEsc
				// handler): Esc blurs the input but PRESERVES the filter value so
				// the user can navigate the filtered results. A second Esc (while
				// blurred) clears it — handled in the m.keys.Back case below.
				m.filterInput.Blur()
				return m, nil
			}
			if msg.String() == "enter" { //nolint:goconst
				m.filterInput.Blur()
				// Reveal: accepting a non-empty search lands the cursor on the
				// first matching note instead of wherever it was clamped.
				if m.filterInput.Value() != "" {
					m.views.CursorToFirstNote()
				}
				return m, nil
			}
			// Pass all other keys to the input, then re-sync. updateViewsState
			// parses the input prefix and routes to grep/tag/plain itself, so we
			// no longer special-case grep here.
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.updateViewsState()
			return m, cmd
		}

		// Preview focus is handled by the terminal host (VDrawer),
		// not internally. Tab keybind is kept for future use.

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
				// Apply the selected tag filter by inserting the "#tag " prefix
				// into the single search input (reconciled model). updateViewsState
				// parses the prefix; the input stays blurred so the user can
				// navigate results immediately, and pressing the search/re-enter
				// key lets them append a within-tag query.
				if selectedItem, ok := m.tagPicker.SelectedItem().(tagItem); ok {
					m.tagPickerMode = false
					m.filterInput.SetValue("#" + selectedItem.tag + " ")
					m.filterInput.CursorEnd()
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

		// Handle plan picker mode (promote to job)
		if m.isPromotingToJob {
			switch msg.String() {
			case "esc":
				m.isPromotingToJob = false
				m.noteToPromote = nil
				return m, nil
			case "enter":
				m.isPromotingToJob = false
				m.statusMessage = "Promoting note to job..."
				return m, m.promoteToJobCmd()
			default:
				m.planPicker, cmd = m.planPicker.Update(msg)
				return m, cmd
			}
		}

		// Handle column selection mode
		if m.columnSelectMode {
			switch msg.String() {
			case "enter", "esc":
				// (toggle-columns is now the `tc` chord; the old flat "V" close key
				// is gone with it.)
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

		// Chord seam via the reusable which-key host. `extra` carries the flat
		// sequence chords — the gg motion, dd (delete), the z* folds — plus yy
		// (Copy). The disabled Base.Yank is deliberately OMITTED: Matches ignores
		// Enabled(), so leaving it in would race Copy for "yy". The host also arms
		// the t…/g… namespaces from Namespaces(). Top-level-only arming (E3) comes
		// free — every modal early-return above runs first, so a chord can never
		// arm in search/create/rename/commit/tagPicker/promote/columnSelect/help.
		extra := []key.Binding{
			m.keys.Top, m.keys.Delete,
			m.keys.FoldOpen, m.keys.FoldClose, m.keys.FoldToggle,
			m.keys.FoldOpenAll, m.keys.FoldCloseAll,
			m.keys.Copy,
		}
		res, matched, chordCmd := m.whichKey.ProcessChord(msg, extra...)
		switch res {
		case keymap.ChordPending:
			// A namespace prefix (t/g) returns the delayed popup tick; a flat
			// prefix (z/d/y/g-for-gg) returns nil. Either way, wait for the
			// next key — the buffer stays armed inside the shared host.
			return m, chordCmd
		case keymap.ChordConsumed:
			// esc dismissed the popup, or a stray key closed an armed namespace
			// menu — swallow it (so "t" then "x" doesn't fire a flat action).
			return m, nil
		case keymap.ChordMatched:
			// Re-synthesize the completed chord's canonical key so the dispatch
			// below resolves it via key.Matches.
			if len(matched.Keys()) > 0 {
				msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(matched.Keys()[0])}
			}
			// gg (top) and the z* folds are executed by the views sub-model,
			// which runs its OWN sequence engine. Hand it the combined chord key
			// so that engine resolves it in one shot; the browser-level dispatch
			// below only handles the flat + namespace actions (dd, ta…tp, ga/gv, yy).
			if _, isViews := keymap.MatchesAny(msg.String(),
				m.keys.Top, m.keys.FoldOpen, m.keys.FoldClose,
				m.keys.FoldToggle, m.keys.FoldOpenAll, m.keys.FoldCloseAll); isViews {
				m.views, cmd = m.views.Update(msg)
				if err := m.saveState(); err != nil {
					m.statusMessage = "Failed to save fold state: " + err.Error()
				}
				return m, tea.Batch(cmd, m.updatePreviewContent())
			}
			// Otherwise fall through to the flat/namespace switch below.
		case keymap.ChordNone:
			// Not a chord — fall through to single-key nav delegation + the switch.
		}

		// Single-key navigation and selection is delegated to the views sub-model.
		// (Multi-key fold/top/dd/yy chords are handled by the host seam above.)
		if key.Matches(msg, m.keys.Up) || key.Matches(msg, m.keys.Down) ||
			key.Matches(msg, m.keys.Left) || key.Matches(msg, m.keys.Right) ||
			key.Matches(msg, m.keys.PageUp) || key.Matches(msg, m.keys.PageDown) ||
			key.Matches(msg, m.keys.Bottom) ||
			key.Matches(msg, m.keys.Select) ||
			key.Matches(msg, m.keys.SelectNone) {
			// Single-key Left/Right change folds; persist the collapse state after.
			isFoldChange := key.Matches(msg, m.keys.Left) || key.Matches(msg, m.keys.Right)
			m.views, cmd = m.views.Update(msg)
			if isFoldChange {
				// Persist collapse state so folds survive a restart.
				if err := m.saveState(); err != nil {
					m.statusMessage = "Failed to save fold state: " + err.Error()
				}
			}
			// After any view update that could change the cursor, update the preview.
			return m, tea.Batch(cmd, m.updatePreviewContent())
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, func() tea.Msg { return embed.CloseRequestMsg{} }
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
				// Recent and archive views are mutually exclusive; clear archive
				// without re-saving (the saved state belongs to the default view).
				m.archiveViewMode = false
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
		case key.Matches(msg, m.keys.FocusArchive):
			// Only the default view's state should be saved for restoration. If
			// recent mode is currently active its TableView/column overrides are
			// already applied, so we switch directly without re-saving.
			cameFromRecent := m.recentNotesMode
			m.recentNotesMode = false
			m.archiveViewMode = !m.archiveViewMode
			if m.archiveViewMode {
				if !cameFromRecent {
					// Save current (default) state and switch to the archive list
					m.savedViewMode = m.views.GetViewMode()
					m.savedModVisibility = m.columnVisibility["MODIFIED"]
					m.savedWsVisibility = m.columnVisibility["WORKSPACE"]
				}
				m.views.SetViewMode(views.TableView)
				m.columnVisibility["MODIFIED"] = true
				m.columnVisibility["WORKSPACE"] = true
				m.views.SetColumnVisibility(m.columnVisibility)
				m.statusMessage = "Archive view (.archive/.closed)"
			} else {
				// Restore previous state
				m.views.SetViewMode(m.savedViewMode)
				m.columnVisibility["MODIFIED"] = m.savedModVisibility
				m.columnVisibility["WORKSPACE"] = m.savedWsVisibility
				m.views.SetColumnVisibility(m.columnVisibility)
				m.statusMessage = "Default view restored"
			}
			m.updateViewsState()
		case key.Matches(msg, m.keys.JumpToArtifacts):
			if m.views.JumpToArtifactsForNote() {
				m.statusMessage = "Jumped to job artifacts"
			} else {
				m.statusMessage = "No artifacts for this note"
			}
		case key.Matches(msg, m.keys.Search):
			// The search key both starts a new search AND re-enters an existing
			// one (vim-style). When the filter input is blurred-but-active (has a
			// value), '/' re-focuses it WITHOUT clearing — preserving the value so
			// the user can keep editing the same query. The Esc-preserve /
			// second-Esc-clear flow stays intact because we never reset here.
			if m.filterInput.Value() != "" {
				m.filterInput.CursorEnd()
				m.filterInput.Focus()
				return m, textinput.Blink
			}
			// No active filter: start fresh. If a tag filter is active, preserve
			// it by keeping the "#tag " prefix and letting the user append a
			// within-tag query; otherwise start from an empty input.
			if m.isFilteringByTag && m.selectedTag != "" {
				m.filterInput.SetValue("#" + m.selectedTag + " ")
				m.filterInput.CursorEnd()
				m.filterInput.Placeholder = fmt.Sprintf("Search in tag '%s'...", m.selectedTag)
			} else {
				m.filterInput.SetValue("")
				m.filterInput.Placeholder = "Search notes... (prefix ? to grep, # to tag)"
			}
			m.filterInput.Focus()
			return m, textinput.Blink
		case key.Matches(msg, m.keys.ReEnterSearch):
			// Vim-style: 'i' re-enters search (insert mode) if filter has value
			if m.filterInput.Value() != "" {
				m.filterInput.CursorEnd()
				m.filterInput.Focus()
				return m, textinput.Blink
			}
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
		case key.Matches(msg, m.keys.AutoArchive):
			// MANUAL auto-archive: stage notes older than 30 days (by ModTime)
			// and ask for confirmation. Never runs on startup.
			m.autoArchivePaths = m.collectStaleNotePaths(autoArchiveMaxAge)
			if len(m.autoArchivePaths) == 0 {
				m.statusMessage = "No notes older than 30 days to archive"
				return m, nil
			}
			m.confirmDialog.Activate(
				fmt.Sprintf("Auto-archive %d notes older than 30 days?", len(m.autoArchivePaths)),
			)
			return m, nil
		case key.Matches(msg, m.keys.FilterByTag):
			// Always show the tag picker - allows switching between tags. On
			// selection it inserts a "#tag " prefix into the single search input.
			m.tagPickerMode = true
			m.populateTagPicker()
			return m, nil
		case key.Matches(msg, m.keys.Grep):
			// Reconciled into the single search input: grep is the "?" prefix.
			// Pre-fill it and focus; updateViewsState parses the prefix and runs
			// the content grep.
			m.filterInput.SetValue("?")
			m.filterInput.CursorEnd()
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
		case key.Matches(msg, m.keys.PriorityUp):
			return m, m.bumpSelectedPriority(true)
		case key.Matches(msg, m.keys.PriorityDown):
			return m, m.bumpSelectedPriority(false)
		case key.Matches(msg, m.keys.CycleGrouping):
			m.groupBy = nextGroupBy(m.groupBy)
			m.views.SetGroupBy(m.groupBy)
			if m.groupBy == "none" {
				m.statusMessage = "Group by: none"
			} else {
				m.statusMessage = "Group by: " + m.groupBy
			}
			m.updateViewsState()
			if err := m.saveState(); err != nil {
				m.statusMessage = "Failed to save group-by: " + err.Error()
			}
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
		case key.Matches(msg, m.keys.Delete):
			// dd — the chord seam re-synthesizes the completed "dd" here (the first
			// "d" press was consumed as ChordPending above).
			pathsToDelete := m.views.GetTargetedNotePaths()
			if len(pathsToDelete) > 0 {
				prompt := fmt.Sprintf("Permanently delete %d note(s)? This cannot be undone.", len(pathsToDelete))
				m.confirmDialog.Activate(prompt)
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
		case key.Matches(msg, m.keys.Yank), key.Matches(msg, m.keys.CopyPath):
			node := m.views.GetCurrentNode()
			if node != nil && node.Item != nil {
				path := node.Item.Path
				// For workspace roots, get the notebook workspace path (parent of notes dir)
				if node.Item.Type == tree.TypeWorkspace {
					for _, ws := range m.workspaces {
						if ws.Name == node.Item.Name {
							if notesDir, err := m.service.GetNotebookLocator().GetNotesDir(ws, ""); err == nil {
								path = filepath.Dir(notesDir) // Parent of notes dir is workspace root
							}
							break
						}
					}
				}
				if err := clipboard.WriteAll(path); err != nil {
					m.statusMessage = fmt.Sprintf("Error copying path: %v", err)
				} else {
					m.statusMessage = fmt.Sprintf("Copied: %s", shortenPath(path))
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
			m.noteCreationMode = "inbox" //nolint:goconst
			m.noteCreationStep = 0       // Start with type picker
			m.noteCreationCursor = m.views.GetCursor()
			m.noteTitleInput.SetValue("")
			return m, nil
		case key.Matches(msg, m.keys.CreateNoteGlobal):
			// Global note creation: show type picker, always create in global
			m.isCreatingNote = true
			m.noteCreationMode = "global" //nolint:goconst
			m.noteCreationStep = 0        // Start with type picker
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
		case key.Matches(msg, m.keys.PromoteToJob):
			node := m.views.GetCurrentNode()
			if node != nil && node.IsNote() {
				note := views.ItemToNote(node.Item)
				m.noteToPromote = note
				if err := m.populatePlanPicker(); err != nil {
					m.statusMessage = fmt.Sprintf("Cannot promote: %s", err)
					m.noteToPromote = nil
					return m, nil
				}
				m.isPromotingToJob = true
				return m, nil
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
					path := noteToOpen.Path
					// enter: dedicated open — the host pins the note to its
					// own per-file editor pane (rail identity stays this note).
					return m, func() tea.Msg {
						return embed.EditRequestMsg{Path: path, Dedicated: true}
					}
				}
			}
		case key.Matches(msg, m.keys.Edit): // e - quick edit in the host's singleton Editor
			node := m.views.GetCurrentNode()
			if node != nil && node.IsNote() {
				note := views.ItemToNote(node.Item)
				if note != nil {
					path := note.Path
					// Quick open: the host routes it into the persistent
					// "Editor" pane, replacing the buffer shown there.
					return m, func() tea.Msg {
						return embed.EditRequestMsg{Path: path}
					}
				}
			}
		case key.Matches(msg, m.keys.TogglePreview): // v - split preview mode
			node := m.views.GetCurrentNode()
			if node == nil || !node.IsNote() {
				return m, nil
			}
			noteToPreview := views.ItemToNote(node.Item)
			if noteToPreview == nil {
				return m, nil
			}
			path := noteToPreview.Path
			m.previewVisible = !m.previewVisible
			if !m.previewVisible {
				m.previewFocused = false
				m.previewFile = ""
				if strings.Contains(m.statusMessage, "Previewing") || strings.Contains(m.statusMessage, "Loading") {
					m.statusMessage = ""
				}
				if m.hosted {
					return m, func() tea.Msg {
						return embed.SplitEditorCloseRequestMsg{}
					}
				}
				return m, func() tea.Msg {
					return embed.PreviewRequestMsg{Path: ""}
				}
			}
			m.previewFile = path
			if m.hosted {
				return m, func() tea.Msg {
					return embed.SplitEditorRequestMsg{Path: path, Ratio: 0.35, Focus: false}
				}
			}
			return m, func() tea.Msg {
				return embed.PreviewRequestMsg{Path: path}
			}
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
			return m, toggleStageFilesCmd(m.service, paths, m.gitFileStatus)
		case key.Matches(msg, m.keys.GitStageAll):
			// Stage all changes
			return m, stageAllCmd(m.service, m.allItems)
		case key.Matches(msg, m.keys.GitUnstageAll):
			// Unstage all changes
			return m, unstageAllCmd(m.service, m.allItems)
		case key.Matches(msg, m.keys.Back):
			if m.previewVisible {
				m.previewVisible = false
				m.previewFocused = false
				m.previewFile = ""
				if strings.Contains(m.statusMessage, "Previewing") || strings.Contains(m.statusMessage, "Loading") {
					m.statusMessage = ""
				}
				if m.hosted {
					return m, func() tea.Msg {
						return embed.SplitEditorCloseRequestMsg{}
					}
				}
				return m, func() tea.Msg {
					return embed.PreviewRequestMsg{Path: ""}
				}
			}
			if m.ecosystemPickerMode {
				m.ecosystemPickerMode = false
				m.updateViewsState()
				return m, nil
			}
			// Second Esc (filter blurred but still active): clear the preserved
			// filter. The first Esc only blurs (preserves) — see the
			// m.filterInput.Focused() Esc handler above. Mirrors nav's teardown.
			// The single input now carries every mode (plain text, "?" grep,
			// "#tag"), so clearing the value via updateViewsState resets all the
			// derived mode flags too.
			if m.filterInput.Value() != "" {
				wasTag := m.isFilteringByTag
				m.filterInput.SetValue("")
				m.updateViewsState()
				if wasTag {
					m.statusMessage = "Tag filter cleared"
				} else {
					m.statusMessage = "Search cleared"
				}
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
			// Non-chord, non-action key: nothing to do. The shared host owns the
			// sequence buffer (cleared inside ProcessChord), so there is no
			// browser-level sequence state to reset here.
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
func (m Model) updateNoteCreation(msg tea.Msg) (tea.Model, tea.Cmd) {
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
				// Prefer the full on-disk group path over the display name: for
				// synthetic group-by buckets ("Today", "P0", "#tag") the name is
				// just a label, and for nested groups the name is only the leaf.
				if group, ok := node.Item.Metadata["Group"].(string); ok && group != "" {
					noteType = models.NoteType(group)
				} else if strings.Contains(node.Item.Path, ".synthetic-") {
					noteType = "inbox"
				} else {
					noteType = models.NoteType(node.Item.Name)
				}
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
func (m Model) updateNoteRename(msg tea.Msg) (tea.Model, tea.Cmd) {
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
					planNotePaths, err := m.service.ArchivePlanDirectory(wsCtx, planName)
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

// autoArchiveMaxAge is the staleness threshold for the manual auto-archive action.
const autoArchiveMaxAge = 30 * 24 * time.Hour

// collectStaleNotePaths returns the paths of note items whose ModTime is older
// than maxAge. It only considers actual notes (skips groups, workspaces, plan
// directories) so directories are never archived as a side effect.
func (m *Model) collectStaleNotePaths(maxAge time.Duration) []string {
	cutoff := time.Now().Add(-maxAge)
	var paths []string
	for _, item := range m.allItems {
		if item == nil || item.IsDir {
			continue
		}
		if item.Type != tree.TypeNote {
			continue
		}
		if item.ModTime.Before(cutoff) {
			paths = append(paths, item.Path)
		}
	}
	return paths
}

// autoArchiveStaleNotesCmd archives the paths staged by the manual auto-archive
// action, grouped by workspace, reusing Service.ArchiveNotes.
func (m *Model) autoArchiveStaleNotesCmd() tea.Cmd {
	staged := m.autoArchivePaths
	return func() tea.Msg {
		if len(staged) == 0 {
			return notesArchivedMsg{err: fmt.Errorf("no stale notes to archive")}
		}

		// Group staged paths by workspace name (via allItems metadata).
		pathToWorkspace := make(map[string]string)
		for _, item := range m.allItems {
			if wsName, ok := item.Metadata["Workspace"].(string); ok {
				pathToWorkspace[item.Path] = wsName
			}
		}

		pathsByWorkspace := make(map[string][]string)
		for _, p := range staged {
			ws := pathToWorkspace[p]
			pathsByWorkspace[ws] = append(pathsByWorkspace[ws], p)
		}

		var archivedPaths []string
		var archiveErr error
		for workspaceName, paths := range pathsByWorkspace {
			var wsCtx *service.WorkspaceContext
			var err error
			if workspaceName == "global" || workspaceName == "" {
				wsCtx, err = m.service.GetWorkspaceContext("global")
			} else {
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
			if err := m.service.ArchiveNotes(wsCtx, paths); err != nil {
				archiveErr = fmt.Errorf("failed to archive notes in workspace %s: %w", workspaceName, err)
				break
			}
			archivedPaths = append(archivedPaths, paths...)
		}

		return notesArchivedMsg{
			archivedPaths: archivedPaths,
			archivedPlans: 0,
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
	_, _ = m.views.ApplyGrepFilter()
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
func (m Model) updateCommitDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	svc := m.service
	items := m.allItems

	return func() tea.Msg {
		// Find a git repo from the current items
		var gitRoot string
		for _, item := range items {
			root, err := svc.FindGitRoot(item.Path)
			if err == nil && root != "" {
				gitRoot = root
				break
			}
		}

		if gitRoot == "" {
			return commitFinishedMsg{success: false, message: "No git repository found", err: nil}
		}

		// Commit only what's already staged (no auto-staging)
		if err := svc.GitCommit(gitRoot, message); err != nil {
			if err == service.ErrNothingToCommit {
				return commitFinishedMsg{success: false, message: "Nothing to commit", err: nil}
			}
			return commitFinishedMsg{success: false, message: "", err: err}
		}

		return commitFinishedMsg{success: true, message: message, err: nil}
	}
}
