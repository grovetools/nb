package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	grovelogging "github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-notebook/pkg/service"
)

var quickUlog = grovelogging.NewUnifiedLogger("grove-notebook.cmd.quick")

func NewQuickCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quick [content]",
		Short: "Create a quick note without opening editor",
		Long: `Create a quick note with timestamp title, no editor.
	
Examples:
  nb quick "Remember to review PR #123"
  nb quick "Meeting at 3pm with team"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := *svc

			// Get workspace context
			ctx, err := s.GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			content := args[0]

			// Create timestamp-based title with "quick" suffix
			title := time.Now().Format("2006-01-02-150405") + "-quick"

			// Create the note without opening editor in the quick directory
			note, err := s.CreateNote(ctx, "quick", title, service.WithoutEditor())
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
			if err := s.UpdateNoteContent(note.Path, newContent); err != nil {
				return fmt.Errorf("update note content: %w", err)
			}

			quickUlog.Success("Created quick note").
				Field("path", note.Path).
				Field("content", content).
				Pretty(fmt.Sprintf("Created quick note: %s", note.Path)).
				PrettyOnly().
				Emit()
			return nil
		},
	}

	return cmd
}
