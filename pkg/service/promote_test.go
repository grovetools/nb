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
