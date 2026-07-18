package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func gapStorePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "concepts", conceptGapsFileName)
}

func TestConceptGapRecordListResolveRoundTrip(t *testing.T) {
	path := gapStorePath(t)
	t0 := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)

	owned, err := recordConceptGap(path, ConceptGap{
		File: "pkg/context/resolve.go", ConceptID: "auth", Reason: "core resolver", Source: "flow-context-tuner",
	}, t0)
	if err != nil {
		t.Fatalf("record owned: %v", err)
	}
	if owned.Kind != ConceptGapKindPreset {
		t.Errorf("owned gap kind = %q, want %q", owned.Kind, ConceptGapKindPreset)
	}
	if len(owned.ID) != 12 {
		t.Errorf("gap id %q should be a 12-char hash", owned.ID)
	}

	unowned, err := recordConceptGap(path, ConceptGap{File: "pkg/orphan/thing.go", Reason: "nothing covers this"}, t0.Add(time.Second))
	if err != nil {
		t.Fatalf("record unowned: %v", err)
	}
	if unowned.Kind != ConceptGapKindConcept {
		t.Errorf("unowned gap kind = %q, want %q", unowned.Kind, ConceptGapKindConcept)
	}
	if unowned.ID == owned.ID {
		t.Errorf("gap ids should be distinct")
	}

	gaps, err := readConceptGaps(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(gaps) != 2 || gaps[0].ID != owned.ID || gaps[1].ID != unowned.ID {
		t.Fatalf("round-trip mismatch: %+v", gaps)
	}
	if !gaps[0].CreatedAt.Equal(t0) {
		t.Errorf("created_at not preserved: %v", gaps[0].CreatedAt)
	}

	resolved, err := resolveConceptGap(path, owned.ID, "rejected")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !resolved.Resolved || resolved.ResolvedBy != "rejected" {
		t.Errorf("resolve result: %+v", resolved)
	}

	gaps, _ = readConceptGaps(path)
	unresolvedOnly := filterConceptGaps(gaps, ConceptGapListFilter{})
	if len(unresolvedOnly) != 1 || unresolvedOnly[0].ID != unowned.ID {
		t.Errorf("default filter should hide resolved gaps: %+v", unresolvedOnly)
	}
	if all := filterConceptGaps(gaps, ConceptGapListFilter{IncludeResolved: true}); len(all) != 2 {
		t.Errorf("--all filter should include resolved gaps: %+v", all)
	}
	resolvedOnly := filterConceptGaps(gaps, ConceptGapListFilter{OnlyResolved: true})
	if len(resolvedOnly) != 1 || resolvedOnly[0].ID != owned.ID {
		t.Errorf("--resolved filter mismatch: %+v", resolvedOnly)
	}
	byConcept := filterConceptGaps(gaps, ConceptGapListFilter{IncludeResolved: true, ConceptID: "auth"})
	if len(byConcept) != 1 || byConcept[0].ID != owned.ID {
		t.Errorf("--concept filter mismatch: %+v", byConcept)
	}
}

func TestConceptGapBatchAppend(t *testing.T) {
	path := gapStorePath(t)
	t0 := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)

	batch := []ConceptGap{
		{File: "a.go", ConceptID: "auth", Reason: "one"},
		{File: "b.go", Reason: "two"},
		{File: "c.go", ConceptID: "auth", Reason: "three"},
	}
	for i, gap := range batch {
		if _, err := recordConceptGap(path, gap, t0.Add(time.Duration(i)*time.Millisecond)); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	gaps, err := readConceptGaps(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(gaps) != 3 {
		t.Fatalf("expected 3 appended gaps, got %d", len(gaps))
	}
	seen := map[string]bool{}
	for _, g := range gaps {
		if seen[g.ID] {
			t.Errorf("duplicate gap id %q", g.ID)
		}
		seen[g.ID] = true
	}
}

func TestConceptGapMissingStore(t *testing.T) {
	path := gapStorePath(t)

	gaps, err := readConceptGaps(path)
	if err != nil {
		t.Fatalf("missing store should read as empty, got error: %v", err)
	}
	if len(gaps) != 0 {
		t.Errorf("missing store should be empty, got %+v", gaps)
	}

	if _, err := resolveConceptGap(path, "deadbeef", "rejected"); err == nil {
		t.Error("resolving against a missing store should fail with not-found")
	}
}

func TestConceptGapResolvePrefix(t *testing.T) {
	path := gapStorePath(t)

	// Hand-written store with controlled ids to exercise prefix matching.
	store := `{"id":"abc123def456","file":"a.go","concept_id":"","kind":"concept","reason":"","source":"","created_at":"2026-07-18T10:00:00Z","resolved":false}
{"id":"abc987xyz654","file":"b.go","concept_id":"","kind":"concept","reason":"","source":"","created_at":"2026-07-18T10:00:01Z","resolved":false}
`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(store), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := resolveConceptGap(path, "abc", "rejected"); err == nil {
		t.Error("ambiguous prefix should fail")
	}

	gap, err := resolveConceptGap(path, "abc1", "added-to-preset")
	if err != nil {
		t.Fatalf("unique prefix resolve: %v", err)
	}
	if gap.ID != "abc123def456" || !gap.Resolved || gap.ResolvedBy != "added-to-preset" {
		t.Errorf("prefix resolve result: %+v", gap)
	}

	gaps, _ := readConceptGaps(path)
	if !gaps[0].Resolved || gaps[1].Resolved {
		t.Errorf("only the first gap should be resolved: %+v", gaps)
	}
}

func TestConceptGapRecordValidation(t *testing.T) {
	path := gapStorePath(t)
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)

	if _, err := recordConceptGap(path, ConceptGap{}, now); err == nil {
		t.Error("missing file should fail")
	}
	if _, err := recordConceptGap(path, ConceptGap{File: "a.go", Kind: ConceptGapKindPreset}, now); err == nil {
		t.Error("preset kind without a concept id should fail")
	}
	if _, err := recordConceptGap(path, ConceptGap{File: "a.go", Kind: "bogus"}, now); err == nil {
		t.Error("invalid kind should fail")
	}
}
