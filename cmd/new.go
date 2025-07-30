package cmd

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/mattsolo1/grove-notebook/cmd/config"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

func NewNewCmd() *cobra.Command {
	var (
		noteType   string
		noteName   string
		noEdit     bool
		globalNote bool
		fromStdin  bool
	)

	cmd := &cobra.Command{
		Use:   "new [title]",
		Short: "Create a new note",
		Long: `Create a new timestamped note in the current workspace.
	
Examples:
  nb new                     # Create note with timestamp only
  nb new "meeting notes"     # Create note with title
  nb new -t llm "chat"       # Create LLM note
  nb new -t learn "golang"   # Create learning note
  nb new -t issues "bug report" # Create issues note
  nb new -t architecture "api design" # Create architecture note
  nb new -t todos "sprint tasks" # Create todos note
  nb new -t blog "blog drafts" # Create blog post
  nb new -t prompts "code review" # Create reusable prompt
  nb new -g "todo list"      # Create global note
  nb new -g -t daily         # Create global daily note
  
  # Nested categories:
  nb new -t architecture/decisions "use postgres"
  nb new -t issues/bugs "login failed"
  nb new -t todos/sprint "planning tasks"
  
  # From stdin (auto-detected):
  echo "Quick thought" | nb new
  cat ideas.txt | nb new "imported ideas"
  
  # Explicit stdin control:
  echo "content" | nb new --stdin "title"
  nb new --stdin "manual" < file.txt`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize config and service
			config.InitConfig()
			svc, err := config.InitService()
			if err != nil {
				return err
			}
			defer svc.Close()

			// Get workspace context
			ctx, err := svc.GetWorkspaceContext()
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			// Auto-detect stdin if not explicitly set
			if !cmd.Flags().Changed("stdin") {
				stat, err := os.Stdin.Stat()
				if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
					// stdin is piped/redirected, auto-enable
					fromStdin = true
				}
			}

			// Get title from args or flag
			title := noteName
			if len(args) > 0 {
				title = args[0]
			}

			// If no title provided, use timestamp
			if title == "" && fromStdin {
				title = time.Now().Format("2006-01-02-150405") + "-quick"
			}

			// Default to quick type when using stdin (only if type wasn't explicitly set)
			actualNoteType := noteType
			if fromStdin && !cmd.Flags().Changed("type") {
				actualNoteType = "quick"
			}

			// Create options
			var opts []service.CreateOption
			if noEdit || fromStdin {
				opts = append(opts, service.WithoutEditor())
			}
			if globalNote {
				opts = append(opts, service.InGlobalWorkspace())
			}

			// Create the note
			note, err := svc.CreateNote(ctx, models.NoteType(actualNoteType), title, opts...)
			if err != nil {
				return err
			}

			// If reading from stdin, append the content
			if fromStdin {
				content, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("read stdin: %w", err)
				}

				// Read existing content
				existingContent, err := os.ReadFile(note.Path)
				if err != nil {
					return fmt.Errorf("read note: %w", err)
				}

				// Append stdin content
				newContent := string(existingContent) + "\n" + string(content)
				if err := svc.UpdateNoteContent(note.Path, newContent); err != nil {
					return fmt.Errorf("update note content: %w", err)
				}
			}

			fmt.Printf("Created: %s\n", note.Path)
			return nil
		},
	}

	cmd.Flags().StringVarP(&noteType, "type", "t", "current", "Note type (e.g. current, llm, learn, daily, issues, architecture, todos, blog, prompts, or nested like architecture/decisions)")
	cmd.Flags().StringVarP(&noteName, "name", "n", "", "Note name/title")
	cmd.Flags().BoolVar(&noEdit, "no-edit", false, "Don't open editor after creating")
	cmd.Flags().BoolVarP(&globalNote, "global", "g", false, "Create note in global workspace")
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "Read content from stdin (auto-detected when piped)")

	// Add global flags
	config.AddGlobalFlags(cmd)

	return cmd
}