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
	var workspaceDir string
	var jobType string
	var jobTemplate string

	cmd := &cobra.Command{
		Use:   "promote <note-path>",
		Short: "Promote a note to a job in an existing flow plan",
		Long: `Promote a notebook entry to a chat job in an existing flow plan.

The note is moved to in_progress/ and a reference job is created in the
target plan. The original note is linked back via plan_ref frontmatter.

Both note-path and --plan accept absolute paths and may be in different workspaces.
Use --workspace to resolve --plan relative to that workspace's plans directory.

Examples:
  nb promote /path/to/note.md --plan /path/to/plan-dir
  nb promote ./inbox/my-note.md --plan ~/plans/sprint-42
  nb promote note.md --plan treemux-pt6 --workspace /path/to/workspace`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := *svc

			notePath, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolving note path: %w", err)
			}

			var absPlanDir string
			if workspaceDir != "" {
				// Resolve --plan relative to the workspace's plans directory
				absWorkspace, err := filepath.Abs(workspaceDir)
				if err != nil {
					return fmt.Errorf("resolving workspace path: %w", err)
				}
				absPlanDir = filepath.Join(absWorkspace, "plans", planDir)
			} else {
				absPlanDir, err = filepath.Abs(planDir)
				if err != nil {
					return fmt.Errorf("resolving plan path: %w", err)
				}
			}

			opts := service.PromoteOptions{
				JobType:     jobType,
				JobTemplate: jobTemplate,
			}
			jobFilename, err := s.PromoteNoteToJob(notePath, absPlanDir, opts)
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
	cmd.Flags().StringVar(&workspaceDir, "workspace", "", "Workspace directory to resolve --plan relative to its plans/")
	cmd.Flags().StringVar(&jobType, "type", "chat", "Job type (chat, interactive_agent, headless_agent, oneshot)")
	cmd.Flags().StringVar(&jobTemplate, "template", "chat", "Job template name")

	return cmd
}
