package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grovetools/flow/pkg/orchestration"
	"github.com/grovetools/nb/pkg/frontmatter"
)

// PromoteOptions configures the job created by PromoteNoteToJob.
type PromoteOptions struct {
	JobType     string // e.g. "chat", "interactive_agent", "headless_agent", "oneshot"
	JobTemplate string // e.g. "chat", "" for none
}

// PromoteNoteToJob promotes a note to a job in an existing flow plan.
// Both notePath and planDir are absolute paths and may be in different workspaces.
// Returns the job filename on success.
func (s *Service) PromoteNoteToJob(notePath string, planDir string, opts PromoteOptions) (string, error) {
	// Load the target plan
	plan, err := orchestration.LoadPlan(planDir)
	if err != nil {
		return "", fmt.Errorf("loading plan %s: %w", planDir, err)
	}

	// Read note content from disk
	noteContent, err := os.ReadFile(notePath)
	if err != nil {
		return "", fmt.Errorf("reading note: %w", err)
	}

	// Parse frontmatter to get the body content
	fm, body, err := frontmatter.Parse(string(noteContent))
	if err != nil {
		// Fall back to raw content if parsing fails
		body = string(noteContent)
	}

	// Determine the note title
	noteTitle := ""
	if fm != nil {
		noteTitle = fm.Title
	}
	if noteTitle == "" {
		// Derive from filename: strip date prefix and extension
		base := strings.TrimSuffix(filepath.Base(notePath), filepath.Ext(notePath))
		noteTitle = base
	}

	// Generate a unique job ID
	jobID := fmt.Sprintf("%s-%s", time.Now().Format("20060102-150405"), sanitizeForJobID(noteTitle))

	// Move the note to in_progress/ before creating the job so note_ref
	// points to the in_progress path.
	noteDir := filepath.Dir(notePath)
	inProgressDir := filepath.Join(filepath.Dir(noteDir), "in_progress")
	if err := os.MkdirAll(inProgressDir, 0755); err != nil {
		return "", fmt.Errorf("creating in_progress directory: %w", err)
	}
	inProgressPath := filepath.Join(inProgressDir, filepath.Base(notePath))
	if err := os.Rename(notePath, inProgressPath); err != nil {
		return "", fmt.Errorf("moving note to in_progress: %w", err)
	}

	// Create the job with note_ref pointing to the in_progress location.
	// Inherit repository/branch/worktree from plan config so the job
	// doesn't fall back to CWD-based resolution in AddJob.
	jobType := orchestration.JobTypeChat
	if opts.JobType != "" {
		jobType = orchestration.JobType(opts.JobType)
	}
	jobStatus := orchestration.JobStatusPendingUser
	if jobType != orchestration.JobTypeChat {
		jobStatus = orchestration.JobStatusPending
	}
	job := &orchestration.Job{
		ID:       jobID,
		Title:    noteTitle,
		Type:     jobType,
		Status:   jobStatus,
		Template: opts.JobTemplate,
		NoteRef:  inProgressPath,
	}
	if plan.Config != nil {
		if plan.Config.Worktree != "" {
			job.Worktree = plan.Config.Worktree
		}
	}

	// Add the job to the plan (writes the job file to disk)
	jobFilename, err := orchestration.AddJob(plan, job)
	if err != nil {
		return "", fmt.Errorf("adding job to plan: %w", err)
	}

	// Append a chat template marker and reference to the linked note
	// (do NOT copy the full note body into the job)
	jobFilePath := filepath.Join(planDir, jobFilename)
	jobContent, err := os.ReadFile(jobFilePath)
	if err != nil {
		return "", fmt.Errorf("reading job file: %w", err)
	}
	updatedContent := string(jobContent) + "\n<!-- grove: {\"template\": \"chat\"} -->\n\nSee linked note: " + inProgressPath + "\n"
	if err := os.WriteFile(jobFilePath, []byte(updatedContent), 0644); err != nil {
		return "", fmt.Errorf("writing job body: %w", err)
	}

	// Update the note's frontmatter with plan_ref at its new in_progress location
	planName := filepath.Base(planDir)
	planRef := fmt.Sprintf("%s/%s", planName, jobFilename)
	if fm != nil {
		fm.PlanRef = planRef
		updatedNote := frontmatter.BuildContent(fm, body)
		if writeErr := os.WriteFile(inProgressPath, []byte(updatedNote), 0644); writeErr != nil {
			s.Logger.WithError(writeErr).Warn("Failed to update note frontmatter with plan_ref")
		}
	}

	return jobFilename, nil
}

// sanitizeForJobID creates a kebab-case slug from a string for use in job IDs.
func sanitizeForJobID(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	// Remove non-alphanumeric characters except hyphens
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result = append(result, c)
		}
	}
	// Collapse multiple hyphens
	cleaned := strings.Join(strings.FieldsFunc(string(result), func(r rune) bool { return r == '-' }), "-")
	if len(cleaned) > 50 {
		cleaned = cleaned[:50]
		cleaned = strings.TrimRight(cleaned, "-")
	}
	return cleaned
}
