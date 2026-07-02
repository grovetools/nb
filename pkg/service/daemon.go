package service

import (
	"context"
	"time"

	"github.com/grovetools/core/pkg/daemon"
	"github.com/grovetools/core/pkg/models"
)

// EmitNoteEvent is the single funnel for note mutations: it records the
// mutation as a lifecycle event in the structured log (event=note.<verb>,
// StructuredOnly — user-facing pretty output is owned by the cmd layer)
// and forwards the typed event to the daemon. It is exported so the cmd
// layer's internal commands can reuse the same funnel.
func EmitNoteEvent(event models.NoteEvent) {
	entry := serviceUlog.Info("Note "+string(event.Event)).
		Field("event", "note."+string(event.Event)).
		Field("path", event.Path).
		Field("type", event.NoteType).
		Field("workspace", event.Workspace)
	if event.PrevPath != "" {
		entry = entry.Field("prev_path", event.PrevPath)
	}
	entry.StructuredOnly().Emit()

	notifyDaemonNoteEvent(event)
}

// notifyDaemonNoteEvent sends a note mutation event to the daemon in a background goroutine.
// This is fire-and-forget — nb never blocks on daemon availability.
// It is a package-level var so tests can capture emitted events.
var notifyDaemonNoteEvent = defaultNotifyDaemonNoteEvent

func defaultNotifyDaemonNoteEvent(event models.NoteEvent) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		client := daemon.NewWithAutoStart()
		defer client.Close()

		if !client.IsRunning() {
			return
		}

		_ = client.NotifyNoteEvent(ctx, event)
	}()
}
