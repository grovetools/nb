package sync

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mattsolo1/grove-notebook/pkg/frontmatter"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

const syncMarker = "<!-- nb-sync-marker -->"

// ProviderFactory is a function that creates a Provider instance.
type ProviderFactory func() Provider

// Syncer orchestrates the synchronization process.
type Syncer struct {
	svc              *service.Service
	providerFactories map[string]ProviderFactory
}

// NewSyncer creates a new Syncer.
func NewSyncer(svc *service.Service) *Syncer {
	return &Syncer{
		svc:              svc,
		providerFactories: make(map[string]ProviderFactory),
	}
}

// RegisterProvider registers a provider factory for a given provider name.
func (s *Syncer) RegisterProvider(name string, factory ProviderFactory) {
	s.providerFactories[name] = factory
}

// findNoteByRemoteID searches for a note with matching remote metadata.
func (s *Syncer) findNoteByRemoteID(ctx *service.WorkspaceContext, remoteID, provider string) (*models.Note, error) {
	// Get all notes in workspace, including archived, but not artifacts.
	notes, err := s.svc.ListAllNotes(ctx, true, false)
	if err != nil {
		return nil, err
	}

	// Search for matching Remote.Provider and Remote.ID
	for _, note := range notes {
		if note.Remote != nil && note.Remote.Provider == provider && note.Remote.ID == remoteID {
			return note, nil
		}
	}

	return nil, nil // Not found
}

// needsUpdate checks if a note needs updating based on the remote item.
func (s *Syncer) needsUpdate(note *models.Note, item *Item) bool {
	// Compare UpdatedAt timestamps. If the remote item's timestamp is after the
	// note's last sync timestamp, it needs an update.
	if note.Remote == nil {
		return true
	}
	return item.UpdatedAt.After(note.Remote.UpdatedAt)
}

// SyncWorkspace syncs a given workspace with its configured remote providers.
func (s *Syncer) SyncWorkspace(ctx *service.WorkspaceContext) ([]*Report, error) {
	// Get notebook name from config
	notebookName := "default"
	if s.svc.CoreConfig != nil && s.svc.CoreConfig.Notebooks != nil && s.svc.CoreConfig.Notebooks.Rules != nil {
		notebookName = s.svc.CoreConfig.Notebooks.Rules.Default
	}

	// Get sync config for this notebook
	syncConfigs, err := GetSyncConfigForNotebook(s.svc.CoreConfig, notebookName)
	if err != nil {
		return nil, fmt.Errorf("failed to read sync config: %w", err)
	}

	// If no sync configured, return empty
	if len(syncConfigs) == 0 {
		return []*Report{}, nil
	}

	var allReports []*Report
	for _, config := range syncConfigs {
		factory, ok := s.providerFactories[config.Provider]
		if !ok {
			// Unsupported or unregistered provider
			fmt.Printf("Unsupported or unregistered provider: %s\n", config.Provider)
			continue
		}

		provider := factory()

		report, err := s.syncWithProvider(ctx, provider, config)
		if err != nil {
			// Log error but continue with other providers
			fmt.Printf("Error syncing with provider %s: %v\n", provider.Name(), err)
			continue
		}
		allReports = append(allReports, report)
	}

	return allReports, nil
}

// syncWithProvider handles the sync logic for a single configured provider.
func (s *Syncer) syncWithProvider(
	ctx *service.WorkspaceContext,
	provider Provider,
	config SyncConfig,
) (*Report, error) {
	report := &Report{Provider: provider.Name()}
	repoPath := ctx.CurrentWorkspace.Path

	providerConfig := map[string]string{
		"issues_type": config.IssuesType,
		"prs_type":    config.PRsType,
	}

	// 1. Fetch remote items and map them by ID
	remoteItems, err := provider.Sync(providerConfig, repoPath)
	if err != nil {
		return nil, fmt.Errorf("provider %s sync failed: %w", provider.Name(), err)
	}
	remoteItemsMap := make(map[string]*Item)
	for _, item := range remoteItems {
		remoteItemsMap[item.ID] = item
	}

	// 2. Fetch local notes with remote metadata and map them by ID
	localNotes, err := s.svc.ListAllNotes(ctx, true, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list local notes: %w", err)
	}
	localNotesMap := make(map[string]*models.Note)
	allRemoteIDs := make(map[string]bool)
	for _, note := range localNotes {
		if note.Remote != nil && note.Remote.Provider == provider.Name() {
			localNotesMap[note.Remote.ID] = note
			allRemoteIDs[note.Remote.ID] = true
		}
	}
	for id := range remoteItemsMap {
		allRemoteIDs[id] = true
	}

	// 3. Iterate and sync
	for id := range allRemoteIDs {
		localNote, localExists := localNotesMap[id]
		remoteItem, remoteExists := remoteItemsMap[id]

		switch {
		// Case 1: Exists locally and remotely -> compare and sync
		case localExists && remoteExists:
			// Get the actual file modification time (not from frontmatter)
			fileInfo, err := os.Stat(localNote.Path)
			if err != nil {
				report.Failed++
				continue
			}
			fileMtime := fileInfo.ModTime()

			// Compare remote's UpdatedAt with local file modification time
			// If remote is newer, pull. If local is newer, push.
			if remoteItem.UpdatedAt.After(fileMtime) {
				// Remote is newer ("pull")
				if s.needsUpdate(localNote, remoteItem) {
					if err := s.updateNoteFromItem(localNote, remoteItem); err != nil {
						report.Failed++
					} else {
						report.Updated++
					}
				} else {
					report.Unchanged++
				}
			} else if fileMtime.After(remoteItem.UpdatedAt) {
				// Local is newer ("push")
				// Check for new local comments to push
				content, err := os.ReadFile(localNote.Path)
				if err != nil {
					report.Failed++
					continue
				}
				contentStr := string(content)

				localComment := ""
				if markerIndex := strings.Index(contentStr, syncMarker); markerIndex != -1 {
					localComment = strings.TrimSpace(contentStr[markerIndex+len(syncMarker):])
				}

				if localComment != "" {
					itemType := "issue"
					if localNote.Remote.Provider == "github" {
						noteTypeStr := string(localNote.Type)
						if strings.Contains(noteTypeStr, "pr") || strings.Contains(noteTypeStr, "pull") {
							itemType = "pr"
						}
					}
					err := provider.AddComment(itemType, localNote.Remote.ID, localComment, repoPath)
					if err != nil {
						report.Failed++
					} else {
						// Comment pushed, now update local state
						updatedRemoteItem, err := provider.GetItem(itemType, localNote.Remote.ID, repoPath)
						if err != nil {
							report.Failed++
						} else {
							// Update local note with new sync state from remote, clearing local content since we just pushed it
							if err := s.updateNoteFromItemPreserveLocal(localNote, updatedRemoteItem, false); err != nil {
								report.Failed++
							} else {
								report.Updated++
							}
						}
					}
				} else {
					// No local comment, but file is modified.
					// Rebuild the synced section from remote to restore any deleted comments
					// and preserve any local content after the marker.
					if err := s.updateNoteFromItem(localNote, remoteItem); err != nil {
						report.Failed++
					} else {
						report.Updated++
					}
				}
			} else {
				// Timestamps are equal, no update needed
				report.Unchanged++
			}

		// Case 2: Exists only remotely -> create locally
		case !localExists && remoteExists:
			var noteType models.NoteType
			if remoteItem.Type == "issue" && config.IssuesType != "" {
				noteType = models.NoteType(config.IssuesType)
			} else if (remoteItem.Type == "pr" || remoteItem.Type == "pull_request") && config.PRsType != "" {
				noteType = models.NoteType(config.PRsType)
			} else {
				continue // Skip
			}
			_, err := s.createNoteFromItem(ctx, remoteItem, noteType)
			if err != nil {
				report.Failed++
			} else {
				report.Created++
			}

		// Case 3: Exists only locally -> remote was deleted
		case localExists && !remoteExists:
			// For now, we do nothing. Deletion sync is a future feature.
			report.Unchanged++
		}
	}

	return report, nil
}

// formatComments formats a slice of comments into a markdown string.
func formatComments(comments []*Comment) string {
	var sb strings.Builder
	for _, comment := range comments {
		sb.WriteString("\n\n---\n\n")
		sb.WriteString(fmt.Sprintf("> **%s** commented at %s:\n\n", comment.Author, comment.CreatedAt.Format(time.RFC822)))
		sb.WriteString(comment.Body)
	}
	return sb.String()
}

// createNoteFromItem creates a new note from a sync.Item and returns the note path.
func (s *Syncer) createNoteFromItem(ctx *service.WorkspaceContext, item *Item, noteType models.NoteType) (string, error) {
	fm := s.buildFrontmatter(item)

	// Build the body with the main content, comments, and sync marker
	var bodyBuilder strings.Builder
	bodyBuilder.WriteString(fmt.Sprintf("# %s\n\n%s", item.Title, item.Body))
	bodyBuilder.WriteString(formatComments(item.Comments))
	bodyBuilder.WriteString("\n\n" + syncMarker)

	note, err := s.svc.CreateNoteWithContent(ctx, noteType, item.Title, fm, bodyBuilder.String())
	if err != nil {
		return "", err
	}
	return note.Path, nil
}

// updateNoteFromItem updates an existing note from a sync.Item.
func (s *Syncer) updateNoteFromItem(note *models.Note, item *Item) error {
	return s.updateNoteFromItemPreserveLocal(note, item, true)
}

// updateNoteFromItemPreserveLocal updates an existing note from a sync.Item with option to preserve local content.
func (s *Syncer) updateNoteFromItemPreserveLocal(note *models.Note, item *Item, preserveLocal bool) error {
	content, err := os.ReadFile(note.Path)
	if err != nil {
		return fmt.Errorf("could not read existing note content: %w", err)
	}
	contentStr := string(content)

	// Get last sync time from the note's frontmatter
	lastSyncTime := note.Remote.UpdatedAt

	// Find new comments
	var newComments []*Comment
	for _, comment := range item.Comments {
		if comment.CreatedAt.After(lastSyncTime) {
			newComments = append(newComments, comment)
		}
	}

	// Split content by sync marker to preserve local content
	localContent := ""
	if preserveLocal {
		if markerIndex := strings.Index(contentStr, syncMarker); markerIndex != -1 {
			localContent = contentStr[markerIndex+len(syncMarker):]
		}
	}

	// Rebuild the synced body from the remote item
	var syncedBodyBuilder strings.Builder
	syncedBodyBuilder.WriteString(fmt.Sprintf("# %s\n\n%s", item.Title, item.Body))

	// Add all existing comments up to the last sync time
	for _, comment := range item.Comments {
		if !comment.CreatedAt.After(lastSyncTime) {
			syncedBodyBuilder.WriteString("\n\n---\n\n")
			syncedBodyBuilder.WriteString(fmt.Sprintf("> **%s** commented at %s:\n\n", comment.Author, comment.CreatedAt.Format(time.RFC822)))
			syncedBodyBuilder.WriteString(comment.Body)
		}
	}

	// Add new comments
	if len(newComments) > 0 {
		syncedBodyBuilder.WriteString(formatComments(newComments))
	}

	// Rebuild body content
	var newBodyBuilder strings.Builder
	newBodyBuilder.WriteString(syncedBodyBuilder.String())
	newBodyBuilder.WriteString("\n\n" + syncMarker)
	newBodyBuilder.WriteString(localContent)
	newBody := newBodyBuilder.String()

	// Parse the original content to get and update the frontmatter
	fm, _, err := frontmatter.Parse(contentStr)
	if err != nil {
		// If parsing fails, create a new frontmatter struct
		fm = s.buildFrontmatter(item)
	} else {
		// Update existing frontmatter
		fm.Title = item.Title
		fm.Remote.State = strings.ToLower(item.State)
		fm.Remote.UpdatedAt = item.UpdatedAt.Format(time.RFC3339)
		fm.Modified = frontmatter.FormatTimestamp(item.UpdatedAt)
		fm.Remote.Labels = item.Labels
		fm.Remote.Assignees = item.Assignees
		fm.Remote.Milestone = item.Milestone
	}

	return s.svc.UpdateNoteWithContent(note.Path, fm, newBody)
}

// noteToSyncItem constructs a sync.Item from a local note file.
func (s *Syncer) noteToSyncItem(note *models.Note) (*Item, error) {
	content, err := os.ReadFile(note.Path)
	if err != nil {
		return nil, err
	}
	fm, body, err := frontmatter.Parse(string(content))
	if err != nil {
		return nil, err
	}

	// Determine type from note metadata
	itemType := "issue"
	noteTypeStr := string(note.Type)
	if strings.Contains(noteTypeStr, "pr") || strings.Contains(noteTypeStr, "pull") {
		itemType = "pr"
	}

	// Use freshly parsed frontmatter values to ensure we get the latest changes
	if fm.Remote == nil {
		return nil, fmt.Errorf("note does not have remote metadata")
	}

	state := fm.Remote.State
	url := fm.Remote.URL
	labels := fm.Remote.Labels
	assignees := fm.Remote.Assignees
	milestone := fm.Remote.Milestone

	return &Item{
		ID:        note.Remote.ID,
		Type:      itemType,
		Title:     fm.Title,
		Body:      body,
		State:     state,
		URL:       url,
		Labels:    labels,
		Assignees: assignees,
		Milestone: milestone,
		UpdatedAt: note.ModifiedAt,
	}, nil
}

// buildFrontmatter creates a Frontmatter struct from a sync.Item.
func (s *Syncer) buildFrontmatter(item *Item) *frontmatter.Frontmatter {
	fm := &frontmatter.Frontmatter{
		Title:    item.Title,
		Created:  frontmatter.FormatTimestamp(item.UpdatedAt),
		Modified: frontmatter.FormatTimestamp(item.UpdatedAt),
		Remote: &frontmatter.RemoteMetadata{
			Provider:  "github",
			ID:        item.ID,
			URL:       item.URL,
			State:     strings.ToLower(item.State),
			UpdatedAt: item.UpdatedAt.Format(time.RFC3339),
			Labels:    item.Labels,
			Assignees: item.Assignees,
			Milestone: item.Milestone,
		},
	}

	// IMPORTANT: We no longer populate the main `tags` field from remote labels.
	// The migration copies existing tags to remote.labels once.
	// Going forward, `tags` is for local use only.
	return fm
}
