package service

// Structured, idempotent note create/update for external producers.
//
// This is the seam the grove-panel-pomodoro work-block notes (and any other
// out-of-process producer) deliver through: `nb new --json --frontmatter-file
// --idempotency-key` and `nb update --json --path`. The design constraints,
// from ticket 20260803-nb-structured-idempotent-note-create-update:
//
//   - nb-owned fields (id, created, modified, directory-derived type) are set
//     by nb and win; validated producer fields (namespaced, e.g. pomodoro_*)
//     are merged and round-trip losslessly (frontmatter.Extra).
//   - Create is idempotent on the key: a repeated create with the same key in
//     the same resolved type directory returns the EXISTING note instead of
//     minting a second one, so producers need no separate lookup and the
//     crash window between nb committing a note and the producer persisting
//     its receipt is harmless.
//   - Update refuses paths outside any notebook root nb knows about — this
//     command is a notebook API, not a general file rewriter.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	coreworkspace "github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/util/pathutil"

	"github.com/grovetools/nb/pkg/frontmatter"
	"github.com/grovetools/nb/pkg/models"
)

// ExpectedNoteIdentity carries the clipping identity the caller expects the
// note to hold. UpdateStructuredNoteForContext validates all three fields
// before any mutation.
type ExpectedNoteIdentity struct {
	TypePath       models.NoteType
	IdempotencyKey string
	Filename       string
}

// StructuredUpdateError is a typed, machine-parseable update failure.
// cmd/update.go renders it as a compact JSON envelope on stdout.
type StructuredUpdateError struct {
	Code    string
	Message string
}

const (
	CodeNoteNotFound         = "note_not_found"
	CodeNoteOutsideTarget    = "note_outside_target"
	CodeNoteOutsideBundle    = "note_outside_bundle"
	CodeNoteIdentityMismatch = "note_identity_mismatch"
)

func (e *StructuredUpdateError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Is supports errors.Is matching by code.
func (e *StructuredUpdateError) Is(target error) bool {
	if t, ok := target.(*StructuredUpdateError); ok {
		return e.Code == t.Code
	}
	return false
}

// StructuredNoteOptions carries the optional knobs of a structured create.
type StructuredNoteOptions struct {
	// IdempotencyKey, when non-empty, is stored in the note's frontmatter
	// (frontmatter.IdempotencyKeyField) and makes the create idempotent
	// within the resolved type directory.
	IdempotencyKey string
	// Filename, when non-empty, replaces the generated YYYYMMDD-title.md
	// basename. Must satisfy ValidateNoteFilename.
	Filename string
	// Priority seeds the note's priority field (p0..p3, empty = none). A
	// priority in the producer frontmatter file wins over this.
	Priority string
}

// ValidateNoteFilename checks a producer-controlled --filename value: a plain
// relative markdown basename, nothing that could escape the resolved type
// directory or land a non-note file in it.
func ValidateNoteFilename(name string) error {
	if name == "" {
		return fmt.Errorf("filename must not be empty")
	}
	if strings.ContainsAny(name, "/\\") || filepath.Base(name) != name {
		return fmt.Errorf("filename %q must be a plain basename without path separators", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("filename %q is not a file name", name)
	}
	if !strings.HasSuffix(name, ".md") || len(name) == len(".md") {
		return fmt.Errorf("filename %q must end in .md with a non-empty stem", name)
	}
	return nil
}

// CreateStructuredNote creates a note with producer frontmatter merged in, or
// — when opts.IdempotencyKey matches an existing note in the resolved type
// directory — returns that existing note untouched. The second return value
// reports which of the two happened (true = existing note, nothing written).
//
// The actual write goes through writeNewNoteFile, the same tail as
// CreateNoteWithContent, so structured creates emit the created event through
// the standard EmitNoteEvent funnel.
func (s *Service) CreateStructuredNote(
	ctx *WorkspaceContext,
	noteType models.NoteType,
	title string,
	producer frontmatter.ProducerFields,
	body string,
	opts StructuredNoteOptions,
) (*models.Note, bool, error) {
	if opts.Filename != "" {
		if err := ValidateNoteFilename(opts.Filename); err != nil {
			return nil, false, err
		}
	}
	if !IsValidPriority(opts.Priority) {
		return nil, false, fmt.Errorf("invalid priority %q (want one of p0,p1,p2,p3 or empty)", opts.Priority)
	}

	// Resolve and ensure the type directory. Nested types ("hn/clippings/…")
	// arrive here as-is; the locator maps them to nested directories.
	noteDir, err := s.getNotePathForContext(ctx, string(noteType))
	if err != nil {
		return nil, false, fmt.Errorf("get note path: %w", err)
	}
	if err := os.MkdirAll(noteDir, 0o755); err != nil {
		return nil, false, fmt.Errorf("ensure directories: %w", err)
	}

	// Idempotency scan BEFORE any write: a repeated create must return the
	// first note's receipt, not a second file.
	if opts.IdempotencyKey != "" {
		existingPath, err := findNoteByIdempotencyKey(noteDir, opts.IdempotencyKey)
		if err != nil {
			return nil, false, err
		}
		if existingPath != "" {
			note, err := ParseNote(existingPath)
			if err != nil {
				return nil, false, fmt.Errorf("parse existing idempotent note %s: %w", existingPath, err)
			}
			return note, true, nil
		}
	}

	// Base frontmatter: the same nb-owned identity and workspace context
	// fields interactive creation stamps (see CreateNoteContent), so a
	// structured note is indistinguishable from a hand-made one apart from
	// its producer fields.
	now := time.Now()
	ts := frontmatter.FormatTimestamp(now)
	workspaceName := ""
	if ctx.NotebookContextWorkspace != nil {
		workspaceName = ctx.NotebookContextWorkspace.Name
	}
	worktreeName := ""
	if ctx.CurrentWorkspace != nil {
		worktreeName = ctx.CurrentWorkspace.GetWorktreeName()
	}
	var repoTags []string
	if workspaceName != "" && workspaceName != globalWorkspace {
		repoTags = []string{workspaceName}
	}

	fm := &frontmatter.Frontmatter{
		ID:       GenerateNoteID(title),
		Title:    title,
		Aliases:  []string{},
		Tags:     frontmatter.MergeTags(frontmatter.ExtractPathTags(string(noteType)), repoTags),
		Created:  ts,
		Modified: ts,
	}
	if workspaceName != "" && workspaceName != globalWorkspace {
		fm.Repository = workspaceName
		if ctx.Branch != "" {
			fm.Branch = ctx.Branch
		}
		if worktreeName != "" {
			fm.Worktree = worktreeName
		}
	}
	if opts.Priority != "" {
		fm.Priority = opts.Priority
	}

	// Producer fields merge on top; nb-owned fields inside the producer map
	// are ignored by ApplyProducerFields (nb wins).
	if err := frontmatter.ApplyProducerFields(fm, producer); err != nil {
		return nil, false, err
	}
	if !IsValidPriority(fm.Priority) {
		return nil, false, fmt.Errorf("invalid priority %q in frontmatter file (want one of p0,p1,p2,p3 or empty)", fm.Priority)
	}

	// The key is persisted in the note itself: the idempotency scan reads it
	// back, and because it lives in frontmatter.Extra it survives moves and
	// copies like every other producer field.
	if opts.IdempotencyKey != "" {
		if err := fm.SetExtra(frontmatter.IdempotencyKeyField, opts.IdempotencyKey); err != nil {
			return nil, false, err
		}
	}

	filename := opts.Filename
	if filename == "" {
		filename = GenerateFilename(title)
	}
	notePath := filepath.Join(noteDir, filename)
	if _, err := os.Stat(notePath); err == nil {
		if opts.Filename != "" {
			// The producer asked for THIS name; silently uniquifying would
			// hand back a receipt for a file it did not ask for, and silently
			// overwriting would destroy someone else's note.
			return nil, false, fmt.Errorf("note %s already exists (use --idempotency-key for repeat-safe creation)", notePath)
		}
		// Generated names collide legitimately (same title, same day):
		// disambiguate the way transferNotes does rather than overwrite.
		notePath = uniquifyNotePath(noteDir, filename)
	}

	note, err := s.writeNewNoteFile(ctx, noteType, notePath, fm, body)
	if err != nil {
		return nil, false, err
	}
	return note, false, nil
}

// uniquifyNotePath appends a timestamp (then a counter, for same-second
// collisions) to a generated filename until it is free in dir.
func uniquifyNotePath(dir, filename string) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	candidate := filepath.Join(dir, fmt.Sprintf("%s-%s%s", base, time.Now().Format("150405"), ext))
	for n := 2; ; n++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
		candidate = filepath.Join(dir, fmt.Sprintf("%s-%s-%d%s", base, time.Now().Format("150405"), n, ext))
	}
}

// findNoteByIdempotencyKey scans the immediate *.md files of dir for a note
// whose frontmatter carries key under frontmatter.IdempotencyKeyField. The
// scan is non-recursive on purpose: nested types resolve to their own
// directories, so "the same resolved type dir" is exactly one directory, and
// a same-keyed note in a sibling or child type is a different note.
func findNoteByIdempotencyKey(dir, key string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("scan type directory for idempotency key: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			continue // an unreadable neighbor must not block creation
		}
		fm, _, err := frontmatter.Parse(string(content))
		if err != nil || fm == nil {
			continue // notes without (parseable) frontmatter can't hold a key
		}
		if existing, ok := fm.ExtraString(frontmatter.IdempotencyKeyField); ok && existing == key {
			return path, nil
		}
	}
	return "", nil
}

// UpdateStructuredNote rewrites the note at path with producer frontmatter
// merged in and (when body is non-nil) the body replaced. Merge policy:
// nb-owned identity (id, created, directory-derived type) is preserved,
// modified is refreshed, validated producer fields replace/extend the rest.
// The write goes through UpdateNoteWithContent, so the updated event reaches
// the EmitNoteEvent funnel.
func (s *Service) UpdateStructuredNote(path string, producer frontmatter.ProducerFields, body *string) (*models.Note, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("--path must be absolute, got %q", path)
	}
	// Canonicalize (symlinks + on-disk case) so the notebook-root membership
	// check below compares one deterministic spelling on both sides — the
	// same treatment core's notebook resolver gives the configured roots.
	absPath := filepath.Clean(path)
	if canonical, err := pathutil.CanonicalPath(absPath); err == nil {
		absPath = canonical
	}
	if err := s.ensureInsideNotebookRoot(absPath); err != nil {
		return nil, err
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read note: %w", err)
	}
	fm, existingBody, err := frontmatter.Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse note frontmatter: %w", err)
	}
	if fm == nil {
		// A structured update of a frontmatter-less file would have to invent
		// an identity for it — that is creation, and `nb new` owns creation.
		return nil, fmt.Errorf("refusing structured update: %s has no frontmatter", absPath)
	}

	// id/created/type survive because ApplyProducerFields never touches
	// nb-owned fields; everything the producer sent merges on top.
	if err := frontmatter.ApplyProducerFields(fm, producer); err != nil {
		return nil, err
	}
	if !IsValidPriority(fm.Priority) {
		return nil, fmt.Errorf("invalid priority %q in frontmatter file (want one of p0,p1,p2,p3 or empty)", fm.Priority)
	}
	fm.Modified = frontmatter.FormatTimestamp(time.Now())

	newBody := existingBody
	if body != nil {
		newBody = *body
	}

	if err := s.UpdateNoteWithContent(absPath, fm, newBody); err != nil {
		return nil, err
	}

	note, err := ParseNote(absPath)
	if err != nil {
		return nil, fmt.Errorf("parse updated note: %w", err)
	}
	return note, nil
}

// UpdateStructuredNoteForContext validates that the note at path belongs to
// the selected workspace context and matches the expected clipping identity
// before applying producer fields and an optional body replacement. All
// checks complete before any write.
func (s *Service) UpdateStructuredNoteForContext(
	ctx *WorkspaceContext,
	path string,
	producer frontmatter.ProducerFields,
	body *string,
	expected ExpectedNoteIdentity,
) (*models.Note, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("--path must be absolute, got %q", path)
	}

	// 1. Stat receipt; absence is note_not_found.
	absPath := filepath.Clean(path)
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, &StructuredUpdateError{Code: CodeNoteNotFound, Message: fmt.Sprintf("note does not exist: %s", absPath)}
	} else if err != nil {
		return nil, fmt.Errorf("stat note: %w", err)
	}

	// Canonicalize receipt (resolve symlinks).
	canonicalReceipt := absPath
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		canonicalReceipt = resolved
	}

	// 2. Resolve the selected route's notes authority root and canonicalize.
	authorityRoot, err := s.getNotePathForContext(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("resolve authority root: %w", err)
	}
	canonicalRoot := filepath.Clean(authorityRoot)
	if resolved, err := filepath.EvalSymlinks(canonicalRoot); err == nil {
		canonicalRoot = resolved
	}

	// Containment: canonical receipt must be under canonical root.
	rootPrefix := canonicalRoot + string(filepath.Separator)
	if canonicalReceipt != canonicalRoot && !strings.HasPrefix(canonicalReceipt, rootPrefix) {
		return nil, &StructuredUpdateError{
			Code:    CodeNoteOutsideTarget,
			Message: fmt.Sprintf("note %s is outside the selected route's authority", absPath),
		}
	}

	// 3. Resolve expected type directory and canonicalize.
	expectedDir, err := s.getNotePathForContext(ctx, string(expected.TypePath))
	if err != nil {
		return nil, fmt.Errorf("resolve expected type path %q: %w", expected.TypePath, err)
	}
	canonicalExpectedDir := filepath.Clean(expectedDir)
	if resolved, err := filepath.EvalSymlinks(canonicalExpectedDir); err == nil {
		canonicalExpectedDir = resolved
	}

	// 4. Require the canonical receipt's parent to equal the canonical expected
	//    type directory directly (not a descendant).
	receiptParent := filepath.Dir(canonicalReceipt)
	if receiptParent != canonicalExpectedDir {
		return nil, &StructuredUpdateError{
			Code:    CodeNoteOutsideBundle,
			Message: fmt.Sprintf("note parent %s does not match expected type directory %s", receiptParent, canonicalExpectedDir),
		}
	}

	// 5. Compare expected filename with canonical receipt's basename.
	canonicalBasename := filepath.Base(canonicalReceipt)
	if expected.Filename != "" && canonicalBasename != expected.Filename {
		return nil, &StructuredUpdateError{
			Code:    CodeNoteOutsideBundle,
			Message: fmt.Sprintf("canonical basename %q does not match expected filename %q", canonicalBasename, expected.Filename),
		}
	}

	// 6. Strictly parse existing frontmatter.
	content, err := os.ReadFile(canonicalReceipt)
	if err != nil {
		return nil, fmt.Errorf("read note: %w", err)
	}
	fm, existingBody, err := frontmatter.Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse note frontmatter: %w", err)
	}
	if fm == nil {
		return nil, &StructuredUpdateError{
			Code:    CodeNoteIdentityMismatch,
			Message: fmt.Sprintf("note %s has no frontmatter", absPath),
		}
	}

	// 7. Require persisted idempotency key to match expected.
	if expected.IdempotencyKey != "" {
		persistedKey, _ := fm.ExtraString(frontmatter.IdempotencyKeyField)
		if persistedKey != expected.IdempotencyKey {
			return nil, &StructuredUpdateError{
				Code:    CodeNoteIdentityMismatch,
				Message: fmt.Sprintf("idempotency key mismatch: persisted %q, expected %q", persistedKey, expected.IdempotencyKey),
			}
		}
	}

	// 8. All checks passed — apply producer fields and write.
	if err := frontmatter.ApplyProducerFields(fm, producer); err != nil {
		return nil, err
	}
	if !IsValidPriority(fm.Priority) {
		return nil, fmt.Errorf("invalid priority %q in frontmatter file (want one of p0,p1,p2,p3 or empty)", fm.Priority)
	}
	fm.Modified = frontmatter.FormatTimestamp(time.Now())

	newBody := existingBody
	if body != nil {
		newBody = *body
	}

	if err := s.UpdateNoteWithContent(canonicalReceipt, fm, newBody); err != nil {
		return nil, err
	}

	note, err := ParseNote(canonicalReceipt)
	if err != nil {
		return nil, fmt.Errorf("parse updated note: %w", err)
	}
	return note, nil
}

// ensureInsideNotebookRoot refuses paths that are not inside any notebook nb
// knows about. Roots are gathered from every place the locator machinery can
// place a note: configured notebook definitions, the (configured or default)
// global notebook, the built-in centralized default, local-mode .notebook
// directories of discovered workspaces, and finally the notebook.yml marker
// fallback for notebooks that exist on disk but not in config.
func (s *Service) ensureInsideNotebookRoot(absPath string) error {
	// 1. Configured notebook definitions (core canonicalizes each root).
	if root, _ := coreworkspace.FindNotebookRootFromConfig(absPath, s.CoreConfig); root != "" {
		return nil
	}

	// 2. Global notebook root and the locator's built-in defaults.
	candidates := []string{"~/.grove/notebooks/global", "~/.grove/notebooks/nb"}
	if s.CoreConfig != nil && s.CoreConfig.Notebooks != nil &&
		s.CoreConfig.Notebooks.Rules != nil && s.CoreConfig.Notebooks.Rules.Global != nil &&
		s.CoreConfig.Notebooks.Rules.Global.RootDir != "" {
		candidates = append(candidates, s.CoreConfig.Notebooks.Rules.Global.RootDir)
	}
	// 3. Local-mode notebooks live inside their workspace directory.
	if s.workspaceProvider != nil {
		for _, ws := range s.workspaceProvider.All() {
			candidates = append(candidates, filepath.Join(ws.GetGroupingKey(), ".notebook"))
		}
	}
	for _, candidate := range candidates {
		root, err := pathutil.Expand(candidate)
		if err != nil {
			continue
		}
		root = filepath.Clean(root)
		if canonical, cerr := pathutil.CanonicalPath(root); cerr == nil {
			root = canonical
		}
		if absPath == root || strings.HasPrefix(absPath, root+string(filepath.Separator)) {
			return nil
		}
	}

	// 4. notebook.yml marker fallback, mirroring GetProjectFromNotebookPath.
	if root := coreworkspace.FindNotebookMarker(filepath.Dir(absPath)); root != "" {
		return nil
	}

	return fmt.Errorf("refusing to update %s: not inside any configured notebook root", absPath)
}
