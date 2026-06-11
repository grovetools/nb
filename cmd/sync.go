package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	grovelogging "github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/paths"
	"github.com/spf13/cobra"

	"github.com/grovetools/nb/pkg/service"
	"github.com/grovetools/nb/pkg/sync"
	"github.com/grovetools/nb/pkg/sync/github"
)

var syncUlog = grovelogging.NewUnifiedLogger("grove-notebook.cmd.sync")

// NewSyncCmd creates the `sync` subcommand.
func NewSyncCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var provider string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync notes with remote services",
		Long:  `Syncs notes with configured remote services like GitHub issues and pull requests.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			s := *svc
			wsCtx, err := s.GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("get workspace context: %w", err)
			}

			// Create syncer and register providers
			syncer := sync.NewSyncer(s)
			syncer.RegisterProvider("github", func() sync.Provider {
				return github.NewProvider()
			})

			// Run sync
			reports, err := syncer.SyncWorkspace(wsCtx)
			if err != nil {
				return err
			}

			// Display results
			for _, report := range reports {
				syncUlog.Success("Sync complete").
					Field("provider", report.Provider).
					Field("created", report.Created).
					Field("updated", report.Updated).
					Field("unchanged", report.Unchanged).
					Field("failed", report.Failed).
					Pretty(fmt.Sprintf("Synced with %s: %d created, %d updated, %d unchanged, %d failed.",
						report.Provider, report.Created, report.Updated, report.Unchanged, report.Failed)).
					PrettyOnly().
					Log(ctx)
				// Show error details if there were any failures
				if len(report.Errors) > 0 {
					syncUlog.Error("Sync errors encountered").
						Field("provider", report.Provider).
						Field("error_count", len(report.Errors)).
						Pretty("Errors:").
						PrettyOnly().
						Log(ctx)
					for _, errMsg := range report.Errors {
						syncUlog.Error("Sync error").
							Field("provider", report.Provider).
							Field("error", errMsg).
							Pretty(fmt.Sprintf("  - %s", errMsg)).
							PrettyOnly().
							Log(ctx)
					}
				}
			}

			notifyDaemonRefreshCmd()

			return nil
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "", "Sync only with a specific provider (e.g., github)")

	// Add subcommands for Notebook Sync Phase 2 (daemon-coordinated)
	cmd.AddCommand(NewSyncHistoryCmd(svc, workspaceOverride))
	cmd.AddCommand(NewSyncRestoreCmd(svc, workspaceOverride))
	cmd.AddCommand(NewSyncConflictsCmd(svc, workspaceOverride))

	return cmd
}

// NewSyncHistoryCmd creates the `sync history` subcommand.
// Displays the version history for a document from the sync server.
func NewSyncHistoryCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	return &cobra.Command{
		Use:   "history <file>",
		Short: "Show version history for a synced document",
		Long:  `Displays the version history of a document stored on the sync server, allowing you to see who changed it and when.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workspace, relPath, err := resolveSyncDocRef(*svc, *workspaceOverride, args[0])
			if err != nil {
				return err
			}

			body, err := daemonSyncGet(cmd.Context(), fmt.Sprintf(
				"/api/sync/history?workspace=%s&path=%s",
				url.QueryEscape(workspace), url.QueryEscape(relPath)))
			if err != nil {
				return err
			}

			var entries []struct {
				Seq        int64  `json:"seq"`
				Version    int64  `json:"version"`
				Actor      string `json:"actor"`
				ReceivedAt string `json:"received_at"`
			}
			if err := json.Unmarshal(body, &entries); err != nil {
				return fmt.Errorf("decode history response: %w", err)
			}
			if len(entries) == 0 {
				fmt.Printf("No sync history for %s/%s\n", workspace, relPath)
				return nil
			}

			fmt.Printf("History for %s/%s:\n", workspace, relPath)
			for _, e := range entries {
				fmt.Printf("  v%-4d  %-20s  %s\n", e.Version, e.Actor, e.ReceivedAt)
			}
			return nil
		},
	}
}

// NewSyncRestoreCmd creates the `sync restore` subcommand.
// Restores a document from a specific version.
func NewSyncRestoreCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var version string
	cmd := &cobra.Command{
		Use:   "restore <file>",
		Short: "Restore a document to a specific version",
		Long:  `Restores a document from the sync server history, allowing you to undo unwanted changes.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if version == "" {
				return fmt.Errorf("--version is required")
			}
			absPath, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}
			workspace, relPath, err := resolveSyncDocRef(*svc, *workspaceOverride, args[0])
			if err != nil {
				return err
			}

			content, err := daemonSyncGet(cmd.Context(), fmt.Sprintf(
				"/api/sync/restore?workspace=%s&path=%s&version=%s",
				url.QueryEscape(workspace), url.QueryEscape(relPath), url.QueryEscape(version)))
			if err != nil {
				return err
			}

			// Write the historical content over the local file as a normal
			// user-initiated edit; fsnotify pushes it as a new head version.
			if err := os.WriteFile(absPath, content, 0o644); err != nil {
				return fmt.Errorf("write restored content: %w", err)
			}
			notifyDaemonRefreshCmd()
			fmt.Printf("Restored %s/%s to version %s (%d bytes)\n", workspace, relPath, version, len(content))
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "Version ID to restore to (required)")
	return cmd
}

// NewSyncConflictsCmd creates the `sync conflicts` subcommand.
// Lists, views, and resolves merge conflicts from sync.
func NewSyncConflictsCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	return &cobra.Command{
		Use:   "conflicts",
		Short: "Manage sync conflicts",
		Long:  `List, view, and resolve conflicts from document synchronization.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			conflictRoot := filepath.Join(paths.StateDir(), "sync", "conflicts")
			var found int
			err := filepath.WalkDir(conflictRoot, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil // missing root = no conflicts
				}
				rel, _ := filepath.Rel(conflictRoot, p)
				info, _ := d.Info()
				if info != nil {
					fmt.Printf("  %s  (%s, %d bytes)\n", rel, info.ModTime().Format("2006-01-02 15:04"), info.Size())
				} else {
					fmt.Printf("  %s\n", rel)
				}
				found++
				return nil
			})
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if found == 0 {
				fmt.Println("No sync conflicts.")
			} else {
				fmt.Printf("%d conflict artifact(s) under %s\n", found, conflictRoot)
			}
			return nil
		},
	}
}

// resolveSyncDocRef maps a local file argument to the (workspace,
// workspace-relative path) pair used as document identity by the sync
// protocol. Notebook files live at <root>/workspaces/<ws>/<type>/file.md;
// the sync-relative path is everything after the workspace segment.
func resolveSyncDocRef(s *service.Service, workspaceOverride, fileArg string) (string, string, error) {
	absPath, err := filepath.Abs(fileArg)
	if err != nil {
		return "", "", err
	}
	note, err := service.ParseNote(absPath)
	if err != nil {
		return "", "", fmt.Errorf("parse note %s: %w", fileArg, err)
	}
	workspace := note.Workspace
	if workspaceOverride != "" {
		workspace = workspaceOverride
	}

	marker := string(filepath.Separator) + filepath.Join("workspaces", workspace) + string(filepath.Separator)
	idx := strings.LastIndex(absPath, marker)
	if idx < 0 {
		return "", "", fmt.Errorf("cannot determine workspace-relative path for %s (workspace %s)", fileArg, workspace)
	}
	rel := absPath[idx+len(marker):]
	return workspace, filepath.ToSlash(rel), nil
}

// daemonSyncGet performs a GET against the global daemon's unix socket.
// Sync credentials live exclusively in the daemon; nb never sees the token.
func daemonSyncGet(ctx context.Context, requestPath string) ([]byte, error) {
	socketPath := paths.SocketPath()
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://groved"+requestPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("daemon request failed (is groved running?): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
