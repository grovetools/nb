package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattsolo1/grove-notebook/cmd/config"
	"github.com/mattsolo1/grove-notebook/pkg/models"
)

func NewListCmd() *cobra.Command {
	var (
		listAll           bool
		listType          string
		listGlobal        bool
		listJSON          bool
		listAllWorkspaces bool
		listAllBranches   bool
	)

	cmd := &cobra.Command{
		Use:     "list [type]",
		Short:   "List notes in current workspace",
		Aliases: []string{"ls"},
		Long: `List notes in the current workspace.

Examples:
  nb list              # List current notes
  nb list llm          # List LLM notes
  nb list learn        # List learning notes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize config and service
			config.InitConfig()
			svc, err := config.InitService()
			if err != nil {
				return err
			}
			defer svc.Close()

			// Get workspace context, potentially overridden
			ctx, err := svc.GetWorkspaceContext(config.WorkspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			// Handle --all-branches flag
			if listAllBranches {
				if !ctx.CurrentWorkspace.IsWorktree() && ctx.CurrentWorkspace.Kind != "StandaloneProject" { // Simplified check
					return fmt.Errorf("--all-branches can only be used within a git repository workspace")
				}

				repoNotes, err := svc.ListAllNotesInWorkspace(ctx.NotebookContextWorkspace)
				if err != nil {
					return err
				}

				if len(repoNotes) == 0 {
					if !listJSON {
						fmt.Printf("No notes found in any branch of the '%s' repository\n", ctx.NotebookContextWorkspace.Name)
					} else {
						fmt.Println("[]")
					}
					return nil
				}

				if listJSON {
					return outputJSON(repoNotes)
				} else {
					printNotesTable(repoNotes)
				}
				return nil
			}

			// Handle --workspaces flag first (list from all workspaces)
			if listAllWorkspaces {
				allNotes, err := svc.ListNotesFromAllWorkspaces()
				if err != nil {
					return err
				}

				if len(allNotes) == 0 {
					if !listJSON {
						fmt.Println("No notes found across all workspaces")
					} else {
						fmt.Println("[]")
					}
					return nil
				}

				// Output based on format
				if listJSON {
					return outputJSON(allNotes)
				} else {
					printNotesTable(allNotes)
				}
				return nil
			}

			if listAll {
				// List all notes in all directories (including custom/nested types)
				var allNotes []*models.Note
				var err error

				if listGlobal {
					allNotes, err = svc.ListAllGlobalNotes()
				} else {
					allNotes, err = svc.ListAllNotes(ctx)
				}

				if err != nil {
					return err
				}

				if len(allNotes) == 0 {
					if !listJSON {
						fmt.Println("No notes found")
					} else {
						fmt.Println("[]")
					}
					return nil
				}

				// Output based on format
				if listJSON {
					return outputJSON(allNotes)
				} else {
					printNotesTable(allNotes)
				}
				return nil
			}

			// Original single-type listing
			noteType := listType
			if len(args) > 0 {
				noteType = args[0]
			}

			notes, err := svc.ListNotes(ctx, models.NoteType(noteType))
			if err != nil {
				return err
			}

			if len(notes) == 0 {
				if listJSON {
					fmt.Println("[]")
				} else {
					fmt.Printf("No %s notes found\n", noteType)
				}
				return nil
			}

			// Output based on format
			if listJSON {
				return outputJSON(notes)
			} else {
				printNotesTable(notes)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&listAll, "all", false, "List all note types")
	cmd.Flags().StringVarP(&listType, "type", "t", "current", "Note type to list")
	cmd.Flags().BoolVarP(&listGlobal, "global", "g", false, "List global notes only")
	cmd.Flags().BoolVar(&listJSON, "json", false, "Output in JSON format")
	cmd.Flags().BoolVarP(&listAllWorkspaces, "workspaces", "w", false, "List notes from all workspaces")
	cmd.Flags().BoolVar(&listAllBranches, "all-branches", false, "List notes from all branches in the current repository")

	// Add global flags
	config.AddGlobalFlags(cmd)

	return cmd
}

func printNotesTable(notes []*models.Note) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Print header
	fmt.Fprintln(w, "TYPE\tDATE\tTITLE\tWORDS")
	fmt.Fprintln(w, "-------\t----------\t-----------------------------\t------")

	// Print each note
	for _, note := range notes {
		typeIcon := getNoteTypeIcon(note.Type)
		typeAbbrev := getTypeAbbreviation(note.Type)
		typeStr := fmt.Sprintf("%s %s", typeIcon, typeAbbrev)
		dateStr := note.ModifiedAt.Format("2006-01-02")
		titleStr := truncateString(note.Title, 29)
		wordsStr := fmt.Sprintf("%d", note.WordCount)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", typeStr, dateStr, titleStr, wordsStr)
	}

	w.Flush()
}

func getNoteTypeIcon(noteType models.NoteType) string {
	switch noteType {
	case models.NoteTypeCurrent:
		return "📝"
	case models.NoteTypeLLM:
		return "🤖"
	case models.NoteTypeLearn:
		return "📚"
	case models.NoteTypeDaily:
		return "📅"
	case models.NoteTypeIssues:
		return "🐛"
	case models.NoteTypeArchitecture:
		return "🏗️"
	case models.NoteTypeTodos:
		return "✅"
	case models.NoteTypeQuick:
		return "⚡"
	case models.NoteTypeBlog:
		return "✍️"
	case models.NoteTypePrompts:
		return "💡"
	default:
		return "📄"
	}
}

func getTypeAbbreviation(noteType models.NoteType) string {
	switch noteType {
	case models.NoteTypeCurrent:
		return "cur"
	case models.NoteTypeLLM:
		return "llm"
	case models.NoteTypeLearn:
		return "lrn"
	case models.NoteTypeDaily:
		return "dly"
	case models.NoteTypeIssues:
		return "iss"
	case models.NoteTypeArchitecture:
		return "arc"
	case models.NoteTypeTodos:
		return "tdo"
	case models.NoteTypeQuick:
		return "qui"
	case models.NoteTypeBlog:
		return "blg"
	case models.NoteTypePrompts:
		return "pmt"
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
