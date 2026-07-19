package frontmatter

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantFM   *Frontmatter
		wantBody string
		wantErr  bool
	}{
		{
			name: "valid frontmatter",
			content: `---
id: test-123
title: Test Note
aliases: []
tags: [test, example]
repository: myrepo
branch: main
created: 2023-01-01 10:00:00
modified: 2023-01-02 11:00:00
---

# Test Content

This is the body.`,
			wantFM: &Frontmatter{
				ID:         "test-123",
				Title:      "Test Note",
				Aliases:    []string{},
				Tags:       []string{"test", "example"},
				Repository: "myrepo",
				Branch:     "main",
				Created:    "2023-01-01 10:00:00",
				Modified:   "2023-01-02 11:00:00",
			},
			wantBody: "\n# Test Content\n\nThis is the body.",
			wantErr:  false,
		},
		{
			name:     "no frontmatter",
			content:  "# Just a title\n\nSome content.",
			wantFM:   nil,
			wantBody: "# Just a title\n\nSome content.",
			wantErr:  false,
		},
		{
			name: "invalid yaml",
			content: `---
id: test
title: [invalid
---

Body`,
			wantFM: nil,
			wantBody: `---
id: test
title: [invalid
---

Body`,
			wantErr: true,
		},
		{
			name: "minimal frontmatter",
			content: `---
id: minimal
title: Minimal Note
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
---

Content`,
			wantFM: &Frontmatter{
				ID:       "minimal",
				Title:    "Minimal Note",
				Aliases:  []string{},
				Tags:     []string{},
				Created:  "2023-01-01 10:00:00",
				Modified: "2023-01-01 10:00:00",
			},
			wantBody: "\nContent",
			wantErr:  false,
		},
		{
			name: "frontmatter with priority",
			content: `---
id: prio
title: Critical Note
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
priority: p0
---

Content`,
			wantFM: &Frontmatter{
				ID:       "prio",
				Title:    "Critical Note",
				Aliases:  []string{},
				Tags:     []string{},
				Created:  "2023-01-01 10:00:00",
				Modified: "2023-01-01 10:00:00",
				Priority: "p0",
			},
			wantBody: "\nContent",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFM, gotBody, err := Parse(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotFM, tt.wantFM) {
				t.Errorf("Parse() gotFM = %+v, want %+v", gotFM, tt.wantFM)
			}
			if gotBody != tt.wantBody {
				t.Errorf("Parse() gotBody = %q, want %q", gotBody, tt.wantBody)
			}
		})
	}
}

func TestBuild(t *testing.T) {
	tests := []struct {
		name string
		fm   *Frontmatter
		want string
	}{
		{
			name: "complete frontmatter",
			fm: &Frontmatter{
				ID:         "test-123",
				Title:      "Test Note",
				Aliases:    []string{"test", "example"},
				Tags:       []string{"tag1", "tag2"},
				Repository: "myrepo",
				Branch:     "main",
				Created:    "2023-01-01 10:00:00",
				Modified:   "2023-01-02 11:00:00",
			},
			want: `---
id: test-123
title: Test Note
aliases: [test, example]
tags: [tag1, tag2]
repository: myrepo
branch: main
created: 2023-01-01 10:00:00
modified: 2023-01-02 11:00:00
---`,
		},
		{
			name: "minimal frontmatter",
			fm: &Frontmatter{
				ID:       "minimal",
				Title:    "Minimal",
				Aliases:  []string{},
				Tags:     []string{},
				Created:  "2023-01-01 10:00:00",
				Modified: "2023-01-01 10:00:00",
			},
			want: `---
id: minimal
title: Minimal
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
---`,
		},
		{
			name: "with special characters",
			fm: &Frontmatter{
				ID:       "special",
				Title:    "Note: Special, Characters",
				Aliases:  []string{"alias:1", "alias,2"},
				Tags:     []string{"tag:special", "tag,comma"},
				Created:  "2023-01-01 10:00:00",
				Modified: "2023-01-01 10:00:00",
			},
			want: `---
id: special
title: "Note: Special, Characters"
aliases: ["alias:1", "alias,2"]
tags: ["tag:special", "tag,comma"]
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
---`,
		},
		{
			name: "with priority",
			fm: &Frontmatter{
				ID:       "prio",
				Title:    "Critical Note",
				Aliases:  []string{},
				Tags:     []string{},
				Created:  "2023-01-01 10:00:00",
				Modified: "2023-01-01 10:00:00",
				Priority: "p0",
			},
			want: `---
id: prio
title: Critical Note
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
priority: p0
---`,
		},
		{
			name: "empty priority omitted",
			fm: &Frontmatter{
				ID:       "noprio",
				Title:    "Normal Note",
				Aliases:  []string{},
				Tags:     []string{},
				Created:  "2023-01-01 10:00:00",
				Modified: "2023-01-01 10:00:00",
				Priority: "",
			},
			want: `---
id: noprio
title: Normal Note
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
---`,
		},
		{
			name: "colon-containing title (from issue)",
			fm: &Frontmatter{
				ID:       "20260611-122606-treemux",
				Title:    "treemux: drag-select offset ~2 lines; copy banner reflows content during drag; chrome not selectable",
				Aliases:  []string{},
				Tags:     []string{"issues", "grovetools"},
				Created:  "2023-01-01 10:00:00",
				Modified: "2023-01-01 10:00:00",
			},
			want: `---
id: 20260611-122606-treemux
title: "treemux: drag-select offset ~2 lines; copy banner reflows content during drag; chrome not selectable"
aliases: []
tags: [issues, grovetools]
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
---`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Build(tt.fm)
			if got != tt.want {
				t.Errorf("Build() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildContent(t *testing.T) {
	fm := &Frontmatter{
		ID:       "test",
		Title:    "Test",
		Aliases:  []string{},
		Tags:     []string{},
		Created:  "2023-01-01 10:00:00",
		Modified: "2023-01-01 10:00:00",
	}

	tests := []struct {
		name        string
		body        string
		wantSpacing bool
	}{
		{
			name:        "body without leading newline",
			body:        "# Title\n\nContent",
			wantSpacing: true,
		},
		{
			name:        "body with leading newline",
			body:        "\n# Title\n\nContent",
			wantSpacing: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildContent(fm, tt.body)
			frontmatter := Build(fm)

			if tt.wantSpacing {
				want := frontmatter + "\n\n" + tt.body
				if got != want {
					t.Errorf("BuildContent() spacing incorrect, got = %q, want = %q", got, want)
				}
			} else {
				want := frontmatter + "\n" + tt.body
				if got != want {
					t.Errorf("BuildContent() spacing incorrect, got = %q, want = %q", got, want)
				}
			}
		})
	}
}

func TestFormatAndParseTimestamp(t *testing.T) {
	now := time.Date(2023, 1, 15, 14, 30, 45, 0, time.UTC)

	formatted := FormatTimestamp(now)
	expected := "2023-01-15T14:30:45Z" // new writes are RFC3339 UTC

	if formatted != expected {
		t.Errorf("FormatTimestamp() = %q, want %q", formatted, expected)
	}

	parsed, err := ParseTimestamp(formatted)
	if err != nil {
		t.Errorf("ParseTimestamp() error = %v", err)
	}

	if !parsed.Equal(now) {
		t.Errorf("ParseTimestamp() = %v, want %v", parsed, now)
	}
}

func TestExtractPathTags(t *testing.T) {
	tests := []struct {
		name     string
		noteType string
		want     []string
	}{
		{
			name:     "simple path",
			noteType: "issues",
			want:     []string{"issues"},
		},
		{
			name:     "nested path",
			noteType: "issues/bugs/critical",
			want:     []string{"issues", "bugs", "critical"},
		},
		{
			name:     "empty path",
			noteType: "",
			want:     []string{},
		},
		{
			name:     "path with spaces",
			noteType: " issues / bugs ",
			want:     []string{"issues", "bugs"},
		},
		{
			name:     "path with empty segments",
			noteType: "issues//bugs",
			want:     []string{"issues", "bugs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractPathTags(tt.noteType)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractPathTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMergeTags(t *testing.T) {
	tests := []struct {
		name    string
		sources [][]string
		want    []string
	}{
		{
			name:    "merge with duplicates",
			sources: [][]string{{"a", "b"}, {"b", "c"}, {"a", "d"}},
			want:    []string{"a", "b", "c", "d"},
		},
		{
			name:    "empty sources",
			sources: [][]string{{}, {}, {}},
			want:    []string{},
		},
		{
			name:    "single source",
			sources: [][]string{{"a", "b", "c"}},
			want:    []string{"a", "b", "c"},
		},
		{
			name:    "with empty strings",
			sources: [][]string{{"a", "", "b"}, {"", "c"}},
			want:    []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeTags(tt.sources...)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MergeTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	// Test that we can parse and rebuild content without losing data
	original := &Frontmatter{
		ID:         "roundtrip-123",
		Title:      "Round Trip Test",
		Aliases:    []string{"rt", "test"},
		Tags:       []string{"test", "frontmatter"},
		Repository: "testrepo",
		Branch:     "feature",
		Created:    "2023-01-01 10:00:00",
		Modified:   "2023-01-02 11:00:00",
		Started:    "2023-01-01 09:30:00",
	}

	body := "# Test Content\n\nThis is a test."

	// Build content
	content := BuildContent(original, body)

	// Parse it back
	parsed, parsedBody, err := Parse(content)
	if err != nil {
		t.Fatalf("Failed to parse round-trip content: %v", err)
	}

	// Compare frontmatter
	if !reflect.DeepEqual(parsed, original) {
		t.Errorf("Round trip frontmatter mismatch\noriginal: %+v\nparsed: %+v", original, parsed)
	}

	// Compare body (accounting for added newline)
	expectedBody := "\n" + body
	if parsedBody != expectedBody {
		t.Errorf("Round trip body mismatch\noriginal: %q\nparsed: %q", expectedBody, parsedBody)
	}
}

func TestRoundTripWithColonInTitle(t *testing.T) {
	// Test that titles with colons round-trip correctly (regression test for double-frontmatter bug)
	original := &Frontmatter{
		ID:       "20260611-122606-treemux",
		Title:    "treemux: drag-select offset ~2 lines; copy banner reflows content during drag",
		Aliases:  []string{},
		Tags:     []string{"issues", "grovetools"},
		Created:  "2023-01-01 10:00:00",
		Modified: "2023-01-01 10:00:00",
	}

	body := "# Issue Description\n\nThis is a test with a colon in the title."

	// Build content
	content := BuildContent(original, body)

	// Parse it back
	parsed, parsedBody, err := Parse(content)
	if err != nil {
		t.Fatalf("Failed to parse round-trip content with colon in title: %v", err)
	}

	// Compare frontmatter - must match exactly
	if !reflect.DeepEqual(parsed, original) {
		t.Errorf("Round trip with colon in title failed\noriginal: %+v\nparsed: %+v", original, parsed)
	}

	// Verify title is preserved exactly
	if parsed.Title != original.Title {
		t.Errorf("Title mismatch after round trip: got %q, want %q", parsed.Title, original.Title)
	}

	// Compare body (accounting for added newline)
	expectedBody := "\n" + body
	if parsedBody != expectedBody {
		t.Errorf("Round trip body mismatch\noriginal: %q\nparsed: %q", expectedBody, parsedBody)
	}
}

// TestPlanJobRoundTrip verifies the new plan_job field parses and re-emits
// deterministically alongside plan_ref.
func TestPlanJobRoundTrip(t *testing.T) {
	content := "---\nid: n1\ntitle: T\naliases: []\ntags: []\nplan_ref: plans/my-feature\nplan_job: 01-foo.md\ncreated: 2026-01-01T00:00:00Z\nmodified: 2026-01-01T00:00:00Z\n---\n\nBody.\n"
	fm, body, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if fm.PlanRef != "plans/my-feature" {
		t.Errorf("PlanRef = %q, want plans/my-feature", fm.PlanRef)
	}
	if fm.PlanJob != "01-foo.md" {
		t.Errorf("PlanJob = %q, want 01-foo.md", fm.PlanJob)
	}
	rebuilt := BuildContent(fm, body)
	if !strings.Contains(rebuilt, "plan_ref: plans/my-feature") {
		t.Errorf("rebuilt missing plan_ref; got:\n%s", rebuilt)
	}
	if !strings.Contains(rebuilt, "plan_job: 01-foo.md") {
		t.Errorf("rebuilt missing plan_job; got:\n%s", rebuilt)
	}
	// Re-parse must be stable.
	fm2, _, err := Parse(rebuilt)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	if fm2.PlanRef != fm.PlanRef || fm2.PlanJob != fm.PlanJob {
		t.Errorf("round-trip changed link fields: %q/%q -> %q/%q", fm.PlanRef, fm.PlanJob, fm2.PlanRef, fm2.PlanJob)
	}
}

// TestUpdateField pins the `nb internal update-frontmatter` field-update
// contract: an empty value CLEARS the link fields (plan_ref, plan_job) — flow's
// demote path depends on it — while other fields reject an empty value.
func TestUpdateField(t *testing.T) {
	// Empty value clears plan_ref and plan_job.
	fm := &Frontmatter{PlanRef: "plans/foo", PlanJob: "01-foo.md"}
	if err := UpdateField(fm, "plan_ref", ""); err != nil {
		t.Fatalf("clear plan_ref: %v", err)
	}
	if fm.PlanRef != "" {
		t.Errorf("plan_ref not cleared: %q", fm.PlanRef)
	}
	if err := UpdateField(fm, "plan_job", ""); err != nil {
		t.Fatalf("clear plan_job: %v", err)
	}
	if fm.PlanJob != "" {
		t.Errorf("plan_job not cleared: %q", fm.PlanJob)
	}
	// Cleared fields must be omitted from Build (omitempty).
	built := Build(fm)
	if strings.Contains(built, "plan_ref:") || strings.Contains(built, "plan_job:") {
		t.Errorf("cleared link fields still emitted:\n%s", built)
	}

	// Setting new values works.
	if err := UpdateField(fm, "plan_ref", "plans/bar"); err != nil {
		t.Fatalf("set plan_ref: %v", err)
	}
	if err := UpdateField(fm, "plan_job", "02-bar.md"); err != nil {
		t.Fatalf("set plan_job: %v", err)
	}
	if fm.PlanRef != "plans/bar" || fm.PlanJob != "02-bar.md" {
		t.Errorf("set failed: %q / %q", fm.PlanRef, fm.PlanJob)
	}

	// Non-link fields reject an empty value.
	if err := UpdateField(fm, "title", ""); err == nil {
		t.Error("expected empty title to be rejected")
	}
	// Unsupported field name errors.
	if err := UpdateField(fm, "bogus", "x"); err == nil {
		t.Error("expected unsupported field to be rejected")
	}
}
