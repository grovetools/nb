package views

import (
	"testing"
	"time"

	"github.com/grovetools/nb/pkg/models"
)

func TestPartitionByDate(t *testing.T) {
	now := time.Now()
	notes := []*models.Note{
		{Path: "today.md", CreatedAt: now},
		{Path: "older.md", CreatedAt: now.AddDate(0, -2, 0)},
	}

	buckets := partitionByDate(notes)
	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(buckets))
	}
	if buckets[0].id != "today" || buckets[0].label != "Today" {
		t.Errorf("expected first bucket to be Today, got %q/%q", buckets[0].id, buckets[0].label)
	}
	if buckets[len(buckets)-1].id != "older" {
		t.Errorf("expected last bucket to be older, got %q", buckets[len(buckets)-1].id)
	}
	// Empty buckets (week, month) must be omitted.
	for _, b := range buckets {
		if len(b.notes) == 0 {
			t.Errorf("bucket %q is empty but was not omitted", b.id)
		}
	}
}

func TestPartitionByStatusUsesRemoteStateOnly(t *testing.T) {
	notes := []*models.Note{
		{Path: "open.md", Remote: &models.RemoteMetadata{State: "open"}},
		{Path: "closed.md", Remote: &models.RemoteMetadata{State: "closed"}},
		// Local note with todos must NOT be treated as a status; goes to No Status.
		{Path: "local.md", HasTodos: true},
		{Path: "local2.md", Remote: &models.RemoteMetadata{State: ""}},
	}

	buckets := partitionByStatus(notes)

	var noStatus *syntheticBucket
	states := map[string]int{}
	for i := range buckets {
		b := buckets[i]
		states[b.id] = len(b.notes)
		if b.id == "none" {
			noStatus = &buckets[i]
		}
	}

	if states["state-open"] != 1 {
		t.Errorf("expected 1 open note, got %d", states["state-open"])
	}
	if states["state-closed"] != 1 {
		t.Errorf("expected 1 closed note, got %d", states["state-closed"])
	}
	if noStatus == nil || len(noStatus.notes) != 2 {
		t.Errorf("expected 2 notes in No Status bucket (local + empty-state), got %v", noStatus)
	}
}

func TestPartitionByTagFansOutAcrossAllTags(t *testing.T) {
	notes := []*models.Note{
		{Path: "multi.md", Tags: []string{"frontend", "urgent"}},
		{Path: "back.md", Tags: []string{"backend"}},
		{Path: "none.md"},
	}

	buckets := partitionByTag(notes)

	counts := map[string]int{}
	var total int
	for _, b := range buckets {
		counts[b.id] = len(b.notes)
		total += len(b.notes)
	}

	// multi.md appears under both frontend and urgent (fan-out), so per-tag
	// counts sum to MORE than the distinct note count (3).
	if counts["tag-frontend"] != 1 || counts["tag-urgent"] != 1 {
		t.Errorf("expected multi-tag note under both frontend and urgent, got %v", counts)
	}
	if counts["tag-backend"] != 1 {
		t.Errorf("expected 1 backend note, got %d", counts["tag-backend"])
	}
	if counts["untagged"] != 1 {
		t.Errorf("expected 1 untagged note, got %d", counts["untagged"])
	}
	if total != 4 {
		t.Errorf("expected fan-out total of 4 (3 distinct + 1 duplicate), got %d", total)
	}
}
