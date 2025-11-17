package main

import (
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/mattsolo1/grove-tend/pkg/assert"
	"github.com/mattsolo1/grove-tend/pkg/fs"
	"github.com/mattsolo1/grove-tend/pkg/git"
	"github.com/mattsolo1/grove-tend/pkg/harness"
)

// NotebookCentralizedStructureScenario verifies note creation in a centralized notebook.
func NotebookCentralizedStructureScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "notebook-centralized-structure",
		Description: "Verifies note creation with a centralized notebook configuration.",
		Tags:        []string{"notebook", "structure", "centralized"},
		Steps: []harness.Step{
			{
				Name: "Setup centralized config, create note, and verify path",
				Func: func(ctx *harness.Context) error {
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
					nbBin, err := findProjectBinary()
					if err != nil {
						return err
					}
					cmd := ctx.Command(nbBin, "new", "--no-edit", "my test note").Dir(projectDir)
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
					if err := assert.Equal(1, len(files), "expected one note file to be created"); err != nil {
						return err
					}
					match, _ := regexp.MatchString(`\d{8}-my-test-note.md`, filepath.Base(files[0]))
					if err := assert.True(match, "note filename should match pattern"); err != nil {
						return err
					}

					// 5. Assert 'nb context'
					cmd = ctx.Command(nbBin, "context", "--json").Dir(projectDir)
					result = cmd.Run()
					if err := assert.Contains(result.Stdout, `"name": "test-project"`, "context should show correct workspace name"); err != nil {
						return err
					}
					return assert.Contains(result.Stdout, `"inbox":`, "context should have inbox path")
				},
			},
		},
	}
}

// NotebookLocalStructureScenario verifies note creation in local mode.
func NotebookLocalStructureScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "notebook-local-structure",
		Description: "Verifies note creation with a local notebook configuration (root_dir: '').",
		Tags:        []string{"notebook", "structure", "local"},
		Steps: []harness.Step{
			{
				Name: "Setup local config, create note, and verify path",
				Func: func(ctx *harness.Context) error {
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
					nbBin, err := findProjectBinary()
					if err != nil {
						return err
					}
					cmd := ctx.Command(nbBin, "new", "--no-edit", "my local note").Dir(projectDir)
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
					if err := assert.Equal(1, len(files), "expected one note file"); err != nil {
						return err
					}
					match, _ := regexp.MatchString(`\d{8}-my-local-note.md`, filepath.Base(files[0]))
					return assert.True(match, "note filename should match pattern")
				},
			},
		},
	}
}

// NotebookWorktreeContextScenario verifies that notes created from a worktree use the main project context.
func NotebookWorktreeContextScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "notebook-worktree-context",
		Description: "Verifies notebook context resolves to the parent project when in a worktree.",
		Tags:        []string{"notebook", "structure", "worktree"},
		Steps: []harness.Step{
			{
				Name: "Setup worktree, create note, and verify context resolution",
				Func: func(ctx *harness.Context) error {
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
					nbBin, err := findProjectBinary()
					if err != nil {
						return err
					}
					cmd := ctx.Command(nbBin, "new", "--no-edit", "note from worktree").Dir(worktreeDir)
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
					if err := assert.Equal(1, len(files), "expected one note file"); err != nil {
						return err
					}

					// 5. Assert 'nb context' from worktree
					cmd = ctx.Command(nbBin, "context", "--json").Dir(worktreeDir)
					result = cmd.Run()
					// Check that current workspace is the worktree
					if err := assert.Contains(result.Stdout, `"current_workspace"`, "should have current_workspace"); err != nil {
						return err
					}
					if err := assert.Contains(result.Stdout, `"name": "feature-branch"`, "current workspace should be worktree"); err != nil {
						return err
					}
					// Check that notebook context workspace is the parent project
					if err := assert.Contains(result.Stdout, `"notebook_context_workspace"`, "should have notebook_context_workspace"); err != nil {
						return err
					}
					return assert.Contains(result.Stdout, `"name": "worktree-project"`, "notebook context should be parent project")
				},
			},
		},
	}
}
