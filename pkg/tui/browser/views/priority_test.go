package views

import (
	"testing"
	"time"

	"github.com/grovetools/nb/pkg/models"
)

func TestBumpPriority(t *testing.T) {
	tests := []struct {
		name         string
		start        string
		moreCritical bool
		want         string
	}{
		{"empty up to p3", "", true, "p3"},
		{"p3 up to p2", "p3", true, "p2"},
		{"p1 up to p0", "p1", true, "p0"},
		{"p0 stays at ceiling", "p0", true, "p0"},
		{"p0 down to p1", "p0", false, "p1"},
		{"p3 down to empty", "p3", false, ""},
		{"empty stays at floor", "", false, ""},
		{"unknown treated as floor up", "garbage", true, "p3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BumpPriority(tt.start, tt.moreCritical); got != tt.want {
				t.Errorf("BumpPriority(%q, %v) = %q, want %q", tt.start, tt.moreCritical, got, tt.want)
			}
		})
	}
}

func TestPriorityRank(t *testing.T) {
	if priorityRank("") != "z" {
		t.Errorf("priorityRank(\"\") = %q, want z (sorts last)", priorityRank(""))
	}
	if priorityRank("p0") != "p0" {
		t.Errorf("priorityRank(\"p0\") = %q, want p0", priorityRank("p0"))
	}
}

// TestPartitionByPriority verifies the group-by-priority axis buckets notes
// most-critical-first (p0..p3) with unset priority collected last, and drops
// empty buckets.
func TestPartitionByPriority(t *testing.T) {
	now := time.Now()
	notes := []*models.Note{
		{Path: "none.md", Priority: "", CreatedAt: now},
		{Path: "p2.md", Priority: "p2", CreatedAt: now},
		{Path: "p0.md", Priority: "p0", CreatedAt: now},
		// no p1 note: that bucket must be dropped
	}

	buckets := partitionByPriority(notes)

	wantIDs := []string{"p0", "p2", "none"}
	if len(buckets) != len(wantIDs) {
		t.Fatalf("partitionByPriority returned %d buckets, want %d", len(buckets), len(wantIDs))
	}
	for i, want := range wantIDs {
		if buckets[i].id != want {
			t.Errorf("bucket[%d].id = %q, want %q", i, buckets[i].id, want)
		}
	}
}

// TestSortNotesByDate confirms sortNotes orders by creation time only (priority
// no longer affects flat ordering after the redesign).
func TestSortNotesByDate(t *testing.T) {
	now := time.Now()
	notes := []*models.Note{
		{Path: "older.md", Priority: "p0", CreatedAt: now.Add(-time.Hour)},
		{Path: "newer.md", Priority: "p3", CreatedAt: now},
	}

	// sortAscending false (newest first); priority must NOT reorder.
	m := &Model{sortAscending: false}
	m.sortNotes(notes)

	if notes[0].Path != "newer.md" {
		t.Errorf("expected date order (newest first); got %q first", notes[0].Path)
	}
}
