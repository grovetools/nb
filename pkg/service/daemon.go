package service

import (
	"context"
	"time"

	"github.com/grovetools/core/pkg/daemon"
	"github.com/grovetools/core/pkg/models"
)

// notifyDaemonNoteEvent sends a note mutation event to the daemon in a background goroutine.
// This is fire-and-forget — nb never blocks on daemon availability.
func notifyDaemonNoteEvent(event models.NoteEvent) {
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
