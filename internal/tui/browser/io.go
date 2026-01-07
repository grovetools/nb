package browser

import (
	"os/exec"
	"path/filepath"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/git"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/util/pathutil"
	"github.com/mattsolo1/grove-notebook/pkg/service"
	"github.com/mattsolo1/grove-notebook/pkg/tree"
)

type workspacesLoadedMsg struct {
	workspaces []*workspace.WorkspaceNode
}

type itemsLoadedMsg struct {
	items []*tree.Item
}

func fetchFocusedItemsCmd(svc *service.Service, focusedWS *workspace.WorkspaceNode, showArtifacts bool) tea.Cmd {
	return func() tea.Msg {
		var itemsToLoad []*workspace.WorkspaceNode
		itemsToLoad = append(itemsToLoad, focusedWS)

		// If focused on an ecosystem, also load items from its direct children
		if focusedWS.IsEcosystem() {
			allWorkspaces := svc.GetWorkspaceProvider().All()
			for _, ws := range allWorkspaces {
				if ws.IsChildOf(focusedWS.Path) {
					itemsToLoad = append(itemsToLoad, ws)
				}
			}
		}

		var allItems []*tree.Item
		// Use a map to deduplicate items by path
		seenItems := make(map[string]bool)

		for _, wsNode := range itemsToLoad {
			// Get context for the workspace
			wsCtx, err := svc.GetWorkspaceContext(wsNode.Path)
			if err != nil {
				// Log or handle error, for now, we skip
				continue
			}

			// Fetch items for the workspace (including archived)
			items, err := svc.ListAllItems(wsCtx, true, showArtifacts)
			if err == nil {
				for _, item := range items {
					if !seenItems[item.Path] {
						allItems = append(allItems, item)
						seenItems[item.Path] = true
					}
				}
			}
		}

		// Also fetch global items explicitly (including archived)
		globalItems, err := svc.ListAllGlobalItems(true, showArtifacts)
		if err == nil {
			for _, item := range globalItems {
				if !seenItems[item.Path] {
					allItems = append(allItems, item)
					seenItems[item.Path] = true
				}
			}
		}

		// Combine and sort
		sort.Slice(allItems, func(i, j int) bool {
			return allItems[i].ModTime.After(allItems[j].ModTime)
		})
		return itemsLoadedMsg{items: allItems}
	}
}

func fetchWorkspacesCmd(provider *workspace.Provider) tea.Cmd {
	return func() tea.Msg {
		// Get the real workspaces from the provider.
		workspaces := provider.All()

		// Create and prepend a synthetic "global" workspace node.
		// This confines the concept of a "global" workspace to the notebook TUI.
		globalNode := &workspace.WorkspaceNode{
			Name:  "global",
			Path:  "::global", // A unique, non-filesystem path
			Kind:  workspace.KindStandaloneProject,
			Depth: 0,
		}
		workspaces = append([]*workspace.WorkspaceNode{globalNode}, workspaces...)

		// We need to build the tree to get depth information for filtering ecosystems.
		workspaces = workspace.BuildWorkspaceTree(workspaces)
		return workspacesLoadedMsg{workspaces: workspaces}
	}
}

func fetchAllItemsCmd(svc *service.Service, showArtifacts bool) tea.Cmd {
	return func() tea.Msg {
		// Fetch items from all provider-known workspaces (including archived)
		items, err := svc.ListItemsFromAllWorkspaces(true, showArtifacts)
		if err != nil {
			// In a real app, we'd return an error message.
			// For now, we return an empty list.
			return itemsLoadedMsg{items: []*tree.Item{}}
		}

		// Also fetch global items explicitly and append them (including archived)
		globalItems, err := svc.ListAllGlobalItems(true, showArtifacts)
		if err == nil {
			items = append(items, globalItems...)
		}

		// Sort by modified date descending by default
		sort.Slice(items, func(i, j int) bool {
			return items[i].ModTime.After(items[j].ModTime)
		})
		return itemsLoadedMsg{items: items}
	}
}

// gitStatusLoadedMsg is sent when git status for a repository has been fetched
type gitStatusLoadedMsg struct {
	repoPath     string
	fileStatus   map[string]string
	deletedFiles []string // paths of deleted files that don't exist on disk
	err          error
}

// fetchGitStatusCmd fetches git status for a given repository path
func fetchGitStatusCmd(itemPath string) tea.Cmd {
	return func() tea.Msg {
		// Use the directory of the file, not the file itself
		dir := filepath.Dir(itemPath)

		// Find git root from this item's directory
		gitRoot, err := git.GetGitRoot(dir)
		if err != nil {
			// Not a git repo, return empty status
			return gitStatusLoadedMsg{repoPath: "", fileStatus: nil, err: nil}
		}

		// Get extended file status including deleted files
		result, err := service.GetFileStatusExtended(gitRoot)
		if err != nil {
			return gitStatusLoadedMsg{repoPath: gitRoot, fileStatus: nil, err: err}
		}

		return gitStatusLoadedMsg{
			repoPath:     gitRoot,
			fileStatus:   result.FileStatus,
			deletedFiles: result.DeletedFiles,
			err:          nil,
		}
	}
}

// commitFinishedMsg is sent when a commit operation completes
type commitFinishedMsg struct {
	success bool
	message string
	err     error
}

// stageFinishedMsg is sent when a stage operation completes
type stageFinishedMsg struct {
	success       bool
	count         int
	err           error
	updatedStatus map[string]string // Updated status for staged files
}

// unstageFinishedMsg is sent when an unstage operation completes
type unstageFinishedMsg struct {
	success       bool
	count         int
	err           error
	updatedStatus map[string]string // Updated status for unstaged files
}

// stageAllCmd stages all changes in the git repo
func stageAllCmd(items []*tree.Item) tea.Cmd {
	return func() tea.Msg {
		if len(items) == 0 {
			return stageFinishedMsg{success: false, count: 0, err: nil}
		}

		// Find git root from first item
		var gitRoot string
		for _, item := range items {
			root, err := git.GetGitRoot(filepath.Dir(item.Path))
			if err == nil && root != "" {
				gitRoot = root
				break
			}
		}

		if gitRoot == "" {
			return stageFinishedMsg{success: false, count: 0, err: nil}
		}

		// Stage all
		cmd := exec.Command("git", "add", ".")
		cmd.Dir = gitRoot
		if _, err := cmd.CombinedOutput(); err != nil {
			return stageFinishedMsg{success: false, count: 0, err: err}
		}

		return stageFinishedMsg{success: true, count: -1, err: nil} // -1 signals "all"
	}
}

// unstageAllCmd unstages all staged changes in the git repo
func unstageAllCmd(items []*tree.Item) tea.Cmd {
	return func() tea.Msg {
		if len(items) == 0 {
			return unstageFinishedMsg{success: false, count: 0, err: nil}
		}

		// Find git root from first item
		var gitRoot string
		for _, item := range items {
			root, err := git.GetGitRoot(filepath.Dir(item.Path))
			if err == nil && root != "" {
				gitRoot = root
				break
			}
		}

		if gitRoot == "" {
			return unstageFinishedMsg{success: false, count: 0, err: nil}
		}

		// Unstage all
		cmd := exec.Command("git", "reset", "HEAD")
		cmd.Dir = gitRoot
		if _, err := cmd.CombinedOutput(); err != nil {
			return unstageFinishedMsg{success: false, count: 0, err: err}
		}

		return unstageFinishedMsg{success: true, count: -1, err: nil} // -1 signals "all"
	}
}

// toggleStageFilesCmd toggles the stage status for the given files.
// If a file is staged, it unstages it. If unstaged, it stages it.
// Returns updated status map for optimistic UI update without full refresh.
func toggleStageFilesCmd(paths []string, gitFileStatus map[string]string) tea.Cmd {
	return func() tea.Msg {
		if len(paths) == 0 {
			return stageFinishedMsg{success: false, count: 0, err: nil}
		}

		// Separate paths into those to stage and those to unstage
		// Also track normalized paths for status updates
		type pathInfo struct {
			path       string
			normalized string
			oldStatus  string
		}
		var toStage, toUnstage []pathInfo

		for _, path := range paths {
			normalizedPath, err := pathutil.NormalizeForLookup(path)
			if err != nil {
				continue
			}

			status := gitFileStatus[normalizedPath]
			info := pathInfo{path: path, normalized: normalizedPath, oldStatus: status}

			if len(status) >= 2 {
				staged := status[0]
				if staged != ' ' && staged != '?' {
					toUnstage = append(toUnstage, info)
				} else {
					toStage = append(toStage, info)
				}
			} else {
				toStage = append(toStage, info)
			}
		}

		totalStaged := 0
		totalUnstaged := 0
		updatedStatus := make(map[string]string)

		// Stage files
		if len(toStage) > 0 {
			pathsByRoot := make(map[string][]pathInfo)
			for _, info := range toStage {
				dir := filepath.Dir(info.path)
				gitRoot, err := git.GetGitRoot(dir)
				if err != nil {
					continue
				}
				pathsByRoot[gitRoot] = append(pathsByRoot[gitRoot], info)
			}
			for gitRoot, infos := range pathsByRoot {
				filePaths := make([]string, len(infos))
				for i, info := range infos {
					filePaths[i] = info.path
				}
				args := append([]string{"add", "--"}, filePaths...)
				cmd := exec.Command("git", args...)
				cmd.Dir = gitRoot
				if _, err := cmd.CombinedOutput(); err != nil {
					return stageFinishedMsg{success: false, count: totalStaged, err: err}
				}
				// Update status optimistically
				for _, info := range infos {
					if info.oldStatus == "??" {
						updatedStatus[info.normalized] = "A "
					} else if len(info.oldStatus) >= 2 && info.oldStatus[1] == 'D' {
						// Unstaged delete -> staged delete
						updatedStatus[info.normalized] = "D "
					} else {
						updatedStatus[info.normalized] = "M "
					}
				}
				totalStaged += len(infos)
			}
		}

		// Unstage files
		if len(toUnstage) > 0 {
			pathsByRoot := make(map[string][]pathInfo)
			for _, info := range toUnstage {
				dir := filepath.Dir(info.path)
				gitRoot, err := git.GetGitRoot(dir)
				if err != nil {
					continue
				}
				pathsByRoot[gitRoot] = append(pathsByRoot[gitRoot], info)
			}
			for gitRoot, infos := range pathsByRoot {
				filePaths := make([]string, len(infos))
				for i, info := range infos {
					filePaths[i] = info.path
				}
				args := append([]string{"reset", "HEAD", "--"}, filePaths...)
				cmd := exec.Command("git", args...)
				cmd.Dir = gitRoot
				if _, err := cmd.CombinedOutput(); err != nil {
					return unstageFinishedMsg{success: false, count: totalUnstaged, err: err}
				}
				// Update status optimistically
				for _, info := range infos {
					if len(info.oldStatus) >= 2 && info.oldStatus[0] == 'A' {
						updatedStatus[info.normalized] = "??"
					} else if len(info.oldStatus) >= 2 && info.oldStatus[0] == 'D' {
						// Staged delete -> unstaged delete
						updatedStatus[info.normalized] = " D"
					} else {
						updatedStatus[info.normalized] = " M"
					}
				}
				totalUnstaged += len(infos)
			}
		}

		// Return appropriate message with updated status
		if totalStaged > 0 && totalUnstaged > 0 {
			return stageFinishedMsg{success: true, count: totalStaged, err: nil, updatedStatus: updatedStatus}
		} else if totalUnstaged > 0 {
			return unstageFinishedMsg{success: true, count: totalUnstaged, err: nil, updatedStatus: updatedStatus}
		}
		return stageFinishedMsg{success: true, count: totalStaged, err: nil, updatedStatus: updatedStatus}
	}
}
