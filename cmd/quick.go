package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/mattsolo1/grove-notebook/cmd/config"
	"github.com/mattsolo1/grove-notebook/pkg/models"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

func NewQuickCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quick [content]",
		Short: "Create a quick note without opening editor",
		Long: `Create a quick note with timestamp title, no editor.
	
Examples:
  nb quick "Remember to review PR #123"
  nb quick "Meeting at 3pm with team"`,
		Args: cobra.ExactArgs(1),
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

			content := args[0]

			// Create timestamp-based title with "quick" suffix
			title := time.Now().Format("2006-01-02-150405") + "-quick"

			// Create the note without opening editor in the quick directory
			note, err := svc.CreateNote(ctx, models.NoteTypeQuick, title, service.WithoutEditor())
			if err != nil {
				return err
			}

			// Read the created note content to preserve the frontmatter
			existingContent, err := os.ReadFile(note.Path)
			if err != nil {
				return fmt.Errorf("read note: %w", err)
			}

			// Append the quick content
			newContent := string(existingContent) + content + "\n"

			// Write the updated content
			if err := svc.UpdateNoteContent(note.Path, newContent); err != nil {
				return fmt.Errorf("update note content: %w", err)
			}

			fmt.Printf("Created quick note: %s\n", note.Path)
			return nil
		},
	}

	// Add global flags
	config.AddGlobalFlags(cmd)

	return cmd
}