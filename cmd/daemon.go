package cmd

import (
	"context"
	"time"

	"github.com/grovetools/core/pkg/daemon"
)

// notifyDaemonRefreshCmd tells the daemon to do a full re-scan.
func notifyDaemonRefreshCmd() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		client := daemon.NewWithAutoStart()
		defer client.Close()

		if !client.IsRunning() {
			return
		}

		_ = client.Refresh(ctx)
	}()
}
