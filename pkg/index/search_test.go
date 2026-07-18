package index

import (
	"testing"
)

func TestSearchScoringTitleOverHeadingOverBody(t *testing.T) {
	root := t.TempDir()
	inTitle := writeNote(t, root, "in-title.md", `---
id: a
title: Kubernetes Guide
created: 2026-01-01 00:00:00
modified: 2026-01-01 00:00:00
---
unrelated body
`)
	inHeading := writeNote(t, root, "in-heading.md", "## Kubernetes setup\n\nunrelated\n")
	inBody := writeNote(t, root, "in-body.md", "something about kubernetes here\n")
	writeNote(t, root, "no-match.md", "nothing relevant\n")

	ix := buildVault(t, []Root{{Dir: root, Workspace: "ws"}})

	hits := ix.Search("kubernetes", SearchOptions{})
	if len(hits) != 3 {
		t.Fatalf("want 3 hits, got %d", len(hits))
	}
	if hits[0].Doc.Path != inTitle || hits[1].Doc.Path != inHeading || hits[2].Doc.Path != inBody {
		t.Errorf("score order wrong: %s, %s, %s",
			hits[0].Doc.Path, hits[1].Doc.Path, hits[2].Doc.Path)
	}
	if !(hits[0].Score > hits[1].Score && hits[1].Score > hits[2].Score) {
		t.Errorf("scores not descending: %v %v %v", hits[0].Score, hits[1].Score, hits[2].Score)
	}
}

func TestSearchOrMatchingAndFraction(t *testing.T) {
	root := t.TempDir()
	both := writeNote(t, root, "both.md", "alpha beta\n")
	oneOf := writeNote(t, root, "one.md", "alpha only\n")

	ix := buildVault(t, []Root{{Dir: root, Workspace: "ws"}})

	hits := ix.Search("alpha beta", SearchOptions{})
	if len(hits) != 2 {
		t.Fatalf("OR matching: want 2 hits, got %d", len(hits))
	}
	if hits[0].Doc.Path != both {
		t.Errorf("full-match doc should rank first, got %s", hits[0].Doc.Path)
	}
	if hits[1].Doc.Path != oneOf {
		t.Errorf("partial-match doc missing: %s", hits[1].Doc.Path)
	}
	// both: 2 body hits * 2/2 = 2; oneOf: 1 body hit * 1/2 = 0.5
	if hits[0].Score <= hits[1].Score*2 {
		t.Errorf("fraction weighting suspicious: %v vs %v", hits[0].Score, hits[1].Score)
	}
}

func TestSearchSnippetsAndCase(t *testing.T) {
	root := t.TempDir()
	note := writeNote(t, root, "n.md", `---
id: n
title: N
created: 2026-01-01 00:00:00
modified: 2026-01-01 00:00:00
---
first line
has NEEDLE here
last line
`)

	ix := buildVault(t, []Root{{Dir: root, Workspace: "ws"}})

	hits := ix.Search("needle", SearchOptions{})
	if len(hits) != 1 || hits[0].Doc.Path != note {
		t.Fatalf("case-insensitive search failed: %+v", hits)
	}
	if len(hits[0].Snippets) != 1 {
		t.Fatalf("want 1 snippet, got %+v", hits[0].Snippets)
	}
	sn := hits[0].Snippets[0]
	if sn.Text != "has NEEDLE here" || sn.Line != 8 {
		t.Errorf("snippet wrong: %+v", sn)
	}
}

func TestSearchFrontmatterNotIndexedAsBody(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "n.md", `---
id: zebrafish
title: N
created: 2026-01-01 00:00:00
modified: 2026-01-01 00:00:00
---
body only
`)

	ix := buildVault(t, []Root{{Dir: root, Workspace: "ws"}})

	if hits := ix.Search("zebrafish", SearchOptions{}); len(hits) != 0 {
		t.Errorf("frontmatter leaked into search: %+v", hits)
	}
}

func TestSearchLimitAndWorkspaceFilter(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	writeNote(t, rootA, "a1.md", "common term\n")
	writeNote(t, rootA, "a2.md", "common term\n")
	b := writeNote(t, rootB, "b1.md", "common term\n")

	ix := buildVault(t, []Root{{Dir: rootA, Workspace: "ws-a"}, {Dir: rootB, Workspace: "ws-b"}})

	if hits := ix.Search("common", SearchOptions{Limit: 2}); len(hits) != 2 {
		t.Errorf("limit not applied: %d hits", len(hits))
	}
	hits := ix.Search("common", SearchOptions{Workspace: "ws-b"})
	if len(hits) != 1 || hits[0].Doc.Path != b {
		t.Errorf("workspace filter wrong: %+v", hits)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "a.md", "text\n")
	ix := buildVault(t, []Root{{Dir: root, Workspace: "ws"}})
	if hits := ix.Search("   ", SearchOptions{}); hits != nil {
		t.Errorf("empty query should return nil, got %+v", hits)
	}
}
