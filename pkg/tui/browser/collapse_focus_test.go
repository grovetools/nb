package browser

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/grovetools/core/pkg/workspace"

	"github.com/grovetools/nb/pkg/service"
	"github.com/grovetools/nb/pkg/tree"
	"github.com/grovetools/nb/pkg/tui/browser/views"
)

// newCollapseTestModel builds a browser Model over an ecosystem with two
// sub-repos plus a standalone project, enough for setCollapseStateForFocus to
// walk m.workspaces.
func newCollapseTestModel() (*Model, []*workspace.WorkspaceNode) {
	wss := []*workspace.WorkspaceNode{
		{Name: "global", Path: "::global"},
		{Name: "grovetools", Path: "/tmp/eco", Kind: workspace.KindEcosystemRoot},
		{Name: "agent", Path: "/tmp/eco/agent", Kind: workspace.KindEcosystemSubProject, ParentEcosystemPath: "/tmp/eco", Depth: 1},
		{Name: "cx", Path: "/tmp/eco/cx", Kind: workspace.KindEcosystemSubProject, ParentEcosystemPath: "/tmp/eco", Depth: 1},
		{Name: "solo", Path: "/tmp/solo", Kind: workspace.KindStandaloneProject},
	}
	vm := views.New(views.KeyMap{}, map[string]bool{})
	m := &Model{
		service:     &service.Service{},
		workspaces:  wss,
		filterInput: textinput.New(),
		views:       vm,
	}
	return m, wss
}

// Every group of the focused workspace starts folded — including the parent
// rows a nested group implies. Notes only carry their innermost group
// ("plans/rolling"), so folding just that left "plans" itself open and the
// workspace opened one level deep. Runs for both focus kinds: an ecosystem and
// a plain repo take the same two branches of setCollapseStateForFocus.
func TestSetCollapseStateForFocusFoldsEveryGroup(t *testing.T) {
	svc, err := service.New(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}

	// Innermost groups as notes carry them, and every row they imply.
	noteGroups := []string{"inbox", "plans/rolling", "plans/phase3", "concepts/vision", "skills/dev-guide", "context"}
	wantFolded := []string{"inbox", "plans", "plans/rolling", "plans/phase3", "concepts", "concepts/vision", "skills", "skills/dev-guide", "context"}

	for _, tc := range []struct{ name, focus string }{
		{"ecosystem focus", "grovetools"},
		{"leaf repo focus", "solo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newCollapseTestModel()
			m.service = svc
			ws := m.workspaceNodeByName(tc.focus)
			m.focusedWorkspace = ws

			for _, g := range noteGroups {
				m.allItems = append(m.allItems, &tree.Item{
					Path: ws.Path + "/nb/" + g + "/n.md",
					Name: "n.md",
					Type: tree.TypeNote,
					Metadata: map[string]interface{}{
						"Workspace": ws.Name,
						"Group":     g,
					},
				})
			}

			m.setCollapseStateForFocus()

			state := m.views.GetCollapseState()
			for _, g := range wantFolded {
				dir, err := svc.GetNotebookLocator().GetGroupDir(ws, g)
				if err != nil {
					t.Fatalf("GetGroupDir(%q): %v", g, err)
				}
				if !state["dir:"+dir] {
					t.Errorf("group %q is expanded on first open, want folded", g)
				}
			}
			if state["dir:"+ws.Path] {
				t.Error("focused workspace itself must stay expanded")
			}
		})
	}
}

// The global view is meant to open as a clean overview with every workspace
// folded. The helper used to write into the live collapse map, which the fresh
// map installed at the end of setCollapseStateForFocus then discarded — so
// nothing was collapsed at all.
func TestSetCollapseStateForFocusGlobalCollapsesEveryWorkspace(t *testing.T) {
	m, wss := newCollapseTestModel()
	m.focusedWorkspace = nil

	m.setCollapseStateForFocus()

	state := m.views.GetCollapseState()
	for _, ws := range wss {
		if !state["dir:"+ws.Path] {
			t.Errorf("workspace %q not collapsed in global view", ws.Name)
		}
	}
}

// Focusing an ecosystem collapses its sub-repos (they render under the
// "repos_notes" container) while leaving the ecosystem itself expanded.
func TestSetCollapseStateForFocusEcosystemCollapsesRepos(t *testing.T) {
	m, wss := newCollapseTestModel()
	m.focusedWorkspace = wss[1] // grovetools

	m.setCollapseStateForFocus()

	state := m.views.GetCollapseState()
	if state["dir:/tmp/eco"] {
		t.Error("focused ecosystem must stay expanded")
	}
	for _, path := range []string{"/tmp/eco/agent", "/tmp/eco/cx"} {
		if !state["dir:"+path] {
			t.Errorf("sub-repo %q not collapsed on ecosystem focus", path)
		}
	}
	// Workspaces outside the ecosystem are none of this branch's business.
	if state["dir:/tmp/solo"] {
		t.Error("unrelated standalone workspace was collapsed")
	}
}
