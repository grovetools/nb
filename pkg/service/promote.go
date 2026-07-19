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
	Force       bool     // Promote even when the note's plan_ref already points at a live plan
	Strict      bool     // Treat a worktree-resolution miss as a hard failure instead of a warning
}

// PromoteResult records one note→job promotion within a batch.
type PromoteResult struct {
	NotePath    string `json:"note"`
	JobFilename string `json:"job_file"`
	// WorktreeMissing is true when the plan declared a worktree that could not
	// be resolved under any worktree base, so the job was created without a
	// repository/branch. Surfaced to stdout and JSON so the miss isn't silent.
	WorktreeMissing bool   `json:"worktree_missing,omitempty"`
	Worktree        string `json:"worktree,omitempty"`
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
// Returns a PromoteResult describing the created job on success.
func (s *Service) PromoteNoteToJob(notePath string, planDir string, opts PromoteOptions) (PromoteResult, error) {
	// Load the target plan
	plan, err := orchestration.LoadPlan(planDir)
	if err != nil {
		return PromoteResult{NotePath: notePath}, fmt.Errorf("loading plan %s: %w", planDir, err)
	}

	// Validate declared dependencies against the plan before any mutation,
	// matching flow plan add semantics (deps are job filenames).
	if err := validateJobDependencies(plan, opts.DependsOn); err != nil {
		return PromoteResult{NotePath: notePath}, err
	}

	fm, body, noteTitle, err := s.parseNoteForPromotion(notePath)
	if err != nil {
		return PromoteResult{NotePath: notePath}, err
	}

	// The note's stable frontmatter id is the provenance anchor: it survives
	// path moves, so both note_ref and the promoted-from trailer prefer it and
	// fall back to the (mutable) in_progress path only when the note has no id.
	noteID := ""
	if fm != nil {
		noteID = fm.ID
	}

	// Generate a unique job ID
	jobID := fmt.Sprintf("%s-%s", time.Now().Format("20060102-150405"), sanitizeForJobID(noteTitle))

	// Resolve the plan's worktree to a repository/branch BEFORE moving the note,
	// so a --strict miss can hard-fail without orphaning the note in in_progress.
	worktree := ""
	if plan.Config != nil {
		worktree = plan.Config.Worktree
	}
	var repository, branch string
	worktreeMissing := false
	if worktree != "" {
		// Find the ecosystem root and look up the worktree path
		if node, err := workspace.GetProjectByPath("."); err == nil && node != nil {
			var ecoRoot string
			if node.RootEcosystemPath != "" {
				ecoRoot = node.RootEcosystemPath
			} else {
				ecoRoot = node.Path
			}
			if wtPath, ok := workspace.ResolveWorktreePathByName(ecoRoot, worktree, nil); ok {
				if repo, br, _ := git.GetRepoInfo(wtPath); repo != "" {
					repository = repo
					branch = br
				}
			} else {
				worktreeMissing = true
				if opts.Strict {
					return PromoteResult{NotePath: notePath, WorktreeMissing: true, Worktree: worktree},
						fmt.Errorf("worktree %q not found under any worktree base for ecosystem %s; refusing to promote a repo/branch-less job (--strict)", worktree, ecoRoot)
				}
				s.Logger.WithFields(logrus.Fields{
					"worktree":  worktree,
					"ecosystem": ecoRoot,
				}).Warn("Worktree not found under any worktree base; promoted job will lack repository/branch")
			}
		}
	}

	// Move the note to in_progress/ before creating the job so the in_progress
	// path (the note_ref/trailer fallback) is stable.
	noteDir := filepath.Dir(notePath)
	inProgressDir := filepath.Join(filepath.Dir(noteDir), "in_progress")
	if err := os.MkdirAll(inProgressDir, 0o755); err != nil {
		return PromoteResult{NotePath: notePath}, fmt.Errorf("creating in_progress directory: %w", err)
	}
	inProgressPath := filepath.Join(inProgressDir, filepath.Base(notePath))
	if err := os.Rename(notePath, inProgressPath); err != nil {
		return PromoteResult{NotePath: notePath}, fmt.Errorf("moving note to in_progress: %w", err)
	}

	// note_ref is a provenance hint only (flow no longer resolves it): prefer the
	// note's stable frontmatter id, falling back to the in_progress path.
	noteRef := inProgressPath
	if noteID != "" {
		noteRef = noteID
	}

	// Create the job. Inherit repository/branch/worktree resolved above so the
	// job doesn't fall back to CWD-based resolution in AddJob.
	jobType := orchestration.JobTypeChat
	if opts.JobType != "" {
		jobType = orchestration.JobType(opts.JobType)
	}
	jobStatus := orchestration.JobStatusPendingUser
	if jobType != orchestration.JobTypeChat {
		jobStatus = orchestration.JobStatusPending
	}
	job := &orchestration.Job{
		ID:         jobID,
		Title:      noteTitle,
		Type:       jobType,
		Status:     jobStatus,
		Template:   opts.JobTemplate,
		Model:      opts.Model,
		Effort:     opts.Effort,
		Skill:      opts.Skill,
		DependsOn:  opts.DependsOn,
		NoteRef:    noteRef,
		Worktree:   worktree,
		Repository: repository,
		Branch:     branch,
	}

	// Add the job to the plan (writes the job file to disk)
	jobFilename, err := orchestration.AddJob(plan, job)
	if err != nil {
		return PromoteResult{NotePath: notePath}, fmt.Errorf("adding job to plan: %w", err)
	}

	// Append the note body to the job file so chat models can read it
	// directly. Also include a reference link for provenance.
	jobFilePath := filepath.Join(planDir, jobFilename)
	jobContent, err := os.ReadFile(jobFilePath)
	if err != nil {
		return PromoteResult{NotePath: notePath, JobFilename: jobFilename}, fmt.Errorf("reading job file: %w", err)
	}
	// The job template already includes a <!-- grove: {"template": "chat"} -->
	// marker, so we just append the note body and reference below it. Reference
	// the note's stable id when present; fall back to the path otherwise.
	trailer := "_Promoted from: " + inProgressPath + "_"
	if noteID != "" {
		trailer = "_Promoted from note: " + noteID + "_"
	}
	updatedContent := string(jobContent) + "\n" + strings.TrimSpace(body) + "\n\n" + trailer + "\n"
	if err := os.WriteFile(jobFilePath, []byte(updatedContent), 0o644); err != nil {
		return PromoteResult{NotePath: notePath, JobFilename: jobFilename}, fmt.Errorf("writing job body: %w", err)
	}

	// Update the note's frontmatter so NOTE frontmatter is the single stored
	// truth for the note↔plan link: plan_ref is the human-legible plan slug
	// (plans/<planName>, the form the TUI join matches on) and plan_job is the
	// promoted job's filename for per-job linkage.
	planName := filepath.Base(planDir)
	if fm != nil {
		fm.PlanRef = fmt.Sprintf("plans/%s", planName)
		fm.PlanJob = jobFilename
		updatedNote := frontmatter.BuildContent(fm, body)
		if writeErr := os.WriteFile(inProgressPath, []byte(updatedNote), 0o644); writeErr != nil {
			s.Logger.WithError(writeErr).Warn("Failed to update note frontmatter with plan_ref/plan_job")
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

	return PromoteResult{
		NotePath:        notePath,
		JobFilename:     jobFilename,
		WorktreeMissing: worktreeMissing,
		Worktree:        worktree,
	}, nil
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

	// Preflight: every note exists, is a regular file, and appears once. Also
	// enforce the idempotency guard: a note whose existing plan_ref points at a
	// live plan directory is refused unless --force, so a re-run doesn't
	// silently double-promote an already-linked note.
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
		if !opts.Force {
			if livePlanDir := s.existingLivePlanForNote(p); livePlanDir != "" {
				return nil, fmt.Errorf("note %s is already linked to live plan %q (plan_ref); pass --force to re-promote", p, filepath.Base(livePlanDir))
			}
		}
	}

	results := make([]PromoteResult, 0, len(notePaths))
	for i, p := range notePaths {
		res, err := s.PromoteNoteToJob(p, planDir, opts)
		if err != nil {
			return results, fmt.Errorf("promoted %d of %d notes; %s failed: %w", i, len(notePaths), p, err)
		}
		results = append(results, res)
	}
	return results, nil
}

// existingLivePlanForNote returns the absolute directory of the plan the note
// is already linked to via its plan_ref frontmatter, but only when that plan
// directory actually exists on disk (a "live" plan). It returns "" when the
// note has no plan_ref, is unreadable/unparseable, or the referenced plan
// directory is absent — i.e. re-promotion is safe.
//
// plan_ref is resolved relative to the notebook content root (the parent of the
// note's containing type directory, e.g. current/ or inbox/). It tolerates both
// the current slug form (plans/<name>) and the legacy <name>/<job>.md form.
func (s *Service) existingLivePlanForNote(notePath string) string {
	content, err := os.ReadFile(notePath)
	if err != nil {
		return ""
	}
	fm, _, err := frontmatter.Parse(string(content))
	if err != nil || fm == nil || fm.PlanRef == "" {
		return ""
	}

	ref := fm.PlanRef
	// Legacy form carried a trailing job filename; strip it to the plan slug.
	if strings.HasSuffix(ref, ".md") {
		ref = filepath.Dir(ref)
	}
	if ref == "" || ref == "." {
		return ""
	}

	contentRoot := filepath.Dir(filepath.Dir(notePath))
	candidates := []string{filepath.Join(contentRoot, ref)}
	if !strings.HasPrefix(ref, "plans/") && !strings.HasPrefix(ref, "plans"+string(filepath.Separator)) {
		candidates = append(candidates, filepath.Join(contentRoot, "plans", ref))
	}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return ""
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

// stripFrontmatterBlock removes a leading frontmatter block from content,
// returning only the body. It is the fallback for parseNoteForPromotion when
// YAML parsing fails, and its one job is to avoid double-frontmatter in the
// promoted job without ever eating body content.
//
// It ONLY strips a block anchored at byte 0 of the file: the content must begin
// with the "---" fence, and stripping stops at the FIRST closing "---" line
// (the frontmatter's own closer). A "---" fence that appears later in the body
// (e.g. an embedded ```-fenced YAML/code block, or a thematic break) is never
// treated as a frontmatter delimiter. If the content does not start with "---",
// or no closing fence is found, the original content is returned unchanged.
func stripFrontmatterBlock(content string) string {
	// Byte-0 anchor: never strip a "---" fence that starts later in the body.
	if !strings.HasPrefix(content, "---") {
		return content
	}

	// Find the closing "---" delimiter
	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return content
	}

	// Skip the opening "---" and stop at the first closing fence (the
	// frontmatter's own closer), leaving all subsequent body fences intact.
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
