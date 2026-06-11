package frontmatter

import (
	"testing"
	"time"
)

func TestParseTimestampLegacy(t *testing.T) {
	got, err := ParseTimestamp("2025-01-11 10:00:00")
	if err != nil {
		t.Fatalf("ParseTimestamp(legacy) error: %v", err)
	}
	want := time.Date(2025, 1, 11, 10, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("ParseTimestamp(legacy) = %v, want %v", got, want)
	}
}

func TestParseTimestampRFC3339(t *testing.T) {
	cases := []struct {
		in   string
		want time.Time
	}{
		{"2026-06-11T09:30:00Z", time.Date(2026, 6, 11, 9, 30, 0, 0, time.UTC)},
		{"2026-06-11T01:30:00-08:00", time.Date(2026, 6, 11, 9, 30, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		got, err := ParseTimestamp(c.in)
		if err != nil {
			t.Fatalf("ParseTimestamp(%q) error: %v", c.in, err)
		}
		if !got.Equal(c.want) {
			t.Errorf("ParseTimestamp(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseTimestampInvalid(t *testing.T) {
	if _, err := ParseTimestamp("not-a-timestamp"); err == nil {
		t.Error("ParseTimestamp(invalid) expected error, got nil")
	}
}

func TestFormatTimestampWritesUTCRFC3339(t *testing.T) {
	loc := time.FixedZone("PST", -8*3600)
	in := time.Date(2026, 6, 11, 1, 30, 0, 0, loc)

	got := FormatTimestamp(in)
	want := "2026-06-11T09:30:00Z"
	if got != want {
		t.Errorf("FormatTimestamp = %q, want %q", got, want)
	}

	// Round-trip: everything FormatTimestamp writes must parse back.
	back, err := ParseTimestamp(got)
	if err != nil {
		t.Fatalf("round-trip parse error: %v", err)
	}
	if !back.Equal(in) {
		t.Errorf("round-trip = %v, want %v", back, in)
	}
}
