package sync

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattsolo1/grove-notebook/pkg/frontmatter"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

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

// findNoteBySyncID searches for a note with matching sync metadata.
func (s *Syncer) findNoteBySyncID(ctx *service.WorkspaceContext, syncID, provider string) (*models.Note, error) {
	// Get all notes in workspace, including archived, but not artifacts.
	notes, err := s.svc.ListAllNotes(ctx, true, false)
	if err != nil {
		return nil, err
	}

	// Search for matching SyncProvider and SyncID
	for _, note := range notes {
		if note.SyncProvider == provider && note.SyncID == syncID {
			return note, nil
		}
	}

	return nil, nil // Not found
}

// needsUpdate checks if a note needs updating based on the remote item.
func (s *Syncer) needsUpdate(note *models.Note, item *Item) bool {
	// Compare UpdatedAt timestamps. If the remote item's timestamp is after the
	// note's last sync timestamp, it needs an update.
	return item.UpdatedAt.After(note.SyncUpdatedAt)
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

	// Fetch remote items
	remoteItems, err := provider.Sync(providerConfig, repoPath)
	if err != nil {
		return nil, fmt.Errorf("provider %s sync failed: %w", provider.Name(), err)
	}

	// Process each remote item
	for _, item := range remoteItems {
		var noteType models.NoteType
		if item.Type == "issue" && config.IssuesType != "" {
			noteType = models.NoteType(config.IssuesType)
		} else if (item.Type == "pr" || item.Type == "pull_request") && config.PRsType != "" {
			noteType = models.NoteType(config.PRsType)
		} else {
			continue // This item type is not configured for sync
		}

		// Check if note exists (by SyncID)
		existingNote, err := s.findNoteBySyncID(ctx, item.ID, provider.Name())
		if err != nil {
			report.Failed++
			continue
		}

		if existingNote == nil {
			// Create new note
			if err := s.createNoteFromItem(ctx, item, noteType); err != nil {
				report.Failed++
			} else {
				report.Created++
			}
		} else {
			// Check if update is needed
			if s.needsUpdate(existingNote, item) {
				if err := s.updateNoteFromItem(existingNote, item); err != nil {
					report.Failed++
				} else {
					report.Updated++
				}
			} else {
				report.Unchanged++
			}
		}
	}

	return report, nil
}

// createNoteFromItem creates a new note from a sync.Item.
func (s *Syncer) createNoteFromItem(ctx *service.WorkspaceContext, item *Item, noteType models.NoteType) error {
	fm := s.buildFrontmatter(item)
	body := fmt.Sprintf("# %s\n\n%s", item.Title, item.Body)
	_, err := s.svc.CreateNoteWithContent(ctx, noteType, item.Title, fm, body)
	return err
}

// updateNoteFromItem updates an existing note from a sync.Item.
func (s *Syncer) updateNoteFromItem(note *models.Note, item *Item) error {
	fm := s.buildFrontmatter(item)
	body := fmt.Sprintf("# %s\n\n%s", item.Title, item.Body)
	return s.svc.UpdateNoteWithContent(note.Path, fm, body)
}

func (s *Syncer) buildFrontmatter(item *Item) *frontmatter.Frontmatter {
	return &frontmatter.Frontmatter{
		Title:         item.Title,
		Tags:          item.Labels,
		Created:       frontmatter.FormatTimestamp(item.UpdatedAt),
		Modified:      frontmatter.FormatTimestamp(time.Now()),
		SyncProvider:  "github",
		SyncID:        item.ID,
		SyncURL:       item.URL,
		SyncState:     strings.ToLower(item.State),
		SyncUpdatedAt: item.UpdatedAt.Format(time.RFC3339),
	}
}
