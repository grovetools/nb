package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/grovetools/flow/pkg/orchestration"
	"github.com/grovetools/nb/pkg/frontmatter"
	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/harness"
	"github.com/grovetools/tend/pkg/verify"
)

// NotebookPromoteToJobScenario tests the `nb promote` CLI command with a
// cross-workspace sandbox: a note in workspace-a is promoted to a job in
// workspace-b's plan.
func NotebookPromoteToJobScenario() *harness.Scenario {
	return harness.NewScenario(
		"notebook-promote-to-job",
		"Verifies nb promote creates a job in the target plan, archives the note, and sets bidirectional links.",
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
				planDir := filepath.Join(notebookRoot, "workspaces", "workspace-b", "plans", "active-plan")
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
				ctx.Set("notebook_root", notebookRoot)
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

			harness.NewStep("Verify job frontmatter has note_ref pointing to workspace-a", func(ctx *harness.Context) error {
				jobPath := ctx.GetString("job_path")
				notePath := ctx.GetString("note_path")

				job, err := orchestration.LoadJob(jobPath)
				if err != nil {
					return fmt.Errorf("loading created job: %w", err)
				}

				return ctx.Verify(func(v *verify.Collector) {
					v.Equal("job type is chat", string(orchestration.JobTypeChat), string(job.Type))
					v.Equal("job status is pending_user", string(orchestration.JobStatusPendingUser), string(job.Status))
					v.Equal("job note_ref points to workspace-a note", notePath, job.NoteRef)
					v.Contains("job title derived from note", job.Title, "Test Bug Report")
				})
			}),

			harness.NewStep("Verify job body contains note content", func(ctx *harness.Context) error {
				jobPath := ctx.GetString("job_path")

				content, err := fs.ReadString(jobPath)
				if err != nil {
					return fmt.Errorf("reading job file: %w", err)
				}

				return ctx.Verify(func(v *verify.Collector) {
					v.Contains("job body has note content", content, "Widget crashes on empty input")
					v.Contains("job body has steps to reproduce", content, "Steps to Reproduce")
				})
			}),

			harness.NewStep("Verify original note archived in workspace-a", func(ctx *harness.Context) error {
				notePath := ctx.GetString("note_path")
				noteDir := filepath.Dir(notePath)
				noteFilename := filepath.Base(notePath)
				archivePath := filepath.Join(noteDir, ".archive", noteFilename)

				// Original location should be gone
				if err := fs.AssertNotExists(notePath); err != nil {
					return fmt.Errorf("original note should have been moved to archive: %w", err)
				}

				// Archived location should exist
				if err := fs.AssertExists(archivePath); err != nil {
					return fmt.Errorf("archived note not found: %w", err)
				}

				ctx.Set("archived_note_path", archivePath)
				return nil
			}),

			harness.NewStep("Verify archived note has plan_ref pointing to workspace-b", func(ctx *harness.Context) error {
				archivedNotePath := ctx.GetString("archived_note_path")
				jobFilename := ctx.GetString("job_filename")

				content, err := fs.ReadString(archivedNotePath)
				if err != nil {
					return fmt.Errorf("reading archived note: %w", err)
				}

				fm, _, err := frontmatter.Parse(content)
				if err != nil {
					return fmt.Errorf("parsing archived note frontmatter: %w", err)
				}

				expectedPlanRef := fmt.Sprintf("active-plan/%s", jobFilename)
				return ctx.Verify(func(v *verify.Collector) {
					v.Equal("archived note plan_ref", expectedPlanRef, fm.PlanRef)
				})
			}),
		},
	)
}
