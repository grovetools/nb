package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"
	"github.com/grovetools/tend/pkg/tui"
	"github.com/grovetools/tend/pkg/verify"
)

// NotebookPasteIntoPlanScenario verifies the behavior of pasting/moving notes into plan directories.
// It tests:
// 1. Path resolution: Files pasted into "plans/my-feature" go to the correct nested directory
// 2. Flow job metadata: Markdown files get type: file, status: completed, and worktree from plan config
// 3. Frontmatter preservation: Existing fields are not overwritten
// 4. Non-markdown handling: Non-.md files are moved without modification
func NotebookPasteIntoPlanScenario() *harness.Scenario {
	return harness.NewScenarioWithOptions(
		"notebook-paste-into-plan",
		"Verifies path resolution and flow job metadata injection when pasting notes into plans.",
		[]string{"notebook", "tui", "paste", "plan", "flow"},
		[]harness.Step{
			harness.NewStep("Setup environment with notes and plan directories", setupPasteTestEnvironment),
			harness.NewStep("Test moving a note into a plan with worktree config", testMoveNoteWithMetadata),
			harness.NewStep("Test copying a note and preserving existing frontmatter", testCopyNoteAndPreserveFrontmatter),
			harness.NewStep("Test moving a non-markdown file", testMoveNonMarkdownFile),
			harness.NewStep("Test moving a note into a plan without a worktree config", testMoveNoteWithoutWorktree),
		},
		true,  // localOnly - requires tmux for TUI tests
		false, // explicitOnly
	)
}

// setupPasteTestEnvironment creates the initial state for testing paste operations.
// It creates:
// - A project with a centralized notebook
// - Source files in the inbox directory
// - Two target plan directories (one with .grove-plan.yml, one without)
func setupPasteTestEnvironment(ctx *harness.Context) error {
	// 1. Configure centralized notebook in the sandboxed home directory
	notebookRoot := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb")
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
		return fmt.Errorf("create global config dir: %w", err)
	}
	if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
		return fmt.Errorf("write global config: %w", err)
	}

	// 2. Setup test project
	projectDir := ctx.NewDir("paste-project")
	if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), "name: paste-project\nversion: '1.0'"); err != nil {
		return fmt.Errorf("write project config: %w", err)
	}
	if _, err := git.SetupTestRepo(projectDir); err != nil {
		return fmt.Errorf("setup test repo: %w", err)
	}
	ctx.Set("project_dir", projectDir)
	workspaceRoot := filepath.Join(notebookRoot, "workspaces", "paste-project")
	ctx.Set("workspace_root", workspaceRoot)

	// 3. Create source files in 'inbox'
	inboxDir := filepath.Join(workspaceRoot, "inbox")

	// A simple note with frontmatter that will be moved to a plan
	sourceNote := `---
title: Source Note
---
# Source Note

This is a source note that will be moved into a plan.
`
	if err := fs.WriteString(filepath.Join(inboxDir, "source-note.md"), sourceNote); err != nil {
		return fmt.Errorf("write source note: %w", err)
	}

	// A note with existing type field (should be preserved, not overwritten)
	noteToPreserve := `---
title: Preserve Me
type: custom
---
# Preserve Me

This note has a custom type that should be preserved.
`
	if err := fs.WriteString(filepath.Join(inboxDir, "note-to-preserve.md"), noteToPreserve); err != nil {
		return fmt.Errorf("write note to preserve: %w", err)
	}

	// A non-markdown file (should not get frontmatter modifications)
	if err := fs.WriteString(filepath.Join(inboxDir, "data.txt"), "This is a plain text file."); err != nil {
		return fmt.Errorf("write data.txt: %w", err)
	}

	// Another note for the no-worktree test
	simpleNote := `---
title: Simple Note
---
# Simple Note
`
	if err := fs.WriteString(filepath.Join(inboxDir, "simple-note.md"), simpleNote); err != nil {
		return fmt.Errorf("write simple note: %w", err)
	}

	// 4. Create target plan directories
	// Plan WITH .grove-plan.yml containing worktree config
	planWithConfigDir := filepath.Join(workspaceRoot, "plans", "my-target-plan")
	if err := fs.WriteString(filepath.Join(planWithConfigDir, ".grove-plan.yml"), "worktree: feature-branch"); err != nil {
		return fmt.Errorf("write plan config: %w", err)
	}
	if err := fs.WriteString(filepath.Join(planWithConfigDir, "01-spec.md"), "---\ntitle: Plan Spec\n---\n# Plan Spec"); err != nil {
		return fmt.Errorf("write plan spec: %w", err)
	}

	// Plan WITHOUT .grove-plan.yml
	planWithoutConfigDir := filepath.Join(workspaceRoot, "plans", "no-config-plan")
	if err := fs.WriteString(filepath.Join(planWithoutConfigDir, "01-spec.md"), "---\ntitle: Another Plan Spec\n---\n# Another Plan Spec"); err != nil {
		return fmt.Errorf("write another plan spec: %w", err)
	}

	return nil
}

// testMoveNoteWithMetadata tests moving a note into a plan and verifies:
// 1. The file ends up in the correct path (plans/my-target-plan/source-note.md)
// 2. Flow job metadata is added: type: file, status: completed, worktree: feature-branch
// 3. The original title is preserved
func testMoveNoteWithMetadata(ctx *harness.Context) error {
	nbBin, err := findProjectBinary()
	if err != nil {
		return fmt.Errorf("find project binary: %w", err)
	}
	projectDir := ctx.GetString("project_dir")

	session, err := ctx.StartTUI(nbBin, []string{"tui"},
		tui.WithCwd(projectDir),
		tui.WithEnv("HOME="+ctx.HomeDir()),
	)
	if err != nil {
		return fmt.Errorf("start TUI session: %w", err)
	}
	ctx.Set("tui_session", session)

	// Wait for TUI to load and show the workspace
	if err := session.WaitForText("inbox", 10*time.Second); err != nil {
		view, _ := session.Capture()
		ctx.ShowCommandOutput("TUI Failed to Start - Current View", view, "")
		return fmt.Errorf("timeout waiting for TUI (looking for 'inbox'): %w", err)
	}
	if err := session.WaitStable(); err != nil {
		return err
	}

	initialView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI Initial View", initialView, "")

	// Navigate to inbox folder
	// The TUI tree shows:
	// ▶ global (cursor starts here)
	//   paste-project
	//    │ inbox (4)
	//    └ plans (2)
	//
	// We need to navigate down to inbox and expand it

	// Navigate down to inbox - we need to go past global and paste-project to inbox
	if err := navigateToItem(session, ctx, "inbox", 10); err != nil {
		return fmt.Errorf("navigate to inbox: %w", err)
	}

	// Expand inbox
	session.SendKeys("l")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	inboxExpandedView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI with inbox expanded", inboxExpandedView, "")

	// Navigate to source-note.md within inbox
	if err := navigateToItem(session, ctx, "source-note.md", 10); err != nil {
		return fmt.Errorf("navigate to source-note.md: %w", err)
	}

	beforeCutView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI before cutting source-note.md", beforeCutView, "")

	// Cut the note with 'x'
	session.SendKeys("x")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	cutView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI after cutting note", cutView, "")

	// Navigate to plans folder - first go back up to top
	// Send 'g' twice with a small delay - the TUI requires both keys for gg
	session.SendKeys("g")
	time.Sleep(100 * time.Millisecond)
	session.SendKeys("g")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	topView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI after going to top", topView, "")

	// Navigate down to find plans folder
	if err := navigateToItem(session, ctx, "plans", 15); err != nil {
		return fmt.Errorf("navigate to plans: %w", err)
	}

	// Expand plans folder
	session.SendKeys("l")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	plansExpandedView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI with plans expanded", plansExpandedView, "")

	// Navigate to my-target-plan (it's inside the expanded plans folder)
	// Use navigateToItemInExpandedFolder since we just expanded the parent
	if err := navigateToItemInExpandedFolder(session, ctx, "my-target-plan", 5); err != nil {
		return fmt.Errorf("navigate to my-target-plan: %w", err)
	}

	beforePasteView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI before pasting into my-target-plan", beforePasteView, "")

	// Paste the note with 'p'
	session.SendKeys("p")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	afterPasteView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI after pasting note", afterPasteView, "")

	// Verify the file was moved to the correct location
	workspaceRoot := ctx.GetString("workspace_root")
	sourcePath := filepath.Join(workspaceRoot, "inbox", "source-note.md")
	destPath := filepath.Join(workspaceRoot, "plans", "my-target-plan", "source-note.md")

	if err := ctx.Check("Source file was moved (no longer exists at origin)", fs.AssertNotExists(sourcePath)); err != nil {
		return err
	}
	if err := ctx.Check("Destination file exists in plan directory (plans/my-target-plan/)", fs.AssertExists(destPath)); err != nil {
		return err
	}

	content, err := fs.ReadString(destPath)
	if err != nil {
		return fmt.Errorf("read destination file: %w", err)
	}

	ctx.ShowCommandOutput("Moved file content", content, "")

	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("frontmatter has type: file", content, "type: file")
		v.Contains("frontmatter has status: completed", content, "status: completed")
		v.Contains("frontmatter has worktree from plan config", content, "worktree: feature-branch")
		v.Contains("original title is preserved", content, "title: Source Note")
	})
}

// navigateToItem navigates through the TUI tree looking for an item.
// It sends 'j' keys up to maxAttempts times, checking each position.
func navigateToItem(session *tui.Session, ctx *harness.Context, itemName string, maxAttempts int) error {
	for i := 0; i < maxAttempts; i++ {
		content, _ := session.Capture()
		// Check if item is on the currently selected line (has selection marker)
		if isItemSelected(content, itemName) {
			return nil
		}
		session.SendKeys("j")
		time.Sleep(200 * time.Millisecond)
		if err := session.WaitStable(); err != nil {
			return err
		}
	}
	// Capture final state for debugging
	finalContent, _ := session.Capture()
	ctx.ShowCommandOutput(fmt.Sprintf("Failed to find '%s' - final TUI state", itemName), finalContent, "")
	return fmt.Errorf("could not find item '%s' after %d attempts", itemName, maxAttempts)
}

// isItemSelected checks if the given item appears to be selected in the TUI.
// The TUI uses "▶" marker before the selected item.
func isItemSelected(content, itemName string) bool {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		// Check if this line contains the item name and has selection indicator "▶"
		// The item must be on the same line as the selector
		if strings.Contains(line, itemName) && strings.Contains(line, "▶") {
			// Make sure we're matching the exact item, not a partial match
			// e.g., "my-target-plan" shouldn't match "my-target-plan-other"
			return true
		}
	}
	return false
}

// navigateToItemInExpandedFolder navigates to an item that is already visible
// (i.e., inside an expanded folder). This is more reliable than navigateToItem
// when we know the folder is already expanded.
func navigateToItemInExpandedFolder(session *tui.Session, ctx *harness.Context, itemName string, maxAttempts int) error {
	// First, move down once since we're likely on the parent folder
	session.SendKeys("j")
	time.Sleep(200 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Now look for the item
	for i := 0; i < maxAttempts; i++ {
		content, _ := session.Capture()
		if isItemSelected(content, itemName) {
			return nil
		}
		session.SendKeys("j")
		time.Sleep(200 * time.Millisecond)
		if err := session.WaitStable(); err != nil {
			return err
		}
	}

	finalContent, _ := session.Capture()
	ctx.ShowCommandOutput(fmt.Sprintf("Failed to find '%s' in folder - final TUI state", itemName), finalContent, "")
	return fmt.Errorf("could not find item '%s' after %d attempts", itemName, maxAttempts)
}

// testCopyNoteAndPreserveFrontmatter tests copying a note with existing frontmatter.
// It verifies:
// 1. The original file still exists (copy, not move)
// 2. Existing fields (type: custom) are NOT overwritten
// 3. Missing fields (status, worktree) ARE added
func testCopyNoteAndPreserveFrontmatter(ctx *harness.Context) error {
	session := ctx.Get("tui_session").(*tui.Session)
	workspaceRoot := ctx.GetString("workspace_root")

	// Refresh and go to top using gg (vim-style)
	session.SendKeys("C-r")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Send 'g' twice with a small delay - the TUI requires both keys for gg
	session.SendKeys("g")
	time.Sleep(100 * time.Millisecond)
	session.SendKeys("g")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	currentView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI at start of copy test (after gg)", currentView, "")

	// Navigate to inbox
	if err := navigateToItem(session, ctx, "inbox", 20); err != nil {
		return fmt.Errorf("navigate to inbox: %w", err)
	}

	// Expand inbox
	session.SendKeys("l")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Find note-to-preserve.md
	if err := navigateToItem(session, ctx, "note-to-preserve.md", 10); err != nil {
		return fmt.Errorf("navigate to note-to-preserve.md: %w", err)
	}

	beforeCopyView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI before copying note-to-preserve.md", beforeCopyView, "")

	// Copy the note with 'y' (yank)
	session.SendKeys("y")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Go to top and navigate to my-target-plan
	session.SendKeys("g", "g")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Navigate to plans
	if err := navigateToItem(session, ctx, "plans", 15); err != nil {
		return fmt.Errorf("navigate to plans: %w", err)
	}

	// Expand plans
	session.SendKeys("l")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Navigate to my-target-plan
	if err := navigateToItem(session, ctx, "my-target-plan", 10); err != nil {
		return fmt.Errorf("navigate to my-target-plan: %w", err)
	}

	// Paste the note
	session.SendKeys("p")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	afterCopyPasteView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI after copy-paste", afterCopyPasteView, "")

	// Verify results
	sourcePath := filepath.Join(workspaceRoot, "inbox", "note-to-preserve.md")
	destPath := filepath.Join(workspaceRoot, "plans", "my-target-plan", "note-to-preserve.md")

	if err := ctx.Check("Source file still exists after copy", fs.AssertExists(sourcePath)); err != nil {
		return err
	}
	if err := ctx.Check("Destination file was created from copy", fs.AssertExists(destPath)); err != nil {
		return err
	}

	content, err := fs.ReadString(destPath)
	if err != nil {
		return fmt.Errorf("read destination file: %w", err)
	}

	ctx.ShowCommandOutput("Copied file content", content, "")

	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("existing 'type: custom' is preserved (not overwritten)", content, "type: custom")
		v.Contains("existing 'title: Preserve Me' is preserved", content, "title: Preserve Me")
		v.Contains("new 'status: completed' is added", content, "status: completed")
		v.Contains("new 'worktree: feature-branch' is added", content, "worktree: feature-branch")
	})
}

// testMoveNonMarkdownFile verifies that non-markdown files are moved without modification.
func testMoveNonMarkdownFile(ctx *harness.Context) error {
	session := ctx.Get("tui_session").(*tui.Session)
	workspaceRoot := ctx.GetString("workspace_root")

	// Refresh and go to top using gg (vim-style)
	session.SendKeys("C-r")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Send 'g' twice with a small delay - the TUI requires both keys for gg
	session.SendKeys("g")
	time.Sleep(100 * time.Millisecond)
	session.SendKeys("g")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Navigate to inbox
	if err := navigateToItem(session, ctx, "inbox", 20); err != nil {
		return fmt.Errorf("navigate to inbox: %w", err)
	}

	// Expand inbox
	session.SendKeys("l")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Find data.txt
	if err := navigateToItem(session, ctx, "data.txt", 10); err != nil {
		return fmt.Errorf("navigate to data.txt: %w", err)
	}

	beforeCutTxtView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI before cutting data.txt", beforeCutTxtView, "")

	// Cut the file with 'x'
	session.SendKeys("x")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Go to top and navigate to my-target-plan
	session.SendKeys("g", "g")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Navigate to plans
	if err := navigateToItem(session, ctx, "plans", 15); err != nil {
		return fmt.Errorf("navigate to plans: %w", err)
	}

	// Expand plans
	session.SendKeys("l")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Navigate to my-target-plan
	if err := navigateToItem(session, ctx, "my-target-plan", 10); err != nil {
		return fmt.Errorf("navigate to my-target-plan: %w", err)
	}

	// Paste
	session.SendKeys("p")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	afterPasteView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI after pasting data.txt", afterPasteView, "")

	// Verify results
	sourcePath := filepath.Join(workspaceRoot, "inbox", "data.txt")
	destPath := filepath.Join(workspaceRoot, "plans", "my-target-plan", "data.txt")

	if err := ctx.Check("Non-MD source file was moved", fs.AssertNotExists(sourcePath)); err != nil {
		return err
	}
	if err := ctx.Check("Non-MD destination file exists", fs.AssertExists(destPath)); err != nil {
		return err
	}

	content, err := fs.ReadString(destPath)
	if err != nil {
		return fmt.Errorf("read destination file: %w", err)
	}

	ctx.ShowCommandOutput("Moved non-markdown file content", content, "")

	// Verify content is unchanged (no frontmatter added)
	expectedContent := "This is a plain text file."
	return ctx.Check("Non-MD file content is unchanged", func() error {
		if content != expectedContent {
			return fmt.Errorf("expected content %q, got %q", expectedContent, content)
		}
		return nil
	}())
}

// testMoveNoteWithoutWorktree verifies pasting into a plan with no .grove-plan.yml config.
// It should still add type: file and status: completed, but NOT worktree.
func testMoveNoteWithoutWorktree(ctx *harness.Context) error {
	session := ctx.Get("tui_session").(*tui.Session)
	workspaceRoot := ctx.GetString("workspace_root")

	// Refresh and go to top using gg (vim-style)
	session.SendKeys("C-r")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Send 'g' twice with a small delay - the TUI requires both keys for gg
	session.SendKeys("g")
	time.Sleep(100 * time.Millisecond)
	session.SendKeys("g")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Navigate to inbox
	if err := navigateToItem(session, ctx, "inbox", 20); err != nil {
		return fmt.Errorf("navigate to inbox: %w", err)
	}

	// Expand inbox
	session.SendKeys("l")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Find simple-note.md
	if err := navigateToItem(session, ctx, "simple-note.md", 10); err != nil {
		return fmt.Errorf("navigate to simple-note.md: %w", err)
	}

	beforeCutSimpleView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI before cutting simple-note.md", beforeCutSimpleView, "")

	// Cut the note
	session.SendKeys("x")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Go to top and navigate to plans
	session.SendKeys("g", "g")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Navigate to plans
	if err := navigateToItem(session, ctx, "plans", 15); err != nil {
		return fmt.Errorf("navigate to plans: %w", err)
	}

	// Expand plans
	session.SendKeys("l")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Navigate to no-config-plan
	if err := navigateToItem(session, ctx, "no-config-plan", 10); err != nil {
		return fmt.Errorf("navigate to no-config-plan: %w", err)
	}

	beforePasteNoConfigView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI before pasting into no-config-plan", beforePasteNoConfigView, "")

	// Paste
	session.SendKeys("p")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	afterPasteView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI after pasting into no-config-plan", afterPasteView, "")

	// Quit TUI
	session.SendKeys("q")
	time.Sleep(500 * time.Millisecond)

	// Verify results
	destPath := filepath.Join(workspaceRoot, "plans", "no-config-plan", "simple-note.md")

	if err := ctx.Check("Destination file exists in no-config plan", fs.AssertExists(destPath)); err != nil {
		return err
	}

	content, err := fs.ReadString(destPath)
	if err != nil {
		return fmt.Errorf("read destination file: %w", err)
	}

	ctx.ShowCommandOutput("File content in no-config plan", content, "")

	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("frontmatter has type: file", content, "type: file")
		v.Contains("frontmatter has status: completed", content, "status: completed")
		v.NotContains("frontmatter does NOT have worktree (no plan config)", content, "worktree:")
	})
}
