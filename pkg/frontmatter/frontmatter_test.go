package frontmatter

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
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

// TestExtraRoundTripPreservesProducerKeys pins the core promise of the
// structured-note contract: unknown frontmatter keys survive a Parse→Build
// cycle. Before the Extra inline map, every such cycle (move, copy, internal
// update-frontmatter) silently stripped producer keys like pomodoro_*.
func TestExtraRoundTripPreservesProducerKeys(t *testing.T) {
	content := `---
id: n1
title: Pomodoro Block
aliases: []
tags: [pomodoro, work-block]
created: 2026-08-05T09:00:00Z
modified: 2026-08-05T09:50:00Z
idempotency_key: pomodoro:blk-1
pomodoro_block_id: blk-1
pomodoro_jobs_completed: 3
source: pomodoro-panel
---

Body.
`
	fm, body, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := fm.ExtraValue("pomodoro_block_id"); got != "blk-1" {
		t.Errorf("Extra[pomodoro_block_id] = %v, want blk-1", got)
	}
	if got := fm.ExtraValue("pomodoro_jobs_completed"); got != 3 {
		t.Errorf("Extra[pomodoro_jobs_completed] = %v (%T), want int 3", got, got)
	}
	if got := fm.ExtraValue(IdempotencyKeyField); got != "pomodoro:blk-1" {
		t.Errorf("Extra[%s] = %v, want pomodoro:blk-1", IdempotencyKeyField, got)
	}

	rebuilt := BuildContent(fm, body)
	for _, want := range []string{
		"idempotency_key: pomodoro:blk-1",
		"pomodoro_block_id: blk-1",
		"pomodoro_jobs_completed: 3",
		"source: pomodoro-panel",
	} {
		if !strings.Contains(rebuilt, want) {
			t.Errorf("rebuilt missing %q; got:\n%s", want, rebuilt)
		}
	}

	// The rebuild must be stable: a second cycle produces identical bytes
	// (extras are emitted in sorted order, so map iteration cannot churn).
	fm2, body2, err := Parse(rebuilt)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	if again := BuildContent(fm2, body2); again != rebuilt {
		t.Errorf("Parse→Build not stable:\nfirst:\n%s\nsecond:\n%s", rebuilt, again)
	}
	if len(fm2.Extra) != len(fm.Extra) {
		t.Errorf("Extra key count changed: %d -> %d", len(fm.Extra), len(fm2.Extra))
	}
	for key := range fm.Extra {
		if before, after := fm.ExtraValue(key), fm2.ExtraValue(key); !reflect.DeepEqual(before, after) {
			t.Errorf("Extra[%s] changed across round-trip: %#v -> %#v", key, before, after)
		}
	}
}

// TestBuildKnownFieldPrefixUnchanged guards against churn: a note that Build
// itself produced (no extension keys) must Parse→Build byte-identically, so
// landing the Extra map rewrites nothing in existing notebooks.
func TestBuildKnownFieldPrefixUnchanged(t *testing.T) {
	fm := &Frontmatter{
		ID:         "stable-1",
		Title:      "Stable Note",
		Aliases:    []string{},
		Tags:       []string{"issues"},
		Repository: "myrepo",
		Branch:     "main",
		Created:    "2026-01-01T00:00:00Z",
		Modified:   "2026-01-02T00:00:00Z",
		Priority:   "p2",
	}
	content := BuildContent(fm, "# Stable Note\n")
	parsed, body, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if rebuilt := BuildContent(parsed, body); rebuilt != content {
		t.Errorf("round-trip churned a Build-produced note:\nbefore:\n%s\nafter:\n%s", content, rebuilt)
	}
}

// TestBuildEmitsName pins the fix for the name field: it was parsed into the
// struct but never re-emitted, so Parse→Build cycles stripped it — the same
// silent-loss class as the unknown-key drop.
func TestBuildEmitsName(t *testing.T) {
	fm := &Frontmatter{
		ID: "n1", Title: "Skill", Aliases: []string{}, Tags: []string{},
		Created: "2026-01-01T00:00:00Z", Modified: "2026-01-01T00:00:00Z",
		Name: "my-skill",
	}
	built := Build(fm)
	if !strings.Contains(built, "name: my-skill") {
		t.Errorf("Build dropped name:\n%s", built)
	}
	parsed, _, err := Parse(built + "\n\nBody")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.Name != "my-skill" {
		t.Errorf("name did not round-trip: %q", parsed.Name)
	}
}

// TestBuildSkipsCollidingExtraKeys: an Extra entry that names a known field
// must not produce a duplicate YAML key.
func TestBuildSkipsCollidingExtraKeys(t *testing.T) {
	fm := &Frontmatter{
		ID: "n1", Title: "Real Title", Aliases: []string{}, Tags: []string{},
		Created: "2026-01-01T00:00:00Z", Modified: "2026-01-01T00:00:00Z",
	}
	// SetExtra refuses the colliding key outright; force it in to prove Build
	// is defensive too, since Extra is a plain map any caller can write.
	if err := fm.SetExtra("title", "Smuggled"); err == nil {
		t.Error("SetExtra accepted a key that names a typed field")
	}
	if err := fm.SetExtra("custom_key", "kept"); err != nil {
		t.Fatalf("SetExtra(custom_key): %v", err)
	}
	var smuggled yaml.Node
	if err := smuggled.Encode("Smuggled"); err != nil {
		t.Fatal(err)
	}
	fm.Extra["title"] = smuggled
	built := Build(fm)
	if strings.Count(built, "title:") != 1 {
		t.Errorf("duplicate title key emitted:\n%s", built)
	}
	if !strings.Contains(built, "custom_key: kept") {
		t.Errorf("legitimate extra key dropped:\n%s", built)
	}
}

// TestApplyProducerFields pins the merge policy of the structured contract:
// nb-owned fields are skipped (nb wins), known fields are typed and replaced,
// namespaced fields land in Extra, malformed keys and shapes are rejected.
func TestApplyProducerFields(t *testing.T) {
	fm := &Frontmatter{
		ID:       "keep-id",
		Title:    "Old Title",
		Aliases:  []string{},
		Tags:     []string{"seed"},
		Created:  "2026-01-01T00:00:00Z",
		Modified: "2026-01-01T00:00:00Z",
	}
	err := ApplyProducerFields(fm, mustProducerFields(t, map[string]any{
		"id":                "forged-id",             // nb-owned: ignored
		"created":           "1999-01-01T00:00:00Z",  // nb-owned: ignored
		"type":              "somewhere/else",        // nb-owned: ignored
		"title":             "New Title",             // known: replaced
		"tags":              []any{"pomodoro", "wb"}, // known: replaced, coerced
		"priority":          "p1",                    // known: replaced
		"pomodoro_block_id": "blk-1",                 // producer: Extra
		"pomodoro_tokens":   1234,                    // producer: Extra
	}))
	if err != nil {
		t.Fatalf("ApplyProducerFields: %v", err)
	}
	if fm.ID != "keep-id" || fm.Created != "2026-01-01T00:00:00Z" || fm.Type != "" {
		t.Errorf("nb-owned fields not preserved: id=%q created=%q type=%q", fm.ID, fm.Created, fm.Type)
	}
	if fm.Title != "New Title" || fm.Priority != "p1" {
		t.Errorf("known fields not replaced: title=%q priority=%q", fm.Title, fm.Priority)
	}
	if !reflect.DeepEqual(fm.Tags, []string{"pomodoro", "wb"}) {
		t.Errorf("tags not coerced/replaced: %v", fm.Tags)
	}
	if fm.ExtraValue("pomodoro_block_id") != "blk-1" || fm.ExtraValue("pomodoro_tokens") != 1234 {
		t.Errorf("producer fields not carried into Extra: %#v", fm.Extra)
	}

	// Shape violations are named errors, not silent coercions.
	if err := ApplyProducerFields(fm, mustProducerFields(t, map[string]any{"title": 42})); err == nil {
		t.Error("expected non-string title to be rejected")
	}
	if err := ApplyProducerFields(fm, mustProducerFields(t, map[string]any{"tags": "not-a-list"})); err == nil {
		t.Error("expected non-list tags to be rejected")
	}
	if err := ApplyProducerFields(fm, mustProducerFields(t, map[string]any{"bad key!": "x"})); err == nil {
		t.Error("expected malformed extension key to be rejected")
	}
}

// TestLoadProducerFields verifies both accepted encodings (JSON is a YAML
// subset) and the mapping-root requirement.
func TestLoadProducerFields(t *testing.T) {
	dir := t.TempDir()

	jsonPath := filepath.Join(dir, "fm.json")
	if err := os.WriteFile(jsonPath, []byte(`{"pomodoro_block_id":"blk-1","pomodoro_jobs_completed":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fields, err := LoadProducerFields(jsonPath)
	if err != nil {
		t.Fatalf("LoadProducerFields(json): %v", err)
	}
	if got := decodeField(t, fields, "pomodoro_block_id"); got != "blk-1" {
		t.Errorf("json pomodoro_block_id = %#v", got)
	}
	if got := decodeField(t, fields, "pomodoro_jobs_completed"); got != 3 {
		t.Errorf("json pomodoro_jobs_completed = %#v", got)
	}

	yamlPath := filepath.Join(dir, "fm.yml")
	if err := os.WriteFile(yamlPath, []byte("hn_item_id: 42\ntags:\n  - hn\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fields, err = LoadProducerFields(yamlPath)
	if err != nil {
		t.Fatalf("LoadProducerFields(yaml): %v", err)
	}
	if got := decodeField(t, fields, "hn_item_id"); got != 42 {
		t.Errorf("yaml hn_item_id = %#v", got)
	}

	badPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badPath, []byte(`["not","a","mapping"]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProducerFields(badPath); err == nil {
		t.Error("expected non-mapping root to be rejected")
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

// mustProducerFields is the in-process form of a --frontmatter-file: the map is
// encoded to nodes through exactly the path LoadProducerFields uses.
func mustProducerFields(t *testing.T, fields map[string]any) ProducerFields {
	t.Helper()
	pf, err := NewProducerFields(fields)
	if err != nil {
		t.Fatalf("NewProducerFields: %v", err)
	}
	return pf
}

// decodeField reads one producer field back as a plain Go value.
func decodeField(t *testing.T, fields ProducerFields, key string) any {
	t.Helper()
	node, ok := fields[key]
	if !ok {
		t.Fatalf("producer field %q absent", key)
	}
	var v any
	if err := node.Decode(&v); err != nil {
		t.Fatalf("decode producer field %q: %v", key, err)
	}
	return v
}
