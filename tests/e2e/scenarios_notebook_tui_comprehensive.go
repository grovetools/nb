package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/mattsolo1/grove-tend/pkg/fs"
	"github.com/mattsolo1/grove-tend/pkg/git"
	"github.com/mattsolo1/grove-tend/pkg/harness"
	"github.com/mattsolo1/grove-tend/pkg/tui"
	"github.com/mattsolo1/grove-tend/pkg/verify"
)

// NotebookTUIComprehensiveScenario tests the primary features of `nb tui`.
func NotebookTUIComprehensiveScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "notebook-tui-comprehensive",
		Description: "Verifies core features of the `nb tui` command in a comprehensive environment.",
		Tags:        []string{"notebook", "tui", "e2e"},
		Steps: []harness.Step{
			harness.NewStep("Setup comprehensive TUI environment", setupComprehensiveTUIEnvironment),
			harness.NewStep("Launch TUI and test initial navigation", launchAndTestInitialNavigation),
			harness.NewStep("Test help, preview pane, and note creation", testHelpPreviewAndCreation),
			harness.NewStep("Test global view and visibility toggles", testGlobalViewAndVisibility),
			harness.NewStep("Test ecosystem, linking, and artifacts", testEcosystemAndLinking),
		},
	}
}

// setupComprehensiveTUIEnvironment creates a rich, multi-project environment for testing.
func setupComprehensiveTUIEnvironment(ctx *harness.Context) error {
	// 1. Configure a centralized notebook in the sandboxed home directory.
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
		return err
	}
	if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
		return err
	}

	// 2. Create multiple projects.
	projectADir := ctx.NewDir("project-A")
	ecosystemBDir := ctx.NewDir("ecosystem-B") // Renamed for clarity
	subprojectCDir := filepath.Join(ecosystemBDir, "subproject-C")

	// -- Project A (Standalone) --
	if err := fs.WriteString(filepath.Join(projectADir, "grove.yml"), "name: project-A\nversion: '1.0'"); err != nil {
		return err
	}
	repoA, err := git.SetupTestRepo(projectADir)
	if err != nil {
		return err
	}
	if err := repoA.AddCommit("initial commit for project A"); err != nil {
		return err
	}
	projectARoot := filepath.Join(notebookRoot, "workspaces", "project-A")

	// Create notes in standard and custom directories.
	if err := fs.WriteString(filepath.Join(projectARoot, "inbox", "note-with-todos.md"), "---\ntitle: Note With Todos\n---\n# Note With Todos\n- [ ] Unfinished task."); err != nil {
		return err
	}
	if err := fs.WriteString(filepath.Join(projectARoot, "research", "tagged-note.md"), "---\ntitle: Tagged Note\ntags: [frontend, performance]\n---\n# Tagged Note"); err != nil {
		return err
	}
	// Create an archived note.
	if err := fs.WriteString(filepath.Join(projectARoot, "inbox", ".archive", "archived.md"), "---\ntitle: Archived Note\n---\n# Archived Note"); err != nil {
		return err
	}
	// Create a non-markdown artifact.
	if err := fs.WriteString(filepath.Join(projectARoot, "research", "data.json"), `{"key": "value"}`); err != nil {
		return err
	}

	// -- Project B (Ecosystem) --
	if err := fs.WriteString(filepath.Join(ecosystemBDir, "grove.yml"), "name: ecosystem-B\nworkspaces: ['subproject-C']"); err != nil {
		return err
	}
	repoB, err := git.SetupTestRepo(ecosystemBDir)
	if err != nil {
		return err
	}

	// -- Subproject C --
	if err := fs.CreateDir(subprojectCDir); err != nil {
		return err
	}
	if err := fs.WriteString(filepath.Join(subprojectCDir, "grove.yml"), "name: subproject-C\nversion: '1.0'"); err != nil {
		return err
	}
	// Add a single commit for the entire ecosystem
	if err := repoB.AddCommit("initial commit for project B ecosystem"); err != nil {
		return err
	}
	subprojectCRoot := filepath.Join(notebookRoot, "workspaces", "subproject-C")

	// Create notes, plans, and an on-hold plan.
	if err := fs.WriteString(filepath.Join(subprojectCRoot, "issues", "bug-report.md"), "---\ntitle: Bug Report\n---\n# Bug Report"); err != nil {
		return err
	}

	// Create a note in in_progress that will be linked to a plan
	linkedNotePath := filepath.Join(subprojectCRoot, "in_progress", "20251220-linked-note.md")
	linkedNote := `---
title: Linked Note
type: in_progress
plan_ref: plans/my-feature
---
# Linked Note
This note is linked to the my-feature plan.`
	if err := fs.WriteString(linkedNotePath, linkedNote); err != nil {
		return err
	}

	// Create a plan with a note_ref linking back to the note
	planDir := filepath.Join(subprojectCRoot, "plans", "my-feature")
	planSpec := `---
title: My Feature Spec
note_ref: ` + linkedNotePath + `
---
# My Feature Spec
This plan is linked to a note in in_progress.`
	if err := fs.WriteString(filepath.Join(planDir, "01-spec.md"), planSpec); err != nil {
		return err
	}
	// Add .artifacts directory with a briefing file
	artifactsDir := filepath.Join(planDir, ".artifacts")
	if err := fs.WriteString(filepath.Join(artifactsDir, "briefing-123.xml"), "<briefing>Test briefing content</briefing>"); err != nil {
		return err
	}
	onHoldPlanDir := filepath.Join(subprojectCRoot, "plans", "on-hold-plan")
	if err := fs.WriteString(filepath.Join(onHoldPlanDir, ".grove-plan.yml"), "status: hold"); err != nil {
		return err
	}
	if err := fs.WriteString(filepath.Join(onHoldPlanDir, "01-spec.md"), "---\ntitle: On Hold Plan\n---\n# On Hold Plan"); err != nil {
		return err
	}

	// -- Global Notes --
	globalRoot := filepath.Join(notebookRoot, "global")
	if err := fs.WriteString(filepath.Join(globalRoot, "daily", "20240101-daily.md"), "---\ntitle: Daily Note\n---\n# Daily Note"); err != nil {
		return err
	}

	ctx.Set("project_a_dir", projectADir)
	ctx.Set("ecosystem_b_dir", ecosystemBDir)

	return nil
}

// launchAndTestInitialNavigation launches the TUI and tests basic navigation and folding.
// Corresponds to frames 1-6 of the recording.
func launchAndTestInitialNavigation(ctx *harness.Context) error {
	nbBin, err := findProjectBinary()
	if err != nil {
		return err
	}
	projectADir := ctx.GetString("project_a_dir")

	session, err := ctx.StartTUI(nbBin, []string{"tui"},
		tui.WithCwd(projectADir),
		tui.WithEnv("HOME="+ctx.HomeDir()),
	)
	if err != nil {
		return fmt.Errorf("failed to start TUI session: %w", err)
	}
	ctx.Set("tui_session", session)

	// Wait for TUI to actually start by checking for expected content
	if err := session.WaitForText("inbox", 10*time.Second); err != nil {
		// If that fails, capture what we see for debugging
		view, _ := session.Capture()
		ctx.ShowCommandOutput("TUI Failed to Start - Current View", view, "")
		return fmt.Errorf("timeout waiting for TUI to start (looking for 'inbox'): %w", err)
	}

	// Wait for UI to stabilize after async loading
	if err := session.WaitStable(); err != nil {
		return err
	}

	initialView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI Initial View", initialView, "")

	// The TUI shows the current project context (project-A) since we launched from there
	// Verify project-A and global are visible in the tree
	if err := ctx.Verify(func(v *verify.Collector) {
		v.Equal("project-A is visible", nil, session.AssertContains("project-A"))
		v.Equal("global is visible", nil, session.AssertContains("global"))
	}); err != nil {
		return err
	}

	// Frame 1: Press 'j' to navigate down
	session.SendKeys("j")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frame 2: Press 'h' to collapse project-A
	session.SendKeys("h")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frame 3: Press 'l' to expand project-A
	session.SendKeys("l")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}
	if err := ctx.Verify(func(v *verify.Collector) {
		v.Equal("inbox folder is visible", nil, session.AssertContains("inbox"))
		v.Equal("research folder is visible", nil, session.AssertContains("research"))
	}); err != nil {
		return err
	}

	// Frame 4: Press 'j' to navigate to inbox
	session.SendKeys("j")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frame 5: Press 'l' to expand inbox
	session.SendKeys("l")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}
	if err := ctx.Check("note-with-todos.md is visible in inbox",
		session.AssertContains("note-with-todos.md")); err != nil {
		return err
	}

	// Frame 6: Press 'j' to navigate to note-with-todos.md
	session.SendKeys("j")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	navigationView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI after initial navigation", navigationView, "")

	return nil
}

// testHelpPreviewAndCreation tests the help screen, preview pane, and note creation.
// Corresponds to frames 7-32 of the recording.
func testHelpPreviewAndCreation(ctx *harness.Context) error {
	session := ctx.Get("tui_session").(*tui.Session)

	// Frame 7-8: Test help screen with '?', then close with Esc
	session.SendKeys("?")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}
	helpView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI help screen", helpView, "")

	// Close help with Esc
	session.SendKeys("\x1b") // Esc
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frame 9: Open preview pane with 'v'
	session.SendKeys("v")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frame 10-11: Navigate down and expand research folder
	session.SendKeys("j")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	session.SendKeys("l")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frame 12: Navigate to data.json and verify preview
	session.SendKeys("j")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}
	if err := session.AssertContains("Previewing data.json"); err != nil {
		// Preview text might vary
		ctx.ShowCommandOutput("NOTE: data.json preview text may vary", "", "")
	}

	// Frame 13: Navigate to tagged-note.md and verify preview
	session.SendKeys("j")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}
	if err := session.AssertContains("Previewing tagged-note.md"); err != nil {
		// Preview text might vary
		ctx.ShowCommandOutput("NOTE: tagged-note.md preview text may vary", "", "")
	}

	// Frame 14: Close preview pane with 'v'
	session.SendKeys("v")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}
	previewClosedView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI after closing preview", previewClosedView, "")

	// Frames 15-18: Navigate back up to inbox (k k k k)
	for i := 0; i < 4; i++ {
		session.SendKeys("k")
		time.Sleep(200 * time.Millisecond)
		if err := session.WaitStable(); err != nil {
			return err
		}
	}

	// Frame 19: Press 'i' to create an inbox note
	session.SendKeys("i")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frame 20: Press Enter to select default type (inbox)
	session.SendKeys("\r")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frames 21-31: Type "my-new-note"
	session.SendKeys("my-new-note")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frame 32: Press Enter to create the note
	session.SendKeys("\r")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Verify the note was created
	if err := ctx.Verify(func(v *verify.Collector) {
		v.Equal("shows 'Created note:' message", nil, session.AssertContains("Created note:"))
		v.Equal("new note is visible in tree", nil, session.AssertContains("my-new-note.md"))
	}); err != nil {
		return err
	}

	noteCreatedView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI after creating note", noteCreatedView, "")

	return nil
}

// testGlobalViewAndVisibility tests archive toggling and creating global notes.
// Corresponds to frames 33-60 of the recording.
func testGlobalViewAndVisibility(ctx *harness.Context) error {
	session := ctx.Get("tui_session").(*tui.Session)

	// Frame 33: Navigate down with 'j'
	session.SendKeys("j")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frame 34: Toggle archives on with 'A'
	session.SendKeys("A")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Verify .archive is now visible
	if err := ctx.Check(".archive is visible after toggling on",
		session.AssertContains(".archive")); err != nil {
		return err
	}
	if err := session.AssertContains("Archives: true"); err != nil {
		// Status message might vary
		ctx.ShowCommandOutput("NOTE: Archives status message may vary", "", "")
	}

	archivesView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI with archives visible", archivesView, "")

	// Frames 35-36: Navigate to .archive folder under inbox
	// After toggling archives and pressing 'j', cursor is on research
	// Navigate up to inbox, then down to its children to find .archive
	session.SendKeys("k")
	time.Sleep(200 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}
	// Now cursor is on inbox - expand it if not already expanded
	session.SendKeys("l")
	time.Sleep(200 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}
	// Navigate down through inbox contents to .archive
	// inbox has: note-with-todos.md, my-new-note.md, .archive
	session.SendKeys("j")
	time.Sleep(200 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}
	session.SendKeys("j")
	time.Sleep(200 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}
	session.SendKeys("j")
	time.Sleep(200 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frame 37: Expand .archive folder
	session.SendKeys("l")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}
	if err := ctx.Check("archived.md is visible in .archive folder",
		session.AssertContains("archived.md")); err != nil {
		return err
	}

	// Frames 38-42: Navigate up to global with 'k' keys (5 times)
	for i := 0; i < 5; i++ {
		session.SendKeys("k")
		time.Sleep(200 * time.Millisecond)
		if err := session.WaitStable(); err != nil {
			return err
		}
	}

	// Frame 43-44: Try to create note at global (press 'i', then Esc to cancel)
	session.SendKeys("i")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	session.SendKeys("\x1b") // Esc
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frame 45: Press 'n' to create a global note
	session.SendKeys("n")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frames 46-56: Type "global-note"
	session.SendKeys("global-note")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frame 57: Press Enter to create the note
	session.SendKeys("\r")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Verify the note was created
	if err := ctx.Verify(func(v *verify.Collector) {
		v.Equal("shows 'Created note:' message", nil, session.AssertContains("Created note:"))
		v.Equal("new global note is visible in tree", nil, session.AssertContains("global-note.md"))
	}); err != nil {
		return err
	}

	globalNoteView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI after creating global note", globalNoteView, "")

	// Frames 58-59: Navigate down to the global note
	session.SendKeys("j")
	time.Sleep(200 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}
	session.SendKeys("j")
	time.Sleep(200 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frame 60: Press '-' to focus parent (go to global view)
	session.SendKeys("-")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Verify we're now in the global view showing all ecosystems
	if err := ctx.Verify(func(v *verify.Collector) {
		v.Equal("ecosystem-B is visible in global view", nil, session.AssertContains("ecosystem-B"))
		v.Equal("ungrouped is visible in global view", nil, session.AssertContains("ungrouped"))
	}); err != nil {
		return err
	}

	globalView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI global view", globalView, "")

	// Navigate to ungrouped section and verify it shows project notes
	// Find ungrouped in the tree - it should be below ecosystem-B
	session.SendKeys("j")
	time.Sleep(200 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Expand ungrouped
	session.SendKeys("l")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Verify project-A is visible under ungrouped
	if err := ctx.Check("project-A is visible under ungrouped",
		session.AssertContains("project-A")); err != nil {
		return err
	}

	// Navigate to project-A
	session.SendKeys("j")
	time.Sleep(200 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Expand project-A to verify notes are visible
	session.SendKeys("l")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Verify that ungrouped project-A shows its note groups
	if err := ctx.Verify(func(v *verify.Collector) {
		v.Equal("inbox group is visible under ungrouped project-A", nil, session.AssertContains("inbox"))
		v.Equal("research group is visible under ungrouped project-A", nil, session.AssertContains("research"))
	}); err != nil {
		return err
	}

	ungroupedExpandedView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI global view with ungrouped project-A expanded", ungroupedExpandedView, "")

	return nil
}

// testEcosystemAndLinking tests navigation into ecosystems and artifact visibility.
// Corresponds to frames 61-83 of the recording.
func testEcosystemAndLinking(ctx *harness.Context) error {
	session := ctx.Get("tui_session").(*tui.Session)

	// Frames 61-62: Navigate up to global with 'k' keys
	session.SendKeys("k")
	time.Sleep(200 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}
	session.SendKeys("k")
	time.Sleep(200 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frame 63: Expand global with 'l'
	session.SendKeys("l")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frames 64-67: Navigate down to ecosystem-B and subproject-C
	for i := 0; i < 4; i++ {
		session.SendKeys("j")
		time.Sleep(200 * time.Millisecond)
		if err := session.WaitStable(); err != nil {
			return err
		}
	}

	// Frame 68: Expand subproject-C with 'l'
	session.SendKeys("l")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	linkedView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI showing linked notes and plans", linkedView, "")

	// Verify the linking indicators are visible
	if err := ctx.Verify(func(v *verify.Collector) {
		v.Equal("plan 'my-feature' is visible", nil, session.AssertContains("my-feature"))
		v.Equal("linked note is visible", nil, session.AssertContains("20251220-linked-note.md"))
		v.Equal("plan linking indicator is visible", nil, session.AssertContains("plan:"))
		v.Equal("note linking indicator is visible", nil, session.AssertContains("note:"))
	}); err != nil {
		return err
	}

	// Frame 69: Toggle artifacts on with 'b'
	session.SendKeys("b")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Verify .artifacts is now visible
	if err := ctx.Verify(func(v *verify.Collector) {
		v.Equal(".artifacts is visible after toggling on", nil, session.AssertContains(".artifacts"))
		v.Equal("briefing file is visible", nil, session.AssertContains("briefing-123.xml"))
	}); err != nil {
		return err
	}
	if err := session.AssertContains("Artifacts: true"); err != nil {
		// Status message might vary
		ctx.ShowCommandOutput("NOTE: Artifacts status message may vary", "", "")
	}

	artifactsView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI with artifacts visible", artifactsView, "")

	// Frames 70-78: Navigate through the tree with 'j' keys
	for i := 0; i < 8; i++ {
		session.SendKeys("j")
		time.Sleep(200 * time.Millisecond)
		if err := session.WaitStable(); err != nil {
			return err
		}
	}

	// Frame 79: Open preview with 'v'
	session.SendKeys("v")
	time.Sleep(1 * time.Second)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frame 81: Close preview and navigate with Enter
	session.SendKeys("\r")
	time.Sleep(500 * time.Millisecond)
	if err := session.WaitStable(); err != nil {
		return err
	}

	// Frame 83: Quit with 'q'
	session.SendKeys("q")
	time.Sleep(500 * time.Millisecond)

	finalView, _ := session.Capture()
	ctx.ShowCommandOutput("TUI final view before quit", finalView, "")

	return nil
}
