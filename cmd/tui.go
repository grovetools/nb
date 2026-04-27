package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/compositor"
	"github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/core/util/pathutil"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/grovetools/nb/pkg/service"
	"github.com/grovetools/nb/pkg/tui/browser"
)

// NewTuiCmd creates the `nb tui` command.
func NewTuiCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch an interactive TUI for browsing notes across workspaces",
		Long: `Launch an interactive Terminal User Interface for browsing and managing notes.
This view provides a workspace-centric way to explore your entire notebook.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check for TTY
			if !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()) {
				return fmt.Errorf("TUI mode requires an interactive terminal")
			}

			s := *svc

			// Get current workspace context to determine initial focus
			ctx, err := s.GetWorkspaceContext(*workspaceOverride)
			if err != nil {
				return fmt.Errorf("failed to get workspace context: %w", err)
			}

			var initialFocus *workspace.WorkspaceNode
			if ctx.NotebookContextWorkspace.Name != "global" {
				rawInitialFocus := ctx.NotebookContextWorkspace
				// The workspace from GetWorkspaceContext might be a "raw" discovery.
				// We need to find the canonical instance from the provider to ensure
				// all metadata (like its Kind) is fully resolved.
				provider := s.GetWorkspaceProvider()
				foundInProvider := false
				for _, ws := range provider.All() {
					isSame, _ := pathutil.ComparePaths(ws.Path, rawInitialFocus.Path)
					if isSame {
						initialFocus = ws
						foundInProvider = true
						break
					}
				}
				if !foundInProvider {
					// Fallback if not found, though this is unlikely.
					initialFocus = rawInitialFocus
				}
			}

			// Create the pure browser model and wrap it in the CLI environment
			// host so the standalone CLI keeps its Neovim /tmp IPC and tmux split
			// behavior, while the browser model itself stays environment-agnostic.
			browserModel := browser.New(browser.Config{
				Service:      s,
				InitialFocus: initialFocus,
				Context:      ctx,
			})
			host := &cliEnvironmentHost{model: browserModel}

			// Wrap in StandaloneHost (handles DoneMsg, CloseRequestMsg,
			// EditRequestMsg) then compositor (GPU-accelerated rendering).
			standaloneHost := embed.NewStandaloneHost(host)
			compModel := compositor.NewModel(standaloneHost)
			p := tea.NewProgram(compModel, tea.WithAltScreen())

			finalModel, runErr := p.Run()

			// Free compositor resources and unwrap to recover the
			// StandaloneHost so post-exit result extraction succeeds.
			if cm, ok := finalModel.(*compositor.Model); ok {
				cm.Free()
				finalModel = cm.Unwrap()
			}

			if runErr != nil {
				return fmt.Errorf("error running TUI: %w", runErr)
			}

			if finalHost, ok := finalModel.(embed.StandaloneHost); ok && finalHost.Err != nil {
				return fmt.Errorf("error running TUI: %w", finalHost.Err)
			}

			return nil
		},
	}
	return cmd
}

// cliEnvironmentHost is a tea.Model wrapper that intercepts embed messages from
// the pure browser model and translates them into CLI-specific behaviors:
//
//   - In a Neovim plugin session (GROVE_NVIM_PLUGIN=true), edit/preview requests
//     are written to a temp file polled by the grove.nvim Lua plugin instead of
//     opening a local editor.
//   - In a tmux session, edit requests open the file in a tmux split pane,
//     reusing an existing split when possible.
//   - Otherwise, the request is passed through to the parent host (typically
//     embed.StandaloneHost), which falls back to $EDITOR via tea.ExecProcess.
//
// All other messages are forwarded to the wrapped browser model unchanged.
type cliEnvironmentHost struct {
	model tea.Model

	// Tmux split state, owned by the host so the browser model stays free of
	// any environment awareness.
	tmuxSplitPaneID string
	tmuxTUIPaneID   string
}

func (h *cliEnvironmentHost) Init() tea.Cmd {
	return h.model.Init()
}

func (h *cliEnvironmentHost) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case embed.EditRequestMsg:
		if os.Getenv("GROVE_NVIM_PLUGIN") == "true" {
			writeNvimIPC("OPEN", msg.Path)
			return h, nil
		}
		if os.Getenv("TMUX") != "" {
			return h, h.openInTmuxCmd(msg.Path)
		}
		// Fall through to the parent host (StandaloneHost) which runs $EDITOR.

	case embed.PreviewRequestMsg:
		if os.Getenv("GROVE_NVIM_PLUGIN") == "true" {
			writeNvimIPC("PREVIEW", msg.Path)
			return h, nil
		}
		// Other environments don't support out-of-band preview today.
		return h, nil

	case tmuxSplitFinishedMsg:
		if msg.clearPanes {
			h.tmuxSplitPaneID = ""
			h.tmuxTUIPaneID = ""
		}
		if msg.paneID != "" {
			h.tmuxSplitPaneID = msg.paneID
		}
		if msg.tuiPaneID != "" {
			h.tmuxTUIPaneID = msg.tuiPaneID
		}
		// Tmux errors are silently swallowed today (matching the prior behavior
		// where the browser TUI showed them in its status bar). For now we just
		// drop them on the floor; the user can re-trigger the open.
		return h, nil
	}

	var cmd tea.Cmd
	h.model, cmd = h.model.Update(msg)
	return h, cmd
}

func (h *cliEnvironmentHost) View() string {
	return h.model.View()
}

// writeNvimIPC writes a single-line action+path entry to the temp file polled
// by the grove.nvim plugin. The session id comes from GROVE_NVIM_SESSION_ID
// when set (so multiple nvim instances stay isolated) and falls back to PID.
func writeNvimIPC(action, path string) {
	sessionID := os.Getenv("GROVE_NVIM_SESSION_ID")
	if sessionID == "" {
		sessionID = fmt.Sprintf("%d", os.Getpid())
	}
	tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("grove-nb-edit-%s", sessionID))
	_ = os.WriteFile(tempFile, []byte(action+":"+path+"\n"), 0o644)
}

// tmuxSplitFinishedMsg reports the result of a tmux split-or-reuse operation
// initiated by the cliEnvironmentHost. It is internal to this file.
type tmuxSplitFinishedMsg struct {
	paneID     string
	tuiPaneID  string
	clearPanes bool
	err        error
}

// openInTmuxCmd intelligently opens a file in tmux. In a popup it opens the
// editor in the parent session and closes the popup; otherwise it reuses an
// existing split or creates a new one alongside the TUI.
func (h *cliEnvironmentHost) openInTmuxCmd(path string) tea.Cmd {
	splitPaneID := h.tmuxSplitPaneID
	tuiPaneID := h.tmuxTUIPaneID
	return func() tea.Msg {
		client, err := tmux.NewClient()
		if err != nil {
			return tmuxSplitFinishedMsg{err: fmt.Errorf("tmux client not found: %w", err)}
		}

		ctx := context.Background()
		isPopup, err := client.IsPopup(ctx)
		if err != nil {
			return tmuxSplitFinishedMsg{err: fmt.Errorf("IsPopup error: %w", err)}
		}

		if isPopup {
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "nvim"
			}
			if err := client.OpenInEditorWindow(ctx, editor, path, "notebook", 2, false); err != nil {
				return tmuxSplitFinishedMsg{err: fmt.Errorf("popup mode - failed to open in editor: %w", err)}
			}
			if err := client.ClosePopup(ctx); err != nil {
				return tmuxSplitFinishedMsg{err: fmt.Errorf("failed to close popup: %w", err)}
			}
			// The popup closing also tears down the TUI, so emit a close request.
			return embed.CloseRequestMsg{}
		}

		return openInTmuxSplit(ctx, client, splitPaneID, tuiPaneID, path)
	}
}

// openInTmuxSplit reuses the host's existing split pane (if any) or creates a
// new one alongside the TUI, then returns a tmuxSplitFinishedMsg with the
// updated pane bookkeeping for the host to absorb.
func openInTmuxSplit(ctx context.Context, client *tmux.Client, splitPaneID, tuiPaneID, path string) tea.Msg {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	// If we already have a split pane, try to reuse it.
	paneStillExists := false
	if splitPaneID != "" {
		if client.PaneExists(ctx, splitPaneID) {
			paneStillExists = true
			if err := client.SendKeys(ctx, splitPaneID, fmt.Sprintf(":e %s", path), "Enter"); err == nil {
				if err := client.SelectPane(ctx, splitPaneID); err != nil {
					return tmuxSplitFinishedMsg{err: fmt.Errorf("failed to switch to editor: %w", err)}
				}
				return tmuxSplitFinishedMsg{paneID: splitPaneID}
			}
			// SendKeys failed; fall through to create a new split.
		}
	}

	shouldClearOldPanes := splitPaneID != "" && !paneStillExists

	newTuiPaneID, err := client.GetCurrentPaneID(ctx)
	if err != nil {
		return tmuxSplitFinishedMsg{err: fmt.Errorf("failed to get current pane ID: %w", err)}
	}

	currentWidth, err := client.GetPaneWidth(ctx, "")
	if err != nil {
		return tmuxSplitFinishedMsg{err: fmt.Errorf("failed to get pane width: %w", err)}
	}

	// Reserve roughly 30% of the screen for the TUI (40-80 cols), then give the
	// editor the rest. Below 120 cols just split 50/50.
	tuiWidth := currentWidth * 30 / 100
	if tuiWidth < 40 {
		tuiWidth = 40
	}
	if tuiWidth > 80 {
		tuiWidth = 80
	}
	if currentWidth < 120 {
		tuiWidth = 0
	}

	editorWidth := 0
	if tuiWidth > 0 {
		editorWidth = currentWidth - tuiWidth - 1
		if editorWidth < 40 {
			editorWidth = 0
		}
	}

	commandToRun := fmt.Sprintf("%s %q", editor, path)
	paneID, err := client.SplitWindow(ctx, "", true, editorWidth, commandToRun)
	if err != nil {
		return tmuxSplitFinishedMsg{err: fmt.Errorf("failed to split tmux window: %w", err)}
	}

	return tmuxSplitFinishedMsg{
		paneID:     paneID,
		tuiPaneID:  newTuiPaneID,
		clearPanes: shouldClearOldPanes,
	}
}
