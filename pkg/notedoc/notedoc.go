// Package notedoc implements the machine-owned / human-owned ownership split
// that nb's sync marker has always encoded, as a reusable seam.
//
// nb/pkg/sync has rebuilt note bodies around the marker since remote sync
// existed: everything ABOVE the marker is regenerated from the machine's view
// of the world, everything from the marker DOWN is whatever a human or an agent
// wrote and is copied through verbatim. That rule was private to the syncer.
// Generated notes that are refreshed in place (the review packet flow writes
// today, PR-body projection later) need exactly the same rule, so it lives here
// and the syncer's own constant is defined from Marker.
//
// The second half of the split is frontmatter. nb's Frontmatter type keeps
// unmodelled keys in an inline Extra map and re-emits them verbatim, which
// makes frontmatter the natural home for a machine-owned structured payload
// (a review-state snapshot, say) that no reader should have to parse out of
// prose. SetExtra writes such a key without letting a caller shadow a field nb
// models itself.
package notedoc

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/grovetools/nb/pkg/frontmatter"
)

// Marker is the machine/human ownership boundary in a note body. Content above
// it is owned by whatever generates the note and is replaced on every refresh;
// the marker line and everything after it is never touched.
//
// The literal is load-bearing history: it is already present in every note the
// remote syncer has ever created, so it must not change.
const Marker = "<!-- nb-sync-marker -->"

// SplitBody splits a note body at the FIRST marker. above is the machine-owned
// region (marker excluded); rest is the marker and everything after it, so
// above + rest reconstructs body exactly. found reports whether a marker was
// present at all.
func SplitBody(body string) (above, rest string, found bool) {
	i := strings.Index(body, Marker)
	if i < 0 {
		return body, "", false
	}
	return body[:i], body[i:], true
}

// Above returns just the machine-owned region of a note body. A body with no
// marker has no machine-owned region — every byte of it belongs to whoever
// wrote it — so the result is empty, which is what makes a first refresh of an
// unmarked note additive rather than destructive.
func Above(body string) string {
	above, _, found := SplitBody(body)
	if !found {
		return ""
	}
	return above
}

// Rewrite replaces a note's machine-owned body region with above and sets the
// given frontmatter keys, returning the new file content.
//
// Everything from the marker down is preserved byte for byte. When the note has
// NO marker yet, its entire existing body is treated as human-owned and is kept
// BELOW a newly inserted marker: a note that predates machine ownership must
// never lose its content to the first refresh.
//
// A nil value in keys deletes that key. Keys that collide with a field nb
// models itself are rejected — writing one through the inline Extra map would
// emit the key twice.
func Rewrite(content, above string, keys map[string]any) (string, error) {
	fm, body, err := frontmatter.Parse(content)
	if err != nil {
		return "", fmt.Errorf("parse note frontmatter: %w", err)
	}
	if fm == nil {
		return "", fmt.Errorf("note has no frontmatter")
	}

	for key, value := range keys {
		if err := SetExtra(fm, key, value); err != nil {
			return "", err
		}
	}

	return frontmatter.BuildContent(fm, rebuildBody(body, above)), nil
}

// rebuildBody assembles the new body: the machine region, the marker, then the
// preserved human region. The leading newline mirrors what frontmatter.Parse
// hands back for a normal note, so BuildContent's spacing round-trips.
func rebuildBody(body, above string) string {
	_, rest, found := SplitBody(body)
	if !found {
		// No marker: the whole existing body is human-owned. Keep it under a
		// marker we insert now.
		rest = Marker + "\n" + strings.TrimLeft(body, "\n")
	}
	return "\n" + strings.Trim(above, "\n") + "\n\n" + rest
}

// SetExtra sets (or, for a nil value, deletes) an unmodelled frontmatter key
// through the inline Extra map. Modelled keys are refused: Build serializes
// them from their own struct fields, so the same key coming back out of Extra
// would be written twice.
func SetExtra(fm *frontmatter.Frontmatter, key string, value any) error {
	if fm == nil {
		return fmt.Errorf("nil frontmatter")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("empty frontmatter key")
	}
	if modelledKeys()[key] {
		return fmt.Errorf("frontmatter key %q is modelled by nb; set it through its own field", key)
	}
	if value == nil {
		delete(fm.Extra, key)
		if len(fm.Extra) == 0 {
			fm.Extra = nil
		}
		return nil
	}
	return fm.SetExtra(key, value)
}

// modelledKeys is the set of frontmatter keys nb serializes from named struct
// fields. It is derived by reflection rather than written out, so a new field
// on Frontmatter becomes reserved automatically instead of silently becoming
// double-writable.
func modelledKeys() map[string]bool {
	out := map[string]bool{}
	t := reflect.TypeOf(frontmatter.Frontmatter{})
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("yaml")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		out[name] = true
	}
	return out
}
