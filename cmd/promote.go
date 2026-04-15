package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	grovelogging "github.com/grovetools/core/logging"
	"github.com/grovetools/nb/pkg/service"
)

var promoteUlog = grovelogging.NewUnifiedLogger("grove-notebook.cmd.promote")

func NewPromoteCmd(svc **service.Service) *cobra.Command {
	var planDir string

	cmd := &cobra.Command{
		Use:   "promote <note-path>",
		Short: "Promote a note to a job in an existing flow plan",
		Long: `Promote a notebook entry to a chat job in an existing flow plan.

The note content becomes the job prompt. The original note is archived
and linked back to the plan via plan_ref frontmatter.

Both note-path and --plan accept absolute paths and may be in different workspaces.

Examples:
  nb promote /path/to/note.md --plan /path/to/plan-dir
  nb promote ./inbox/my-note.md --plan ~/plans/sprint-42`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := *svc

			notePath, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolving note path: %w", err)
			}

			absPlanDir, err := filepath.Abs(planDir)
			if err != nil {
				return fmt.Errorf("resolving plan path: %w", err)
			}

			jobFilename, err := s.PromoteNoteToJob(notePath, absPlanDir)
			if err != nil {
				return err
			}

			promoteUlog.Success("Note promoted to job").
				Field("job", jobFilename).
				Field("plan", absPlanDir).
				Pretty(fmt.Sprintf("Promoted to %s/%s", filepath.Base(absPlanDir), jobFilename)).
				PrettyOnly().
				Emit()

			// Print job filename to stdout for scripting
			fmt.Println(jobFilename)
			return nil
		},
	}

	cmd.Flags().StringVar(&planDir, "plan", "", "Path to the target flow plan directory (required)")
	_ = cmd.MarkFlagRequired("plan")

	return cmd
}
