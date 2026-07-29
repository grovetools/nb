package views

import (
	"testing"
	"time"

	workspace "github.com/grovetools/core/pkg/workspace"

	"github.com/grovetools/nb/pkg/models"
	"github.com/grovetools/nb/pkg/service"
	"github.com/grovetools/nb/pkg/tree"
)

// newEcoTestModel builds a Model focused on an ecosystem that owns two sub-repo
// workspaces, each carrying one note, plus a note of the ecosystem's own.
func newEcoTestModel(t *testing.T) (*Model, *workspace.WorkspaceNode, []*workspace.WorkspaceNode) {
	t.Helper()
	svc, err := service.New(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}

	eco := &workspace.WorkspaceNode{
		Name: "grovetools",
		Path: "/tmp/eco",
		Kind: workspace.KindEcosystemRoot,
	}
	repos := []*workspace.WorkspaceNode{
		{
			Name:                "agent",
			Path:                "/tmp/eco/agent",
			Kind:                workspace.KindEcosystemSubProject,
			ParentEcosystemPath: eco.Path,
			Depth:               1,
		},
		{
			Name:                "cx",
			Path:                "/tmp/eco/cx",
			Kind:                workspace.KindEcosystemSubProject,
			ParentEcosystemPath: eco.Path,
			Depth:               1,
		},
	}

	all := append([]*workspace.WorkspaceNode{eco}, repos...)
	m := &Model{
		collapsedNodes:   map[string]bool{},
		seededCollapse:   map[string]bool{},
		service:          svc,
		groupBy:          "none",
		workspaces:       all,
		focusedWorkspace: eco,
	}

	note := func(ws *workspace.WorkspaceNode, group, filename string) *tree.Item {
		return noteToItem(&models.Note{
			Path:      ws.Path + "/nb/" + group + "/" + filename,
			Title:     filename,
			Group:     group,
			Workspace: ws.Name,
			CreatedAt: time.Now(),
		})
	}
	m.allItems = []*tree.Item{
		note(eco, "inbox", "eco-note.md"),
		note(repos[0], "inbox", "agent-note.md"),
		note(repos[1], "inbox", "cx-note.md"),
	}
	return m, eco, repos
}

func findRepoNotesNode(m *Model) *DisplayNode {
	for _, n := range m.displayNodes {
		if n.IsRepoNotes() {
			return n
		}
	}
	return nil
}

func workspaceRowNames(m *Model) []string {
	var names []string
	for _, n := range m.displayNodes {
		if n.IsWorkspace() {
			names = append(names, n.Item.Name)
		}
	}
	return names
}

// Opening a notebook lands on the ecosystem's own groups plus one collapsed
// repos row — not on every repo.
func TestReposContainerCollapsedOnFirstOpen(t *testing.T) {
	m, eco, _ := newEcoTestModel(t)
	m.BuildDisplayTree()

	container := findRepoNotesNode(m)
	if container == nil {
		t.Fatalf("no %q container in tree", RepoNotesGroupName)
	}
	if !m.collapsedNodes[repoNotesNodeID(eco.Path)] {
		t.Error("repos container should be collapsed on first sight")
	}
	if got := workspaceRowNames(m); len(got) != 1 || got[0] != eco.Name {
		t.Errorf("workspace rows = %v, want only %q", got, eco.Name)
	}

	// A user toggle survives later rebuilds — the seed fires once.
	delete(m.collapsedNodes, repoNotesNodeID(eco.Path))
	m.BuildDisplayTree()
	m.BuildDisplayTree()
	if len(workspaceRowNames(m)) != 3 {
		t.Errorf("workspace rows after expanding = %v, want the ecosystem + 2 repos", workspaceRowNames(m))
	}
}

// The ecosystem's sub-repos render under a single "repos_notes" container that
// sits with the ecosystem's own groups, one level above the repos themselves.
func TestReposRenderUnderContainer(t *testing.T) {
	m, eco, repos := newEcoTestModel(t)
	m.BuildDisplayTree()
	// The container is collapsed by default; this test is about what it holds.
	delete(m.collapsedNodes, repoNotesNodeID(eco.Path))
	m.BuildDisplayTree()

	container := findRepoNotesNode(m)
	if container == nil {
		t.Fatalf("no %q container in tree; rows: %v", RepoNotesGroupName, workspaceRowNames(m))
	}
	if container.Item.Name != RepoNotesGroupName {
		t.Errorf("container name = %q, want %q", container.Item.Name, RepoNotesGroupName)
	}
	if container.ChildCount != len(repos) {
		t.Errorf("container ChildCount = %d, want %d", container.ChildCount, len(repos))
	}
	if container.Depth != eco.Depth+1 {
		t.Errorf("container depth = %d, want ecosystem depth + 1 = %d", container.Depth, eco.Depth+1)
	}
	if !container.IsFoldable() {
		t.Error("container must be foldable")
	}
	// It is not a note-bearing row: note actions must not resolve against it.
	if container.IsGroup() || container.IsWorkspace() || container.IsNote() {
		t.Error("container must be neither group, workspace, nor note")
	}

	// Every repo row sits below the container, deeper than it.
	seenContainer := false
	seenRepos := 0
	for _, n := range m.displayNodes {
		if n.IsRepoNotes() {
			seenContainer = true
			continue
		}
		if !n.IsWorkspace() || n.Item.Name == eco.Name {
			continue
		}
		seenRepos++
		if !seenContainer {
			t.Errorf("repo %q rendered before the %q container", n.Item.Name, RepoNotesGroupName)
		}
		if n.Depth != container.Depth+1 {
			t.Errorf("repo %q depth = %d, want %d", n.Item.Name, n.Depth, container.Depth+1)
		}
	}
	if seenRepos != len(repos) {
		t.Errorf("saw %d repo rows, want %d", seenRepos, len(repos))
	}
}

// The reported bug: folding the ecosystem hid its own notes but left the repos
// on screen. Now it folds the repos away too.
func TestCollapsedEcosystemHidesRepos(t *testing.T) {
	m, eco, _ := newEcoTestModel(t)
	m.collapsedNodes["dir:"+eco.Path] = true
	m.BuildDisplayTree()

	if got := workspaceRowNames(m); len(got) != 1 || got[0] != eco.Name {
		t.Errorf("workspace rows = %v, want only %q", got, eco.Name)
	}
	if findRepoNotesNode(m) != nil {
		t.Errorf("collapsed ecosystem still renders its %q container", RepoNotesGroupName)
	}
	for _, n := range m.displayNodes {
		if n.IsNote() {
			t.Errorf("collapsed ecosystem still renders note %q", n.Item.Path)
		}
	}
}

// Folding the container alone keeps the ecosystem's own notes visible.
func TestCollapsedContainerHidesOnlyRepos(t *testing.T) {
	m, eco, _ := newEcoTestModel(t)
	m.collapsedNodes[repoNotesNodeID(eco.Path)] = true
	m.BuildDisplayTree()

	if got := workspaceRowNames(m); len(got) != 1 || got[0] != eco.Name {
		t.Errorf("workspace rows = %v, want only %q", got, eco.Name)
	}
	container := findRepoNotesNode(m)
	if container == nil {
		t.Fatalf("collapsed container row disappeared entirely")
	}
	if container.ChildCount != 2 {
		t.Errorf("collapsed container ChildCount = %d, want 2", container.ChildCount)
	}
	// The ecosystem's own inbox group still renders.
	var groups []string
	for _, n := range m.displayNodes {
		if n.IsGroup() {
			groups = append(groups, n.Item.Name)
		}
	}
	if len(groups) == 0 {
		t.Error("collapsing the repos container also hid the ecosystem's own groups")
	}
}

// Rebuilding must not compound the extra depth level onto the shared workspace
// nodes: repos stay one level under the container however often we rebuild.
func TestRepoDepthStableAcrossRebuilds(t *testing.T) {
	m, eco, repos := newEcoTestModel(t)
	m.BuildDisplayTree()
	delete(m.collapsedNodes, repoNotesNodeID(eco.Path))
	m.BuildDisplayTree()
	m.BuildDisplayTree()
	m.BuildDisplayTree()

	container := findRepoNotesNode(m)
	if container == nil {
		t.Fatal("no repos container after repeated rebuilds")
	}
	seenRepos := 0
	for _, n := range m.displayNodes {
		if n.IsWorkspace() && n.Item.Name != "grovetools" {
			seenRepos++
			if n.Depth != container.Depth+1 {
				t.Errorf("repo %q depth = %d after rebuilds, want %d", n.Item.Name, n.Depth, container.Depth+1)
			}
		}
	}
	if seenRepos != len(repos) {
		t.Errorf("saw %d repo rows after rebuilds, want %d", seenRepos, len(repos))
	}
	for _, ws := range repos {
		if ws.Depth != 1 {
			t.Errorf("shared workspace %q depth mutated to %d, want 1", ws.Name, ws.Depth)
		}
	}
}

// A search filter ignores collapse state, so repos (and their container) stay
// reachable while filtering even with everything folded shut.
func TestSearchFilterIgnoresContainerCollapse(t *testing.T) {
	m, eco, _ := newEcoTestModel(t)
	m.collapsedNodes["dir:"+eco.Path] = true
	m.collapsedNodes[repoNotesNodeID(eco.Path)] = true
	m.filterValue = "cx-note"
	m.BuildDisplayTree()
	m.FilterDisplayTree()

	var found bool
	for _, n := range m.displayNodes {
		if n.IsNote() && n.Item.Name == "cx-note.md" {
			found = true
		}
	}
	if !found {
		t.Error("search did not reach a note inside a collapsed repo")
	}
	if findRepoNotesNode(m) == nil {
		t.Error("filtered tree dropped the repos container ancestor")
	}
}

// The treemux drawer jump targets a sub-repo group; the repo row only exists
// once its ecosystem AND the repos container are re-opened. Run on a model that
// has never rendered — the jump can arrive right after a focus change, before
// any default-collapse seed has fired, which is the case that needs
// revealGroupRow's two-pass expand.
func TestRevealGroupReachesRepoWorkspace(t *testing.T) {
	m, _, repos := newEcoTestModel(t)

	if !m.RevealGroup(repos[1].Name, "inbox", "") {
		t.Fatalf("RevealGroup(%q, inbox) failed; rows: %v", repos[1].Name, workspaceRowNames(m))
	}
	node := m.GetCurrentNode()
	if node == nil || !node.IsGroup() {
		t.Fatalf("cursor did not land on a group row: %+v", node)
	}
	if got := node.GroupKey(); got != repos[1].Name+":inbox" {
		t.Errorf("cursor group key = %q, want %q", got, repos[1].Name+":inbox")
	}
}
