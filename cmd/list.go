package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	coreconfig "github.com/grovetools/core/config"
	grovelogging "github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/daemon"
	coremodels "github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/tui/theme"

	"github.com/grovetools/nb/pkg/models"
	"github.com/grovetools/nb/pkg/service"
)

var listUlog = grovelogging.NewUnifiedLogger("grove-notebook.cmd.list")

func NewListCmd(svc **service.Service, workspaceOverride *string) *cobra.Command { //nolint:gocyclo
	var (
		listAll           bool
		listType          string
		listGlobal        bool
		listJSON          bool
		listAllWorkspaces bool
		listAllBranches   bool
		listTag           string
		listCounts        bool
		listPriority      string
		listCriticalOnly  bool
		listPlanRef       string
	)

	cmd := &cobra.Command{
		Use:     "list [type]",
		Short:   "List notes in current workspace",
		Aliases: []string{"ls"},
		Long: `List notes in the current workspace.

Examples:
  nb list              # List current notes
  nb list llm          # List LLM notes
  nb list learn        # List learning notes
  nb list docs         # List documentation notes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			s := *svc

			// Get workspace context, potentially overridden
			wsCtx, err := s.GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			// Resolve the effective priority filter. --critical-only is
			// shorthand for --priority p0; both may not conflict.
			priorityFilter := listPriority
			if listCriticalOnly {
				if priorityFilter != "" && priorityFilter != "p0" {
					return fmt.Errorf("--critical-only conflicts with --priority %s", priorityFilter)
				}
				priorityFilter = "p0"
			}
			if !service.IsValidPriority(priorityFilter) {
				return fmt.Errorf("invalid priority %q (want one of p0,p1,p2,p3 or empty)", priorityFilter)
			}

			// Handle --all-branches flag
			if listAllBranches {
				if wsCtx.NotebookContextWorkspace.IsWorktree() {
					return fmt.Errorf("--all-branches can only be used from a main project directory, not a worktree")
				}

				repoNotes, err := s.ListAllNotesInWorkspace(wsCtx.NotebookContextWorkspace)
				if err != nil {
					return err
				}

				// Filter by tag if provided
				if listTag != "" {
					var filteredNotes []*models.Note
					for _, note := range repoNotes {
						for _, tag := range note.Tags {
							if tag == listTag {
								filteredNotes = append(filteredNotes, note)
								break
							}
						}
					}
					repoNotes = filteredNotes
				}

				repoNotes = filterNotesByPriority(repoNotes, priorityFilter)
				repoNotes = filterNotesByPlanRef(repoNotes, listPlanRef)

				if len(repoNotes) == 0 {
					if !listJSON {
						listUlog.Info("No notes found in repository").
							Field("repository", wsCtx.NotebookContextWorkspace.Name).
							Pretty(fmt.Sprintf("No notes found in any branch of the '%s' repository", wsCtx.NotebookContextWorkspace.Name)).
							PrettyOnly().
							Log(ctx)
					} else {
						listUlog.Info("No notes found in repository").
							Field("repository", wsCtx.NotebookContextWorkspace.Name).
							Pretty("[]").
							PrettyOnly().
							Log(ctx)
					}
					return nil
				}

				if listJSON {
					return outputJSON(repoNotes)
				} else {
					printNotesTable(repoNotes, s.NoteTypes)
				}
				return nil
			}

			// Fast-path: --workspaces --counts reads cached counts from daemon
			if listAllWorkspaces && listCounts {
				client := daemon.NewWithAutoStart()
				defer client.Close()

				if client.IsRunning() {
					counts, err := client.GetNoteCounts(ctx)
					if err == nil && len(counts) > 0 {
						if listJSON {
							return outputJSONCounts(counts)
						}
						printCountsTable(counts)
						return nil
					}
				}
				// Fallback: daemon not available, fall through to filesystem walk
			}

			// Handle --workspaces flag first (list from all workspaces)
			if listAllWorkspaces {
				// Try daemon index first for fast listing
				allNotes, err := tryDaemonListNotes(ctx, "", listTag)
				if allNotes == nil && err == nil {
					// Fallback to filesystem
					allNotes, err = s.ListNotesFromAllWorkspaces(false, false)
				}
				if err != nil {
					return err
				}

				// Filter by tag if provided
				if listTag != "" {
					var filteredNotes []*models.Note
					for _, note := range allNotes {
						for _, tag := range note.Tags {
							if tag == listTag {
								filteredNotes = append(filteredNotes, note)
								break
							}
						}
					}
					allNotes = filteredNotes
				}

				allNotes = filterNotesByPriority(allNotes, priorityFilter)
				allNotes = filterNotesByPlanRef(allNotes, listPlanRef)

				if len(allNotes) == 0 {
					if !listJSON {
						listUlog.Info("No notes found across all workspaces").
							Pretty("No notes found across all workspaces").
							PrettyOnly().
							Log(ctx)
					} else {
						listUlog.Info("No notes found across all workspaces").
							Pretty("[]").
							PrettyOnly().
							Log(ctx)
					}
					return nil
				}

				// Output based on format
				if listJSON {
					return outputJSON(allNotes)
				} else {
					printNotesTable(allNotes, s.NoteTypes)
				}
				return nil
			}

			if listAll {
				// List all notes in all directories (including custom/nested types)
				var allNotes []*models.Note
				var err error

				// Try daemon index first
				wsFilter := wsCtx.NotebookContextWorkspace.Name
				if listGlobal {
					wsFilter = "global"
				}
				allNotes, err = tryDaemonListNotes(ctx, wsFilter, listTag)
				if allNotes == nil && err == nil {
					// Fallback to filesystem
					if listGlobal {
						allNotes, err = s.ListAllGlobalNotes(false, false)
					} else {
						allNotes, err = s.ListAllNotes(wsCtx, false, false)
					}
				}

				if err != nil {
					return err
				}

				// Filter by tag if provided
				if listTag != "" {
					var filteredNotes []*models.Note
					for _, note := range allNotes {
						for _, tag := range note.Tags {
							if tag == listTag {
								filteredNotes = append(filteredNotes, note)
								break
							}
						}
					}
					allNotes = filteredNotes
				}

				allNotes = filterNotesByPriority(allNotes, priorityFilter)
				allNotes = filterNotesByPlanRef(allNotes, listPlanRef)

				if len(allNotes) == 0 {
					if !listJSON {
						listUlog.Info("No notes found").
							Pretty("No notes found").
							PrettyOnly().
							Log(ctx)
					} else {
						listUlog.Info("No notes found").
							Pretty("[]").
							PrettyOnly().
							Log(ctx)
					}
					return nil
				}

				// Output based on format
				if listJSON {
					return outputJSON(allNotes)
				} else {
					printNotesTable(allNotes, s.NoteTypes)
				}
				return nil
			}

			// Original single-type listing
			noteType := listType
			if len(args) > 0 {
				noteType = args[0]
			}

			// Try the daemon's cached note index first; fall back to the
			// filesystem walk when the daemon is down or has nothing indexed
			// under the type's directory.
			notes, err := tryDaemonListNotesForType(ctx, s, wsCtx, models.NoteType(noteType))
			if notes == nil && err == nil {
				notes, err = s.ListNotes(wsCtx, models.NoteType(noteType))
			}
			if err != nil {
				return err
			}

			// Filter by tag if provided
			if listTag != "" {
				var filteredNotes []*models.Note
				for _, note := range notes {
					for _, tag := range note.Tags {
						if tag == listTag {
							filteredNotes = append(filteredNotes, note)
							break
						}
					}
				}
				notes = filteredNotes
			}

			notes = filterNotesByPriority(notes, priorityFilter)
			notes = filterNotesByPlanRef(notes, listPlanRef)

			if len(notes) == 0 {
				if listJSON {
					listUlog.Info("No notes found").
						Field("note_type", noteType).
						Pretty("[]").
						PrettyOnly().
						Log(ctx)
				} else {
					listUlog.Info("No notes found").
						Field("note_type", noteType).
						Pretty(fmt.Sprintf("No %s notes found", noteType)).
						PrettyOnly().
						Log(ctx)
				}
				return nil
			}

			// Output based on format
			if listJSON {
				return outputJSON(notes)
			} else {
				printNotesTable(notes, s.NoteTypes)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&listAll, "all", false, "List all note types")
	cmd.Flags().StringVarP(&listType, "type", "t", "inbox", "Note type to list")
	cmd.Flags().BoolVarP(&listGlobal, "global", "g", false, "List global notes only")
	cmd.Flags().BoolVar(&listJSON, "json", false, "Output in JSON format")
	cmd.Flags().BoolVarP(&listAllWorkspaces, "workspaces", "w", false, "List notes from all workspaces")
	cmd.Flags().BoolVar(&listAllBranches, "all-branches", false, "List notes from all branches in the current repository")
	cmd.Flags().StringVar(&listTag, "tag", "", "Filter notes by a specific tag")
	cmd.Flags().BoolVar(&listCounts, "counts", false, "Show aggregate counts per workspace (fast, uses daemon cache with --workspaces)")
	cmd.Flags().StringVar(&listPriority, "priority", "", "Filter notes by priority level: p0 (most critical) .. p3")
	cmd.Flags().BoolVar(&listCriticalOnly, "critical-only", false, "Show only p0 (critical) notes; shorthand for --priority p0")
	cmd.Flags().StringVar(&listPlanRef, "plan-ref", "", "Filter to notes whose plan_ref frontmatter exactly matches this value (e.g. plans/my-feature)")

	return cmd
}

func printNotesTable(notes []*models.Note, noteTypes map[string]*coreconfig.NoteTypeConfig) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Print header
	fmt.Fprintln(w, "TYPE\tDATE\tTITLE\tWORDS")
	fmt.Fprintln(w, "-------\t----------\t-----------------------------\t------")

	// Print each note
	for _, note := range notes {
		typeIcon := getNoteTypeIcon(noteTypes, note.Type)
		typeAbbrev := getTypeAbbreviation(note.Type)
		typeStr := fmt.Sprintf("%s %s", typeIcon, typeAbbrev)
		dateStr := note.ModifiedAt.Format("2006-01-02")
		titleStr := truncateString(note.Title, 29)
		wordsStr := fmt.Sprintf("%d", note.WordCount)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", typeStr, dateStr, titleStr, wordsStr)
	}

	w.Flush()
}

func getNoteTypeIcon(noteTypes map[string]*coreconfig.NoteTypeConfig, noteType models.NoteType) string {
	// Look up the icon from the NoteTypes registry
	if typeConfig, ok := noteTypes[string(noteType)]; ok && typeConfig.Icon != "" {
		return typeConfig.Icon
	}
	// Fallback to a generic note icon
	return theme.IconNote
}

func getTypeAbbreviation(noteType models.NoteType) string {
	switch noteType {
	case "current", "inbox":
		return "ibx"
	case "llm":
		return "llm"
	case "learn":
		return "lrn"
	case "daily":
		return "dly"
	case "issues":
		return "iss"
	case "github-prs":
		return "prs"
	case "architecture":
		return "arc"
	case "todos":
		return "tdo"
	case "quick":
		return "qui"
	case "blog":
		return "blg"
	case "prompts":
		return "pmt"
	case "docs":
		return "doc"
	default:
		// For unknown types, take first 3 chars
		typeStr := string(noteType)
		if len(typeStr) >= 3 {
			return typeStr[:3]
		}
		return typeStr
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// filterNotesByPriority returns only the notes whose Priority matches the
// requested level. An empty filter is a no-op (returns the input unchanged).
//
// The daemon note index does not carry the priority field, so notes sourced
// from the daemon have an empty Priority. When a filter is active we re-parse
// each note from disk to populate Priority before comparing. This keeps
// daemon-backed listings correct at the cost of a stat+parse per note, which
// only happens when the user explicitly filters.
func filterNotesByPriority(notes []*models.Note, priority string) []*models.Note {
	if priority == "" {
		return notes
	}
	filtered := make([]*models.Note, 0, len(notes))
	for _, note := range notes {
		p := note.Priority
		if p == "" {
			// Daemon-index notes lack priority; re-parse from disk.
			if parsed, err := service.ParseNote(note.Path); err == nil {
				p = parsed.Priority
			}
		}
		if p == priority {
			filtered = append(filtered, note)
		}
	}
	return filtered
}

// filterNotesByPlanRef returns only the notes whose PlanRef exactly matches the
// requested value. An empty filter is a no-op (returns the input unchanged).
//
// This is flow's seam for "give me this plan's notes": it works across every
// lifecycle group `nb list` surfaces and with `--json`. Like the priority
// filter, daemon-index notes normally carry plan_ref, but when a note's PlanRef
// is empty we re-parse it from disk before comparing so a stale/omitted index
// entry can't hide a match. The re-parse only happens when a filter is active.
func filterNotesByPlanRef(notes []*models.Note, planRef string) []*models.Note {
	if planRef == "" {
		return notes
	}
	filtered := make([]*models.Note, 0, len(notes))
	for _, note := range notes {
		ref := note.PlanRef
		if ref == "" || note.PlanJob == "" {
			// Daemon-index entries carry no plan_job, so a matched note must
			// also backfill it from disk — consumers key on it per-job.
			if parsed, err := service.ParseNote(note.Path); err == nil {
				if ref == "" {
					ref = parsed.PlanRef
				}
				if note.PlanJob == "" {
					note.PlanJob = parsed.PlanJob
				}
			}
		}
		if ref == planRef {
			filtered = append(filtered, note)
		}
	}
	return filtered
}

func outputJSON(notes []*models.Note) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(notes)
}

func outputJSONCounts(counts map[string]*coremodels.NoteCounts) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(counts)
}

// tryDaemonListNotes attempts to list notes from the daemon's cached index.
// Returns nil, nil if the daemon is unavailable (caller should fall back to filesystem).
// If tagFilter is non-empty, the --workspaces caller can skip its own tag filtering
// since we filter here.
func tryDaemonListNotes(ctx context.Context, wsFilter, tagFilter string) ([]*models.Note, error) {
	client := daemon.NewWithAutoStart()
	defer client.Close()

	if !client.IsRunning() {
		return nil, nil
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	entries, err := client.GetNoteIndex(fetchCtx, wsFilter)
	if err != nil || len(entries) == 0 {
		return nil, nil // Fallback to filesystem
	}

	notes := make([]*models.Note, 0, len(entries))
	for _, e := range entries {
		// Skip non-note types for nb list (artifacts, generic files)
		if e.Type != "note" {
			continue
		}

		// Apply tag filter if specified
		if tagFilter != "" {
			hasTag := false
			for _, t := range e.Tags {
				if t == tagFilter {
					hasTag = true
					break
				}
			}
			if !hasTag {
				continue
			}
		}

		notes = append(notes, noteFromIndexEntry(e))
	}

	if len(notes) == 0 {
		return nil, nil
	}
	return notes, nil
}

// noteFromIndexEntry converts a daemon NoteIndexEntry into an nb Note model.
func noteFromIndexEntry(e *coremodels.NoteIndexEntry) *models.Note {
	noteType := models.NoteType(e.Group)
	// For notes content dir, group is the subdirectory name (inbox, issues, etc.)
	// For plans/chats, strip the prefix
	if e.ContentDir == "plans" {
		noteType = "plan"
	} else if e.ContentDir == "chats" {
		noteType = "chat"
	} else if parts := strings.SplitN(e.Group, "/", 2); len(parts) > 0 {
		noteType = models.NoteType(parts[0])
	}

	return &models.Note{
		Path:             e.Path,
		Title:            filepath.Base(e.Path),
		FrontmatterTitle: e.Title,
		Type:             noteType,
		Group:            e.Group,
		Workspace:        e.Workspace,
		CreatedAt:        e.Created,
		ModifiedAt:       e.ModTime,
		ID:               e.ID,
		Tags:             e.Tags,
		PlanRef:          e.PlanRef,
	}
}

// tryDaemonListNotesForType lists a single note type from the daemon's cached
// index. It filters index entries by the path prefix of the directory that
// backs the type, which gives exact parity with the ListNotes filesystem walk
// (including nested subgroups and .archive contents).
// Returns nil, nil when the daemon is unavailable or nothing is indexed under
// the directory — the caller falls back to the filesystem walk.
func tryDaemonListNotesForType(ctx context.Context, s *service.Service, wsCtx *service.WorkspaceContext, noteType models.NoteType) ([]*models.Note, error) {
	dirPath, err := s.NoteTypeDir(wsCtx, noteType)
	if err != nil || dirPath == "" {
		return nil, nil
	}

	client := daemon.NewWithAutoStart()
	defer client.Close()

	if !client.IsRunning() {
		return nil, nil
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	entries, err := client.GetNoteIndex(fetchCtx, wsCtx.NotebookContextWorkspace.Name)
	if err != nil || len(entries) == 0 {
		return nil, nil // Fallback to filesystem
	}

	prefix := strings.TrimSuffix(dirPath, string(filepath.Separator)) + string(filepath.Separator)
	notes := make([]*models.Note, 0, len(entries))
	for _, e := range entries {
		if e.Type != "note" || !strings.HasPrefix(e.Path, prefix) {
			continue
		}
		note := noteFromIndexEntry(e)
		// Mirror the filesystem path: ListNotes stamps the requested type and
		// the context's workspace/branch on every result.
		note.Type = noteType
		note.Workspace = wsCtx.NotebookContextWorkspace.Name
		note.Branch = wsCtx.Branch
		notes = append(notes, note)
	}

	if len(notes) == 0 {
		return nil, nil
	}
	// The index is a map server-side; sort by path to match the
	// lexical order of filepath.Walk.
	sort.Slice(notes, func(i, j int) bool { return notes[i].Path < notes[j].Path })
	return notes, nil
}

func printCountsTable(counts map[string]*coremodels.NoteCounts) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "WORKSPACE\tINBOX\tISSUES\tDOCS\tREVIEW\tIN-PROG\tCOMPL\tOTHER")
	fmt.Fprintln(w, "---------\t-----\t------\t----\t------\t-------\t-----\t-----")

	// Sort workspace names for stable output
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		c := counts[name]
		total := c.Inbox + c.Issues + c.Docs + c.Review + c.InProgress + c.Completed + c.Other
		if total == 0 {
			continue
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
			name, c.Inbox, c.Issues, c.Docs, c.Review, c.InProgress, c.Completed, c.Other)
	}
	w.Flush()
}
