package cmd

import (
	"bytes"
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
	cmd.AddCommand(NewSyncAdoptCmd(svc, workspaceOverride))
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

// NewSyncAdoptCmd creates the `sync adopt` subcommand: the one sanctioned,
// user-initiated write that resolves a diverged document. Sync itself never
// writes the notebook tree (strict push-only, S5); when a push-side rebase
// merges a remote edit that the local file does not carry, the document enters
// the `diverged` state and its local file is deliberately left alone. `adopt`
// fetches the server head from the daemon and writes it over the local file —
// the user asked for it, so this is not sync writing the tree. Distinct from
// `restore`, which plays back a specific historical version.
func NewSyncAdoptCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	return &cobra.Command{
		Use:   "adopt <file>",
		Short: "Adopt the server head for a diverged document",
		Long: `Take the current server head as the local content for a document that has
diverged (a merged remote edit the local file does not carry). Sync never
writes the notebook tree on its own; adopt is the explicit, user-initiated
write that resolves the divergence.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			absPath, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}
			workspace, relPath, err := resolveSyncDocRef(*svc, *workspaceOverride, args[0])
			if err != nil {
				return err
			}

			reqBody, err := json.Marshal(map[string]string{"workspace": workspace, "path": relPath})
			if err != nil {
				return err
			}
			resp, err := daemonSyncRequest(cmd.Context(), http.MethodPost, "/api/sync/adopt", bytes.NewReader(reqBody))
			if err != nil {
				return fmt.Errorf("daemon request failed (is groved running?): %w", err)
			}
			defer func() { _ = resp.Body.Close() }()

			content, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			if resp.StatusCode == http.StatusConflict {
				return fmt.Errorf("cannot adopt %s/%s yet: %s (the merged push must drain first)",
					workspace, relPath, strings.TrimSpace(string(content)))
			}
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("daemon returned %d: %s", resp.StatusCode, strings.TrimSpace(string(content)))
			}

			// The sanctioned user-initiated write (mirrors NewSyncRestoreCmd).
			if err := os.WriteFile(absPath, content, 0o644); err != nil {
				return fmt.Errorf("write adopted content: %w", err)
			}
			notifyDaemonRefreshCmd()
			fmt.Printf("Adopted %s/%s at server head (%d bytes)\n", workspace, relPath, len(content))
			return nil
		},
	}
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

// daemonSyncRequest performs an arbitrary-method request against the global
// daemon's unix socket and returns the raw response (caller closes the body).
// A JSON body sets Content-Type. Sync credentials live exclusively in the
// daemon; nb never sees the token. daemonSyncGet is the GET convenience built on
// top; POST callers (nb sync adopt) use this directly so they can branch on the
// status code (e.g. 409 pending-push).
func daemonSyncRequest(ctx context.Context, method, requestPath string, body io.Reader) (*http.Response, error) {
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

	req, err := http.NewRequestWithContext(ctx, method, "http://groved"+requestPath, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return client.Do(req)
}

// daemonSyncGet performs a GET against the global daemon's unix socket,
// returning the body only on a 200.
func daemonSyncGet(ctx context.Context, requestPath string) ([]byte, error) {
	resp, err := daemonSyncRequest(ctx, http.MethodGet, requestPath, nil)
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
