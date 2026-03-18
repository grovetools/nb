package cmd

import (
	"context"
	"time"

	"github.com/grovetools/core/pkg/daemon"
	"github.com/grovetools/core/pkg/models"
)

// notifyDaemonNoteEventCmd sends a note mutation event to the daemon in a background goroutine.
// This is the cmd-package equivalent of service.notifyDaemonNoteEvent.
func notifyDaemonNoteEventCmd(event models.NoteEvent) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		client := daemon.New()
		defer client.Close()

		if !client.IsRunning() {
			return
		}

		_ = client.NotifyNoteEvent(ctx, event)
	}()
}

// notifyDaemonRefreshCmd tells the daemon to do a full re-scan.
func notifyDaemonRefreshCmd() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		client := daemon.New()
		defer client.Close()

		if !client.IsRunning() {
			return
		}

		_ = client.Refresh(ctx)
	}()
}
