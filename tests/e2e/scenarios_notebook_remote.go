package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/mattsolo1/grove-tend/pkg/assert"
	"github.com/mattsolo1/grove-tend/pkg/fs"
	"github.com/mattsolo1/grove-tend/pkg/git"
	"github.com/mattsolo1/grove-tend/pkg/harness"
)

// NotebookRemoteSyncScenario verifies that 'nb sync' creates and updates notes from a remote source.
func NotebookRemoteSyncScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "notebook-remote-sync",
		Description: "Verifies 'nb sync' creates and updates notes from a remote source.",
		Tags:        []string{"notebook", "remote", "sync", "github"},
		Steps: []harness.Step{
			// Step 1: Set up mock for 'gh' CLI. tend will compile our Go mock
			// from tests/e2e/mocks/src/gh and place it on the PATH.
			harness.SetupMocks(harness.Mock{CommandName: "gh"}),

			// Step 2: Prepare the test environment, project, and initial mock data.
			{
				Name: "Setup test environment and initial mock data",
				Func: func(ctx *harness.Context) error {
					// Create a directory to hold the JSON files our mock 'gh' will read.
					stateDir := ctx.NewDir("gh_state")
					ctx.Set("state_dir", stateDir)

					// Write initial mock data for one issue and one PR.
					initialIssuesJSON := `[
						{
							"number": 101,
							"title": "Initial Issue",
							"body": "This is the body of the initial issue.",
							"state": "OPEN",
							"url": "https://github.com/test/repo/issues/101",
							"updatedAt": "2024-01-01T12:00:00Z",
							"labels": [{"name": "bug"}, {"name": "critical"}],
							"assignees": [{"login": "user1"}, {"login": "user2"}],
							"milestone": {"title": "v1.0"}
						}
					]`
					if err := fs.WriteString(filepath.Join(stateDir, "issues.json"), initialIssuesJSON); err != nil {
						return err
					}

					initialPRsJSON := `[
						{
							"number": 202,
							"title": "Initial PR",
							"body": "This is the body of the initial PR.",
							"state": "OPEN",
							"url": "https://github.com/test/repo/pull/202",
							"updatedAt": "2024-01-01T12:00:00Z",
							"labels": [{"name": "feature"}],
							"assignees": [{"login": "reviewer1"}],
							"milestone": {"title": "v1.1"}
						}
					]`
					if err := fs.WriteString(filepath.Join(stateDir, "prs.json"), initialPRsJSON); err != nil {
						return err
					}

					// Set up a centralized notebook config in the sandboxed home directory.
					globalYAML := `
version: "1.0"
notebooks:
  rules:
    default: "main"
  definitions:
    main:
      root_dir: "~/.grove/notebooks/nb"
      sync:
        - provider: github
          issues_type: "issues"
          prs_type: "prs"
`
					globalConfigDir := filepath.Join(ctx.HomeDir(), ".config", "grove")
					if err := fs.CreateDir(globalConfigDir); err != nil {
						return err
					}
					if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
						return err
					}

					// Set up a test project and initialize a Git repository.
					projectDir := ctx.NewDir("sync-project")
					if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), "name: sync-project\nversion: '1.0'"); err != nil {
						return err
					}
					if _, err := git.SetupTestRepo(projectDir); err != nil {
						return err
					}

					ctx.Set("project_dir", projectDir)
					return nil
				},
			},

			// Step 3: Run the initial sync and verify that notes are created correctly.
			{
				Name: "Run initial sync and verify note creation",
				Func: func(ctx *harness.Context) error {
					nbBin, err := findProjectBinary()
					if err != nil {
						return err
					}
					projectDir := ctx.GetString("project_dir")
					stateDir := ctx.GetString("state_dir")

					// Run `nb remote sync` with the environment variable pointing to our mock data.
					cmd := ctx.Command(nbBin, "remote", "sync").Dir(projectDir).Env("GH_MOCK_STATE_DIR=" + stateDir)
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.Error != nil {
						return result.Error
					}

					// Verify issue note
					issueNoteDir := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb", "workspaces", "sync-project", "issues")
					issueFiles, err := fs.ListFiles(issueNoteDir)
					if err != nil {
						return fmt.Errorf("failed to list issue notes: %w", err)
					}
					if err := assert.Equal(1, len(issueFiles), "expected one issue note"); err != nil {
						return err
					}
					issueNotePath := filepath.Join(issueNoteDir, issueFiles[0])
					ctx.Set("issue_note_path", issueNotePath) // Save for update test

					issueContent, _ := fs.ReadString(issueNotePath)
					assert.Contains(issueContent, "remote:")
					assert.Contains(issueContent, "  id: 101")
					assert.Contains(issueContent, "title: Initial Issue")
					assert.Contains(issueContent, "This is the body of the initial issue.")
					assert.Contains(issueContent, "  labels: [bug, critical]")
					assert.Contains(issueContent, "  assignees: [user1, user2]")
					assert.Contains(issueContent, "  milestone: v1.0")

					// Verify PR note
					prNoteDir := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb", "workspaces", "sync-project", "prs")
					prFiles, err := fs.ListFiles(prNoteDir)
					if err != nil {
						return fmt.Errorf("failed to list pr notes: %w", err)
					}
					if err := assert.Equal(1, len(prFiles), "expected one PR note"); err != nil {
						return err
					}
					prNotePath := filepath.Join(prNoteDir, prFiles[0])
					ctx.Set("pr_note_path", prNotePath) // Save for update test

					prContent, _ := fs.ReadString(prNotePath)
					if err := assert.Contains(prContent, "remote:"); err != nil {
						return err
					}
					if err := assert.Contains(prContent, "  id: 202"); err != nil {
						return err
					}
					if err := assert.Contains(prContent, "  assignees: [reviewer1]"); err != nil {
						return err
					}
					return assert.Contains(prContent, "  milestone: v1.1")
				},
			},

			// Step 4: Modify the mock data to test the update logic.
			{
				Name: "Modify mock data for update test",
				Func: func(ctx *harness.Context) error {
					stateDir := ctx.GetString("state_dir")
					prNotePath := ctx.GetString("pr_note_path")

					// Record mtime of the PR note to verify it's not updated later.
					info, err := os.Stat(prNotePath)
					if err != nil {
						return err
					}
					ctx.Set("pr_note_mtime", info.ModTime())

					// Update issues.json: modify the first issue and add a new one.
					updatedIssuesJSON := `[
						{
							"number": 101,
							"title": "Updated Issue Title",
							"body": "This issue body has been updated.",
							"state": "CLOSED",
							"url": "https://github.com/test/repo/issues/101",
							"updatedAt": "2024-01-02T12:00:00Z",
							"labels": [{"name": "bug"}],
							"assignees": [{"login": "user3"}],
							"milestone": {"title": "v2.0"}
						},
						{
							"number": 102,
							"title": "A Brand New Issue",
							"body": "Body of the new issue.",
							"state": "OPEN",
							"url": "https://github.com/test/repo/issues/102",
							"updatedAt": "2024-01-02T13:00:00Z",
							"labels": [],
							"assignees": [],
							"milestone": null
						}
					]`
					return fs.WriteString(filepath.Join(stateDir, "issues.json"), updatedIssuesJSON)
				},
			},

			// Step 5: Run the second sync and verify the update/creation behavior.
			{
				Name: "Run second sync and verify update behavior",
				Func: func(ctx *harness.Context) error {
					nbBin, err := findProjectBinary()
					if err != nil {
						return err
					}
					projectDir := ctx.GetString("project_dir")
					stateDir := ctx.GetString("state_dir")
					originalMtime := ctx.Get("pr_note_mtime").(time.Time)

					// Run `nb remote sync` again.
					cmd := ctx.Command(nbBin, "remote", "sync").Dir(projectDir).Env("GH_MOCK_STATE_DIR=" + stateDir)
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.Error != nil {
						return result.Error
					}

					// Verify updated note.
					issueNotePath := ctx.GetString("issue_note_path")
					updatedContent, _ := fs.ReadString(issueNotePath)
					if err := assert.Contains(updatedContent, "title: Updated Issue Title"); err != nil {
						return fmt.Errorf("issue note title was not updated: %w", err)
					}
					if err := assert.Contains(updatedContent, "  state: closed"); err != nil {
						return fmt.Errorf("issue note state was not updated: %w", err)
					}
					if err := assert.NotContains(updatedContent, "critical"); err != nil {
						return fmt.Errorf("issue note labels were not updated: %w", err)
					}
					if err := assert.Contains(updatedContent, "  assignees: [user3]"); err != nil {
						return fmt.Errorf("issue note assignees were not updated: %w", err)
					}
					if err := assert.Contains(updatedContent, "  milestone: v2.0"); err != nil {
						return fmt.Errorf("issue note milestone was not updated: %w", err)
					}

					// Verify new note was created.
					issueNoteDir := filepath.Dir(issueNotePath)
					issueFiles, _ := fs.ListFiles(issueNoteDir)
					if err := assert.Equal(2, len(issueFiles), "expected two issue notes after second sync"); err != nil {
						return err
					}
					// Find the new file
					foundNewFile := false
					for _, file := range issueFiles {
						if match, _ := regexp.MatchString(`-a-brand-new-issue.md$`, file); match {
							foundNewFile = true
							break
						}
					}
					if !foundNewFile {
						return fmt.Errorf("new issue note for 'A Brand New Issue' was not created")
					}


					// Verify unchanged note.
					prNotePath := ctx.GetString("pr_note_path")
					info, err := os.Stat(prNotePath)
					if err != nil {
						return err
					}
					if !info.ModTime().Equal(originalMtime) {
						return fmt.Errorf("PR note was modified but should not have been (mtime: %v vs %v)", info.ModTime(), originalMtime)
					}

					return nil
				},
			},
		},
	}
}
