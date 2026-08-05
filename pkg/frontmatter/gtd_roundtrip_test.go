package frontmatter

import (
	"strings"
	"testing"
)

// TestGtdRoundTripFullBlock: a note with a full gtd block survives
// Parse→Build with every key intact — the property the passthrough field
// exists for. Key order is Build's deterministic sorted order, so the
// assertion parses the result back rather than comparing bytes against the
// input's arbitrary order.
func TestGtdRoundTripFullBlock(t *testing.T) {
	content := `---
id: my-note
title: The Note
aliases: []
tags: [deep]
created: 2026-08-01 09:00:00
modified: 2026-08-01 09:00:00
gtd:
  kind: project
  status: active
  flagged: true
  project: parent
  defer: 2026-08-10
  due: 2026-08-15
  waiting:
    job: plans/rolling/03-fix.md
    pr: grovetools/nb#42
  review_interval: 7d
---
body text
`
	fm, body, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	if fm.Gtd == nil {
		t.Fatal("Parse dropped the gtd block")
	}

	rebuilt := BuildContent(fm, body)
	fm2, _, err := Parse(rebuilt)
	if err != nil {
		t.Fatalf("re-parse of built content: %v\n%s", err, rebuilt)
	}
	g, ok := fm2.Gtd.(map[string]interface{})
	if !ok {
		t.Fatalf("gtd after round trip = %T", fm2.Gtd)
	}
	for _, key := range []string{"kind", "status", "flagged", "project", "defer", "due", "waiting", "review_interval"} {
		if _, present := g[key]; !present {
			t.Errorf("gtd.%s lost in round trip:\n%s", key, rebuilt)
		}
	}
	if w, ok := g["waiting"].(map[string]interface{}); !ok || w["pr"] != "grovetools/nb#42" {
		t.Errorf("nested waiting map mangled: %#v", g["waiting"])
	}
	if g["flagged"] != true {
		t.Errorf("flagged = %#v, want true", g["flagged"])
	}

	// Determinism: two builds are byte-identical.
	if BuildContent(fm, body) != rebuilt {
		t.Error("Build is not deterministic for the gtd block")
	}
}

// TestGtdRoundTripBoolShorthand: `gtd: true` / `gtd: false` are preserved as
// the scalar the user wrote — the documented choice, verbatim over
// normalized.
func TestGtdRoundTripBoolShorthand(t *testing.T) {
	for _, val := range []string{"true", "false"} {
		content := "---\nid: x\ntitle: x\naliases: []\ntags: []\ncreated: c\nmodified: m\ngtd: " + val + "\n---\nbody\n"
		fm, body, err := Parse(content)
		if err != nil {
			t.Fatal(err)
		}
		rebuilt := BuildContent(fm, body)
		if !strings.Contains(rebuilt, "gtd: "+val+"\n") {
			t.Errorf("gtd: %s not preserved verbatim:\n%s", val, rebuilt)
		}
	}
}

// TestGtdAbsentStaysAbsent: notes without gtd must not grow the key.
func TestGtdAbsentStaysAbsent(t *testing.T) {
	content := "---\nid: x\ntitle: x\naliases: []\ntags: []\ncreated: c\nmodified: m\n---\nbody\n"
	fm, body, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt := BuildContent(fm, body); strings.Contains(rebuilt, "gtd") {
		t.Errorf("gtd appeared from nowhere:\n%s", rebuilt)
	}
}
