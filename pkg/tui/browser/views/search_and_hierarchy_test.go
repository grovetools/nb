package views

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	workspace "github.com/grovetools/core/pkg/workspace"

	"github.com/grovetools/nb/pkg/models"
	"github.com/grovetools/nb/pkg/service"
	"github.com/grovetools/nb/pkg/tree"
)

// newTreeTestModel builds a Model wired with a minimal real service (default
// NoteTypes + locator) and a single focused workspace, ready for
// BuildDisplayTree-driven tests.
func newTreeTestModel(t *testing.T) (*Model, *workspace.WorkspaceNode) {
	t.Helper()
	svc, err := service.New(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	ws := &workspace.WorkspaceNode{Name: "demo", Path: "/tmp/ws", Depth: 0}
	m := &Model{
		collapsedNodes:   map[string]bool{},
		seededCollapse:   map[string]bool{},
		service:          svc,
		groupBy:          "none",
		workspaces:       []*workspace.WorkspaceNode{ws},
		focusedWorkspace: ws,
	}
	return m, ws
}

func testNoteItem(group, filename, fmTitle string, aliases, tags []string) *tree.Item {
	return noteToItem(&models.Note{
		Path:             "/tmp/ws/nb/" + group + "/" + filename,
		Title:            filename,
		FrontmatterTitle: fmTitle,
		Aliases:          aliases,
		Tags:             tags,
		Group:            group,
		Workspace:        "demo",
		CreatedAt:        time.Now(),
	})
}

func visibleNotePaths(m *Model) []string {
	var paths []string
	for _, n := range m.displayNodes {
		if n.IsNote() {
			paths = append(paths, n.Item.Path)
		}
	}
	return paths
}

// C2: the plain substring filter must match frontmatter title, aliases, and
// tags — not just the filename.
func TestFilterDisplayTreeMatchesWiderCorpus(t *testing.T) {
	m, _ := newTreeTestModel(t)
	vault := testNoteItem("alpha", "20260701-vi.md", "Vault indexer", []string{"vidx"}, []string{"infra"})
	other := testNoteItem("alpha", "other-note.md", "", nil, nil)
	m.allItems = []*tree.Item{vault, other}

	cases := []struct {
		filter string
		want   []string
	}{
		{"vault", []string{vault.Path}},    // frontmatter title
		{"vidx", []string{vault.Path}},     // alias
		{"infra", []string{vault.Path}},    // tag
		{"20260701", []string{vault.Path}}, // filename
		{"other-note", []string{other.Path}},
		{"no-such-thing", nil},
	}
	for _, tc := range cases {
		m.filterValue = tc.filter
		m.BuildDisplayTree()
		m.FilterDisplayTree()
		if got := visibleNotePaths(m); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("filter %q: got notes %v, want %v", tc.filter, got, tc.want)
		}
	}
}

// C1: grep mode must feed the searcher's result paths into the tree filter —
// only matching notes (plus their ancestors) survive.
func TestApplyGrepFilterUsesSearcherResults(t *testing.T) {
	m, _ := newTreeTestModel(t)
	hit := testNoteItem("alpha", "hit.md", "", nil, nil)
	miss := testNoteItem("alpha", "miss.md", "", nil, nil)
	m.allItems = []*tree.Item{hit, miss}

	orig := grepSearcher
	defer func() { grepSearcher = orig }()

	var gotQuery string
	var gotDirs []string
	grepSearcher = func(query string, dirs []string) ([]string, error) {
		gotQuery = query
		gotDirs = dirs
		return []string{hit.Path}, nil
	}

	m.filterValue = "needle"
	m.isGrepping = true
	msg, err := m.ApplyGrepFilter()
	if err != nil {
		t.Fatalf("ApplyGrepFilter: %v", err)
	}
	if gotQuery != "needle" {
		t.Errorf("searcher got query %q, want %q", gotQuery, "needle")
	}
	if len(gotDirs) == 0 {
		t.Errorf("searcher got no search dirs")
	}
	if got := visibleNotePaths(m); !reflect.DeepEqual(got, []string{hit.Path}) {
		t.Errorf("got notes %v, want only %v", got, hit.Path)
	}
	if !strings.Contains(msg, "1") {
		t.Errorf("status message %q should report 1 match", msg)
	}
}

// C1: a searcher error must surface instead of silently pruning everything.
func TestApplyGrepFilterSurfacesSearcherError(t *testing.T) {
	m, _ := newTreeTestModel(t)
	m.allItems = []*tree.Item{testNoteItem("alpha", "a.md", "", nil, nil)}

	orig := grepSearcher
	defer func() { grepSearcher = orig }()
	grepSearcher = func(query string, dirs []string) ([]string, error) {
		return nil, fmt.Errorf("boom")
	}

	m.filterValue = "needle"
	if _, err := m.ApplyGrepFilter(); err == nil {
		t.Fatal("expected error from ApplyGrepFilter")
	}
}

// C3: accepting a search moves the cursor to the first NOTE node, not an
// ancestor group or workspace row.
func TestCursorToFirstNote(t *testing.T) {
	m, _ := newTreeTestModel(t)
	m.allItems = []*tree.Item{testNoteItem("alpha", "a.md", "", nil, nil)}
	m.BuildDisplayTree()
	m.cursor = 0

	if !m.CursorToFirstNote() {
		t.Fatal("CursorToFirstNote returned false on a non-empty tree")
	}
	if node := m.displayNodes[m.cursor]; !node.IsNote() {
		t.Errorf("cursor landed on non-note node %+v", node.Item)
	}

	m.displayNodes = nil
	if m.CursorToFirstNote() {
		t.Error("CursorToFirstNote should return false on an empty tree")
	}
}

// C5: with groupBy=none and no filters, the sequence of group nodes for a
// workspace equals the sorted set of on-disk note dirs, nested subdirectories
// render nested, and no real directory is renamed in display.
func TestHierarchyMirrorsDiskDirs(t *testing.T) {
	m, _ := newTreeTestModel(t)
	// Names deliberately absent from the NoteTypes registry so ordering is the
	// plain alphabetical (= sorted on-disk) order, including a two-level dir.
	dirs := []string{"zeta", "alpha/nested", "beta", "alpha"}
	var items []*tree.Item
	for i, d := range dirs {
		items = append(items, testNoteItem(d, fmt.Sprintf("note-%d.md", i), "", nil, nil))
	}
	m.allItems = items
	m.BuildDisplayTree()

	var groups []string
	depthByGroup := map[string]int{}
	labelByGroup := map[string]string{}
	for _, n := range m.displayNodes {
		if !n.IsGroup() {
			continue
		}
		g, _ := n.Item.Metadata["Group"].(string)
		groups = append(groups, g)
		depthByGroup[g] = n.Depth
		labelByGroup[g] = m.getNodeRenderInfo(n).name
	}

	want := []string{"alpha", "alpha/nested", "beta", "zeta"}
	if !reflect.DeepEqual(groups, want) {
		t.Fatalf("group sequence = %v, want sorted on-disk dirs %v", groups, want)
	}
	if depthByGroup["alpha/nested"] != depthByGroup["alpha"]+1 {
		t.Errorf("nested dir depth = %d, want parent depth %d + 1",
			depthByGroup["alpha/nested"], depthByGroup["alpha"])
	}
	// Display labels must be the raw last path segment — no relabeling of real
	// on-disk directories.
	wantLabels := map[string]string{"alpha": "alpha", "alpha/nested": "nested", "beta": "beta", "zeta": "zeta"}
	for g, wantLabel := range wantLabels {
		if labelByGroup[g] != wantLabel {
			t.Errorf("group %q displayed as %q, want %q", g, labelByGroup[g], wantLabel)
		}
	}
}

// C4: STATUS cell formatting for todo-aware notes.
func TestGetNoteStatusTodoCells(t *testing.T) {
	cases := []struct {
		name string
		note models.Note
		want string
	}{
		{"open remaining", models.Note{TodoOpen: 3, TodoDone: 2}, "☐ 2/5"},
		{"all done", models.Note{TodoDone: 5}, "✓ 5/5"},
		{"all done with cancelled", models.Note{TodoDone: 3, TodoCancelled: 2}, "✓ 3/3 (2 ✕)"},
		{"open with cancelled", models.Note{TodoOpen: 1, TodoDone: 1, TodoCancelled: 1}, "☐ 1/2 (1 ✕)"},
		{"no todos", models.Note{}, ""},
		{"only cancelled stays empty", models.Note{TodoCancelled: 2}, ""},
		{"remote state wins", models.Note{TodoOpen: 1, Remote: &models.RemoteMetadata{State: "open"}}, "open"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := getNoteStatus(&tc.note); got != tc.want {
				t.Errorf("getNoteStatus = %q, want %q", got, tc.want)
			}
		})
	}
}
