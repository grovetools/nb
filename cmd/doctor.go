package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/grovetools/nb/pkg/doctor"
	"github.com/grovetools/nb/pkg/service"
)

// NewDoctorCmd builds the `nb doctor` parent command. Its subcommands audit —
// and optionally repair — invariants in the notebook. Today it hosts `notes`,
// the note↔plan link reconciler.
func NewDoctorCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose and repair notebook invariants",
		Long:  "Diagnostics for the notebook. Subcommands audit invariants and, with --fix, repair them.",
	}
	cmd.AddCommand(newDoctorNotesCmd(svc, workspaceOverride))
	return cmd
}

func newDoctorNotesCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var jsonOut bool
	var fix bool

	cmd := &cobra.Command{
		Use:   "notes",
		Short: "Reconcile note↔plan links (plan_ref / plan_job)",
		Long: `Reconcile the note↔plan linkage recorded in note frontmatter against the
plans present on disk.

For each note in in_progress/ and review/, plan_ref is classified as LIVE
(plan exists under plans/), ARCHIVED (plans/.archive/), GONE (neither), or
MALFORMED-LEGACY (the old <plan>/<job>.md ref form); a note with no plan_ref is
UNLINKED. In the forward direction, each live plan job carrying a note_ref that
no note claims is reported UNCLAIMED.

Read-only by default. With --fix:
  - MALFORMED-LEGACY (live/archived plan) is rewritten to plans/<name> + plan_job
  - ARCHIVED / GONE notes are moved to completed/
  - UNLINKED in_progress notes are moved back to inbox/
  - UNCLAIMED jobs whose legacy note_ref resolves to a still-unlinked note file
    have that note's plan_ref/plan_job backfilled

The target workspace defaults to the current one; use the global --workspace/-W
flag to point at another workspace by path. Exits non-zero when problems remain.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s := *svc

			ctx, err := s.GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("resolving workspace context: %w", err)
			}
			node := ctx.NotebookContextWorkspace
			plansDir, err := s.GetNotebookLocator().GetPlansDir(node)
			if err != nil {
				return fmt.Errorf("resolving plans directory: %w", err)
			}
			workspaceDir := filepath.Dir(plansDir)

			report, err := doctor.Run(workspaceDir, node.Name, fix)
			if err != nil {
				return err
			}

			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(report); err != nil {
					return err
				}
			} else {
				printDoctorReport(report)
			}

			if remaining := report.ProblemsRemaining(); remaining > 0 {
				return fmt.Errorf("%d link problem(s) remain; re-run with --fix to repair", remaining)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON output")
	cmd.Flags().BoolVar(&fix, "fix", false, "Apply repairs (default: read-only report)")
	return cmd
}

func printDoctorReport(r *doctor.Report) {
	fmt.Printf("Workspace: %s\n", r.Workspace)
	if len(r.Notes) == 0 && len(r.UnclaimedJobs) == 0 {
		fmt.Println("No linked notes or plan jobs to reconcile.")
		return
	}

	// Group notes by classification for a scannable table.
	order := []doctor.Classification{
		doctor.Unlinked,
		doctor.MalformedLegacy,
		doctor.Gone,
		doctor.Archived,
		doctor.Live,
	}
	byClass := map[doctor.Classification][]doctor.NoteEntry{}
	for _, n := range r.Notes {
		byClass[n.Classification] = append(byClass[n.Classification], n)
	}

	for _, class := range order {
		entries := byClass[class]
		if len(entries) == 0 {
			continue
		}
		fmt.Printf("\n%s (%d)\n", class, len(entries))
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  NOTE\tPLAN\tPLAN_JOB\tACTION")
		for _, n := range entries {
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n",
				filepath.Base(n.Path), dash(n.Plan), dash(n.PlanJob), dash(n.ActionTaken))
		}
		_ = w.Flush()
	}

	if len(r.UnclaimedJobs) > 0 {
		fmt.Printf("\nUNCLAIMED jobs (%d)\n", len(r.UnclaimedJobs))
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  PLAN\tJOB\tNOTE_REF\tACTION")
		for _, j := range r.UnclaimedJobs {
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n",
				j.Plan, j.JobFile, j.NoteRef, dash(j.ActionTaken))
		}
		_ = w.Flush()
	}

	fmt.Printf("\nSummary: %d problem(s), %d action(s) taken, %d remaining\n",
		r.Summary["problems"], r.Summary["actions_taken"], r.Summary["problems_remaining"])
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
