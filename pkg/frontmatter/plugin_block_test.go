package frontmatter

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// A plugin's namespaced frontmatter block is the hardest case the Extra
// passthrough has to carry: nested, mixed-typed, and — critically — full of
// unquoted dates. grove-gtd's `gtd:` block is the real instance these tests
// use, but nb declares nothing about it and interprets none of it; what is
// pinned here is that ANY consumer's block comes back out exactly as it went
// in. nb is a courier for this data, and a courier that rewrites the package
// is worse than one that loses it, because the damage is silent.
//
// These started life as a gtd-specific passthrough field in core (reverted:
// it named one plugin, and its hand-rolled emitter printed parsed dates with
// %v, so `defer: 2026-08-10` was rewritten as
// `defer: 2026-08-10 00:00:00 +0000 UTC` — a spelling grove-gtd's own reader
// rejects). The assertions below are therefore on VALUES, not just on key
// presence: presence-only assertions are what let that corruption ship.

const pluginBlockNote = `---
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

// TestPluginBlockRoundTripsVerbatim: the whole block comes back byte-for-byte,
// including the unquoted dates and the nested waiting map.
func TestPluginBlockRoundTripsVerbatim(t *testing.T) {
	fm, body, err := Parse(pluginBlockNote)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fm.Extra["gtd"]; !ok {
		t.Fatal("Parse dropped the plugin block")
	}

	rebuilt := BuildContent(fm, body)

	// Every line of the block, with its original spelling. A date that lost
	// its bare-date form, a bool that became a string, or a re-indented
	// nested map all fail here.
	for _, want := range []string{
		"gtd:",
		"  kind: project",
		"  status: active",
		"  flagged: true",
		"  project: parent",
		"  defer: 2026-08-10",
		"  due: 2026-08-15",
		"  waiting:",
		"    job: plans/rolling/03-fix.md",
		"    pr: grovetools/nb#42",
		"  review_interval: 7d",
	} {
		if !strings.Contains(rebuilt, want) {
			t.Errorf("plugin block line %q not reproduced verbatim; got:\n%s", want, rebuilt)
		}
	}

	// No coercion residue: these are the shapes the reverted gtd-specific
	// emitter and a naive interface{} passthrough respectively produced.
	for _, forbidden := range []string{
		"00:00:00 +0000 UTC",
		"2026-08-10T00:00:00Z",
		`"2026-08-10"`,
	} {
		if strings.Contains(rebuilt, forbidden) {
			t.Errorf("plugin value was rewritten (%q present):\n%s", forbidden, rebuilt)
		}
	}

	// The author's key order inside the block is preserved, so a note only
	// changes where the change was.
	if got := blockLines(rebuilt); !strings.HasPrefix(got, "kind|status|flagged|project|defer|due|waiting|") {
		t.Errorf("plugin block key order churned: %s", got)
	}
}

// TestPluginBlockSecondGenerationStable: repeated rewrites converge. A note
// that has been through nb once must not keep changing on every subsequent
// move or retag.
func TestPluginBlockSecondGenerationStable(t *testing.T) {
	fm, body, err := Parse(pluginBlockNote)
	if err != nil {
		t.Fatal(err)
	}
	first := BuildContent(fm, body)

	fm2, body2, err := Parse(first)
	if err != nil {
		t.Fatalf("re-Parse of rebuilt note: %v\n%s", err, first)
	}
	second := BuildContent(fm2, body2)
	if second != first {
		t.Errorf("second generation differs:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}

	// And Build is deterministic within a generation: map iteration order
	// must not leak into the file.
	for i := 0; i < 20; i++ {
		if again := BuildContent(fm, body); again != first {
			t.Fatalf("Build nondeterministic at iteration %d:\n%s\n--- vs ---\n%s", i, again, first)
		}
	}
}

// TestPluginBlockScalarShorthand: grove-gtd also writes `gtd: true` / `false`
// as a whole-note shorthand. The scalar the user wrote is what comes back —
// nb neither normalizes it nor promotes it to a map.
func TestPluginBlockScalarShorthand(t *testing.T) {
	for _, val := range []string{"true", "false"} {
		content := "---\nid: x\ntitle: x\naliases: []\ntags: []\ncreated: c\nmodified: m\ngtd: " + val + "\n---\nbody\n"
		fm, body, err := Parse(content)
		if err != nil {
			t.Fatal(err)
		}
		rebuilt := BuildContent(fm, body)
		if !strings.Contains(rebuilt, "gtd: "+val) {
			t.Errorf("gtd: %s not preserved as written:\n%s", val, rebuilt)
		}
	}
}

// TestPluginBlockAbsentStaysAbsent: a note with no plugin block gains no
// empty one, so landing this passthrough rewrites nothing in existing
// notebooks.
func TestPluginBlockAbsentStaysAbsent(t *testing.T) {
	content := "---\nid: x\ntitle: x\naliases: []\ntags: []\ncreated: c\nmodified: m\n---\nbody\n"
	fm, body, err := Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt := BuildContent(fm, body); strings.Contains(rebuilt, "gtd") {
		t.Errorf("absent plugin block was invented:\n%s", rebuilt)
	}
}

// blockLines joins the top-level keys of the gtd block in emitted order, so a
// key-order assertion reads as one string.
func blockLines(built string) string {
	var keys []string
	inBlock := false
	for _, line := range strings.Split(built, "\n") {
		switch {
		case line == "gtd:":
			inBlock = true
		case inBlock && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    "):
			keys = append(keys, strings.TrimSpace(strings.SplitN(strings.TrimSpace(line), ":", 2)[0]))
		case inBlock && !strings.HasPrefix(line, "  "):
			inBlock = false
		}
	}
	return strings.Join(keys, "|") + "|"
}

// TestInterfacePassthroughWouldCoerce is the negative control for the design
// choice above: it pins the yaml.v3 behavior that makes a `map[string]any`
// extension map unsafe for plugin data. Decoding an unquoted YYYY-MM-DD into
// interface{} yields a time.Time, and re-encoding it can only produce a
// timestamp spelling — so anyone tempted to "simplify" Extra back to decoded
// Go values should fail this test's premise first.
func TestInterfacePassthroughWouldCoerce(t *testing.T) {
	var decoded struct {
		Gtd map[string]any `yaml:"gtd"`
	}
	if err := yaml.Unmarshal([]byte("gtd:\n  defer: 2026-08-10\n"), &decoded); err != nil {
		t.Fatal(err)
	}
	if _, isTime := decoded.Gtd["defer"].(time.Time); !isTime {
		t.Skipf("yaml.v3 no longer resolves bare dates to time.Time (got %T) — "+
			"the coercion hazard Extra's node values guard against may be gone",
			decoded.Gtd["defer"])
	}

	// The node path, on the same input, keeps the spelling.
	fm, body, err := Parse("---\nid: x\ntitle: x\naliases: []\ntags: []\ncreated: c\nmodified: m\ngtd:\n  defer: 2026-08-10\n---\nb\n")
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt := BuildContent(fm, body); !strings.Contains(rebuilt, "defer: 2026-08-10\n") {
		t.Errorf("node passthrough coerced the date after all:\n%s", rebuilt)
	}
}
