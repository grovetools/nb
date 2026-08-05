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
	assert.Equal(t, "blk-1", fm.Extra["pomodoro_block_id"])
	assert.Equal(t, "pomodoro:blk-1", fm.Extra[frontmatter.IdempotencyKeyField])
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
	updateCmd := cmd.NewUpdateCmd(&svc)
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
	assert.Equal(t, "final", fm.Extra["pomodoro_summary_status"])
	assert.Equal(t, "pomodoro:blk-1", fm.Extra[frontmatter.IdempotencyKeyField], "the key survives updates")
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
