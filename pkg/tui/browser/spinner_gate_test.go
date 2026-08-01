package browser

import (
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// newSpinnerTestModel builds a browser whose spinner ticks fast enough that a
// test can pump a chain of them without sleeping for tenths of a second.
func newSpinnerTestModel(t *testing.T) Model {
	t.Helper()
	m := *newLayoutTestModel(t, 3)
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Spinner.FPS = time.Millisecond
	m.spinner = s
	return m
}

// pumpTick runs a spinner tick command and feeds the resulting message back
// into Update, returning the new model and the command it re-armed (if any).
func pumpTick(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	msg := cmd()
	if _, ok := msg.(spinner.TickMsg); !ok {
		t.Fatalf("expected a spinner.TickMsg from the re-armed command, got %T", msg)
	}
	updated, next := m.Update(msg)
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want browser.Model", updated)
	}
	return got, next
}

// TestSpinnerTickStopsWhenNothingIsLoading is the guard on the idle busy loop.
// bubbles re-arms spinner.TickMsg forever, but View draws the spinner only
// while loadingCount > 0 — so an ungated re-arm costs a full Model.View() ten
// times a second for a frame index nothing on screen reads. The tick must die
// with the last in-flight load, and must not advance the frame on its way out.
func TestSpinnerTickStopsWhenNothingIsLoading(t *testing.T) {
	m := newSpinnerTestModel(t)
	m.loadingCount = 1

	updated, cmd := m.Update(m.spinner.Tick())
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("spinner tick did not arm while a load was in flight")
	}

	// While loading, every tick keeps the chain alive and animates.
	for i := range 3 {
		before := m.spinner.View()
		m, cmd = pumpTick(t, m, cmd)
		if cmd == nil {
			t.Fatalf("spinner chain died at tick %d while loadingCount=%d", i, m.loadingCount)
		}
		if after := m.spinner.View(); after == before {
			t.Fatalf("tick %d did not advance the spinner frame (%q)", i, after)
		}
	}

	// The last load lands: the next tick is the final one.
	m.loadingCount = 0
	frozen := m.spinner.View()
	m, cmd = pumpTick(t, m, cmd)
	if cmd != nil {
		t.Fatal("spinner tick re-armed with nothing loading — the idle 10fps re-render is back")
	}
	if got := m.spinner.View(); got != frozen {
		t.Fatalf("idle tick advanced the frame: %q -> %q", frozen, got)
	}
}

// TestSpinnerTickRestartsOnNewLoad covers the other half of the gate: every
// site that starts a load batches m.spinner.Tick, and bubbles rejects ticks
// whose tag does not match the spinner's current one. Stopping the chain must
// leave the spinner in a state where that fresh Tick is still accepted,
// otherwise a real load renders a frozen spinner.
func TestSpinnerTickRestartsOnNewLoad(t *testing.T) {
	m := newSpinnerTestModel(t)

	// Run and stop a chain first, so the spinner's tag is well past zero.
	m.loadingCount = 1
	updated, cmd := m.Update(m.spinner.Tick())
	m = updated.(Model)
	for range 3 {
		m, cmd = pumpTick(t, m, cmd)
	}
	m.loadingCount = 0
	m, cmd = pumpTick(t, m, cmd)
	if cmd != nil {
		t.Fatal("spinner chain did not stop when loading finished")
	}

	// A new load begins, exactly as the fetch sites do it.
	m.loadingCount++
	updated, cmd = m.Update(m.spinner.Tick())
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("spinner did not restart when a new load began")
	}

	before := m.spinner.View()
	m, cmd = pumpTick(t, m, cmd)
	if cmd == nil {
		t.Fatal("restarted spinner chain died after one tick")
	}
	if after := m.spinner.View(); after == before {
		t.Fatalf("restarted spinner is frozen at %q", after)
	}
}
