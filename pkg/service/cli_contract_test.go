// CLI contract tests for the structured note seam.
//
// These drive the REAL cobra commands with the EXACT argument vectors the
// grove-panel-pomodoro CLIWriter produces (see its note.go: Create invokes
// `nb new --json --type <t> --title <t> --idempotency-key <k>
// --frontmatter-file <f> --body-file <f> --no-edit`, Update invokes
// `nb update --json --path <abs> --frontmatter-file <f> --body-file <f>`) and
// assert stdout is EXACTLY the {"id","path"} receipt. Those invocations
// working verbatim IS the acceptance criterion of ticket
// 20260803-nb-structured-idempotent-note-create-update.
//
// The file lives in pkg/service (as the external service_test package, so it
// may import cmd without a cycle) because only this directory can reach the
// daemon-notifier seam via StubDaemonNotifierForTests — running the real
// commands without the stub would auto-start a daemon from inside `go test`.
package service_test

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coreconfig "github.com/grovetools/core/config"
	coremodels "github.com/grovetools/core/pkg/models"
	coreworkspace "github.com/grovetools/core/pkg/workspace"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/nb/cmd"
	"github.com/grovetools/nb/pkg/frontmatter"
	"github.com/grovetools/nb/pkg/service"
)

// newCLITestService builds a real Service whose global notebook resolves into
// a throwaway root, so commands run with workspaceOverride="global" write
// only inside the test sandbox. The root is also a configured notebook
// definition, which is what entitles `nb update` to touch notes under it.
func newCLITestService(t *testing.T) (*service.Service, string) {
	t.Helper()
	root := t.TempDir()
	cfg := &coreconfig.Config{
		Notebooks: &coreconfig.NotebooksConfig{
			Definitions: map[string]*coreconfig.Notebook{"main": {RootDir: root}},
			Rules: &coreconfig.NotebookRules{
				Default: "main",
				Global:  &coreconfig.GlobalNotebookConfig{RootDir: root},
			},
		},
	}
	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	logger.SetLevel(logrus.PanicLevel)
	svc, err := service.New(&service.Config{}, coreworkspace.NewProviderFromNodes(nil), cfg, logrus.NewEntry(logger))
	require.NoError(t, err)
	return svc, root
}

// runCapturingStdout executes fn with os.Stdout swapped for a pipe and
// returns everything written to it. Swapping the real fd-backed variable
// (not just cobra's out writer) is the point: the receipt promise is about
// process stdout, and any stray print from a logger would land here too.
func runCapturingStdout(t *testing.T, fn func() error) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	runErr := fn()

	require.NoError(t, w.Close())
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	os.Stdout = orig
	require.NoError(t, runErr)
	return string(out)
}

// parseReceipt asserts stdout is EXACTLY one JSON receipt line with a
// non-empty absolute path — the purity check pomodoro's finishNote depends on.
func parseReceipt(t *testing.T, stdout string) (id, path string) {
	t.Helper()
	trimmed := strings.TrimSpace(stdout)
	require.NotEmpty(t, trimmed, "expected a receipt on stdout")
	require.NotContains(t, trimmed, "\n", "stdout must carry the receipt and nothing else, got:\n%s", stdout)
	var rec struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	}
	require.NoError(t, json.Unmarshal([]byte(trimmed), &rec), "stdout is not a bare JSON receipt: %q", stdout)
	require.NotEmpty(t, rec.Path, "receipt path must be non-empty")
	require.True(t, filepath.IsAbs(rec.Path), "receipt path must be absolute, got %q", rec.Path)
	return rec.ID, rec.Path
}

// writePomodoroFixtures renders a producer frontmatter file (JSON, as the
// panel writes note-frontmatter.json) and a body file.
func writePomodoroFixtures(t *testing.T, dir, status string) (fmFile, bodyFile string) {
	t.Helper()
	fmFile = filepath.Join(dir, "note-frontmatter.json")
	fields := map[string]any{
		"title":                   "Pomodoro 2026-08-05 09:00–09:50",
		"tags":                    []string{"pomodoro", "work-block"},
		"pomodoro_block_id":       "blk-1",
		"pomodoro_summary_status": status,
		"pomodoro_jobs_completed": 3,
	}
	data, err := json.Marshal(fields)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(fmFile, data, 0o644))

	bodyFile = filepath.Join(dir, "note-body.md")
	require.NoError(t, os.WriteFile(bodyFile, []byte("# Work-block summary\n\nStatus: "+status+"\n"), 0o644))
	return fmFile, bodyFile
}

func TestPomodoroCreateInvocationVerbatim(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, root := newCLITestService(t)
	override := "global"
	fmFile, bodyFile := writePomodoroFixtures(t, t.TempDir(), "pending")

	// The exact argument vector pomodoro's CLIWriter.Create builds.
	args := []string{
		"--json",
		"--type", "worklog/pomodoro",
		"--title", "Pomodoro 2026-08-05 09:00–09:50",
		"--idempotency-key", "pomodoro:blk-1",
		"--frontmatter-file", fmFile,
		"--body-file", bodyFile,
		"--no-edit",
	}

	runNew := func() string {
		c := cmd.NewNewCmd(&svc, &override)
		c.SetArgs(args)
		return runCapturingStdout(t, c.Execute)
	}

	id, path := parseReceipt(t, runNew())
	assert.NotEmpty(t, id)
	assert.Equal(t, filepath.Join(root, "notes", "worklog", "pomodoro"), filepath.Dir(path))

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	fm, body, err := frontmatter.Parse(string(content))
	require.NoError(t, err)
	require.NotNil(t, fm)
	assert.Equal(t, id, fm.ID)
	assert.Equal(t, "blk-1", fm.ExtraValue("pomodoro_block_id"))
	assert.Equal(t, "pomodoro:blk-1", fm.ExtraValue(frontmatter.IdempotencyKeyField))
	assert.Contains(t, body, "# Work-block summary")

	// Idempotency across full invocations: the retry returns the FIRST
	// note's receipt and creates nothing.
	id2, path2 := parseReceipt(t, runNew())
	assert.Equal(t, id, id2)
	assert.Equal(t, path, path2)
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "repeat create must not mint a second note")
}

func TestPomodoroUpdateInvocationVerbatim(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, _ := newCLITestService(t)
	override := "global"

	// Seed the note through the create half of the contract.
	fmFile, bodyFile := writePomodoroFixtures(t, t.TempDir(), "pending")
	createCmd := cmd.NewNewCmd(&svc, &override)
	createCmd.SetArgs([]string{
		"--json",
		"--type", "worklog/pomodoro",
		"--title", "Pomodoro 2026-08-05 09:00–09:50",
		"--idempotency-key", "pomodoro:blk-1",
		"--frontmatter-file", fmFile,
		"--body-file", bodyFile,
		"--no-edit",
	})
	createdID, createdPath := parseReceipt(t, runCapturingStdout(t, createCmd.Execute))

	// The exact argument vector pomodoro's CLIWriter.Update builds.
	fmFile2, bodyFile2 := writePomodoroFixtures(t, t.TempDir(), "final")
	updateOverride := ""
	updateCmd := cmd.NewUpdateCmd(&svc, &updateOverride)
	updateCmd.SetArgs([]string{
		"--json",
		"--path", createdPath,
		"--frontmatter-file", fmFile2,
		"--body-file", bodyFile2,
	})
	updatedID, updatedPath := parseReceipt(t, runCapturingStdout(t, updateCmd.Execute))

	// Same note, same identity; canonical spelling may differ (symlinks).
	assert.Equal(t, createdID, updatedID)
	wantPath, err := filepath.EvalSymlinks(createdPath)
	require.NoError(t, err)
	assert.Equal(t, wantPath, updatedPath)

	content, err := os.ReadFile(updatedPath)
	require.NoError(t, err)
	fm, body, err := frontmatter.Parse(string(content))
	require.NoError(t, err)
	require.NotNil(t, fm)
	assert.Equal(t, createdID, fm.ID, "update must preserve the note id")
	assert.Equal(t, "final", fm.ExtraValue("pomodoro_summary_status"))
	assert.Equal(t, "pomodoro:blk-1", fm.ExtraValue(frontmatter.IdempotencyKeyField), "the key survives updates")
	assert.Contains(t, body, "Status: final", "body must be replaced")
}

// TestFlowPositionalCreateStillWorks pins the pre-existing caller shape:
// flow's plan demote path runs `nb new <title> --type inbox --no-edit` with
// the body piped on stdin, and none of the structured additions may change
// what it gets back (a pretty "Created:" line, NOT a JSON receipt).
func TestFlowPositionalCreateStillWorks(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, root := newCLITestService(t)
	override := "global"

	c := cmd.NewNewCmd(&svc, &override)
	c.SetArgs([]string{"Demoted job title", "--type", "inbox", "--no-edit"})
	stdout := runCapturingStdout(t, c.Execute)

	assert.NotContains(t, stdout, `{"id"`, "non-json invocations must not emit receipts")

	inboxDir := filepath.Join(root, "notes", "inbox")
	entries, err := os.ReadDir(inboxDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Name(), "demoted-job-title")
}

// ---------------------------------------------------------------------------
// A3: Create disposition tests
// ---------------------------------------------------------------------------

// parseReceiptWithDisposition is like parseReceipt but also returns disposition.
func parseReceiptWithDisposition(t *testing.T, stdout string) (id, path, disposition string) {
	t.Helper()
	trimmed := strings.TrimSpace(stdout)
	require.NotEmpty(t, trimmed)
	require.NotContains(t, trimmed, "\n", "stdout must carry the receipt and nothing else")
	var rec struct {
		ID          string `json:"id"`
		Path        string `json:"path"`
		Disposition string `json:"disposition"`
	}
	require.NoError(t, json.Unmarshal([]byte(trimmed), &rec))
	require.NotEmpty(t, rec.Path)
	require.True(t, filepath.IsAbs(rec.Path))
	return rec.ID, rec.Path, rec.Disposition
}

func TestCreateDispositionCreated(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, _ := newCLITestService(t)
	override := "global"

	c := cmd.NewNewCmd(&svc, &override)
	c.SetArgs([]string{"--json", "--type", "inbox", "--title", "Fresh Note", "--no-edit"})
	stdout := runCapturingStdout(t, c.Execute)

	_, _, disposition := parseReceiptWithDisposition(t, stdout)
	assert.Equal(t, "created", disposition)
}

func TestCreateDispositionExisting(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, _ := newCLITestService(t)
	override := "global"

	args := []string{"--json", "--type", "inbox", "--title", "Idem Note",
		"--idempotency-key", "test:1", "--no-edit"}

	c1 := cmd.NewNewCmd(&svc, &override)
	c1.SetArgs(args)
	stdout1 := runCapturingStdout(t, c1.Execute)
	_, path1, disp1 := parseReceiptWithDisposition(t, stdout1)
	assert.Equal(t, "created", disp1)

	before, err := os.ReadFile(path1)
	require.NoError(t, err)

	// Same idempotency key, different title: `existing` must return the first
	// note's path without rewriting anything.
	args2 := []string{"--json", "--type", "inbox", "--title", "Changed Title",
		"--idempotency-key", "test:1", "--no-edit"}
	c2 := cmd.NewNewCmd(&svc, &override)
	c2.SetArgs(args2)
	stdout2 := runCapturingStdout(t, c2.Execute)
	_, path2, disp2 := parseReceiptWithDisposition(t, stdout2)
	assert.Equal(t, "existing", disp2)
	assert.Equal(t, path1, path2)

	after, err := os.ReadFile(path1)
	require.NoError(t, err)
	assert.Equal(t, before, after, "existing disposition must not imply the note was rewritten")
}

func TestCreateDispositionNotInLegacyPath(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, _ := newCLITestService(t)
	override := "global"

	c := cmd.NewNewCmd(&svc, &override)
	c.SetArgs([]string{"Title Only", "--type", "inbox", "--no-edit"})
	stdout := runCapturingStdout(t, c.Execute)
	assert.NotContains(t, stdout, "disposition")
}

// ---------------------------------------------------------------------------
// A1: JSONL streaming tests
// ---------------------------------------------------------------------------

func seedNote(t *testing.T, dir, filename, id, title string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, filename)
	content := fmt.Sprintf("---\nid: %s\ntitle: %s\naliases: []\ntags: []\ncreated: 2026-08-05T10:00:00Z\nmodified: 2026-08-05T10:00:00Z\n---\n\n# %s\n", id, title, title)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestListJSONLBasic(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, root := newCLITestService(t)
	override := ""

	clippingsDir := filepath.Join(root, "notes", "hn", "clippings")
	seedNote(t, clippingsDir, "comments.md", "n1", "Story One")
	seedNote(t, clippingsDir, "article.md", "n2", "Story Two")

	c := cmd.NewListCmd(&svc, &override)
	c.SetArgs([]string{"hn/clippings", "--jsonl", "--include-frontmatter", "-g"})
	stdout := runCapturingStdout(t, c.Execute)

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.GreaterOrEqual(t, len(lines), 4) // catalog + 2 notes + end

	// Catalog header
	var cat map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &cat))
	assert.Equal(t, "catalog", cat["kind"])
	assert.Equal(t, float64(1), cat["schema_version"])
	assert.Equal(t, "hn/clippings", cat["query_type"])

	// End record
	var end map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &end))
	assert.Equal(t, "end", end["kind"])
	assert.Equal(t, true, end["complete"])
	assert.Equal(t, float64(2), end["notes"])

	// Note records — no target_id
	for _, line := range lines[1 : len(lines)-1] {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		assert.Equal(t, "note", rec["kind"])
		assert.NotEmpty(t, rec["note_id"])
		assert.NotEmpty(t, rec["receipt_path"])
		assert.NotEmpty(t, rec["bundle_type"])
		_, hasTargetID := rec["target_id"]
		assert.False(t, hasTargetID, "records must not contain target_id")
		fm, hasFM := rec["frontmatter"]
		assert.True(t, hasFM)
		assert.NotNil(t, fm)
	}
}

func TestListJSONLGlobalFlag(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, root := newCLITestService(t)
	override := ""

	// Seed a note in the global notebook's hn/clippings.
	clippingsDir := filepath.Join(root, "notes", "hn", "clippings")
	seedNote(t, clippingsDir, "global-clip.md", "gc1", "Global Clipping")

	c := cmd.NewListCmd(&svc, &override)
	c.SetArgs([]string{"hn/clippings", "--jsonl", "-g"})
	stdout := runCapturingStdout(t, c.Execute)

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.GreaterOrEqual(t, len(lines), 3, "expected catalog + note + end")

	// The end record should reflect exactly the note we seeded in the global root.
	var end map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &end))
	assert.Equal(t, "end", end["kind"])
	assert.Equal(t, float64(1), end["notes"])
}

func TestListJSONandJSONLConflict(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, _ := newCLITestService(t)
	override := ""

	c := cmd.NewListCmd(&svc, &override)
	c.SetArgs([]string{"--json", "--jsonl"})
	err := c.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestListIncludeFrontmatterRequiresJSONL(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, _ := newCLITestService(t)
	override := ""

	c := cmd.NewListCmd(&svc, &override)
	c.SetArgs([]string{"--include-frontmatter"})
	err := c.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--include-frontmatter requires --jsonl")
}

func TestListJSONShapeUnchanged(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, root := newCLITestService(t)
	override := ""

	inboxDir := filepath.Join(root, "notes", "inbox")
	seedNote(t, inboxDir, "note.md", "j1", "Test Note")

	c := cmd.NewListCmd(&svc, &override)
	c.SetArgs([]string{"inbox", "--json", "-g"})
	stdout := runCapturingStdout(t, c.Execute)

	var notes []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &notes))
	require.Len(t, notes, 1)
	assert.Equal(t, "j1", notes[0]["id"])
}

func TestListJSONLMalformedNote(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, root := newCLITestService(t)
	override := ""

	dir := filepath.Join(root, "notes", "hn", "clippings")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	seedNote(t, dir, "good.md", "g1", "Good Note")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.md"), []byte("---\n!!invalid yaml{{{\n---\n\nbody\n"), 0o644))

	c := cmd.NewListCmd(&svc, &override)
	c.SetArgs([]string{"hn/clippings", "--jsonl", "-g"})
	stdout := runCapturingStdout(t, c.Execute)

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	hasError := false
	hasNote := false
	for _, line := range lines {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		switch rec["kind"] {
		case "error":
			hasError = true
			assert.NotEmpty(t, rec["code"])
		case "note":
			hasNote = true
		}
	}
	assert.True(t, hasError, "malformed note should produce an error record")
	assert.True(t, hasNote, "good note should still be present")
}

func TestListGlobalFlagAndWorkspaceConflict(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, _ := newCLITestService(t)
	override := "/some/path"

	c := cmd.NewListCmd(&svc, &override)
	c.SetArgs([]string{"--jsonl", "-g"})
	err := c.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict")
}

// ---------------------------------------------------------------------------
// A1/A4: strict explicit -W resolution and the machine error channel
// ---------------------------------------------------------------------------

// runCapturingStdoutExpectError is runCapturingStdout's counterpart for
// invocations that MUST fail: it returns the captured stdout and the error.
func runCapturingStdoutExpectError(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	runErr := fn()

	require.NoError(t, w.Close())
	out, readErr := io.ReadAll(r)
	require.NoError(t, readErr)
	os.Stdout = orig
	require.Error(t, runErr, "invocation was expected to fail")
	return string(out), runErr
}

// seedClipNote writes a note with an idempotency key the structured update
// contract can validate against.
func seedClipNote(t *testing.T, dir, filename, id, key string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, filename)
	content := "---\nid: " + id + "\ntitle: Clip\naliases: []\ntags: []\ncreated: 2026-08-05T10:00:00Z\nmodified: 2026-08-05T10:00:00Z\nidempotency_key: " + key + "\n---\n\n# body\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestListStaleExplicitWorkspaceFailsClosed(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, root := newCLITestService(t)

	// A global note that must NOT leak through a failed -W resolution.
	seedNote(t, filepath.Join(root, "notes", "hn", "clippings"), "comments.md", "gn1", "Global Note")

	override := filepath.Join(t.TempDir(), "definitely", "not", "a", "workspace")
	c := cmd.NewListCmd(&svc, &override)
	c.SetArgs([]string{"hn/clippings", "--jsonl"})
	stdout, err := runCapturingStdoutExpectError(t, c.Execute)

	assert.Contains(t, err.Error(), "workspace_not_found")
	assert.NotContains(t, stdout, "\"kind\":\"note\"",
		"a stale -W must expose no notes (global or otherwise)")
}

func TestUpdateStaleExplicitWorkspaceFailsBeforeMutation(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, root := newCLITestService(t)

	notePath := seedClipNote(t, filepath.Join(root, "notes", "hn", "clippings"), "comments.md", "clip-1", "hn:111")
	before, err := os.ReadFile(notePath)
	require.NoError(t, err)

	override := filepath.Join(t.TempDir(), "gone", "workspace")
	c := cmd.NewUpdateCmd(&svc, &override)
	c.SetArgs([]string{
		"--json",
		"--path", notePath,
		"--expect-type", "hn/clippings",
		"--expect-idempotency-key", "hn:111",
		"--expect-filename", "comments.md",
	})
	stdout, runErr := runCapturingStdoutExpectError(t, c.Execute)
	assert.Contains(t, runErr.Error(), "workspace_not_found")

	// workspace_not_found is NOT one of the four typed update errors: the
	// machine channel stays silent and the failure surfaces to callers as a
	// generic (fail-closed) one — no create fallback may be inferred from it.
	assert.Empty(t, strings.TrimSpace(stdout),
		"a stale -W failure must not write a machine envelope on stdout")

	after, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Equal(t, before, after, "stale -W must fail before any mutation")
}

func TestUpdateStructuredSuccessOnGlobalRoute(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, root := newCLITestService(t)
	override := ""

	notePath := seedClipNote(t, filepath.Join(root, "notes", "hn", "clippings"), "comments.md", "clip-1", "hn:111")
	bodyFile := filepath.Join(t.TempDir(), "body.md")
	require.NoError(t, os.WriteFile(bodyFile, []byte("# refreshed\n"), 0o644))

	c := cmd.NewUpdateCmd(&svc, &override)
	c.SetArgs([]string{
		"--json",
		"--path", notePath,
		"--body-file", bodyFile,
		"-g",
		"--expect-type", "hn/clippings",
		"--expect-idempotency-key", "hn:111",
		"--expect-filename", "comments.md",
	})
	stdout := runCapturingStdout(t, c.Execute)

	_, path := parseReceipt(t, stdout)
	// The receipt carries the canonical (symlink-resolved) path; canonicalize
	// the seeded path too so macOS's /var -> /private/var alias compares equal.
	canonicalNotePath, err := filepath.EvalSymlinks(notePath)
	require.NoError(t, err)
	assert.Equal(t, canonicalNotePath, path)

	content, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "# refreshed")
}

func TestUpdateTypedFailureWritesOneStdoutEnvelope(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, root := newCLITestService(t)
	override := ""

	notePath := seedClipNote(t, filepath.Join(root, "notes", "hn", "clippings"), "comments.md", "clip-1", "hn:111")

	c := cmd.NewUpdateCmd(&svc, &override)
	c.SetArgs([]string{
		"--json",
		"--path", notePath,
		"-g",
		"--expect-type", "hn/clippings",
		"--expect-idempotency-key", "hn:222", // wrong: persisted is hn:111
		"--expect-filename", "comments.md",
	})
	stdout, runErr := runCapturingStdoutExpectError(t, c.Execute)

	trimmed := strings.TrimSpace(stdout)
	require.NotEmpty(t, trimmed, "typed failure must write the stdout envelope")
	require.NotContains(t, trimmed, "\n", "stdout must carry exactly one envelope line, got:\n%s", stdout)
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(trimmed), &env))
	assert.Equal(t, "note_identity_mismatch", env.Error.Code)
	assert.NotEmpty(t, env.Error.Message)
	assert.Contains(t, runErr.Error(), "note_identity_mismatch")
}

func TestUpdateExpectFlagsRequiredTogether(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, root := newCLITestService(t)
	override := ""

	notePath := seedClipNote(t, filepath.Join(root, "notes", "hn", "clippings"), "comments.md", "clip-1", "hn:111")

	c := cmd.NewUpdateCmd(&svc, &override)
	c.SetArgs([]string{
		"--json",
		"--path", notePath,
		"-g",
		"--expect-type", "hn/clippings", // missing the other two
	})
	err := c.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be provided together")
}

// ---------------------------------------------------------------------------
// Phase E: strict -W on create, explicit-workspace scans, route separation,
// disk-hydrated frontmatter, and producer-string serialization conformance
// ---------------------------------------------------------------------------

// makeWorkspaceDir fabricates the smallest directory GetProjectByPath
// classifies as a workspace: a named dir carrying a .git marker. The name is
// what nb keys the workspace's notebook subtree on (workspaces/<name>).
func makeWorkspaceDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	return dir
}

func TestCreateStaleExplicitWorkspaceFailsClosed(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, root := newCLITestService(t)
	fmFile, bodyFile := writePomodoroFixtures(t, t.TempDir(), "pending")

	override := filepath.Join(t.TempDir(), "definitely", "gone")
	c := cmd.NewNewCmd(&svc, &override)
	c.SetArgs([]string{
		"--json",
		"--type", "hn/clippings",
		"--title", "Stale Route Clip",
		"--idempotency-key", "hn:404",
		"--filename", "comments.md",
		"--frontmatter-file", fmFile,
		"--body-file", bodyFile,
		"--no-edit",
	})
	stdout, runErr := runCapturingStdoutExpectError(t, c.Execute)

	assert.Contains(t, runErr.Error(), "workspace_not_found")
	assert.Empty(t, strings.TrimSpace(stdout), "a failed create must emit no receipt")

	// Nothing may have been written into the global notebook: the historical
	// bug was exactly this silent fall-through.
	_, statErr := os.Stat(filepath.Join(root, "notes", "hn", "clippings"))
	assert.True(t, os.IsNotExist(statErr),
		"a stale -W create must not write into the global notebook")
}

func TestCreateGlobalTokenOverrideStillForcesGlobal(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, root := newCLITestService(t)
	override := "global" // the historical forced-global token, not a path

	c := cmd.NewNewCmd(&svc, &override)
	c.SetArgs([]string{"--json", "--type", "inbox", "--title", "Token Note", "--no-edit"})
	stdout := runCapturingStdout(t, c.Execute)

	_, path := parseReceipt(t, stdout)
	assert.True(t, strings.HasPrefix(path, filepath.Join(root, "notes")+string(filepath.Separator)),
		"override \"global\" must keep resolving into the global notebook, got %q", path)
}

func TestCreateGlobalFlagAndWorkspaceConflict(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, _ := newCLITestService(t)
	override := "/some/workspace/path"

	c := cmd.NewNewCmd(&svc, &override)
	c.SetArgs([]string{"--json", "--type", "inbox", "--title", "Conflicted", "--no-edit", "-g"})
	err := c.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict")
}

func TestListJSONLExplicitWorkspaceScans(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, root := newCLITestService(t)
	wsDir := makeWorkspaceDir(t, "ws-alpha")
	fmFile, bodyFile := writePomodoroFixtures(t, t.TempDir(), "pending")

	// Create THROUGH the workspace route, so the test learns where the
	// workspace's clippings actually land instead of assuming a layout.
	override := wsDir
	create := cmd.NewNewCmd(&svc, &override)
	create.SetArgs([]string{
		"--json",
		"--type", "hn/clippings",
		"--title", "Workspace Clip",
		"--idempotency-key", "hn:42",
		"--filename", "comments.md",
		"--frontmatter-file", fmFile,
		"--body-file", bodyFile,
		"--no-edit",
	})
	stdout := runCapturingStdout(t, create.Execute)
	_, wsPath := parseReceipt(t, stdout)
	assert.False(t, strings.HasPrefix(wsPath, filepath.Join(root, "notes")+string(filepath.Separator)),
		"a workspace-routed create must not land in the global notebook, got %q", wsPath)

	// A global note that must NOT appear in the workspace scan.
	seedNote(t, filepath.Join(root, "notes", "hn", "clippings"), "comments.md", "gn1", "Global Clip")

	list := cmd.NewListCmd(&svc, &override)
	list.SetArgs([]string{"hn/clippings", "--jsonl", "--include-frontmatter"})
	listOut := runCapturingStdout(t, list.Execute)

	lines := strings.Split(strings.TrimSpace(listOut), "\n")
	require.GreaterOrEqual(t, len(lines), 3, "expected catalog + note + end")
	var notePaths []string
	for _, line := range lines {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		if rec["kind"] == "note" {
			notePaths = append(notePaths, rec["receipt_path"].(string))
		}
	}
	require.Len(t, notePaths, 1, "the workspace scan must see exactly the workspace note")
	assert.Equal(t, wsPath, notePaths[0])

	var end map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &end))
	assert.Equal(t, "end", end["kind"])
	assert.Equal(t, true, end["complete"])
}

func TestListJSONLSameHNIDDistinctRoutes(t *testing.T) {
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, root := newCLITestService(t)
	wsDir := makeWorkspaceDir(t, "ws-beta")
	fmFile, bodyFile := writePomodoroFixtures(t, t.TempDir(), "pending")

	// The SAME story (same idempotency key, same bundle shape) delivered to
	// the workspace route and to the global route.
	wsOverride := wsDir
	wsCreate := cmd.NewNewCmd(&svc, &wsOverride)
	wsCreate.SetArgs([]string{
		"--json", "--type", "hn/clippings/44556677-story", "--title", "Same Story",
		"--idempotency-key", "hn:44556677", "--filename", "comments.md",
		"--frontmatter-file", fmFile, "--body-file", bodyFile, "--no-edit",
	})
	_, wsPath := parseReceipt(t, runCapturingStdout(t, wsCreate.Execute))

	gOverride := ""
	gCreate := cmd.NewNewCmd(&svc, &gOverride)
	gCreate.SetArgs([]string{
		"--json", "--type", "hn/clippings/44556677-story", "--title", "Same Story",
		"--idempotency-key", "hn:44556677", "--filename", "comments.md",
		"--frontmatter-file", fmFile, "--body-file", bodyFile, "--no-edit", "-g",
	})
	_, gPath := parseReceipt(t, runCapturingStdout(t, gCreate.Execute))

	require.NotEqual(t, wsPath, gPath, "two routes must hold two independent notes")
	assert.True(t, strings.HasPrefix(gPath, filepath.Join(root, "notes")+string(filepath.Separator)))

	collect := func(args []string, override string) []string {
		c := cmd.NewListCmd(&svc, &override)
		c.SetArgs(args)
		out := runCapturingStdout(t, c.Execute)
		var paths []string
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			var rec map[string]any
			require.NoError(t, json.Unmarshal([]byte(line), &rec))
			if rec["kind"] == "note" {
				paths = append(paths, rec["receipt_path"].(string))
			}
		}
		return paths
	}

	wsPaths := collect([]string{"hn/clippings", "--jsonl", "--include-frontmatter"}, wsDir)
	require.Len(t, wsPaths, 1, "workspace route must see only its own copy")
	assert.Equal(t, wsPath, wsPaths[0])

	gPaths := collect([]string{"hn/clippings", "--jsonl", "--include-frontmatter", "-g"}, "")
	require.Len(t, gPaths, 1, "global route must see only its own copy")
	assert.Equal(t, gPath, gPaths[0])
}

func TestListJSONLReflectsExternalEditsDespiteStaleDaemon(t *testing.T) {
	// The daemon notifier is stubbed to swallow every event, so as far as any
	// daemon index could know, the note below never changed after creation.
	// JSONL discovery reads every note from DISK, so the external edit must be
	// visible anyway — that disk hydration is the contract this test pins.
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, _ := newCLITestService(t)
	override := ""
	fmFile, bodyFile := writePomodoroFixtures(t, t.TempDir(), "pending")

	create := cmd.NewNewCmd(&svc, &override)
	create.SetArgs([]string{
		"--json", "--type", "hn/clippings", "--title", "Editable",
		"--idempotency-key", "hn:edit", "--filename", "comments.md",
		"--frontmatter-file", fmFile, "--body-file", bodyFile, "--no-edit", "-g",
	})
	_, notePath := parseReceipt(t, runCapturingStdout(t, create.Execute))

	// An EXTERNAL editor rewrites the note: nb is not told (and the stub
	// guarantees no daemon heard anything).
	content, err := os.ReadFile(notePath)
	require.NoError(t, err)
	edited := strings.Replace(string(content),
		`pomodoro_summary_status: "pending"`,
		`pomodoro_summary_status: "edited-elsewhere"`, 1)
	require.NotEqual(t, string(content), edited, "the fixture edit must apply")
	require.NoError(t, os.WriteFile(notePath, []byte(edited), 0o644))

	list := cmd.NewListCmd(&svc, &override)
	list.SetArgs([]string{"hn/clippings", "--jsonl", "--include-frontmatter", "-g"})
	out := runCapturingStdout(t, list.Execute)

	sawEdited := false
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var rec struct {
			Kind        string         `json:"kind"`
			Frontmatter map[string]any `json:"frontmatter"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		if rec.Kind == "note" && rec.Frontmatter["pomodoro_summary_status"] == "edited-elsewhere" {
			sawEdited = true
		}
	}
	assert.True(t, sawEdited, "an external edit must be visible to JSONL discovery without daemon help")
}

func TestProducerStringVerbatimOnDiskPlainInJSONL(t *testing.T) {
	// Serialization conformance for the panel contract (nb 30d8567 made the
	// Extra passthrough verbatim): a producer string like hn_extraction "ok"
	// keeps its quoting ON DISK, while the JSONL frontmatter map decodes it
	// back to the PLAIN string. The hn panel's smoke test asserts the on-disk
	// half; this test keeps both halves from drifting inside nb itself.
	restore := service.StubDaemonNotifierForTests(func(coremodels.NoteEvent) {})
	defer restore()

	svc, _ := newCLITestService(t)
	override := ""

	fmFile := filepath.Join(t.TempDir(), "fm.json")
	require.NoError(t, os.WriteFile(fmFile, []byte(`{"hn_extraction":"ok","hn_id":44556677}`), 0o644))
	bodyFile := filepath.Join(t.TempDir(), "body.md")
	require.NoError(t, os.WriteFile(bodyFile, []byte("# article\n"), 0o644))

	create := cmd.NewNewCmd(&svc, &override)
	create.SetArgs([]string{
		"--json", "--type", "hn/clippings", "--title", "Graded",
		"--idempotency-key", "hn:grade", "--filename", "article.md",
		"--frontmatter-file", fmFile, "--body-file", bodyFile, "--no-edit", "-g",
	})
	_, notePath := parseReceipt(t, runCapturingStdout(t, create.Execute))

	onDisk, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Contains(t, string(onDisk), `hn_extraction: "ok"`,
		"producer strings serialize verbatim (JSON quoting intact) on disk")
	assert.Contains(t, string(onDisk), "hn_id: 44556677",
		"producer numbers serialize verbatim (unquoted) on disk")

	list := cmd.NewListCmd(&svc, &override)
	list.SetArgs([]string{"hn/clippings", "--jsonl", "--include-frontmatter", "-g"})
	out := runCapturingStdout(t, list.Execute)

	sawNote := false
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var rec struct {
			Kind        string         `json:"kind"`
			Frontmatter map[string]any `json:"frontmatter"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		if rec.Kind != "note" {
			continue
		}
		sawNote = true
		assert.Equal(t, "ok", rec.Frontmatter["hn_extraction"],
			"the JSONL frontmatter map must decode the string PLAIN, not doubly quoted")
		assert.Equal(t, float64(44556677), rec.Frontmatter["hn_id"],
			"the JSONL frontmatter map must decode the number as a number")
	}
	require.True(t, sawNote)
}
