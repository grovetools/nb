package frontmatter

import (
	"reflect"
	"strings"
	"testing"
)

// noteWith wraps a frontmatter body in the document form Parse expects.
func noteWith(fmBody string) string {
	return "---\n" + strings.TrimSuffix(fmBody, "\n") + "\n---\n\nBody text.\n"
}

// TestParsePRs covers the `prs:` join shape: the documented happy path, the
// versioned-shape rules (D6), and the manual-edit tolerance that matters
// because nothing writes `prs:` automatically in this wave.
func TestParsePRs(t *testing.T) {
	tests := []struct {
		name        string
		fmBody      string
		wantVersion int
		wantEntries []PRRef
		wantRaw     bool
	}{
		{
			name: "documented shape",
			fmBody: `id: t
title: Ticket
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
prs_schema_version: 1
prs:
  - repo: flow
    provider: forgejo
    url: https://forge.example.com/grove/flow/pulls/12
    state: open
    updated_at: 2026-08-02T10:00:00Z
  - repo: core
    provider: github
    url: https://github.com/grovetools/core/pull/7
    state: merged`,
			wantVersion: 1,
			wantEntries: []PRRef{
				{Repo: "flow", Provider: "forgejo", URL: "https://forge.example.com/grove/flow/pulls/12", State: "open", UpdatedAt: "2026-08-02T10:00:00Z"},
				{Repo: "core", Provider: "github", URL: "https://github.com/grovetools/core/pull/7", State: "merged"},
			},
		},
		{
			name: "absent version reads as version 1",
			fmBody: `id: t
title: Ticket
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
prs:
  - repo: nb
    url: https://github.com/grovetools/nb/pull/3
    state: open`,
			wantVersion: 0, // stored as absent; readers treat 0 as PRsSchemaVersion
			wantEntries: []PRRef{{Repo: "nb", URL: "https://github.com/grovetools/nb/pull/3", State: "open"}},
		},
		{
			name: "newer schema version keeps unknown per-entry keys",
			fmBody: `id: t
title: Ticket
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
prs_schema_version: 99
prs:
  - repo: nb
    url: https://github.com/grovetools/nb/pull/3
    state: open
    reviewers: [alice, bob]`,
			wantVersion: 99,
			wantEntries: []PRRef{{
				Repo:  "nb",
				URL:   "https://github.com/grovetools/nb/pull/3",
				State: "open",
				Extra: map[string]any{"reviewers": []any{"alice", "bob"}},
			}},
		},
		{
			name: "no prs key at all",
			fmBody: `id: t
title: Ticket
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00`,
			wantVersion: 0,
			wantEntries: nil,
		},
		{
			name: "hand-edited garbage is tolerated, not fatal",
			fmBody: `id: t
title: Ticket
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
prs: not-a-list`,
			wantVersion: 0,
			wantEntries: nil,
			wantRaw:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, _, err := Parse(noteWith(tt.fmBody))
			if err != nil {
				t.Fatalf("Parse() error = %v, want nil (a bad prs: value must not fail the note)", err)
			}
			if fm == nil {
				t.Fatal("Parse() returned nil frontmatter")
			}
			if fm.PRsSchemaVersion != tt.wantVersion {
				t.Errorf("PRsSchemaVersion = %d, want %d", fm.PRsSchemaVersion, tt.wantVersion)
			}
			if !reflect.DeepEqual(fm.PRs.Entries, tt.wantEntries) {
				t.Errorf("PRs.Entries = %#v, want %#v", fm.PRs.Entries, tt.wantEntries)
			}
			if got := fm.PRs.Unparsed(); got != tt.wantRaw {
				t.Errorf("PRs.Unparsed() = %v, want %v", got, tt.wantRaw)
			}
		})
	}
}

// comparable strips the two fields that legitimately differ across a rebuild:
// the preserved yaml.Node (it carries source line/column, which move when a
// version key is inserted above it) and the schema version (an unversioned
// list is stamped on write — the D6 upgrade). Both are asserted separately.
func comparable(fm *Frontmatter) Frontmatter {
	c := *fm
	c.PRsSchemaVersion = 0
	c.PRs.raw = nil
	return c
}

// TestPRsRoundTrip is the acceptance criterion: parse → build → parse must be a
// fixed point, with and without `prs:`.
func TestPRsRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		fmBody string
	}{
		{
			name: "without prs",
			fmBody: `id: t
title: Ticket
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
plan_ref: plans/hosted-git-and-prs`,
		},
		{
			name: "with prs",
			fmBody: `id: t
title: Ticket
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
plan_ref: plans/hosted-git-and-prs
prs_schema_version: 1
prs:
  - repo: flow
    provider: forgejo
    url: https://forge.example.com/grove/flow/pulls/12
    state: open
    updated_at: 2026-08-02T10:00:00Z`,
		},
		{
			name: "with prs carrying a colon-heavy title in extra keys",
			fmBody: `id: t
title: Ticket
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
prs:
  - repo: nb
    url: https://github.com/grovetools/nb/pull/3
    state: open
    summary: "nb: split the remote family"`,
		},
		{
			name: "with an unparseable prs value",
			fmBody: `id: t
title: Ticket
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
prs: not-a-list`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm1, body1, err := Parse(noteWith(tt.fmBody))
			if err != nil {
				t.Fatalf("first Parse() error = %v", err)
			}

			rebuilt := BuildContent(fm1, body1)

			fm2, body2, err := Parse(rebuilt)
			if err != nil {
				t.Fatalf("second Parse() error = %v\nrebuilt:\n%s", err, rebuilt)
			}
			if !reflect.DeepEqual(comparable(fm1), comparable(fm2)) {
				t.Errorf("frontmatter changed across round-trip:\n first: %#v\nsecond: %#v\nrebuilt:\n%s", fm1, fm2, rebuilt)
			}
			// An unversioned `prs:` is stamped with the current version on the
			// way out — that is the D6 upgrade, not a round-trip failure.
			if len(fm1.PRs.Entries) > 0 && fm2.PRsSchemaVersion != PRsSchemaVersion {
				t.Errorf("PRsSchemaVersion after rebuild = %d, want %d", fm2.PRsSchemaVersion, PRsSchemaVersion)
			}
			if fm1.PRs.Unparsed() != fm2.PRs.Unparsed() {
				t.Errorf("Unparsed() changed across round-trip: %v -> %v", fm1.PRs.Unparsed(), fm2.PRs.Unparsed())
			}
			if body1 != body2 {
				t.Errorf("body changed across round-trip:\n first: %q\nsecond: %q", body1, body2)
			}

			// A second rebuild must be byte-identical to the first: the
			// serializer has to be a fixed point, not merely stable in shape.
			if again := BuildContent(fm2, body2); again != rebuilt {
				t.Errorf("Build is not idempotent:\nfirst:\n%s\nsecond:\n%s", rebuilt, again)
			}
		})
	}
}

// TestUnknownKeysPreserved is the non-destructive-write guard: nb's Build is a
// whitelist serializer, so any key it does not model must survive a rewrite
// rather than being deleted. Flow job notes carry exactly such keys
// (`status:`, `model:`, `rules_file:`), and `nb internal update-frontmatter`
// rewrites those files.
func TestUnknownKeysPreserved(t *testing.T) {
	original := noteWith(`id: job-1
title: ticket-pr-join
aliases: []
tags: []
type: interactive_agent
worktree: hosted-git-and-prs
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
model: claude-opus-5
provider: claude
rules_file: rules/12-ticket-pr-join.md.rules
status: running
nested:
  a: 1
  b: [x, y]`)

	fm, body, err := Parse(original)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Simulate the `nb internal update-frontmatter` path.
	if err := UpdateField(fm, "plan_ref", "plans/hosted-git-and-prs"); err != nil {
		t.Fatalf("UpdateField() error = %v", err)
	}
	rebuilt := BuildContent(fm, body)

	for _, want := range []string{
		"model: claude-opus-5",
		"provider: claude",
		"rules_file: rules/12-ticket-pr-join.md.rules",
		"status: running",
		"plan_ref: plans/hosted-git-and-prs",
	} {
		if !strings.Contains(rebuilt, want) {
			t.Errorf("rebuilt frontmatter lost %q:\n%s", want, rebuilt)
		}
	}

	// The nested value must survive with its structure intact.
	fm2, _, err := Parse(rebuilt)
	if err != nil {
		t.Fatalf("re-Parse() error = %v", err)
	}
	nested, ok := fm2.ExtraValue("nested").(map[string]any)
	if !ok {
		t.Fatalf("nested key lost its mapping shape: %#v", fm2.ExtraValue("nested"))
	}
	if nested["a"] != 1 {
		t.Errorf("nested.a = %#v, want 1", nested["a"])
	}
	if !reflect.DeepEqual(nested["b"], []any{"x", "y"}) {
		t.Errorf("nested.b = %#v, want [x y]", nested["b"])
	}

	// Known fields must NOT leak into the catch-all.
	for _, known := range []string{"id", "title", "type", "worktree", "created", "modified", "plan_ref"} {
		if _, leaked := fm2.Extra[known]; leaked {
			t.Errorf("modelled key %q leaked into Extra", known)
		}
	}
}

// TestNotesWithoutUnknownKeysAreUnchanged pins that the catch-all is inert for
// ordinary notes — no stray keys, no empty map, byte-identical output.
func TestNotesWithoutUnknownKeysAreUnchanged(t *testing.T) {
	original := noteWith(`id: plain
title: Plain Note
aliases: []
tags: [a, b]
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00`)

	fm, body, err := Parse(original)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if fm.Extra != nil {
		t.Errorf("Extra = %#v, want nil for a note with no unknown keys", fm.Extra)
	}
	if got := BuildContent(fm, body); got != original {
		t.Errorf("rebuild changed a plain note:\n got: %q\nwant: %q", got, original)
	}
}

func TestValidatePRs(t *testing.T) {
	tests := []struct {
		name      string
		list      PRList
		wantCount int
		wantHas   string
	}{
		{
			name: "well formed",
			list: PRList{Entries: []PRRef{
				{Repo: "flow", Provider: "forgejo", URL: "https://f/1", State: "open", UpdatedAt: "2026-08-02T10:00:00Z"},
			}},
			wantCount: 0,
		},
		{
			name:      "missing url and repo",
			list:      PRList{Entries: []PRRef{{State: "open"}}},
			wantCount: 2,
			wantHas:   "prs[0].url: required",
		},
		{
			name: "duplicate url",
			list: PRList{Entries: []PRRef{
				{Repo: "a", URL: "https://f/1"},
				{Repo: "b", URL: "https://f/1"},
			}},
			wantCount: 1,
			wantHas:   "duplicate of entry 0",
		},
		{
			name:      "unknown state is reported but not coerced",
			list:      PRList{Entries: []PRRef{{Repo: "a", URL: "https://f/1", State: "rejected"}}},
			wantCount: 1,
			wantHas:   `unrecognized state "rejected"`,
		},
		{
			name:      "bad timestamp",
			list:      PRList{Entries: []PRRef{{Repo: "a", URL: "https://f/1", UpdatedAt: "yesterday"}}},
			wantCount: 1,
			wantHas:   "unparseable timestamp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := ValidatePRs(tt.list)
			if len(issues) != tt.wantCount {
				t.Fatalf("got %d issues %v, want %d", len(issues), issues, tt.wantCount)
			}
			if tt.wantHas == "" {
				return
			}
			var joined []string
			for _, i := range issues {
				joined = append(joined, i.String())
			}
			if !strings.Contains(strings.Join(joined, "\n"), tt.wantHas) {
				t.Errorf("issues %v do not mention %q", joined, tt.wantHas)
			}
		})
	}
}

func TestValidatePRsReportsUnparsedList(t *testing.T) {
	fm, _, err := Parse(noteWith(`id: t
title: T
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
prs: 42`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	issues := ValidatePRs(fm.PRs)
	if len(issues) != 1 || !strings.Contains(issues[0].String(), "preserved verbatim") {
		t.Fatalf("got %v, want one issue reporting the value is preserved verbatim", issues)
	}
}

func TestFindPRByURL(t *testing.T) {
	list := PRList{Entries: []PRRef{
		{Repo: "a", URL: "https://f/1"},
		{Repo: "b", URL: "https://f/2"},
	}}
	if got := FindPRByURL(list, "https://f/2"); got != 1 {
		t.Errorf("FindPRByURL(existing) = %d, want 1", got)
	}
	if got := FindPRByURL(list, "https://f/9"); got != -1 {
		t.Errorf("FindPRByURL(missing) = %d, want -1", got)
	}
}
