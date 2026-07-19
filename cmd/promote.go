package cmd

import (
	"encoding/json"
	"fmt"
	"os"
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
	var model string
	var effort string
	var skill string
	var dependsOn []string
	var dryRun bool
	var jsonOut bool
	var force bool
	var strict bool

	cmd := &cobra.Command{
		Use:   "promote <note-path> [<note-path>...]",
		Short: "Promote one or more notes to jobs in an existing flow plan",
		Long: `Promote notebook entries to jobs in an existing flow plan.

Each note is moved to in_progress/ and a reference job is created in the
target plan. The original note is linked back via plan_ref frontmatter.
Passing several notes promotes a roster into one plan: all inputs are
validated before the first note moves.

Both note-path and --plan accept absolute paths and may be in different workspaces.
Use --workspace to resolve --plan relative to that workspace's plans directory.

Examples:
  nb promote /path/to/note.md --plan /path/to/plan-dir
  nb promote a.md b.md c.md --plan ~/plans/sprint-42
  nb promote ./inbox/my-note.md --plan ~/plans/sprint-42 --type headless_agent --model claude-3-5-sonnet --effort large
  nb promote note.md --plan treemux-pt6 --workspace /path/to/workspace
  nb promote note.md --plan feature-x --type interactive_agent --skill grove-feature-subcoordinator
  nb promote late-join.md --plan feature-x --depends-on 01-contract.md
  nb promote a.md b.md --plan feature-x --dry-run --json`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := *svc

			notePaths := make([]string, 0, len(args))
			for _, arg := range args {
				notePath, err := filepath.Abs(arg)
				if err != nil {
					return fmt.Errorf("resolving note path %s: %w", arg, err)
				}
				notePaths = append(notePaths, notePath)
			}

			var absPlanDir string
			var err error
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
				Model:       model,
				Effort:      effort,
				Skill:       skill,
				DependsOn:   dependsOn,
				Force:       force,
				Strict:      strict,
			}

			if dryRun {
				previews, err := s.PreviewPromoteNotes(notePaths, absPlanDir, opts)
				if err != nil {
					return err
				}
				if jsonOut {
					out := struct {
						Plan   string                   `json:"plan"`
						DryRun bool                     `json:"dry_run"`
						Jobs   []service.PromotePreview `json:"jobs"`
					}{Plan: absPlanDir, DryRun: true, Jobs: previews}
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					return enc.Encode(out)
				}
				fmt.Printf("Dry run — nothing promoted. Plan: %s\n", absPlanDir)
				for _, p := range previews {
					fmt.Printf("  %s -> %s (type=%s)\n", filepath.Base(p.NotePath), p.PredictedJobFile, p.JobType)
				}
				return nil
			}

			results, err := s.PromoteNotesToJobs(notePaths, absPlanDir, opts)
			if err != nil {
				// Surface any partial progress before failing so the user
				// knows which notes already moved to in_progress.
				for _, r := range results {
					fmt.Fprintf(os.Stderr, "promoted: %s -> %s\n", r.NotePath, r.JobFilename)
				}
				return err
			}

			if jsonOut {
				out := struct {
					Plan string                  `json:"plan"`
					Jobs []service.PromoteResult `json:"jobs"`
				}{Plan: absPlanDir, Jobs: results}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}

			for _, r := range results {
				promoteUlog.Success("Note promoted to job").
					Field("job", r.JobFilename).
					Field("plan", absPlanDir).
					Pretty(fmt.Sprintf("Promoted to %s/%s", filepath.Base(absPlanDir), r.JobFilename)).
					PrettyOnly().
					Emit()

				// Surface a worktree-resolution miss so it isn't silent: the job
				// was created without a repository/branch.
				if r.WorktreeMissing {
					fmt.Fprintf(os.Stderr, "warning: worktree %q not found under any worktree base; %s created without repository/branch\n", r.Worktree, r.JobFilename)
				}

				// Print job filename to stdout for scripting
				fmt.Println(r.JobFilename)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&planDir, "plan", "", "Path to the target flow plan directory (required)")
	_ = cmd.MarkFlagRequired("plan")
	cmd.Flags().StringVar(&workspaceDir, "workspace", "", "Workspace directory to resolve --plan relative to its plans/")
	cmd.Flags().StringVar(&jobType, "type", "chat", "Job type (chat, interactive_agent, headless_agent, oneshot)")
	cmd.Flags().StringVar(&jobTemplate, "template", "chat", "Job template name")
	cmd.Flags().StringVar(&model, "model", "", "LLM model to use for this job (e.g., claude-3-5-sonnet-20241022)")
	cmd.Flags().StringVar(&effort, "effort", "", "Effort level for claude agent jobs; passed to the claude CLI as --effort")
	cmd.Flags().StringVar(&skill, "skill", "", "Skill name to inject into the agent context (parity with flow plan add --skill; resolved at job run time)")
	cmd.Flags().StringArrayVarP(&dependsOn, "depends-on", "d", nil, "Job filename(s) in the target plan the promoted job(s) depend on (repeatable)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview what would be promoted without moving notes or creating jobs")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON output")
	cmd.Flags().BoolVar(&force, "force", false, "Promote even when a note's plan_ref already points at a live plan")
	cmd.Flags().BoolVar(&strict, "strict", false, "Fail hard if the plan's worktree can't be resolved (instead of creating a repo/branch-less job)")

	return cmd
}
