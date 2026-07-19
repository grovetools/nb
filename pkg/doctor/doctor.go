// Package doctor implements nb's link reconciler: it audits the note↔plan
// linkage recorded in note frontmatter (plan_ref / plan_job) against the plans
// actually present on disk, and — in --fix mode — heals the drift.
//
// The engine operates purely over a workspace directory with the standard
// lifecycle layout (inbox/ in_progress/ review/ completed/ and plans/, with
// finished plans under plans/.archive/). It is deliberately decoupled from the
// nb service/cobra layers so it can be exercised against tempdir fixtures.
package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	coremodels "github.com/grovetools/core/pkg/models"
	"gopkg.in/yaml.v3"

	"github.com/grovetools/nb/pkg/frontmatter"
	"github.com/grovetools/nb/pkg/service"
)

// emitEvent is the funnel doctor uses to record note moves as lifecycle events.
// It is a package var so tests can swap in a no-op and avoid the fire-and-forget
// daemon notifier's autostart behaviour.
var emitEvent = service.EmitNoteEvent

// Classification is the diagnosis assigned to a note (or, for the UNCLAIMED
// value, a plan job) by the reconciler.
type Classification string

const (
	// Live: plan_ref points at a plan directory that exists under plans/.
	Live Classification = "LIVE"
	// Archived: plan_ref points at a plan under plans/.archive/.
	Archived Classification = "ARCHIVED"
	// Gone: plan_ref points at a plan that exists nowhere.
	Gone Classification = "GONE"
	// MalformedLegacy: plan_ref uses the old <plan>/<job>.md form (a job
	// filename baked into the ref instead of the plans/<name> + plan_job pair).
	MalformedLegacy Classification = "MALFORMED-LEGACY"
	// Unlinked: an in_progress/review note with no plan_ref at all.
	Unlinked Classification = "UNLINKED"
	// Unclaimed is the forward-direction diagnosis for a plan job whose note_ref
	// hint no note claims.
	Unclaimed Classification = "UNCLAIMED"
)

// NoteEntry is one note's diagnosis (and, under --fix, the action applied).
type NoteEntry struct {
	Path           string         `json:"path"`
	Workspace      string         `json:"workspace"`
	Classification Classification `json:"classification"`
	Plan           string         `json:"plan,omitempty"`
	PlanJob        string         `json:"plan_job,omitempty"`
	ActionTaken    string         `json:"action_taken,omitempty"`

	problem  bool
	resolved bool
}

// Repair names the forward-direction remedy planned (read-only) or applied
// (--fix) for an UNCLAIMED job.
type Repair string

const (
	// RepairRelink: the note_ref hint resolved to exactly one assignable note.
	// The job's note_ref is rewritten to that note's stable id and the note's
	// plan_ref/plan_job are pointed back at this job.
	RepairRelink Repair = "relink"
	// RepairClear: the hint resolves to nothing assignable — the referenced note
	// is gone, or is already owned by another job. The dead hint is cleared.
	RepairClear Repair = "clear"
	// RepairAmbiguous: several notes match the hint. Never guessed; left alone
	// and reported for a human to disambiguate.
	RepairAmbiguous Repair = "ambiguous"
	// RepairManual: the hint matched a note whose frontmatter cannot be parsed
	// (e.g. an unquoted colon in the title). The note plainly exists, so the
	// hint is not dead and must not be cleared — but it cannot be rewritten
	// safely either. Reported for a human to repair the note itself.
	RepairManual Repair = "manual"
)

// JobEntry is one UNCLAIMED plan job's diagnosis, the repair planned for it,
// and (under --fix) the action applied.
type JobEntry struct {
	Plan           string         `json:"plan"`
	JobFile        string         `json:"job_file"`
	NoteRef        string         `json:"note_ref"`
	Classification Classification `json:"classification"`
	Repair         Repair         `json:"repair"`
	ResolvedNote   string         `json:"resolved_note,omitempty"`
	Candidates     []string       `json:"candidates,omitempty"`
	ActionTaken    string         `json:"action_taken,omitempty"`

	resolved bool
	jobPath  string // absolute path to the job file
	noteID   string // stable frontmatter id of the resolved note (may be empty)
}

// ProposedFix is the human-readable repair the reconciler would apply in --fix
// mode. It is what the report shows in the ACTION column of a read-only run.
func (j *JobEntry) ProposedFix() string {
	switch j.Repair {
	case RepairRelink:
		return fmt.Sprintf("would relink to %s", filepath.Base(j.ResolvedNote))
	case RepairClear:
		return "would clear dead note_ref"
	case RepairAmbiguous:
		return fmt.Sprintf("ambiguous: %d candidates (manual)", len(j.Candidates))
	case RepairManual:
		return fmt.Sprintf("note %s has unreadable frontmatter (manual)", filepath.Base(j.ResolvedNote))
	}
	return ""
}

// Report is the full result of a reconciler run.
type Report struct {
	Workspace     string         `json:"workspace"`
	Fixed         bool           `json:"fixed"`
	Notes         []NoteEntry    `json:"notes"`
	UnclaimedJobs []JobEntry     `json:"unclaimed_jobs"`
	Summary       map[string]int `json:"summary"`
}

// ProblemsRemaining counts diagnoses that are still problems after the run
// (every problem when read-only; only the unresolved ones after --fix).
func (r *Report) ProblemsRemaining() int {
	n := 0
	for _, e := range r.Notes {
		if e.problem && !e.resolved {
			n++
		}
	}
	for _, j := range r.UnclaimedJobs {
		if !j.resolved {
			n++
		}
	}
	return n
}

var jobFileRe = regexp.MustCompile(`^\d+-.*\.md$`)

// noteState is the pre-fix analysis of one scanned note.
type noteState struct {
	entry      *NoteEntry
	dir        string // lifecycle dir the note lives in (in_progress/review)
	fm         *frontmatter.Frontmatter
	body       string
	planName   string // parsed plan name (from plan_ref)
	planJob    string // parsed/authoritative job filename
	malformed  bool
	underlying Classification // plan state behind a malformed ref
}

// Run reconciles the note↔plan links for the workspace rooted at workspaceDir.
// workspaceName labels the entries in the report. When fix is true the healing
// actions are applied; otherwise the run is read-only.
func Run(workspaceDir, workspaceName string, fix bool) (*Report, error) {
	plansDir := filepath.Join(workspaceDir, "plans")
	inProgressDir := filepath.Join(workspaceDir, "in_progress")
	reviewDir := filepath.Join(workspaceDir, "review")
	completedDir := filepath.Join(workspaceDir, "completed")
	inboxDir := filepath.Join(workspaceDir, "inbox")

	report := &Report{
		Workspace: workspaceName,
		Fixed:     fix,
		Summary:   map[string]int{},
	}

	classifyPlan := func(planName string) Classification {
		if planName == "" {
			return Gone
		}
		if isDir(filepath.Join(plansDir, planName)) {
			return Live
		}
		if isDir(filepath.Join(plansDir, ".archive", planName)) {
			return Archived
		}
		return Gone
	}

	// --- Pass 1: scan notes in in_progress/ and review/ ---
	var states []*noteState
	for _, dir := range []string{inProgressDir, reviewDir} {
		files, err := listMarkdown(dir)
		if err != nil {
			return nil, err
		}
		for _, path := range files {
			st, err := analyzeNote(path, dir, classifyPlan)
			if err != nil {
				return nil, err
			}
			states = append(states, st)
		}
	}

	// --- Pass 2: forward scan — live plan jobs whose note_ref no note claims ---
	// A job is claimed by ANY note in the workspace (not just in_progress/review):
	// a note backfilled into inbox on a prior --fix run must still count, so the
	// same job isn't flagged UNCLAIMED forever.
	claims := scanClaims(workspaceDir)
	noteIdx := buildNoteIndex(workspaceDir)
	backfillTargets := map[string]struct{}{} // abs note path -> will be healed by a job relink
	jobEntries := scanUnclaimedJobs(plansDir, workspaceDir, claims, noteIdx, backfillTargets)

	// --- Pass 3: finalize note dispositions and apply fixes ---
	for _, st := range states {
		e := st.entry
		switch e.Classification {
		case Unlinked:
			e.problem = true
			if _, isTarget := backfillTargets[e.Path]; isTarget {
				// A pre-inversion note living in in_progress that a job's
				// note_ref points back at: leave it in place; the forward
				// backfill re-establishes its plan_ref/plan_job.
				continue
			}
			if fix {
				if _, err := moveNote(e.Path, inboxDir); err != nil {
					return nil, err
				}
				e.ActionTaken = "moved to inbox/"
				e.resolved = true
			}
		case MalformedLegacy:
			e.problem = true
			if fix {
				switch st.underlying {
				case Live, Archived:
					if err := rewriteLink(st, "plans/"+st.planName, st.planJob); err != nil {
						return nil, err
					}
					e.ActionTaken = fmt.Sprintf("rewrote plan_ref -> plans/%s, plan_job -> %s", st.planName, st.planJob)
					if st.underlying == Archived {
						if _, err := moveNote(e.Path, completedDir); err != nil {
							return nil, err
						}
						e.ActionTaken += "; moved to completed/"
					}
				default: // Gone
					if _, err := moveNote(e.Path, completedDir); err != nil {
						return nil, err
					}
					e.ActionTaken = "plan gone; moved to completed/"
				}
				e.resolved = true
			}
		case Archived:
			e.problem = true
			if fix {
				if _, err := moveNote(e.Path, completedDir); err != nil {
					return nil, err
				}
				e.ActionTaken = "plan archived; moved to completed/"
				e.resolved = true
			}
		case Gone:
			e.problem = true
			if fix {
				if _, err := moveNote(e.Path, completedDir); err != nil {
					return nil, err
				}
				e.ActionTaken = "plan gone; moved to completed/"
				e.resolved = true
			}
		case Live:
			// Healthy, well-formed link: nothing to do.
		}
	}

	// Apply the forward-direction repairs planned during the job scan.
	if fix {
		for i := range jobEntries {
			j := &jobEntries[i]
			switch j.Repair {
			case RepairRelink:
				if err := backfillNoteLink(j.ResolvedNote, "plans/"+j.Plan, j.JobFile); err != nil {
					return nil, err
				}
				// Point the job's provenance hint at the note's stable id. If
				// the note has no id there is no stable ref to write, so the
				// path hint is cleared instead — the note-side link (the source
				// of truth) is what keeps the job claimed from here on.
				if err := setJobNoteRef(j.jobPath, j.noteID); err != nil {
					return nil, err
				}
				if j.noteID != "" {
					j.ActionTaken = fmt.Sprintf("relinked note %s (plan_ref=plans/%s, plan_job=%s); note_ref -> %s",
						j.ResolvedNote, j.Plan, j.JobFile, j.noteID)
				} else {
					j.ActionTaken = fmt.Sprintf("relinked note %s (plan_ref=plans/%s, plan_job=%s); note_ref cleared (note has no id)",
						j.ResolvedNote, j.Plan, j.JobFile)
				}
				j.resolved = true
				// Reflect the heal on the note-side entry when the note was
				// scanned (a relink target in in_progress/review left in place).
				for _, st := range states {
					if st.entry.Path == j.ResolvedNote {
						st.entry.ActionTaken = fmt.Sprintf("linked to plans/%s job %s via job note_ref relink", j.Plan, j.JobFile)
						st.entry.resolved = true
					}
				}
			case RepairClear:
				if err := setJobNoteRef(j.jobPath, ""); err != nil {
					return nil, err
				}
				j.ActionTaken = "cleared dead note_ref"
				j.resolved = true
			case RepairAmbiguous, RepairManual:
				// Never guessed / never rewritten: reported for a human.
			}
		}
	}

	for _, st := range states {
		report.Notes = append(report.Notes, *st.entry)
	}
	report.UnclaimedJobs = jobEntries

	sortReport(report)
	fillSummary(report)
	return report, nil
}

// analyzeNote reads a note and produces its pre-fix diagnosis.
func analyzeNote(path, dir string, classifyPlan func(string) Classification) (*noteState, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading note %s: %w", path, err)
	}
	fm, body, _ := frontmatter.Parse(string(content))
	ws, _, _ := service.GetNoteMetadata(path)

	st := &noteState{
		dir:  dir,
		fm:   fm,
		body: body,
		entry: &NoteEntry{
			Path:      path,
			Workspace: ws,
		},
	}

	planRef := ""
	if fm != nil {
		planRef = fm.PlanRef
	}
	if planRef == "" {
		st.entry.Classification = Unlinked
		return st, nil
	}

	planName, jobFile, malformed := parsePlanRef(planRef)
	st.planName = planName
	st.malformed = malformed
	if malformed {
		st.planJob = jobFile
		st.underlying = classifyPlan(planName)
		st.entry.Classification = MalformedLegacy
		st.entry.Plan = planName
		st.entry.PlanJob = jobFile
		return st, nil
	}

	// Well-formed plan_ref: plan_job is the separate frontmatter field.
	if fm != nil {
		st.planJob = fm.PlanJob
	}
	st.entry.Classification = classifyPlan(planName)
	st.entry.Plan = planName
	st.entry.PlanJob = st.planJob
	return st, nil
}

// parsePlanRef splits a plan_ref value into a plan name and (for the legacy
// form) a job filename. malformed is true for the old <plan>/<job>.md shape,
// where a job filename is baked into the ref.
func parsePlanRef(ref string) (planName, jobFile string, malformed bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", false
	}
	if strings.HasSuffix(ref, ".md") {
		// Legacy: <plan>/<job>.md or plans/<plan>/<job>.md
		jobFile = filepath.Base(ref)
		planDir := filepath.Dir(ref) // "<plan>", "plans/<plan>", or "."
		if planDir == "." || planDir == "" {
			return "", jobFile, true
		}
		return filepath.Base(planDir), jobFile, true
	}
	// Current slug form (plans/<name>) or a bare plan name.
	name := strings.TrimPrefix(ref, "plans/")
	name = strings.Trim(name, "/")
	return name, "", false
}

// scanClaims records which (plan, job) pairs are claimed by any note across the
// workspace's lifecycle directories. Both well-formed links and the effective
// claim recoverable from a malformed legacy ref count, so a malformed note that
// will be rewritten does not cause its own job to be flagged UNCLAIMED.
func scanClaims(workspaceDir string) map[string]struct{} {
	claims := map[string]struct{}{}
	for _, sub := range []string{"inbox", "in_progress", "review", "completed"} {
		files, err := listMarkdown(filepath.Join(workspaceDir, sub))
		if err != nil {
			continue
		}
		for _, path := range files {
			planName, planJob := claimFromNote(path)
			if planName == "" || planJob == "" {
				continue
			}
			claims[planName+"/"+planJob] = struct{}{}
		}
	}
	return claims
}

// claimFromNote extracts the (plan, job) a note claims, handling both the
// well-formed plan_ref/plan_job pair and the malformed legacy ref that bakes a
// job filename into plan_ref.
func claimFromNote(path string) (planName, planJob string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	fm, _, err := frontmatter.Parse(string(content))
	if err != nil || fm == nil || fm.PlanRef == "" {
		return "", ""
	}
	name, jobFile, malformed := parsePlanRef(fm.PlanRef)
	if malformed {
		return name, jobFile
	}
	return name, fm.PlanJob
}

type jobFrontmatter struct {
	ID      string `yaml:"id"`
	NoteRef string `yaml:"note_ref"`
	Status  string `yaml:"status"`
}

var fmBlockRe = regexp.MustCompile(`(?s)^---\n(.*?)\n---`)

// noteIndex is a snapshot of every note in the workspace's lifecycle
// directories, used to resolve a job's note_ref hint back to a real note.
type noteIndex struct {
	byPath map[string]*indexedNote
	byBase map[string][]*indexedNote // "20260611-foo.md" -> notes
	byID   map[string][]*indexedNote // stable frontmatter id -> notes
	order  []*indexedNote
}

type indexedNote struct {
	path     string
	base     string
	id       string
	hasLink  bool // plan_ref or plan_job already set
	readable bool // frontmatter parsed cleanly — required before we rewrite it
	reserved bool // already claimed as another job's relink target this run
}

// assignable reports whether this note may be adopted as a job's relink target.
func (n *indexedNote) assignable() bool {
	return n.readable && !n.hasLink && !n.reserved
}

// buildNoteIndex indexes inbox/in_progress/review/completed. Paths are sorted,
// so candidate selection (and therefore repair) is deterministic.
func buildNoteIndex(workspaceDir string) *noteIndex {
	idx := &noteIndex{
		byPath: map[string]*indexedNote{},
		byBase: map[string][]*indexedNote{},
		byID:   map[string][]*indexedNote{},
	}
	for _, sub := range lifecycleDirs {
		files, err := listMarkdown(filepath.Join(workspaceDir, sub))
		if err != nil {
			continue
		}
		for _, path := range files {
			n := &indexedNote{path: path, base: filepath.Base(path)}
			if content, err := os.ReadFile(path); err == nil {
				if fm, _, err := frontmatter.Parse(string(content)); err == nil && fm != nil {
					n.id = strings.TrimSpace(fm.ID)
					n.hasLink = fm.PlanRef != "" || fm.PlanJob != ""
					n.readable = true
				}
			}
			idx.byPath[path] = n
			idx.byBase[n.base] = append(idx.byBase[n.base], n)
			if n.id != "" {
				idx.byID[n.id] = append(idx.byID[n.id], n)
			}
			idx.order = append(idx.order, n)
		}
	}
	return idx
}

var lifecycleDirs = []string{"inbox", "in_progress", "review", "completed"}

// resolveJobNoteRef maps a job's note_ref hint onto the notes it could mean.
//
// Path-shaped hints (the legacy absolute-path form) are resolved first by exact
// path, then — since notes move through the lifecycle, typically ending up in
// completed/ — by basename across the lifecycle dirs, preferring candidates
// whose stable frontmatter id matches the filename stem. Id-shaped hints (the
// current form) resolve directly through the id index.
//
// It returns every note the hint could denote; the caller partitions those into
// assignable and not (a note already linked to some other job is owned by that
// job — note frontmatter is the source of truth — and a note reserved by an
// earlier job this run is spoken for).
//
// Only notes inside workspaceDir are ever returned, even for an absolute hint:
// resolution goes through the index, so doctor can never reach outside the
// workspace it was pointed at.
func resolveJobNoteRef(ref, workspaceDir string, idx *noteIndex) []*indexedNote {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}

	var candidates []*indexedNote
	if strings.HasSuffix(ref, ".md") {
		// Exact path hit (absolute, or workspace-relative) wins outright.
		var exact []string
		if filepath.IsAbs(ref) {
			exact = append(exact, ref)
		} else if strings.Contains(ref, "/") {
			exact = append(exact, filepath.Join(workspaceDir, ref), ref)
		}
		for _, p := range exact {
			if n, ok := idx.byPath[filepath.Clean(p)]; ok {
				return []*indexedNote{n}
			}
		}
		// Stale path: fall back to the basename across lifecycle dirs.
		base := filepath.Base(ref)
		candidates = idx.byBase[base]
		stem := strings.TrimSuffix(base, ".md")
		if len(candidates) == 0 {
			candidates = idx.byID[stem]
		} else if len(candidates) > 1 {
			// Narrow a basename collision using the stable id when it agrees.
			var confirmed []*indexedNote
			for _, n := range candidates {
				if n.id != "" && n.id == stem {
					confirmed = append(confirmed, n)
				}
			}
			if len(confirmed) > 0 {
				candidates = confirmed
			}
		}
	} else {
		candidates = idx.byID[ref]
	}
	return candidates
}

// scanUnclaimedJobs walks each live plan (plans/*, skipping .archive) and
// reports jobs whose note_ref hint no note claims, planning a repair for each.
// backfillTargets collects notes a relink will heal, so pass 3 leaves them in
// place instead of treating them as UNLINKED strays.
func scanUnclaimedJobs(plansDir, workspaceDir string, claims map[string]struct{}, idx *noteIndex, backfillTargets map[string]struct{}) []JobEntry {
	var out []JobEntry
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		return out
	}
	for _, pe := range entries {
		if !pe.IsDir() || pe.Name() == ".archive" {
			continue
		}
		planName := pe.Name()
		planDir := filepath.Join(plansDir, planName)
		jobFiles, err := os.ReadDir(planDir)
		if err != nil {
			continue
		}
		for _, jf := range jobFiles {
			if jf.IsDir() || !jobFileRe.MatchString(jf.Name()) {
				continue
			}
			jobPath := filepath.Join(planDir, jf.Name())
			jfm := parseJobFrontmatter(jobPath)
			if jfm == nil || jfm.NoteRef == "" {
				continue // jobs without a note_ref hint are ignored
			}
			if _, claimed := claims[planName+"/"+jf.Name()]; claimed {
				continue
			}
			je := JobEntry{
				Plan:           planName,
				JobFile:        jf.Name(),
				NoteRef:        jfm.NoteRef,
				Classification: Unclaimed,
				jobPath:        jobPath,
			}
			matches := resolveJobNoteRef(jfm.NoteRef, workspaceDir, idx)
			var open []*indexedNote
			var unreadable *indexedNote
			for _, n := range matches {
				switch {
				case n.assignable():
					open = append(open, n)
				case !n.readable && unreadable == nil:
					unreadable = n
				}
			}
			switch {
			case len(open) == 1:
				n := open[0]
				n.reserved = true
				je.Repair = RepairRelink
				je.ResolvedNote = n.path
				je.noteID = n.id
				backfillTargets[n.path] = struct{}{}
			case len(open) > 1:
				je.Repair = RepairAmbiguous
				for _, n := range open {
					je.Candidates = append(je.Candidates, n.path)
				}
			case unreadable != nil:
				// The note exists, so the hint is not dead — but its
				// frontmatter can't be parsed, so it can't be rewritten.
				je.Repair = RepairManual
				je.ResolvedNote = unreadable.path
			default:
				je.Repair = RepairClear
			}
			out = append(out, je)
		}
	}
	return out
}

func parseJobFrontmatter(path string) *jobFrontmatter {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	m := fmBlockRe.FindSubmatch(content)
	if len(m) != 2 {
		return nil
	}
	var jf jobFrontmatter
	if err := yaml.Unmarshal(m[1], &jf); err != nil {
		return nil
	}
	return &jf
}

// rewriteLink rewrites a note's frontmatter plan_ref/plan_job in place (the
// MALFORMED-LEGACY backfill migration).
func rewriteLink(st *noteState, planRef, planJob string) error {
	if st.fm == nil {
		return fmt.Errorf("cannot rewrite %s: no frontmatter", st.entry.Path)
	}
	st.fm.PlanRef = planRef
	st.fm.PlanJob = planJob
	updated := frontmatter.BuildContent(st.fm, st.body)
	return os.WriteFile(st.entry.Path, []byte(updated), 0o644)
}

// backfillNoteLink sets plan_ref/plan_job on a note that currently lacks them
// (forward-direction healing driven by a job's note_ref).
func backfillNoteLink(path, planRef, planJob string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading note %s: %w", path, err)
	}
	fm, body, err := frontmatter.Parse(string(content))
	if err != nil || fm == nil {
		return fmt.Errorf("parsing note %s frontmatter: %w", path, err)
	}
	fm.PlanRef = planRef
	fm.PlanJob = planJob
	updated := frontmatter.BuildContent(fm, body)
	return os.WriteFile(path, []byte(updated), 0o644)
}

// setJobNoteRef rewrites the note_ref line inside a job file's YAML
// frontmatter, leaving every other line (and any unknown field flow may carry)
// byte-for-byte intact — a full YAML round-trip would silently drop them. An
// empty value writes `note_ref: ""`, the form flow itself emits for "no note".
func setJobNoteRef(jobPath, value string) error {
	content, err := os.ReadFile(jobPath)
	if err != nil {
		return fmt.Errorf("reading job %s: %w", jobPath, err)
	}
	loc := fmBlockRe.FindSubmatchIndex(content)
	if loc == nil {
		return fmt.Errorf("job %s has no frontmatter block", jobPath)
	}
	// loc[2]:loc[3] spans the frontmatter body (capture group 1).
	body := string(content[loc[2]:loc[3]])
	lines := strings.Split(body, "\n")
	replaced := false
	for i, line := range lines {
		if !strings.HasPrefix(line, "note_ref:") {
			continue
		}
		lines[i] = "note_ref: " + yamlScalar(value)
		replaced = true
		break
	}
	if !replaced {
		return fmt.Errorf("job %s has no note_ref field", jobPath)
	}
	var out []byte
	out = append(out, content[:loc[2]]...)
	out = append(out, []byte(strings.Join(lines, "\n"))...)
	out = append(out, content[loc[3]:]...)
	return os.WriteFile(jobPath, out, 0o644)
}

var plainScalarRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\-/]*$`)

// yamlScalar renders a string as a YAML scalar, quoting whenever a bare word
// would be unsafe (empty, or containing YAML-significant characters).
func yamlScalar(s string) string {
	if plainScalarRe.MatchString(s) {
		return s
	}
	return strconv.Quote(s)
}

// moveNote moves a note into destDir, emitting a typed move event (best-effort).
// On a name collision at the destination a numeric suffix is appended.
func moveNote(src, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", destDir, err)
	}
	dest := filepath.Join(destDir, filepath.Base(src))
	if fileExists(dest) && dest != src {
		base := strings.TrimSuffix(filepath.Base(src), ".md")
		for i := 1; ; i++ {
			cand := filepath.Join(destDir, fmt.Sprintf("%s-%d.md", base, i))
			if !fileExists(cand) {
				dest = cand
				break
			}
		}
	}
	if err := os.Rename(src, dest); err != nil {
		return "", fmt.Errorf("moving %s -> %s: %w", src, dest, err)
	}
	ws, _, noteType := service.GetNoteMetadata(dest)
	prevWs, _, prevType := service.GetNoteMetadata(src)
	emitEvent(coremodels.NoteEvent{
		Event:         coremodels.NoteEventMoved,
		Workspace:     ws,
		NoteType:      noteType,
		Path:          dest,
		PrevWorkspace: prevWs,
		PrevNoteType:  prevType,
		PrevPath:      src,
	})
	return dest, nil
}

func listMarkdown(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

func fillSummary(r *Report) {
	counts := map[Classification]int{}
	for _, e := range r.Notes {
		counts[e.Classification]++
	}
	r.Summary["live"] = counts[Live]
	r.Summary["archived"] = counts[Archived]
	r.Summary["gone"] = counts[Gone]
	r.Summary["malformed_legacy"] = counts[MalformedLegacy]
	r.Summary["unlinked"] = counts[Unlinked]
	r.Summary["unclaimed_jobs"] = len(r.UnclaimedJobs)

	repairs := map[Repair]int{}
	for _, j := range r.UnclaimedJobs {
		repairs[j.Repair]++
	}
	r.Summary["jobs_relinked"] = repairs[RepairRelink]
	r.Summary["jobs_cleared"] = repairs[RepairClear]
	r.Summary["jobs_ambiguous"] = repairs[RepairAmbiguous]
	r.Summary["jobs_manual"] = repairs[RepairManual]

	problems, actions := 0, 0
	for _, e := range r.Notes {
		if e.problem {
			problems++
		}
		if e.ActionTaken != "" {
			actions++
		}
	}
	for _, j := range r.UnclaimedJobs {
		problems++
		if j.ActionTaken != "" {
			actions++
		}
	}
	r.Summary["problems"] = problems
	r.Summary["actions_taken"] = actions
	r.Summary["problems_remaining"] = r.ProblemsRemaining()
}

func sortReport(r *Report) {
	sort.SliceStable(r.Notes, func(i, j int) bool {
		if r.Notes[i].Classification != r.Notes[j].Classification {
			return r.Notes[i].Classification < r.Notes[j].Classification
		}
		return r.Notes[i].Path < r.Notes[j].Path
	})
	sort.SliceStable(r.UnclaimedJobs, func(i, j int) bool {
		if r.UnclaimedJobs[i].Plan != r.UnclaimedJobs[j].Plan {
			return r.UnclaimedJobs[i].Plan < r.UnclaimedJobs[j].Plan
		}
		return r.UnclaimedJobs[i].JobFile < r.UnclaimedJobs[j].JobFile
	})
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
