package index

import (
	"os"
	"path/filepath"
	"testing"
)

// writeNote creates a markdown file (and parent dirs) under root.
func writeNote(t *testing.T, root, rel, content string) string {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func buildVault(t *testing.T, roots []Root) *Index {
	t.Helper()
	ix := New()
	if err := ix.Build(roots); err != nil {
		t.Fatalf("Build: %v", err)
	}
	return ix
}

func TestBuildParsesFrontmatterAndTitleFallbacks(t *testing.T) {
	root := t.TempDir()
	withFM := writeNote(t, root, "notes/with-fm.md", `---
id: note-one
title: "My Note"
aliases: [alias-a, "Alias B"]
tags: [fmtag]
created: 2026-01-01 00:00:00
modified: 2026-01-01 00:00:00
plan_ref: some-plan
---

# Heading One

body text
`)
	withH1 := writeNote(t, root, "notes/h1-only.md", "# From The H1\n\ntext\n")
	bare := writeNote(t, root, "notes/bare-stem.md", "just text, no heading\n")

	ix := buildVault(t, []Root{{Dir: root, Workspace: "ws1"}})

	d, ok := ix.Doc(withFM)
	if !ok {
		t.Fatalf("doc not indexed: %s", withFM)
	}
	if d.ID != "note-one" || d.Title != "My Note" || d.PlanRef != "some-plan" {
		t.Errorf("frontmatter fields wrong: %+v", d)
	}
	if len(d.Aliases) != 2 || d.Aliases[0] != "alias-a" {
		t.Errorf("aliases wrong: %v", d.Aliases)
	}
	if d.Workspace != "ws1" || d.NoteType != "notes" {
		t.Errorf("workspace/notetype wrong: %q %q", d.Workspace, d.NoteType)
	}
	if len(d.Headings) != 1 || d.Headings[0].Text != "Heading One" || d.Headings[0].Level != 1 {
		t.Errorf("headings wrong: %+v", d.Headings)
	}

	if d, _ := ix.Doc(withH1); d.Title != "From The H1" {
		t.Errorf("H1 title fallback wrong: %q", d.Title)
	}
	if d, _ := ix.Doc(bare); d.Title != "bare-stem" {
		t.Errorf("stem title fallback wrong: %q", d.Title)
	}
}

func TestWikilinkGrammarAndBacklinks(t *testing.T) {
	root := t.TempDir()
	target := writeNote(t, root, "notes/target-note.md", "# Target\n")
	source := writeNote(t, root, "notes/source.md", `plain [[Target-Note]]
display [[target-note|the label]]
heading [[target-note#Section A]]
embed ![[target-note]]
combined [[target-note#Sec|Label]]
`)

	ix := buildVault(t, []Root{{Dir: root, Workspace: "ws"}})

	d, _ := ix.Doc(source)
	if len(d.Links) != 5 {
		t.Fatalf("want 5 links, got %d: %+v", len(d.Links), d.Links)
	}
	for i, l := range d.Links {
		if l.ResolvedPath != target {
			t.Errorf("link %d unresolved: %+v", i, l)
		}
		if l.SourcePath != source {
			t.Errorf("link %d source wrong: %q", i, l.SourcePath)
		}
	}
	if d.Links[1].Display != "the label" {
		t.Errorf("display wrong: %+v", d.Links[1])
	}
	if d.Links[2].Heading != "Section A" {
		t.Errorf("heading fragment wrong: %+v", d.Links[2])
	}
	if d.Links[4].Heading != "Sec" || d.Links[4].Display != "Label" {
		t.Errorf("combined form wrong: %+v", d.Links[4])
	}
	if d.Links[0].Line != 1 || d.Links[4].Line != 5 {
		t.Errorf("line numbers wrong: %d %d", d.Links[0].Line, d.Links[4].Line)
	}

	back := ix.Backlinks(target)
	if len(back) != 5 {
		t.Fatalf("want 5 backlinks, got %d", len(back))
	}
	for _, l := range back {
		if l.SourcePath != source {
			t.Errorf("backlink source wrong: %+v", l)
		}
	}
}

func TestLineNumbersAccountForFrontmatter(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "a.md", "# A\n")
	src := writeNote(t, root, "b.md", `---
id: b
title: B
created: 2026-01-01 00:00:00
modified: 2026-01-01 00:00:00
---

[[a]]
`)
	ix := buildVault(t, []Root{{Dir: root, Workspace: "ws"}})
	d, _ := ix.Doc(src)
	if len(d.Links) != 1 || d.Links[0].Line != 8 {
		t.Errorf("frontmatter-offset line wrong: %+v", d.Links)
	}
}

func TestResolutionOrderStemIDAliasTitle(t *testing.T) {
	root := t.TempDir()
	stem := writeNote(t, root, "stem-match.md", "x\n")
	byID := writeNote(t, root, "other-a.md", `---
id: special-id
title: Whatever
created: 2026-01-01 00:00:00
modified: 2026-01-01 00:00:00
---
x
`)
	byAlias := writeNote(t, root, "other-b.md", `---
id: b
title: Whatever2
aliases: [nickname]
created: 2026-01-01 00:00:00
modified: 2026-01-01 00:00:00
---
x
`)
	byTitle := writeNote(t, root, "other-c.md", `---
id: c
title: Exact Title Here
created: 2026-01-01 00:00:00
modified: 2026-01-01 00:00:00
---
x
`)
	// skills/<name>/SKILL.md pattern: generic stem + id, canonical frontmatter name.
	byName := writeNote(t, root, "skills/my-skill/SKILL.md", `---
id: SKILL
title: ""
name: my-skill
created: 2026-01-01 00:00:00
modified: 2026-01-01 00:00:00
---
x
`)

	ix := buildVault(t, []Root{{Dir: root, Workspace: "ws"}})

	cases := []struct {
		target string
		want   string
	}{
		{"stem-match", stem},
		{"STEM-MATCH", stem},    // case-insensitive stem
		{"stem-match.md", stem}, // explicit extension
		{"special-id", byID},
		{"NICKNAME", byAlias},
		{"exact title here", byTitle},
		{"my-skill", byName}, // frontmatter name: acts as an alias
	}
	for _, c := range cases {
		got := ix.Resolve(c.target)
		if len(got) != 1 || got[0].Path != c.want {
			t.Errorf("Resolve(%q) = %v, want %s", c.target, paths(got), c.want)
		}
	}
	if got := ix.Resolve("no-such-note"); len(got) != 0 {
		t.Errorf("Resolve(no-such-note) = %v, want none", paths(got))
	}
}

func TestPathQualifiedResolution(t *testing.T) {
	root := t.TempDir()
	a := writeNote(t, root, "plans/alpha/overview.md", "a\n")
	b := writeNote(t, root, "plans/beta/overview.md", "b\n")

	ix := buildVault(t, []Root{{Dir: root, Workspace: "ws"}})

	got := ix.Resolve("alpha/overview")
	if len(got) != 1 || got[0].Path != a {
		t.Errorf("path-qualified resolve = %v, want %s", paths(got), a)
	}
	// Bare stem is ambiguous — both docs are candidates.
	if got := ix.Resolve("overview"); len(got) != 2 {
		t.Errorf("ambiguous stem resolve = %v, want both", paths(got))
	}
	_ = b
}

func TestSameWorkspaceTieBreak(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeNote(t, rootA, "shared.md", "in ws-a\n")
	inB := writeNote(t, rootB, "shared.md", "in ws-b\n")
	src := writeNote(t, rootB, "src.md", "[[shared]]\n")

	ix := buildVault(t, []Root{{Dir: rootA, Workspace: "ws-a"}, {Dir: rootB, Workspace: "ws-b"}})

	d, _ := ix.Doc(src)
	if len(d.Links) != 1 || d.Links[0].ResolvedPath != inB {
		t.Errorf("workspace tie-break failed: %+v", d.Links)
	}
}

func TestUnresolvedLinks(t *testing.T) {
	root := t.TempDir()
	src := writeNote(t, root, "src.md", "[[ghost-note]] and [[real]]\n")
	writeNote(t, root, "real.md", "x\n")

	ix := buildVault(t, []Root{{Dir: root, Workspace: "ws"}})

	un := ix.Unresolved()
	if len(un) != 1 || un[0].RawTarget != "ghost-note" || un[0].SourcePath != src {
		t.Errorf("unresolved wrong: %+v", un)
	}
}

func TestCodeFenceAndInlineCodeExclusion(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "real.md", "x\n")
	src := writeNote(t, root, "src.md", "before [[real]]\n"+
		"```\n[[fenced-link]] #fencedtag\n```\n"+
		"inline `[[code-link]]` and `#codetag` after\n"+
		"~~~\n[[tilde-fenced]]\n~~~\n"+
		"#realtag\n")

	ix := buildVault(t, []Root{{Dir: root, Workspace: "ws"}})

	d, _ := ix.Doc(src)
	if len(d.Links) != 1 || d.Links[0].RawTarget != "real" {
		t.Errorf("code exclusion failed, links: %+v", d.Links)
	}
	if len(ix.Unresolved()) != 0 {
		t.Errorf("fenced links leaked into unresolved: %+v", ix.Unresolved())
	}
	tags := ix.Tags()
	if tags["realtag"] != 1 {
		t.Errorf("realtag missing: %v", tags)
	}
	if _, ok := tags["fencedtag"]; ok {
		t.Errorf("fenced tag leaked: %v", tags)
	}
	if _, ok := tags["codetag"]; ok {
		t.Errorf("inline-code tag leaked: %v", tags)
	}
}

func TestTagsMergeFrontmatterPathAndInline(t *testing.T) {
	root := t.TempDir()
	note := writeNote(t, root, "issues/bugs/crash.md", `---
id: crash
title: Crash
tags: [urgent]
created: 2026-01-01 00:00:00
modified: 2026-01-01 00:00:00
---

Seen with #segfault and #nested/tag but not #123 numeric.
`)

	ix := buildVault(t, []Root{{Dir: root, Workspace: "ws"}})

	d, _ := ix.Doc(note)
	want := map[string]bool{"urgent": true, "issues": true, "bugs": true, "segfault": true, "nested/tag": true}
	got := map[string]bool{}
	for _, tag := range d.Tags {
		got[tag] = true
	}
	for tag := range want {
		if !got[tag] {
			t.Errorf("missing tag %q in %v", tag, d.Tags)
		}
	}
	if got["123"] {
		t.Errorf("numeric-only tag accepted: %v", d.Tags)
	}

	byTag := ix.DocsByTag("segfault")
	if len(byTag) != 1 || byTag[0].Path != note {
		t.Errorf("DocsByTag wrong: %v", paths(byTag))
	}
	if ix.Tags()["urgent"] != 1 {
		t.Errorf("Tags() count wrong: %v", ix.Tags())
	}
}

func TestMarkdownLinksToVaultFiles(t *testing.T) {
	root := t.TempDir()
	target := writeNote(t, root, "sub/target.md", "x\n")
	spaced := writeNote(t, root, "sub/with space.md", "x\n")
	src := writeNote(t, root, "src.md", `see [target](sub/target.md)
anchored [t](sub/target.md#section)
escaped [s](sub/with%20space.md)
external [x](https://example.com/page.md)
non-vault [n](../outside/nope.md)
non-md [i](sub/image.png)
`)

	ix := buildVault(t, []Root{{Dir: root, Workspace: "ws"}})

	d, _ := ix.Doc(src)
	if len(d.Links) != 3 {
		t.Fatalf("want 3 markdown links kept, got %d: %+v", len(d.Links), d.Links)
	}
	if d.Links[0].ResolvedPath != target || d.Links[1].ResolvedPath != target {
		t.Errorf("markdown resolution wrong: %+v", d.Links)
	}
	if d.Links[1].Heading != "section" {
		t.Errorf("markdown anchor wrong: %+v", d.Links[1])
	}
	if d.Links[2].ResolvedPath != spaced {
		t.Errorf("%%20 unescape failed: %+v", d.Links[2])
	}
	// Dropped markdown links must not appear as unresolved.
	if len(ix.Unresolved()) != 0 {
		t.Errorf("markdown links leaked into unresolved: %+v", ix.Unresolved())
	}
	if len(ix.Backlinks(target)) != 2 {
		t.Errorf("markdown backlinks wrong: %+v", ix.Backlinks(target))
	}
}

func TestUnicodeLinksAndTags(t *testing.T) {
	root := t.TempDir()
	target := writeNote(t, root, "日本語ノート.md", "# ノート\n")
	src := writeNote(t, root, "src.md", "link [[日本語ノート]] tag #日本語 done\n")

	ix := buildVault(t, []Root{{Dir: root, Workspace: "ws"}})

	d, _ := ix.Doc(src)
	if len(d.Links) != 1 || d.Links[0].ResolvedPath != target {
		t.Errorf("unicode link failed: %+v", d.Links)
	}
	if ix.Tags()["日本語"] != 1 {
		t.Errorf("unicode tag failed: %v", ix.Tags())
	}
}

func TestArchivedIncludedAndFlagged(t *testing.T) {
	root := t.TempDir()
	archived := writeNote(t, root, "notes/.archive/old.md", "# Old\n")
	live := writeNote(t, root, "notes/live.md", "[[old]]\n")

	ix := buildVault(t, []Root{{Dir: root, Workspace: "ws"}})

	d, ok := ix.Doc(archived)
	if !ok {
		t.Fatal("archived note not indexed")
	}
	if !d.Archived {
		t.Error("archived note not flagged")
	}
	if l, _ := ix.Doc(live); l.Archived {
		t.Error("live note flagged archived")
	}
	// Archived notes stay link-resolvable.
	if got := ix.Backlinks(archived); len(got) != 1 {
		t.Errorf("backlink into archive missing: %+v", got)
	}
	// Hidden dirs other than .archive are skipped, and .archive dirs are not path tags.
	writeNote(t, root, "notes/.obsidian/config.md", "skip me\n")
	ix2 := buildVault(t, []Root{{Dir: root, Workspace: "ws"}})
	if _, ok := ix2.Doc(filepath.Join(root, "notes/.obsidian/config.md")); ok {
		t.Error(".obsidian dir was indexed")
	}
	if d, _ := ix2.Doc(archived); contains(d.Tags, ".archive") {
		t.Errorf(".archive leaked into tags: %v", d.Tags)
	}
}

func TestOverlappingRootsDedupe(t *testing.T) {
	root := t.TempDir()
	note := writeNote(t, root, "concepts/idea.md", "# Idea\n")

	// Second root is nested inside the first — the file must be indexed once.
	ix := buildVault(t, []Root{
		{Dir: root, Workspace: "ws"},
		{Dir: filepath.Join(root, "concepts"), Workspace: "ws"},
	})

	if len(ix.Docs()) != 1 {
		t.Fatalf("want 1 doc, got %d", len(ix.Docs()))
	}
	if d, ok := ix.Doc(note); !ok || d.NoteType != "concepts" {
		t.Errorf("dedupe kept wrong doc: %+v", d)
	}
}

func TestMissingRootSkipped(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "a.md", "x\n")
	ix := New()
	err := ix.Build([]Root{
		{Dir: filepath.Join(root, "does-not-exist"), Workspace: "ws"},
		{Dir: root, Workspace: "ws"},
	})
	if err != nil {
		t.Fatalf("Build with missing root: %v", err)
	}
	if len(ix.Docs()) != 1 {
		t.Errorf("want 1 doc, got %d", len(ix.Docs()))
	}
}

func paths(docs []*Doc) []string {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.Path
	}
	return out
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
