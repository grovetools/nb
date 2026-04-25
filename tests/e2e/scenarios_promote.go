package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/grovetools/flow/pkg/orchestration"
	"github.com/grovetools/nb/pkg/frontmatter"
	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"
	"github.com/grovetools/tend/pkg/verify"
)

// NotebookPromoteToJobScenario tests the `nb promote` CLI command with a
// cross-workspace sandbox: a note in workspace-a is promoted to a job in
// workspace-b's plan.
func NotebookPromoteToJobScenario() *harness.Scenario {
	return harness.NewScenario(
		"notebook-promote-to-job",
		"Verifies nb promote creates a job in the target plan, moves note to in_progress, and sets bidirectional links.",
		[]string{"notebook", "promote", "cross-workspace"},
		[]harness.Step{
			harness.NewStep("Setup cross-workspace sandbox", func(ctx *harness.Context) error {
				homeDir := ctx.HomeDir()

				// Create a multi-workspace notebook structure:
				//   notebooks/test-notebook/
				//     workspaces/
				//       workspace-a/inbox/test-bug.md
				//       workspace-b/plans/active-plan/.grove-plan.yml + 01-existing-job.md
				notebookRoot := filepath.Join(homeDir, "notebooks", "test-notebook")

				// Workspace A: source workspace with a note to promote
				wsAInbox := filepath.Join(notebookRoot, "workspaces", "workspace-a", "inbox")
				if err := fs.CreateDir(wsAInbox); err != nil {
					return fmt.Errorf("creating workspace-a inbox: %w", err)
				}

				noteContent := `---
title: Test Bug Report
type: inbox
tags: [bug, triage]
---

# Bug: Widget crashes on empty input

## Steps to Reproduce
1. Open the widget panel
2. Click submit without entering any text
3. Application crashes with a nil pointer error

## Expected Behavior
The form should show a validation error instead of crashing.
`
				notePath := filepath.Join(wsAInbox, "test-bug.md")
				if err := fs.WriteString(notePath, noteContent); err != nil {
					return fmt.Errorf("writing test note: %w", err)
				}
				ctx.Set("note_path", notePath)

				// Workspace B: target workspace with an existing plan
				wsBDir := filepath.Join(notebookRoot, "workspaces", "workspace-b")
				planDir := filepath.Join(wsBDir, "plans", "active-plan")
				if err := fs.CreateDir(planDir); err != nil {
					return fmt.Errorf("creating plan dir: %w", err)
				}

				planConfig := `name: active-plan
worktree: ""
`
				if err := fs.WriteString(filepath.Join(planDir, ".grove-plan.yml"), planConfig); err != nil {
					return fmt.Errorf("writing plan config: %w", err)
				}

				existingJob := `---
id: existing-job
title: Existing Job
type: chat
status: completed
---

This is a pre-existing job in the plan.
`
				if err := fs.WriteString(filepath.Join(planDir, "01-existing-job.md"), existingJob); err != nil {
					return fmt.Errorf("writing existing job: %w", err)
				}

				ctx.Set("plan_dir", planDir)
				ctx.Set("workspace_b_dir", wsBDir)
				ctx.Set("notebook_root", notebookRoot)

				// Second note for --workspace flag test
				noteContent2 := `---
title: Second Bug
type: inbox
---

Another bug to fix.
`
				note2Path := filepath.Join(wsAInbox, "second-bug.md")
				if err := fs.WriteString(note2Path, noteContent2); err != nil {
					return fmt.Errorf("writing second test note: %w", err)
				}
				ctx.Set("note2_path", note2Path)

				return nil
			}),

			harness.NewStep("Run nb promote command", func(ctx *harness.Context) error {
				notePath := ctx.GetString("note_path")
				planDir := ctx.GetString("plan_dir")

				cmd := ctx.Bin("promote", notePath, "--plan", planDir)
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if err := result.AssertSuccess(); err != nil {
					return fmt.Errorf("nb promote failed: %w", err)
				}

				// The command prints the job filename to stdout (last non-empty line)
				lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
				var jobFilename string
				for i := len(lines) - 1; i >= 0; i-- {
					line := strings.TrimSpace(lines[i])
					if line != "" && strings.HasSuffix(line, ".md") {
						jobFilename = line
						break
					}
				}
				if jobFilename == "" {
					return fmt.Errorf("nb promote did not output a job filename, stdout: %q", result.Stdout)
				}
				ctx.Set("job_filename", jobFilename)
				return nil
			}),

			harness.NewStep("Verify job file exists in workspace-b's plan", func(ctx *harness.Context) error {
				planDir := ctx.GetString("plan_dir")
				jobFilename := ctx.GetString("job_filename")
				jobPath := filepath.Join(planDir, jobFilename)

				if err := fs.AssertExists(jobPath); err != nil {
					return fmt.Errorf("job file not found in target plan: %w", err)
				}

				ctx.Set("job_path", jobPath)
				return nil
			}),

			harness.NewStep("Verify job frontmatter has note_ref pointing to in_progress", func(ctx *harness.Context) error {
				jobPath := ctx.GetString("job_path")
				notePath := ctx.GetString("note_path")

				job, err := orchestration.LoadJob(jobPath)
				if err != nil {
					return fmt.Errorf("loading created job: %w", err)
				}

				// note_ref should point to in_progress/ location, not original inbox/ path
				expectedNoteRef := filepath.Join(filepath.Dir(filepath.Dir(notePath)), "in_progress", filepath.Base(notePath))

				return ctx.Verify(func(v *verify.Collector) {
					v.Equal("job type is chat", string(orchestration.JobTypeChat), string(job.Type))
					v.Equal("job status is pending_user", string(orchestration.JobStatusPendingUser), string(job.Status))
					v.Equal("job note_ref points to in_progress", expectedNoteRef, job.NoteRef)
					v.Contains("job title derived from note", job.Title, "Test Bug Report")
				})
			}),

			harness.NewStep("Verify job body inlines note content with promoted-from trailer", func(ctx *harness.Context) error {
				jobPath := ctx.GetString("job_path")

				content, err := fs.ReadString(jobPath)
				if err != nil {
					return fmt.Errorf("reading job file: %w", err)
				}

				return ctx.Verify(func(v *verify.Collector) {
					v.Contains("job body has chat template", content, "template")
					v.Contains("job body inlines note body", content, "Steps to Reproduce")
					v.Contains("job body has promoted-from trailer", content, "_Promoted from:")
				})
			}),

			harness.NewStep("Verify original note moved to in_progress", func(ctx *harness.Context) error {
				notePath := ctx.GetString("note_path")
				noteDir := filepath.Dir(notePath)
				noteFilename := filepath.Base(notePath)
				inProgressPath := filepath.Join(filepath.Dir(noteDir), "in_progress", noteFilename)

				// Original location should be gone
				if err := fs.AssertNotExists(notePath); err != nil {
					return fmt.Errorf("original note should have been moved to in_progress: %w", err)
				}

				// in_progress location should exist
				if err := fs.AssertExists(inProgressPath); err != nil {
					return fmt.Errorf("in_progress note not found: %w", err)
				}

				ctx.Set("in_progress_note_path", inProgressPath)
				return nil
			}),

			harness.NewStep("Verify in_progress note has plan_ref", func(ctx *harness.Context) error {
				inProgressPath := ctx.GetString("in_progress_note_path")
				jobFilename := ctx.GetString("job_filename")

				content, err := fs.ReadString(inProgressPath)
				if err != nil {
					return fmt.Errorf("reading in_progress note: %w", err)
				}

				fm, _, err := frontmatter.Parse(content)
				if err != nil {
					return fmt.Errorf("parsing in_progress note frontmatter: %w", err)
				}

				expectedPlanRef := fmt.Sprintf("active-plan/%s", jobFilename)
				return ctx.Verify(func(v *verify.Collector) {
					v.Equal("in_progress note plan_ref", expectedPlanRef, fm.PlanRef)
				})
			}),

			harness.NewStep("Promote second note with --workspace flag", func(ctx *harness.Context) error {
				note2Path := ctx.GetString("note2_path")
				wsBDir := ctx.GetString("workspace_b_dir")

				// Use --workspace to resolve --plan relative to workspace-b's plans/
				cmd := ctx.Bin("promote", note2Path, "--plan", "active-plan", "--workspace", wsBDir)
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if err := result.AssertSuccess(); err != nil {
					return fmt.Errorf("nb promote with --workspace failed: %w", err)
				}

				// Verify the job landed in workspace-b's plan
				lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
				var jobFilename string
				for i := len(lines) - 1; i >= 0; i-- {
					line := strings.TrimSpace(lines[i])
					if line != "" && strings.HasSuffix(line, ".md") {
						jobFilename = line
						break
					}
				}
				if jobFilename == "" {
					return fmt.Errorf("nb promote --workspace did not output a job filename, stdout: %q", result.Stdout)
				}

				planDir := ctx.GetString("plan_dir")
				jobPath := filepath.Join(planDir, jobFilename)
				if err := fs.AssertExists(jobPath); err != nil {
					return fmt.Errorf("job from --workspace promote not found in target plan: %w", err)
				}

				return nil
			}),
		},
	)
}

// findFlowBinary locates the flow binary for cross-tool e2e tests.
func findFlowBinary() (string, error) {
	if bin, err := exec.LookPath("flow"); err == nil {
		return bin, nil
	}
	// Try the real home directory (test sandbox uses a fake home)
	realHomeBin := filepath.Join("/Users/solom4", ".grove", "bin", "flow")
	if fs.Exists(realHomeBin) {
		return realHomeBin, nil
	}
	return "", fmt.Errorf("flow binary not found in PATH or ~/.grove/bin")
}

// NotebookFullLifecycleScenario tests the complete note lifecycle:
// promote → complete (note moves inbox → in_progress → completed)
// promote → demote  (note moves inbox → in_progress → inbox)
func NotebookFullLifecycleScenario() *harness.Scenario {
	return harness.NewScenario(
		"notebook-full-lifecycle",
		"Full round-trip: promote note to job, complete moves note to completed, demote moves note back to inbox.",
		[]string{"notebook", "promote", "complete", "demote", "lifecycle", "cross-workspace"},
		[]harness.Step{
			harness.NewStep("Setup multi-workspace sandbox with git", func(ctx *harness.Context) error {
				homeDir := ctx.HomeDir()
				notebookRoot := filepath.Join(homeDir, "notebooks", "lifecycle-test")

				// Workspace A: source workspace with notes
				wsADir := filepath.Join(notebookRoot, "workspaces", "workspace-a")
				wsAInbox := filepath.Join(wsADir, "inbox")
				if err := fs.CreateDir(wsAInbox); err != nil {
					return fmt.Errorf("creating workspace-a inbox: %w", err)
				}

				// Workspace B: target workspace with a plan
				wsBDir := filepath.Join(notebookRoot, "workspaces", "workspace-b")
				planDir := filepath.Join(wsBDir, "plans", "test-plan")
				if err := fs.CreateDir(planDir); err != nil {
					return fmt.Errorf("creating plan dir: %w", err)
				}

				planConfig := "name: test-plan\nworktree: test-worktree\n"
				if err := fs.WriteString(filepath.Join(planDir, ".grove-plan.yml"), planConfig); err != nil {
					return fmt.Errorf("writing plan config: %w", err)
				}

				// Initialize a git repo at the sandbox root so git.GetRepoInfo works
				if err := git.Init(notebookRoot); err != nil {
					return fmt.Errorf("git init: %w", err)
				}
				if err := git.SetupTestConfig(notebookRoot); err != nil {
					return fmt.Errorf("git config: %w", err)
				}
				repo := git.New(notebookRoot)
				if err := repo.AddCommit("initial commit"); err != nil {
					return fmt.Errorf("git commit: %w", err)
				}

				ctx.Set("notebook_root", notebookRoot)
				ctx.Set("ws_a_dir", wsADir)
				ctx.Set("ws_a_inbox", wsAInbox)
				ctx.Set("ws_b_dir", wsBDir)
				ctx.Set("plan_dir", planDir)

				return nil
			}),

			harness.NewStep("Create note for promote→complete lifecycle", func(ctx *harness.Context) error {
				wsAInbox := ctx.GetString("ws_a_inbox")
				noteContent := `---
title: Lifecycle Test Note
type: inbox
tags: [test]
---

# Lifecycle Test

This note will be promoted and then completed.
`
				notePath := filepath.Join(wsAInbox, "test-lifecycle.md")
				if err := fs.WriteString(notePath, noteContent); err != nil {
					return fmt.Errorf("writing note: %w", err)
				}
				ctx.Set("note1_path", notePath)
				return nil
			}),

			harness.NewStep("Promote note to job", func(ctx *harness.Context) error {
				notePath := ctx.GetString("note1_path")
				planDir := ctx.GetString("plan_dir")

				cmd := ctx.Bin("promote", notePath, "--plan", planDir, "--type", "chat")
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if err := result.AssertSuccess(); err != nil {
					return fmt.Errorf("nb promote failed: %w", err)
				}

				// Extract job filename from stdout
				lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
				var jobFilename string
				for i := len(lines) - 1; i >= 0; i-- {
					line := strings.TrimSpace(lines[i])
					if line != "" && strings.HasSuffix(line, ".md") {
						jobFilename = line
						break
					}
				}
				if jobFilename == "" {
					return fmt.Errorf("nb promote did not output a job filename, stdout: %q", result.Stdout)
				}
				ctx.Set("job1_filename", jobFilename)
				return nil
			}),

			harness.NewStep("Verify promote: note moved to in_progress", func(ctx *harness.Context) error {
				notePath := ctx.GetString("note1_path")
				wsADir := ctx.GetString("ws_a_dir")
				inProgressPath := filepath.Join(wsADir, "in_progress", filepath.Base(notePath))

				if err := fs.AssertNotExists(notePath); err != nil {
					return fmt.Errorf("original note should be gone from inbox: %w", err)
				}
				if err := fs.AssertExists(inProgressPath); err != nil {
					return fmt.Errorf("note should exist in in_progress: %w", err)
				}

				ctx.Set("note1_in_progress", inProgressPath)
				return nil
			}),

			harness.NewStep("Verify promote: job created with correct frontmatter", func(ctx *harness.Context) error {
				planDir := ctx.GetString("plan_dir")
				jobFilename := ctx.GetString("job1_filename")
				note1InProgress := ctx.GetString("note1_in_progress")
				jobPath := filepath.Join(planDir, jobFilename)

				if err := fs.AssertExists(jobPath); err != nil {
					return fmt.Errorf("job file not in plan: %w", err)
				}

				job, err := orchestration.LoadJob(jobPath)
				if err != nil {
					return fmt.Errorf("loading job: %w", err)
				}

				return ctx.Verify(func(v *verify.Collector) {
					v.Equal("job type is chat", string(orchestration.JobTypeChat), string(job.Type))
					v.Equal("job status is pending_user", string(orchestration.JobStatusPendingUser), string(job.Status))
					v.Equal("job note_ref points to in_progress", note1InProgress, job.NoteRef)
					v.Equal("job worktree from plan config", "test-worktree", job.Worktree)
					v.Contains("job title from note", job.Title, "Lifecycle Test Note")
				})
			}),

			harness.NewStep("Complete job and verify note moves to completed", func(ctx *harness.Context) error {
				flowBin, err := findFlowBinary()
				if err != nil {
					return fmt.Errorf("finding flow binary: %w", err)
				}

				planDir := ctx.GetString("plan_dir")
				jobFilename := ctx.GetString("job1_filename")

				cmd := ctx.Command(flowBin, "complete", planDir, jobFilename).
					Env("HOME=" + ctx.HomeDir())
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if err := result.AssertSuccess(); err != nil {
					return fmt.Errorf("flow complete failed: %w", err)
				}

				return nil
			}),

			harness.NewStep("Verify complete: note in completed/, job status completed", func(ctx *harness.Context) error {
				note1InProgress := ctx.GetString("note1_in_progress")
				wsADir := ctx.GetString("ws_a_dir")
				completedPath := filepath.Join(wsADir, "completed", filepath.Base(note1InProgress))

				// Note should be gone from in_progress
				if err := fs.AssertNotExists(note1InProgress); err != nil {
					return fmt.Errorf("note should be gone from in_progress: %w", err)
				}

				// Note should be in completed/
				if err := fs.AssertExists(completedPath); err != nil {
					return fmt.Errorf("note should exist in completed: %w", err)
				}

				// Job status should be completed
				planDir := ctx.GetString("plan_dir")
				jobFilename := ctx.GetString("job1_filename")
				jobPath := filepath.Join(planDir, jobFilename)

				job, err := orchestration.LoadJob(jobPath)
				if err != nil {
					return fmt.Errorf("loading completed job: %w", err)
				}

				return ctx.Verify(func(v *verify.Collector) {
					v.Equal("job status is completed", string(orchestration.JobStatusCompleted), string(job.Status))
				})
			}),

			// --- Demote round-trip ---

			harness.NewStep("Create second note for promote→demote lifecycle", func(ctx *harness.Context) error {
				wsAInbox := ctx.GetString("ws_a_inbox")
				noteContent := `---
title: Demote Test Note
type: inbox
tags: [test]
---

# Demote Test

This note will be promoted and then demoted back to inbox.
`
				notePath := filepath.Join(wsAInbox, "test-demote.md")
				if err := fs.WriteString(notePath, noteContent); err != nil {
					return fmt.Errorf("writing second note: %w", err)
				}
				ctx.Set("note2_path", notePath)
				return nil
			}),

			harness.NewStep("Promote second note", func(ctx *harness.Context) error {
				notePath := ctx.GetString("note2_path")
				planDir := ctx.GetString("plan_dir")

				cmd := ctx.Bin("promote", notePath, "--plan", planDir, "--type", "chat")
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if err := result.AssertSuccess(); err != nil {
					return fmt.Errorf("nb promote (demote test) failed: %w", err)
				}

				lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
				var jobFilename string
				for i := len(lines) - 1; i >= 0; i-- {
					line := strings.TrimSpace(lines[i])
					if line != "" && strings.HasSuffix(line, ".md") {
						jobFilename = line
						break
					}
				}
				if jobFilename == "" {
					return fmt.Errorf("nb promote did not output a job filename, stdout: %q", result.Stdout)
				}
				ctx.Set("job2_filename", jobFilename)
				return nil
			}),

			harness.NewStep("Verify second note moved to in_progress", func(ctx *harness.Context) error {
				notePath := ctx.GetString("note2_path")
				wsADir := ctx.GetString("ws_a_dir")
				inProgressPath := filepath.Join(wsADir, "in_progress", filepath.Base(notePath))

				if err := fs.AssertNotExists(notePath); err != nil {
					return fmt.Errorf("original note should be gone from inbox: %w", err)
				}
				if err := fs.AssertExists(inProgressPath); err != nil {
					return fmt.Errorf("note should exist in in_progress: %w", err)
				}

				ctx.Set("note2_in_progress", inProgressPath)
				return nil
			}),

			harness.NewStep("Demote second job back to inbox", func(ctx *harness.Context) error {
				flowBin, err := findFlowBinary()
				if err != nil {
					return fmt.Errorf("finding flow binary: %w", err)
				}

				planDir := ctx.GetString("plan_dir")
				jobFilename := ctx.GetString("job2_filename")
				jobFilePath := filepath.Join(planDir, jobFilename)

				cmd := ctx.Command(flowBin, "plan", "demote", jobFilePath).
					Env("HOME=" + ctx.HomeDir())
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if err := result.AssertSuccess(); err != nil {
					return fmt.Errorf("flow plan demote failed: %w", err)
				}

				return nil
			}),

			harness.NewStep("Verify demote: note back in inbox, job abandoned", func(ctx *harness.Context) error {
				note2InProgress := ctx.GetString("note2_in_progress")
				wsADir := ctx.GetString("ws_a_dir")
				inboxPath := filepath.Join(wsADir, "inbox", filepath.Base(note2InProgress))

				// Note should be gone from in_progress
				if err := fs.AssertNotExists(note2InProgress); err != nil {
					return fmt.Errorf("note should be gone from in_progress: %w", err)
				}

				// Note should be back in inbox
				if err := fs.AssertExists(inboxPath); err != nil {
					return fmt.Errorf("note should be back in inbox: %w", err)
				}

				// Job status should be abandoned
				planDir := ctx.GetString("plan_dir")
				jobFilename := ctx.GetString("job2_filename")
				jobPath := filepath.Join(planDir, jobFilename)

				job, err := orchestration.LoadJob(jobPath)
				if err != nil {
					return fmt.Errorf("loading demoted job: %w", err)
				}

				return ctx.Verify(func(v *verify.Collector) {
					v.Equal("job status is abandoned", string(orchestration.JobStatusAbandoned), string(job.Status))
				})
			}),
		},
	)
}
