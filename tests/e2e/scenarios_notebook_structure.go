package main

import (
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/mattsolo1/grove-tend/pkg/assert"
	"github.com/mattsolo1/grove-tend/pkg/fs"
	"github.com/mattsolo1/grove-tend/pkg/git"
	"github.com/mattsolo1/grove-tend/pkg/harness"
	"github.com/mattsolo1/grove-tend/pkg/verify"
)

// NotebookCentralizedStructureScenario verifies note creation in a centralized notebook.
func NotebookCentralizedStructureScenario() *harness.Scenario {
	return harness.NewScenario(
		"notebook-centralized-structure",
		"Verifies note creation with a centralized notebook configuration.",
		[]string{"notebook", "structure", "centralized"},
		[]harness.Step{
			harness.NewStep("Setup centralized config, create note, and verify path", func(ctx *harness.Context) error {
					// 1. Setup global config for centralized notebook
					globalYAML := `
version: "1.0"
notebooks:
  rules:
    default: "main"
  definitions:
    main:
      root_dir: "~/.grove/notebooks/nb"
`
					globalConfigDir := filepath.Join(ctx.HomeDir(), ".config", "grove")
					if err := fs.CreateDir(globalConfigDir); err != nil {
						return fmt.Errorf("failed to create global config dir: %w", err)
					}
					if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
						return err
					}

					// 2. Setup test project
					projectDir := ctx.NewDir("test-project")
					if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), "name: test-project\nversion: '1.0'"); err != nil {
						return err
					}
					if _, err := git.SetupTestRepo(projectDir); err != nil {
						return err
					}

					// 3. Execute 'nb new'
					cmd := ctx.Bin("new", "--no-edit", "my test note").Dir(projectDir)
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.Error != nil {
						return result.Error
					}

					// 4. Assert file path
					// Path: ~/.grove/notebooks/nb/workspaces/test-project/inbox/YYYYMMDD-my-test-note.md
					expectedDir := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb", "workspaces", "test-project", "inbox")
					files, err := fs.ListFiles(expectedDir)
					if err != nil {
						return fmt.Errorf("failed to read expected note directory: %w", err)
					}
					if err := ctx.Check("one note file was created", assert.Equal(1, len(files))); err != nil {
						return err
					}
					match, _ := regexp.MatchString(`\d{8}-my-test-note.md`, filepath.Base(files[0]))
					if err := ctx.Check("note filename matches pattern", assert.True(match)); err != nil {
						return err
					}

					// 5. Assert 'nb context'
					cmd = ctx.Bin("context", "--json").Dir(projectDir)
					result = cmd.Run()
					return ctx.Verify(func(v *verify.Collector) {
						v.Contains("context shows correct workspace name", result.Stdout, `"name": "test-project"`)
						v.Contains("context has inbox path", result.Stdout, `"inbox":`)
					})
				}),
		},
	)
}

// NotebookLocalStructureScenario verifies note creation in local mode.
func NotebookLocalStructureScenario() *harness.Scenario {
	return harness.NewScenario(
		"notebook-local-structure",
		"Verifies note creation with a local notebook configuration (root_dir: '').",
		[]string{"notebook", "structure", "local"},
		[]harness.Step{
			harness.NewStep("Setup local config, create note, and verify path", func(ctx *harness.Context) error {
					// 1. Setup test project with local notebook config
					projectDir := ctx.NewDir("local-project")
					localYAML := `
name: local-project
version: '1.0'
notebooks:
  rules:
    default: "local"
  definitions:
    local:
      root_dir: ""
`
					if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), localYAML); err != nil {
						return err
					}

					// 2. Execute 'nb new'
					cmd := ctx.Bin("new", "--no-edit", "my local note").Dir(projectDir)
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.Error != nil {
						return result.Error
					}

					// 3. Assert file path
					// Path: ./.notebook/notes/inbox/YYYYMMDD-my-local-note.md
					expectedDir := filepath.Join(projectDir, ".notebook", "notes", "inbox")
					if !fs.Exists(expectedDir) {
						return fmt.Errorf("expected directory does not exist: %s", expectedDir)
					}
					files, err := fs.ListFiles(expectedDir)
					if err != nil {
						return err
					}
					if err := ctx.Check("one note file was created in local notebook", assert.Equal(1, len(files))); err != nil {
						return err
					}

					match, _ := regexp.MatchString(`\d{8}-my-local-note.md`, filepath.Base(files[0]))
					return ctx.Check("local note filename matches pattern", assert.True(match))
				}),
		},
	)
}

// NotebookWorktreeContextScenario verifies that notes created from a worktree use the main project context.
func NotebookWorktreeContextScenario() *harness.Scenario {
	return harness.NewScenario(
		"notebook-worktree-context",
		"Verifies notebook context resolves to the parent project when in a worktree.",
		[]string{"notebook", "structure", "worktree"},
		[]harness.Step{
			harness.NewStep("Setup worktree, create note, and verify context resolution", func(ctx *harness.Context) error {
					// 1. Setup centralized config
					globalYAML := `
version: "1.0"
notebooks:
  rules:
    default: "main"
  definitions:
    main:
      root_dir: "~/.grove/notebooks/nb"
`
					globalConfigDir := filepath.Join(ctx.HomeDir(), ".config", "grove")
					if err := fs.CreateDir(globalConfigDir); err != nil {
						return fmt.Errorf("failed to create global config dir: %w", err)
					}
					if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
						return err
					}

					// 2. Setup project and worktree
					projectDir := ctx.NewDir("worktree-project")
					if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), "name: worktree-project\nversion: '1.0'"); err != nil {
						return err
					}
					repo, err := git.SetupTestRepo(projectDir)
					if err != nil {
						return err
					}
					// Need an initial commit before creating a worktree
					if err := repo.AddCommit("initial commit"); err != nil {
						return err
					}
					worktreeDir := filepath.Join(projectDir, ".grove-worktrees", "feature-branch")
					if err := repo.CreateWorktree(worktreeDir, "feature-branch"); err != nil {
						return err
					}

					// 3. Execute 'nb new' from the worktree
					cmd := ctx.Bin("new", "--no-edit", "note from worktree").Dir(worktreeDir)
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.Error != nil {
						return result.Error
					}

					// 4. Assert file path uses parent project context
					expectedDir := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb", "workspaces", "worktree-project", "inbox")
					files, err := fs.ListFiles(expectedDir)
					if err != nil {
						return fmt.Errorf("failed to read expected note directory: %w", err)
					}
					if err := ctx.Check("one note file was created from worktree", assert.Equal(1, len(files))); err != nil {
						return err
					}

					// 5. Assert 'nb context' from worktree
					cmd = ctx.Bin("context", "--json").Dir(worktreeDir)
					result = cmd.Run()
					return ctx.Verify(func(v *verify.Collector) {
						// Check that current workspace is the worktree
						v.Contains("context has current_workspace", result.Stdout, `"current_workspace"`)
						v.Contains("current workspace is worktree", result.Stdout, `"name": "feature-branch"`)
						// Check that notebook context workspace is the parent project
						v.Contains("context has notebook_context_workspace", result.Stdout, `"notebook_context_workspace"`)
						v.Contains("notebook context is parent project", result.Stdout, `"name": "worktree-project"`)
					})
				}),
		},
	)
}
