package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/grovetools/tend/pkg/assert"
	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"
)

// NotebookGitWorktreeScenario tests the interaction between notebook git repos and grove-flow worktrees.
// This reproduces the bug where running `flow plan init --worktree` from a project creates
// worktrees inside the notebook directory instead of the project, when the notebook has a .git repo.
func NotebookGitWorktreeScenario() *harness.Scenario {
	return harness.NewScenario(
		"notebook-git-worktree-bug",
		"Reproduces bug where flow creates worktrees in notebook instead of project when notebook has .git",
		[]string{"notebook", "git", "worktree", "bug"},
		[]harness.Step{
			harness.NewStep("Setup environment with project and notebook workspace", setupEnvironment),
			harness.NewStep("Run nb git init and verify it targets workspace directory", verifyNbGitInit),
			harness.NewStep("Verify nb context resolves correct workspace", verifyNbContext),
			harness.NewStep("Verify .git is not listed by nb list", verifyGitNotInNbList),
			harness.NewStep("Run flow from project dir and verify worktree location", verifyWorktreeLocation),
			harness.NewStep("Run flow from notebook dir and verify worktree location", verifyWorktreeFromNotebook),
		},
	)
}

// setupEnvironment creates:
// 1. A centralized notebook configuration
// 2. A project with git repo
// 3. A notebook workspace directory (WITHOUT git - that's tested separately)
func setupEnvironment(ctx *harness.Context) error {
	// 1. Configure centralized notebook
	notebookRoot := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb")

	// Add the project to groves so discovery finds it
	globalYAML := fmt.Sprintf(`
version: "1.0"
groves:
  e2e-projects:
    path: "%s"
notebooks:
  rules:
    default: "main"
  definitions:
    main:
      root_dir: "%s"
`, ctx.RootDir, notebookRoot)

	globalConfigDir := filepath.Join(ctx.HomeDir(), ".config", "grove")
	if err := fs.CreateDir(globalConfigDir); err != nil {
		return fmt.Errorf("failed to create global config dir: %w", err)
	}
	if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
		return err
	}

	// 2. Create the actual project with git
	projectDir := ctx.NewDir("my-project")
	if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), "name: my-project\nversion: '1.0'"); err != nil {
		return err
	}
	projectRepo, err := git.SetupTestRepo(projectDir)
	if err != nil {
		return fmt.Errorf("failed to setup project git repo: %w", err)
	}
	if err := projectRepo.AddCommit("initial commit for project"); err != nil {
		return err
	}

	// 3. Create the notebook workspace directory structure (no git yet!)
	notebookWorkspaceDir := filepath.Join(notebookRoot, "workspaces", "my-project")
	if err := fs.CreateDir(notebookWorkspaceDir); err != nil {
		return fmt.Errorf("failed to create notebook workspace dir: %w", err)
	}

	// Create some notebook content
	if err := fs.WriteString(filepath.Join(notebookWorkspaceDir, "inbox", "test-note.md"), "---\ntitle: Test Note\n---\n# Test"); err != nil {
		return err
	}
	if err := fs.CreateDir(filepath.Join(notebookWorkspaceDir, "plans")); err != nil {
		return err
	}

	// Store paths for the next steps
	ctx.Set("project_dir", projectDir)
	ctx.Set("notebook_workspace_dir", notebookWorkspaceDir)
	ctx.Set("notebook_root", notebookRoot)

	// Log the setup for debugging
	ctx.ShowCommandOutput("Setup Complete", fmt.Sprintf(`
Project directory: %s
Notebook workspace directory: %s
Notebook workspace has .git: %v (should be false - nb git init not run yet)
`, projectDir, notebookWorkspaceDir, fs.Exists(filepath.Join(notebookWorkspaceDir, ".git"))), "")

	return nil
}

// verifyNbGitInit runs `nb git init` and verifies it targets the workspace directory (not notebook root)
func verifyNbGitInit(ctx *harness.Context) error {
	notebookWorkspaceDir := ctx.GetString("notebook_workspace_dir")
	notebookRoot := ctx.GetString("notebook_root")

	// Find nb binary
	nbBin, err := findProjectBinary()
	if err != nil {
		return err
	}

	// Run nb git init from the notebook workspace directory
	// Use -W flag to specify the workspace context since we're in an isolated test environment
	projectDir := ctx.GetString("project_dir")
	cmd := ctx.Command(nbBin, "git", "init", "-W", projectDir).
		Dir(notebookWorkspaceDir).
		Env("HOME=" + ctx.HomeDir())

	result := cmd.Run()
	ctx.ShowCommandOutput("nb git init", result.Stdout, result.Stderr)

	if result.Error != nil {
		return fmt.Errorf("nb git init failed: %w", result.Error)
	}

	// Verify the output shows it's targeting the WORKSPACE directory, not notebook root
	if strings.Contains(result.Stdout, notebookRoot) && !strings.Contains(result.Stdout, notebookWorkspaceDir) {
		return fmt.Errorf("BUG: nb git init targeted notebook root (%s) instead of workspace directory (%s)", notebookRoot, notebookWorkspaceDir)
	}

	// Verify .git was created in the WORKSPACE directory
	workspaceGitDir := filepath.Join(notebookWorkspaceDir, ".git")
	if err := ctx.Check(".git created in workspace directory",
		assert.True(fs.Exists(workspaceGitDir))); err != nil {
		return err
	}

	// Verify .git was NOT created in the notebook root
	rootGitDir := filepath.Join(notebookRoot, ".git")
	if err := ctx.Check(".git NOT created in notebook root",
		assert.False(fs.Exists(rootGitDir))); err != nil {
		return fmt.Errorf("BUG: .git was created in notebook root instead of workspace directory")
	}

	// Verify marker file was created in the workspace directory
	markerFile := filepath.Join(notebookWorkspaceDir, "notebook.yml")
	if err := ctx.Check("notebook marker created in workspace directory",
		assert.True(fs.Exists(markerFile))); err != nil {
		return err
	}

	ctx.ShowCommandOutput("nb git init verification", fmt.Sprintf(`
Workspace .git exists: %v (expected: true)
Notebook root .git exists: %v (expected: false)
Marker file exists: %v (expected: true)
`, fs.Exists(workspaceGitDir), fs.Exists(rootGitDir), fs.Exists(markerFile)), "")

	return nil
}

// verifyNbContext verifies that `nb context` correctly resolves the workspace name
func verifyNbContext(ctx *harness.Context) error {
	notebookWorkspaceDir := ctx.GetString("notebook_workspace_dir")

	// Find nb binary
	nbBin, err := findProjectBinary()
	if err != nil {
		return err
	}

	// Run nb context from the notebook workspace directory
	// Use -W flag to specify the workspace context since we're in an isolated test environment
	// Use NO_COLOR=1 to disable ANSI color codes which interfere with string matching
	projectDir := ctx.GetString("project_dir")
	cmd := ctx.Command(nbBin, "context", "-W", projectDir).
		Dir(notebookWorkspaceDir).
		Env("HOME="+ctx.HomeDir(), "NO_COLOR=1")

	result := cmd.Run()
	ctx.ShowCommandOutput("nb context", result.Stdout, result.Stderr)

	if result.Error != nil {
		return fmt.Errorf("nb context failed: %w", result.Error)
	}

	// Verify the output shows "my-project" as the workspace name, not "nb"
	if strings.Contains(result.Stdout, "Name: nb") && !strings.Contains(result.Stdout, "Name: my-project") {
		return fmt.Errorf("BUG: nb context returned 'nb' as workspace name instead of 'my-project'")
	}

	// Verify paths point to the workspace directory
	expectedPath := filepath.Join(notebookWorkspaceDir, "inbox")
	if !strings.Contains(result.Stdout, expectedPath) {
		return fmt.Errorf("BUG: nb context paths don't point to workspace directory. Expected to find: %s\nGot output:\n%s", expectedPath, result.Stdout)
	}

	ctx.ShowCommandOutput("nb context verification", "Workspace name and paths correctly resolved", "")

	return nil
}

// verifyGitNotInNbList verifies that .git directories don't appear in nb list output
func verifyGitNotInNbList(ctx *harness.Context) error {
	projectDir := ctx.GetString("project_dir")

	// Find nb binary
	nbBin, err := findProjectBinary()
	if err != nil {
		return err
	}

	// Run nb list from the project directory
	cmd := ctx.Command(nbBin, "list", "--json").
		Dir(projectDir).
		Env("HOME=" + ctx.HomeDir())

	result := cmd.Run()
	ctx.ShowCommandOutput("nb list --json", result.Stdout, result.Stderr)

	// Verify .git is NOT in the output
	if strings.Contains(result.Stdout, ".git") {
		return fmt.Errorf(".git should not appear in nb list output, but it does")
	}

	// Verify .grove-worktrees is NOT in the output (if worktrees were created in notebook)
	if strings.Contains(result.Stdout, ".grove-worktrees") {
		ctx.ShowCommandOutput("Warning", ".grove-worktrees found in nb list output - this may indicate worktrees in notebook", "")
	}

	ctx.ShowCommandOutput("Verification", ".git correctly filtered from nb list output", "")
	return nil
}

// verifyWorktreeLocation runs flow plan init --worktree and checks where the worktree was created.
// The bug: worktree is created in notebook_workspace_dir/.grove-worktrees/ instead of project_dir/.grove-worktrees/
func verifyWorktreeLocation(ctx *harness.Context) error {
	projectDir := ctx.GetString("project_dir")
	notebookWorkspaceDir := ctx.GetString("notebook_workspace_dir")

	// Find the flow binary - check multiple locations
	flowBin, err := exec.LookPath("flow")
	if err != nil {
		// Try common installation locations
		homeBin := filepath.Join(ctx.HomeDir(), ".grove", "bin", "flow")
		if fs.Exists(homeBin) {
			flowBin = homeBin
		} else {
			// Try the real home directory (test sandbox uses fake home)
			realHomeBin := filepath.Join("/Users/solom4", ".grove", "bin", "flow")
			if fs.Exists(realHomeBin) {
				flowBin = realHomeBin
			} else {
				ctx.ShowCommandOutput("Warning", "flow binary not found, skipping flow command test", "")
				return verifySetupOnly(ctx, projectDir, notebookWorkspaceDir)
			}
		}
	}

	// Run flow plan init --worktree from the PROJECT directory
	planName := "test-plan"
	cmd := ctx.Command(flowBin, "plan", "init", "--worktree", planName).
		Dir(projectDir).
		Env("HOME=" + ctx.HomeDir())

	result := cmd.Run()
	ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)

	// Check for errors (but some warnings are OK)
	if result.Error != nil && !strings.Contains(result.Stderr, "Warning") {
		return fmt.Errorf("flow plan init failed: %w", result.Error)
	}

	// Now verify WHERE the worktree was created

	// EXPECTED: Worktree should be in the PROJECT
	projectWorktreesDir := filepath.Join(projectDir, ".grove-worktrees")
	expectedWorktreePath := filepath.Join(projectWorktreesDir, planName)

	// BUG: Worktree might be in the NOTEBOOK instead
	notebookWorktreesDir := filepath.Join(notebookWorkspaceDir, ".grove-worktrees")
	buggyWorktreePath := filepath.Join(notebookWorktreesDir, planName)

	projectHasWorktree := fs.Exists(expectedWorktreePath)
	notebookHasWorktree := fs.Exists(buggyWorktreePath)

	ctx.ShowCommandOutput("Worktree Location Check", fmt.Sprintf(`
Expected worktree path (project): %s
  Exists: %v

Buggy worktree path (notebook): %s
  Exists: %v
`, expectedWorktreePath, projectHasWorktree, buggyWorktreePath, notebookHasWorktree), "")

	// This is the assertion that will FAIL if the bug is present
	if err := ctx.Check("worktree created in PROJECT directory (not notebook)",
		assert.True(projectHasWorktree)); err != nil {
		// If the bug is present, provide diagnostic info
		if notebookHasWorktree {
			ctx.ShowCommandOutput("BUG CONFIRMED",
				"Worktree was created in NOTEBOOK directory instead of PROJECT directory!", "")
		}
		return err
	}

	// Also verify worktree was NOT created in notebook (double-check)
	if err := ctx.Check("worktree NOT created in notebook directory",
		assert.False(notebookHasWorktree)); err != nil {
		return err
	}

	return nil
}

// verifyWorktreeFromNotebook runs flow plan init --worktree from INSIDE the notebook workspace.
// With the notebook marker file present, flow should refuse to create a worktree here.
func verifyWorktreeFromNotebook(ctx *harness.Context) error {
	notebookWorkspaceDir := ctx.GetString("notebook_workspace_dir")

	// Find flow binary
	flowBin, err := exec.LookPath("flow")
	if err != nil {
		realHomeBin := filepath.Join("/Users/solom4", ".grove", "bin", "flow")
		if fs.Exists(realHomeBin) {
			flowBin = realHomeBin
		} else {
			ctx.ShowCommandOutput("Warning", "flow binary not found, skipping this test", "")
			return nil
		}
	}

	// Run flow plan init --worktree from the NOTEBOOK WORKSPACE directory
	// With the .grove/notebook.yml marker present, this should be rejected
	planName := "notebook-plan"
	cmd := ctx.Command(flowBin, "plan", "init", "--worktree", planName).
		Dir(notebookWorkspaceDir). // Running from NOTEBOOK, not project!
		Env("HOME=" + ctx.HomeDir())

	result := cmd.Run()
	ctx.ShowCommandOutput("flow plan init from NOTEBOOK dir", result.Stdout, result.Stderr)

	// Verify that flow detected the notebook repo and refused to create worktree
	notebookWorktreesDir := filepath.Join(notebookWorkspaceDir, ".grove-worktrees")
	notebookWorktreePath := filepath.Join(notebookWorktreesDir, planName)

	notebookHasWorktree := fs.Exists(notebookWorktreePath)

	ctx.ShowCommandOutput("Worktree Location Check (from notebook)", fmt.Sprintf(`
Running flow from: %s (notebook workspace with .grove/notebook.yml marker)

Notebook worktree path: %s
  Exists: %v

Expected: Worktree should NOT be created (flow should detect notebook and refuse)
`, notebookWorkspaceDir, notebookWorktreePath, notebookHasWorktree), "")

	// With the marker file, flow should either:
	// 1. Refuse to create the worktree and show an error
	// 2. Or the centralized safeguard in workspace.Prepare() blocks it
	if notebookHasWorktree {
		return fmt.Errorf("BUG: worktree was created in notebook despite marker file. The notebook detection is not working")
	}

	// Check if the error message contains the expected notebook rejection message
	if strings.Contains(result.Stderr, "notebook") || strings.Contains(result.Stdout, "notebook") {
		ctx.ShowCommandOutput("FIX VERIFIED",
			"Flow correctly detected the notebook repository and refused to create a worktree.\n"+
				"This protects users from accidentally creating worktrees inside their notebook storage.", "")
	} else {
		ctx.ShowCommandOutput("Note",
			"Worktree was not created (good!), but no explicit notebook error message was shown.\n"+
				"This may indicate the safeguard worked via workspace.Prepare() centralized check.", "")
	}

	return nil
}

// verifySetupOnly just verifies the test setup is correct when flow binary isn't available
func verifySetupOnly(ctx *harness.Context, projectDir, notebookWorkspaceDir string) error {
	// Verify project has .git
	if err := ctx.Check("project has .git directory",
		assert.True(fs.Exists(filepath.Join(projectDir, ".git")))); err != nil {
		return err
	}

	// Verify notebook workspace has .git (the problematic condition)
	if err := ctx.Check("notebook workspace has .git directory (bug precondition)",
		assert.True(fs.Exists(filepath.Join(notebookWorkspaceDir, ".git")))); err != nil {
		return err
	}

	ctx.ShowCommandOutput("Setup Verified",
		"Test environment is correctly configured. Run manually with `flow plan init --worktree test-plan` from the project directory to reproduce.", "")

	return nil
}

// NotebookGitRootScenario tests the `nb git init --root` flag which initializes git
// at the notebook root level (all workspaces in one repo).
func NotebookGitRootScenario() *harness.Scenario {
	return harness.NewScenario(
		"notebook-git-init-root",
		"Tests nb git init --root flag to initialize git at notebook root level",
		[]string{"notebook", "git", "root"},
		[]harness.Step{
			harness.NewStep("Setup notebook environment", setupRootTestEnvironment),
			harness.NewStep("Run nb git init --root and verify it targets notebook root", verifyNbGitInitRoot),
			harness.NewStep("Verify marker file at notebook root", verifyRootMarkerFile),
		},
	)
}

// setupRootTestEnvironment creates a notebook structure for testing --root flag
func setupRootTestEnvironment(ctx *harness.Context) error {
	// Configure centralized notebook
	notebookRoot := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb")

	// Create a project for context
	projectDir := ctx.NewDir("root-test-project")
	if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), "name: root-test-project\nversion: '1.0'"); err != nil {
		return err
	}
	projectRepo, err := git.SetupTestRepo(projectDir)
	if err != nil {
		return fmt.Errorf("failed to setup project git repo: %w", err)
	}
	if err := projectRepo.AddCommit("initial commit"); err != nil {
		return err
	}

	globalYAML := fmt.Sprintf(`
version: "1.0"
groves:
  e2e-projects:
    path: "%s"
notebooks:
  rules:
    default: "main"
  definitions:
    main:
      root_dir: "%s"
`, ctx.RootDir, notebookRoot)

	globalConfigDir := filepath.Join(ctx.HomeDir(), ".config", "grove")
	if err := fs.CreateDir(globalConfigDir); err != nil {
		return fmt.Errorf("failed to create global config dir: %w", err)
	}
	if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
		return err
	}

	// Create notebook structure with multiple workspaces
	workspace1 := filepath.Join(notebookRoot, "workspaces", "root-test-project")
	workspace2 := filepath.Join(notebookRoot, "workspaces", "another-project")
	globalDir := filepath.Join(notebookRoot, "global")

	for _, dir := range []string{workspace1, workspace2, globalDir} {
		if err := fs.CreateDir(filepath.Join(dir, "inbox")); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}

	// Add some content
	if err := fs.WriteString(filepath.Join(workspace1, "inbox", "note1.md"), "# Note 1"); err != nil {
		return err
	}
	if err := fs.WriteString(filepath.Join(workspace2, "inbox", "note2.md"), "# Note 2"); err != nil {
		return err
	}
	if err := fs.WriteString(filepath.Join(globalDir, "inbox", "global-note.md"), "# Global Note"); err != nil {
		return err
	}

	ctx.Set("project_dir", projectDir)
	ctx.Set("notebook_root", notebookRoot)
	ctx.Set("workspace1", workspace1)
	ctx.Set("workspace2", workspace2)

	ctx.ShowCommandOutput("Setup Complete", fmt.Sprintf(`
Notebook root: %s
Workspace 1: %s
Workspace 2: %s
Global: %s
`, notebookRoot, workspace1, workspace2, globalDir), "")

	return nil
}

// verifyNbGitInitRoot runs `nb git init --root` and verifies it targets the notebook root
func verifyNbGitInitRoot(ctx *harness.Context) error {
	notebookRoot := ctx.GetString("notebook_root")
	workspace1 := ctx.GetString("workspace1")
	projectDir := ctx.GetString("project_dir")

	nbBin, err := findProjectBinary()
	if err != nil {
		return err
	}

	// Run nb git init --root
	cmd := ctx.Command(nbBin, "git", "init", "--root", "-W", projectDir).
		Dir(workspace1).
		Env("HOME=" + ctx.HomeDir())

	result := cmd.Run()
	ctx.ShowCommandOutput("nb git init --root", result.Stdout, result.Stderr)

	if result.Error != nil {
		return fmt.Errorf("nb git init --root failed: %w", result.Error)
	}

	// Verify output mentions the notebook root
	if !strings.Contains(result.Stdout, notebookRoot) {
		return fmt.Errorf("expected output to mention notebook root %s", notebookRoot)
	}

	// Verify .git was created at NOTEBOOK ROOT
	rootGitDir := filepath.Join(notebookRoot, ".git")
	if err := ctx.Check(".git created at notebook root",
		assert.True(fs.Exists(rootGitDir))); err != nil {
		return err
	}

	// Verify .git was NOT created at workspace level
	workspaceGitDir := filepath.Join(workspace1, ".git")
	if err := ctx.Check(".git NOT created at workspace level",
		assert.False(fs.Exists(workspaceGitDir))); err != nil {
		return fmt.Errorf("BUG: .git was created at workspace level instead of notebook root")
	}

	ctx.ShowCommandOutput("Verification", fmt.Sprintf(`
Notebook root .git exists: %v (expected: true)
Workspace .git exists: %v (expected: false)
`, fs.Exists(rootGitDir), fs.Exists(workspaceGitDir)), "")

	return nil
}

// verifyRootMarkerFile verifies the marker file was created at the notebook root
func verifyRootMarkerFile(ctx *harness.Context) error {
	notebookRoot := ctx.GetString("notebook_root")
	workspace1 := ctx.GetString("workspace1")

	// Verify marker at notebook root
	rootMarker := filepath.Join(notebookRoot, "notebook.yml")
	if err := ctx.Check("marker file at notebook root",
		assert.True(fs.Exists(rootMarker))); err != nil {
		return err
	}

	// Verify NO marker at workspace level
	workspaceMarker := filepath.Join(workspace1, "notebook.yml")
	if err := ctx.Check("NO marker at workspace level",
		assert.False(fs.Exists(workspaceMarker))); err != nil {
		return fmt.Errorf("BUG: marker was created at workspace level instead of notebook root")
	}

	// Verify .gitignore at notebook root
	rootGitignore := filepath.Join(notebookRoot, ".gitignore")
	if err := ctx.Check(".gitignore at notebook root",
		assert.True(fs.Exists(rootGitignore))); err != nil {
		return err
	}

	ctx.ShowCommandOutput("Root Marker Verification", fmt.Sprintf(`
Root marker exists: %v (expected: true)
Workspace marker exists: %v (expected: false)
Root .gitignore exists: %v (expected: true)
`, fs.Exists(rootMarker), fs.Exists(workspaceMarker), fs.Exists(rootGitignore)), "")

	return nil
}
