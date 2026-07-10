package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grovetools/core/git"
	coremodels "github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/flow/pkg/orchestration"
	"github.com/sirupsen/logrus"

	"github.com/grovetools/nb/pkg/frontmatter"
)

// PromoteOptions configures the job created by PromoteNoteToJob.
type PromoteOptions struct {
	JobType     string   // e.g. "chat", "interactive_agent", "headless_agent", "oneshot"
	JobTemplate string   // e.g. "chat", "" for none
	Model       string   // LLM model to use for this job (optional)
	Effort      string   // Effort level for claude agent jobs (optional)
	Skill       string   // Skill name to inject into the agent context, written to job frontmatter (optional)
	DependsOn   []string // Job filenames in the target plan the promoted job depends on (optional)
}

// PromoteResult records one note→job promotion within a batch.
type PromoteResult struct {
	NotePath    string `json:"note"`
	JobFilename string `json:"job_file"`
}

// PromotePreview describes what promoting a note WOULD create, without writing.
type PromotePreview struct {
	NotePath         string   `json:"note"`
	Title            string   `json:"title"`
	JobType          string   `json:"job_type"`
	Model            string   `json:"model,omitempty"`
	Skill            string   `json:"skill,omitempty"`
	DependsOn        []string `json:"depends_on,omitempty"`
	PredictedJobFile string   `json:"predicted_job_file"`
	InProgressPath   string   `json:"in_progress_path"`
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

	// Validate declared dependencies against the plan before any mutation,
	// matching flow plan add semantics (deps are job filenames).
	if err := validateJobDependencies(plan, opts.DependsOn); err != nil {
		return "", err
	}

	fm, body, noteTitle, err := s.parseNoteForPromotion(notePath)
	if err != nil {
		return "", err
	}

	// Generate a unique job ID
	jobID := fmt.Sprintf("%s-%s", time.Now().Format("20060102-150405"), sanitizeForJobID(noteTitle))

	// Move the note to in_progress/ before creating the job so note_ref
	// points to the in_progress path.
	noteDir := filepath.Dir(notePath)
	inProgressDir := filepath.Join(filepath.Dir(noteDir), "in_progress")
	if err := os.MkdirAll(inProgressDir, 0o755); err != nil {
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
		ID:        jobID,
		Title:     noteTitle,
		Type:      jobType,
		Status:    jobStatus,
		Template:  opts.JobTemplate,
		Model:     opts.Model,
		Effort:    opts.Effort,
		Skill:     opts.Skill,
		DependsOn: opts.DependsOn,
		NoteRef:   inProgressPath,
	}
	if plan.Config != nil {
		if plan.Config.Worktree != "" {
			job.Worktree = plan.Config.Worktree
		}
	}
	// Resolve repo/branch from the worktree (a git repo) rather than the
	// plan directory (which lives in the notebook filesystem, not in git).
	// This ensures promoted jobs get "grovetools" not "nb".
	if job.Worktree != "" {
		// Find the ecosystem root and look up the worktree path
		if node, err := workspace.GetProjectByPath("."); err == nil && node != nil {
			var ecoRoot string
			if node.RootEcosystemPath != "" {
				ecoRoot = node.RootEcosystemPath
			} else {
				ecoRoot = node.Path
			}
			if wtPath, ok := workspace.ResolveWorktreePathByName(ecoRoot, job.Worktree, nil); ok {
				if repo, branch, _ := git.GetRepoInfo(wtPath); repo != "" {
					job.Repository = repo
					job.Branch = branch
				}
			} else {
				s.Logger.WithFields(logrus.Fields{
					"worktree":  job.Worktree,
					"ecosystem": ecoRoot,
				}).Warn("Worktree not found under any worktree base; promoted job will lack repository/branch")
			}
		}
	}

	// Add the job to the plan (writes the job file to disk)
	jobFilename, err := orchestration.AddJob(plan, job)
	if err != nil {
		return "", fmt.Errorf("adding job to plan: %w", err)
	}

	// Append the note body to the job file so chat models can read it
	// directly. Also include a reference link for provenance.
	jobFilePath := filepath.Join(planDir, jobFilename)
	jobContent, err := os.ReadFile(jobFilePath)
	if err != nil {
		return "", fmt.Errorf("reading job file: %w", err)
	}
	// The job template already includes a <!-- grove: {"template": "chat"} -->
	// marker, so we just append the note body and reference below it.
	updatedContent := string(jobContent) + "\n" + strings.TrimSpace(body) + "\n\n_Promoted from: " + inProgressPath + "_\n"
	if err := os.WriteFile(jobFilePath, []byte(updatedContent), 0o644); err != nil {
		return "", fmt.Errorf("writing job body: %w", err)
	}

	// Update the note's frontmatter with plan_ref at its new in_progress location
	planName := filepath.Base(planDir)
	planRef := fmt.Sprintf("%s/%s", planName, jobFilename)
	if fm != nil {
		fm.PlanRef = planRef
		updatedNote := frontmatter.BuildContent(fm, body)
		if writeErr := os.WriteFile(inProgressPath, []byte(updatedNote), 0o644); writeErr != nil {
			s.Logger.WithError(writeErr).Warn("Failed to update note frontmatter with plan_ref")
		}
	}

	// Typed move event for the inbox -> in_progress promotion. PrevPath is the
	// rename-detection linchpin: the daemon's sync handler turns this into a
	// first-class document_moved instead of a delete+create pair. Emitted after
	// the frontmatter rewrite so the daemon indexes the final content (plan_ref).
	ws, _, noteType := GetNoteMetadata(inProgressPath)
	prevWs, _, prevType := GetNoteMetadata(notePath)
	EmitNoteEvent(coremodels.NoteEvent{
		Event:         coremodels.NoteEventMoved,
		Workspace:     ws,
		NoteType:      noteType,
		Path:          inProgressPath,
		PrevWorkspace: prevWs,
		PrevNoteType:  prevType,
		PrevPath:      notePath,
	})

	return jobFilename, nil
}

// PromoteNotesToJobs promotes a batch of notes into one plan (a triage
// roster). All inputs are preflighted before the first note moves: the plan
// must load, dependencies must exist, and every note must be a readable file —
// so validation failures cost nothing. A mid-batch I/O failure stops the batch
// and returns the results already promoted alongside the error.
func (s *Service) PromoteNotesToJobs(notePaths []string, planDir string, opts PromoteOptions) ([]PromoteResult, error) {
	if len(notePaths) == 0 {
		return nil, fmt.Errorf("no notes to promote")
	}

	// Preflight: plan + deps
	plan, err := orchestration.LoadPlan(planDir)
	if err != nil {
		return nil, fmt.Errorf("loading plan %s: %w", planDir, err)
	}
	if err := validateJobDependencies(plan, opts.DependsOn); err != nil {
		return nil, err
	}

	// Preflight: every note exists, is a regular file, and appears once
	seen := make(map[string]struct{}, len(notePaths))
	for _, p := range notePaths {
		if _, dup := seen[p]; dup {
			return nil, fmt.Errorf("note listed twice: %s", p)
		}
		seen[p] = struct{}{}
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("note not found: %s: %w", p, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("note is a directory: %s", p)
		}
	}

	results := make([]PromoteResult, 0, len(notePaths))
	for i, p := range notePaths {
		jobFilename, err := s.PromoteNoteToJob(p, planDir, opts)
		if err != nil {
			return results, fmt.Errorf("promoted %d of %d notes; %s failed: %w", i, len(notePaths), p, err)
		}
		results = append(results, PromoteResult{NotePath: p, JobFilename: jobFilename})
	}
	return results, nil
}

// PreviewPromoteNotes computes what PromoteNotesToJobs would create, without
// writing or moving anything. Predicted job filenames assume the batch runs
// against the plan in its current state.
func (s *Service) PreviewPromoteNotes(notePaths []string, planDir string, opts PromoteOptions) ([]PromotePreview, error) {
	plan, err := orchestration.LoadPlan(planDir)
	if err != nil {
		return nil, fmt.Errorf("loading plan %s: %w", planDir, err)
	}
	if err := validateJobDependencies(plan, opts.DependsOn); err != nil {
		return nil, err
	}

	nextNum, err := orchestration.GetNextJobNumber(plan.Directory)
	if err != nil {
		return nil, fmt.Errorf("getting next job number: %w", err)
	}

	jobType := string(orchestration.JobTypeChat)
	if opts.JobType != "" {
		jobType = opts.JobType
	}

	previews := make([]PromotePreview, 0, len(notePaths))
	for i, p := range notePaths {
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("note not found: %s: %w", p, err)
		}
		_, _, title, err := s.parseNoteForPromotion(p)
		if err != nil {
			return nil, err
		}
		previews = append(previews, PromotePreview{
			NotePath:         p,
			Title:            title,
			JobType:          jobType,
			Model:            opts.Model,
			Skill:            opts.Skill,
			DependsOn:        opts.DependsOn,
			PredictedJobFile: orchestration.GenerateJobFilename(nextNum+i, title),
			InProgressPath:   filepath.Join(filepath.Dir(filepath.Dir(p)), "in_progress", filepath.Base(p)),
		})
	}
	return previews, nil
}

// parseNoteForPromotion reads a note and returns its frontmatter (nil when
// unparseable), body, and resolved title — the shared front half of promote
// and its dry-run preview.
func (s *Service) parseNoteForPromotion(notePath string) (fm *frontmatter.Frontmatter, body string, title string, err error) {
	noteContent, err := os.ReadFile(notePath)
	if err != nil {
		return nil, "", "", fmt.Errorf("reading note: %w", err)
	}

	fm, body, parseErr := frontmatter.Parse(string(noteContent))
	if parseErr != nil {
		// On parse failure, strip the frontmatter block textually to avoid double-frontmatter.
		// Log a warning for visibility.
		s.Logger.WithError(parseErr).Warnf("Failed to parse frontmatter in note %s, using fallback body extraction", notePath)
		fm = nil
		body = stripFrontmatterBlock(string(noteContent))
	}

	if fm != nil {
		title = fm.Title
	}
	if title == "" {
		// Derive from filename: strip extension
		title = strings.TrimSuffix(filepath.Base(notePath), filepath.Ext(notePath))
	}
	return fm, body, title, nil
}

// validateJobDependencies checks each declared dependency is an existing job
// filename in the plan, matching flow plan add's validation.
func validateJobDependencies(plan *orchestration.Plan, deps []string) error {
	for _, dep := range deps {
		found := false
		for _, j := range plan.Jobs {
			if j.Filename == dep {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("dependency not found in plan: %s", dep)
		}
	}
	return nil
}

// stripFrontmatterBlock removes the frontmatter block from content, returning only the body.
// If the file starts with "---", it strips everything up to and including the closing "---".
// If no frontmatter block is found, returns the original content.
func stripFrontmatterBlock(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}

	// Find the closing "---" delimiter
	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return content
	}

	// Skip the opening "---"
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			// Found the closing delimiter; return everything after it
			return strings.Join(lines[i+1:], "\n")
		}
	}

	// No closing delimiter found, return original content
	return content
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
