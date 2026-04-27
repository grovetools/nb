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

			notes, err := s.ListNotes(wsCtx, models.NoteType(noteType))
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

		note := &models.Note{
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
		notes = append(notes, note)
	}

	if len(notes) == 0 {
		return nil, nil
	}
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
