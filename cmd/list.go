package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	coreconfig "github.com/grovetools/core/config"
	grovelogging "github.com/grovetools/core/logging"
	"github.com/grovetools/core/tui/theme"
	"github.com/grovetools/nb/pkg/models"
	"github.com/grovetools/nb/pkg/service"
)

var listUlog = grovelogging.NewUnifiedLogger("grove-notebook.cmd.list")

func NewListCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var (
		listAll           bool
		listType          string
		listGlobal        bool
		listJSON          bool
		listAllWorkspaces bool
		listAllBranches   bool
		listTag           string
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

			// Handle --workspaces flag first (list from all workspaces)
			if listAllWorkspaces {
				allNotes, err := s.ListNotesFromAllWorkspaces(false, false)
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

				if listGlobal {
					allNotes, err = s.ListAllGlobalNotes(false, false)
				} else {
					allNotes, err = s.ListAllNotes(wsCtx, false, false)
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
