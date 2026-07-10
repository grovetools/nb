package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
)

// TestPromoteNoteToJob_WarnsWhenWorktreeMissing verifies the warn-on-miss
// behavior added for XDG worktrees: when a plan declares a worktree that
// cannot be resolved under any worktree base (legacy .grove-worktrees OR the
// XDG WorktreesDir()/<DirIdentifier> base), PromoteNoteToJob must log a
// warning rather than silently dropping the repository/branch resolution.
//
// Sandboxing is mandatory for any XDG-touching test: XDG_DATA_HOME is pinned
// to a temp dir and GROVE_HOME is cleared (it beats XDG_DATA_HOME in
// paths.getDataHome()), so workspace.FindWorktreePath probes the sandboxed
// XDG base instead of the real ~/.local/share/grove.
func TestPromoteNoteToJob_WarnsWhenWorktreeMissing(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("GROVE_HOME", "")

	// A real grove project at cwd so GetProjectByPath(".") resolves to a
	// non-nil node; its path becomes the ecosystem root probed for the
	// (intentionally absent) worktree.
	ecoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(ecoRoot, "grove.yml"),
		[]byte("version: '1.0'\nname: my-eco\n"), 0o644); err != nil {
		t.Fatalf("write grove.yml: %v", err)
	}
	t.Chdir(ecoRoot)

	// A plan that declares a worktree which exists in NO base — the miss the
	// warning guards against.
	planDir := filepath.Join(t.TempDir(), "my-plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan: %v", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, ".grove-plan.yml"),
		[]byte("worktree: ghost-worktree\n"), 0o644); err != nil {
		t.Fatalf("write plan config: %v", err)
	}

	// A note under a notebook-style current/ dir so the in_progress sibling
	// can be created.
	noteDir := filepath.Join(t.TempDir(), "nb", "current")
	if err := os.MkdirAll(noteDir, 0o755); err != nil {
		t.Fatalf("mkdir note dir: %v", err)
	}
	notePath := filepath.Join(noteDir, "20250101-test-note.md")
	noteContent := "---\nid: 20250101-test-note\ntitle: Test Note\n---\n\nNote body content.\n"
	if err := os.WriteFile(notePath, []byte(noteContent), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	logger, hook := logtest.NewNullLogger()
	svc := &Service{Logger: logrus.NewEntry(logger)}

	jobFilename, err := svc.PromoteNoteToJob(notePath, planDir, PromoteOptions{})
	if err != nil {
		t.Fatalf("PromoteNoteToJob: %v", err)
	}
	if jobFilename == "" {
		t.Fatal("expected a job filename, got empty")
	}

	// The miss must surface as a warning carrying the worktree + ecosystem
	// identity — not be swallowed.
	var warned bool
	for _, e := range hook.AllEntries() {
		if e.Level == logrus.WarnLevel && strings.Contains(e.Message, "Worktree not found") {
			warned = true
			if got := e.Data["worktree"]; got != "ghost-worktree" {
				t.Errorf("warning worktree field = %v, want ghost-worktree", got)
			}
			if _, ok := e.Data["ecosystem"]; !ok {
				t.Error("warning missing ecosystem field")
			}
		}
	}
	if !warned {
		t.Fatalf("expected a 'Worktree not found' warning; got entries: %v", hook.AllEntries())
	}
}

// TestPromoteNoteToJob_WritesSkillFrontmatter verifies parity with
// `flow plan add --skill`: a non-empty PromoteOptions.Skill is written into the
// promoted job's frontmatter as `skill:`. Resolution itself happens at job run
// time (via the executor), so promote only needs to persist the field.
func TestPromoteNoteToJob_WritesSkillFrontmatter(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("GROVE_HOME", "")

	ecoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(ecoRoot, "grove.yml"),
		[]byte("version: '1.0'\nname: my-eco\n"), 0o644); err != nil {
		t.Fatalf("write grove.yml: %v", err)
	}
	t.Chdir(ecoRoot)

	planDir := filepath.Join(t.TempDir(), "my-plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan: %v", err)
	}

	noteDir := filepath.Join(t.TempDir(), "nb", "current")
	if err := os.MkdirAll(noteDir, 0o755); err != nil {
		t.Fatalf("mkdir note dir: %v", err)
	}
	notePath := filepath.Join(noteDir, "20250101-skill-note.md")
	noteContent := "---\nid: 20250101-skill-note\ntitle: Skill Note\n---\n\nNote body.\n"
	if err := os.WriteFile(notePath, []byte(noteContent), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	logger, _ := logtest.NewNullLogger()
	svc := &Service{Logger: logrus.NewEntry(logger)}

	jobFilename, err := svc.PromoteNoteToJob(notePath, planDir, PromoteOptions{
		JobType: "interactive_agent",
		Skill:   "grove-feature-subcoordinator",
	})
	if err != nil {
		t.Fatalf("PromoteNoteToJob: %v", err)
	}

	jobBytes, err := os.ReadFile(filepath.Join(planDir, jobFilename))
	if err != nil {
		t.Fatalf("read job file: %v", err)
	}
	if !strings.Contains(string(jobBytes), "skill: grove-feature-subcoordinator") {
		t.Errorf("job frontmatter missing skill field; got:\n%s", jobBytes)
	}
}

// promoteTestEnv builds the standard sandbox for promote tests: XDG pinned to
// a temp dir, a grove project as cwd, an empty plan dir, and a notebook-style
// current/ dir for notes. Returns the plan dir and the note dir.
func promoteTestEnv(t *testing.T) (planDir, noteDir string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("GROVE_HOME", "")

	ecoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(ecoRoot, "grove.yml"),
		[]byte("version: '1.0'\nname: my-eco\n"), 0o644); err != nil {
		t.Fatalf("write grove.yml: %v", err)
	}
	t.Chdir(ecoRoot)

	planDir = filepath.Join(t.TempDir(), "my-plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan: %v", err)
	}

	noteDir = filepath.Join(t.TempDir(), "nb", "current")
	if err := os.MkdirAll(noteDir, 0o755); err != nil {
		t.Fatalf("mkdir note dir: %v", err)
	}
	return planDir, noteDir
}

func writeTestNote(t *testing.T, noteDir, name, title string) string {
	t.Helper()
	notePath := filepath.Join(noteDir, name)
	content := "---\nid: " + strings.TrimSuffix(name, ".md") + "\ntitle: " + title + "\n---\n\nBody of " + title + ".\n"
	if err := os.WriteFile(notePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	return notePath
}

// TestPromoteNotesToJobs_Batch verifies the roster path: several notes promote
// into one plan in a single call, each getting its own job file and its own
// in_progress move.
func TestPromoteNotesToJobs_Batch(t *testing.T) {
	planDir, noteDir := promoteTestEnv(t)
	logger, _ := logtest.NewNullLogger()
	svc := &Service{Logger: logrus.NewEntry(logger)}

	notes := []string{
		writeTestNote(t, noteDir, "20250101-note-a.md", "Note A"),
		writeTestNote(t, noteDir, "20250102-note-b.md", "Note B"),
		writeTestNote(t, noteDir, "20250103-note-c.md", "Note C"),
	}

	results, err := svc.PromoteNotesToJobs(notes, planDir, PromoteOptions{})
	if err != nil {
		t.Fatalf("PromoteNotesToJobs: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	inProgressDir := filepath.Join(filepath.Dir(noteDir), "in_progress")
	for i, r := range results {
		if r.JobFilename == "" {
			t.Errorf("result %d has empty job filename", i)
		}
		if _, err := os.Stat(filepath.Join(planDir, r.JobFilename)); err != nil {
			t.Errorf("job file %s missing: %v", r.JobFilename, err)
		}
		// The original note must be gone; its in_progress twin must exist.
		if _, err := os.Stat(r.NotePath); !os.IsNotExist(err) {
			t.Errorf("note %s still in current/ after promotion", r.NotePath)
		}
		if _, err := os.Stat(filepath.Join(inProgressDir, filepath.Base(r.NotePath))); err != nil {
			t.Errorf("note %s missing from in_progress: %v", filepath.Base(r.NotePath), err)
		}
	}
	if results[0].JobFilename == results[1].JobFilename {
		t.Errorf("batch produced duplicate job filenames: %s", results[0].JobFilename)
	}
}

// TestPromoteNotesToJobs_PreflightBlocksBatch verifies all-inputs-validated-
// before-first-move: one bad note path means NO note moves.
func TestPromoteNotesToJobs_PreflightBlocksBatch(t *testing.T) {
	planDir, noteDir := promoteTestEnv(t)
	logger, _ := logtest.NewNullLogger()
	svc := &Service{Logger: logrus.NewEntry(logger)}

	good := writeTestNote(t, noteDir, "20250101-good.md", "Good Note")
	missing := filepath.Join(noteDir, "20250102-missing.md")

	results, err := svc.PromoteNotesToJobs([]string{good, missing}, planDir, PromoteOptions{})
	if err == nil {
		t.Fatal("expected preflight error for missing note")
	}
	if len(results) != 0 {
		t.Fatalf("expected no promotions, got %d", len(results))
	}
	// The good note must be untouched.
	if _, statErr := os.Stat(good); statErr != nil {
		t.Errorf("good note was moved despite preflight failure: %v", statErr)
	}
}

// TestPromoteNoteToJob_DependsOn verifies chain wiring: a valid dependency is
// written to the job frontmatter; an unknown dependency fails BEFORE the note
// moves (flow plan add parity — deps are job filenames).
func TestPromoteNoteToJob_DependsOn(t *testing.T) {
	planDir, noteDir := promoteTestEnv(t)
	logger, _ := logtest.NewNullLogger()
	svc := &Service{Logger: logrus.NewEntry(logger)}

	// Seed the plan with a first job to depend on.
	contract := writeTestNote(t, noteDir, "20250101-contract.md", "Contract")
	contractJob, err := svc.PromoteNoteToJob(contract, planDir, PromoteOptions{})
	if err != nil {
		t.Fatalf("seeding contract job: %v", err)
	}

	// Unknown dependency: rejected pre-move.
	child := writeTestNote(t, noteDir, "20250102-child.md", "Child")
	if _, err := svc.PromoteNoteToJob(child, planDir, PromoteOptions{
		DependsOn: []string{"99-nonexistent.md"},
	}); err == nil {
		t.Fatal("expected error for unknown dependency")
	}
	if _, statErr := os.Stat(child); statErr != nil {
		t.Errorf("note moved despite dependency validation failure: %v", statErr)
	}

	// Valid dependency: written to frontmatter.
	childJob, err := svc.PromoteNoteToJob(child, planDir, PromoteOptions{
		DependsOn: []string{contractJob},
	})
	if err != nil {
		t.Fatalf("PromoteNoteToJob with valid dep: %v", err)
	}
	jobBytes, err := os.ReadFile(filepath.Join(planDir, childJob))
	if err != nil {
		t.Fatalf("read child job: %v", err)
	}
	if !strings.Contains(string(jobBytes), "depends_on:") || !strings.Contains(string(jobBytes), contractJob) {
		t.Errorf("child job frontmatter missing depends_on %s; got:\n%s", contractJob, jobBytes)
	}
}

// TestPreviewPromoteNotes verifies the dry-run path: previews are computed,
// nothing on disk changes.
func TestPreviewPromoteNotes(t *testing.T) {
	planDir, noteDir := promoteTestEnv(t)
	logger, _ := logtest.NewNullLogger()
	svc := &Service{Logger: logrus.NewEntry(logger)}

	notes := []string{
		writeTestNote(t, noteDir, "20250101-note-a.md", "Note A"),
		writeTestNote(t, noteDir, "20250102-note-b.md", "Note B"),
	}

	previews, err := svc.PreviewPromoteNotes(notes, planDir, PromoteOptions{JobType: "interactive_agent"})
	if err != nil {
		t.Fatalf("PreviewPromoteNotes: %v", err)
	}
	if len(previews) != 2 {
		t.Fatalf("expected 2 previews, got %d", len(previews))
	}
	if previews[0].Title != "Note A" || previews[1].Title != "Note B" {
		t.Errorf("preview titles wrong: %+v", previews)
	}
	if previews[0].JobType != "interactive_agent" {
		t.Errorf("preview job type = %s, want interactive_agent", previews[0].JobType)
	}
	if previews[0].PredictedJobFile == "" || previews[0].PredictedJobFile == previews[1].PredictedJobFile {
		t.Errorf("predicted filenames not distinct: %+v", previews)
	}

	// Dry run: notes still in current/, plan dir still empty of jobs.
	for _, n := range notes {
		if _, err := os.Stat(n); err != nil {
			t.Errorf("dry run moved note %s: %v", n, err)
		}
	}
	entries, _ := os.ReadDir(planDir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			t.Errorf("dry run created job file %s", e.Name())
		}
	}
}

func TestStripFrontmatterBlock(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "valid frontmatter block",
			input: `---
id: test-123
title: Test Note
---

# Content

This is the body.`,
			expected: "\n# Content\n\nThis is the body.",
		},
		{
			name: "no frontmatter",
			input: `# Content

Just body content.`,
			expected: `# Content

Just body content.`,
		},
		{
			name: "broken YAML in frontmatter",
			input: `---
id: broken-yaml-note
title: treemux: drag-select offset ~2 lines; copy banner reflows
tags: [issues, grovetools]
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
---

# Issue Description

This note has a colon in the title which, when unquoted, creates invalid YAML.`,
			expected: "\n# Issue Description\n\nThis note has a colon in the title which, when unquoted, creates invalid YAML.",
		},
		{
			name: "frontmatter with empty body",
			input: `---
id: test
title: Test
---

`,
			expected: "\n",
		},
		{
			name: "only opening delimiter",
			input: `---
incomplete frontmatter`,
			expected: `---
incomplete frontmatter`,
		},
		{
			name: "frontmatter with multiple --- in body",
			input: `---
id: test
title: Test
---

# Content

Some text with --- separator

More content`,
			expected: "\n# Content\n\nSome text with --- separator\n\nMore content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripFrontmatterBlock(tt.input)
			if got != tt.expected {
				t.Errorf("stripFrontmatterBlock() = %q, want %q", got, tt.expected)
			}
		})
	}
}
