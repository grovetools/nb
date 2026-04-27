package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/grovetools/core/git"
)

// GitCommit runs `git commit -m <message>` in the given repo. Only what's already
// staged is committed (no auto-staging). Returns ErrNothingToCommit if there's
// nothing staged.
func (s *Service) GitCommit(repoPath, message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "nothing to commit") {
			return ErrNothingToCommit
		}
		return fmt.Errorf("git commit failed: %w\n%s", err, string(output))
	}
	return nil
}

// ErrNothingToCommit is returned by GitCommit when the index is clean.
var ErrNothingToCommit = fmt.Errorf("nothing to commit")

// GitStageAll runs `git add .` in the given repo.
func (s *Service) GitStageAll(repoPath string) error {
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %w\n%s", err, string(output))
	}
	return nil
}

// GitUnstageAll runs `git reset HEAD` in the given repo.
func (s *Service) GitUnstageAll(repoPath string) error {
	cmd := exec.Command("git", "reset", "HEAD")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git reset failed: %w\n%s", err, string(output))
	}
	return nil
}

// GitStagePaths runs `git add -- <paths>...` in the given repo.
func (s *Service) GitStagePaths(repoPath string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "--"}, paths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %w\n%s", err, string(output))
	}
	return nil
}

// GitUnstagePaths runs `git reset HEAD -- <paths>...` in the given repo.
func (s *Service) GitUnstagePaths(repoPath string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"reset", "HEAD", "--"}, paths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git reset failed: %w\n%s", err, string(output))
	}
	return nil
}

// FindGitRoot returns the git repository root containing the given file path,
// or the empty string if the path is not inside a git repository.
func (s *Service) FindGitRoot(path string) (string, error) {
	return git.GetGitRoot(path)
}

// ArchivePlanDirectory moves a plan directory under "plans/<planName>" to
// "plans/.archive/<planName>" within the workspace's plans directory and
// returns the absolute paths of all note files that lived inside it.
//
// planGroup is expected to be in the form "plans/<planName>".
func (s *Service) ArchivePlanDirectory(ctx *WorkspaceContext, planGroup string) ([]string, error) {
	plansBaseDir, err := s.GetNotebookLocator().GetPlansDir(ctx.NotebookContextWorkspace)
	if err != nil {
		return nil, fmt.Errorf("get plans directory: %w", err)
	}

	planName := strings.TrimPrefix(planGroup, "plans/")
	sourcePath := filepath.Join(plansBaseDir, planName)

	// Collect note paths inside the plan directory before we move it.
	var notePaths []string
	if err := filepath.Walk(sourcePath, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		notePaths = append(notePaths, p)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk plan directory: %w", err)
	}

	archiveDir := filepath.Join(plansBaseDir, ".archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return nil, fmt.Errorf("create archive directory: %w", err)
	}

	destPath := filepath.Join(archiveDir, planName)
	if _, err := os.Stat(destPath); err == nil {
		// Destination exists, create unique name with timestamp.
		timestamp := time.Now().Format("20060102150405")
		destPath = filepath.Join(archiveDir, fmt.Sprintf("%s-%s", planName, timestamp))
	}

	if err := os.Rename(sourcePath, destPath); err != nil {
		return nil, fmt.Errorf("move plan directory: %w", err)
	}

	return notePaths, nil
}
