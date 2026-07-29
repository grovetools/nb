package views

import (
	"strings"
	"testing"
	"time"

	workspace "github.com/grovetools/core/pkg/workspace"

	"github.com/grovetools/nb/pkg/models"
	"github.com/grovetools/nb/pkg/service"
	"github.com/grovetools/nb/pkg/tree"
)

// newPlanArtifactModel builds a model holding one plan ("demo") with two job
// notes, one of which (01-job.md → job-abc) owns artifacts both directly and in
// a nested "workflows/" dir, plus an orphaned artifact dir owned by no job, plus
// a loose file sitting in plans/ itself.
func newPlanArtifactModel(t *testing.T) (*Model, *workspace.WorkspaceNode) {
	t.Helper()
	svc, err := service.New(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	ws := &workspace.WorkspaceNode{Name: "grovetools", Path: "/tmp/eco", Kind: workspace.KindEcosystemRoot}
	m := &Model{
		collapsedNodes:   map[string]bool{},
		seededCollapse:   map[string]bool{},
		service:          svc,
		groupBy:          "none",
		workspaces:       []*workspace.WorkspaceNode{ws},
		focusedWorkspace: ws,
		showArtifacts:    true,
	}
	note := func(group, filename string) *tree.Item {
		return noteToItem(&models.Note{
			Path:      ws.Path + "/nb/" + group + "/" + filename,
			Title:     filename,
			Group:     group,
			Workspace: ws.Name,
			CreatedAt: time.Now(),
		})
	}
	m.allItems = []*tree.Item{
		note("inbox", "eco-note.md"),
		note("plans/demo", "01-job.md"),
		note("plans/demo", "02-other.md"),
		note("plans/demo/.artifacts/job-abc", "briefing.xml"),
		note("plans/demo/.artifacts/job-abc/workflows", "wf.md"),
		note("plans/demo/.artifacts/orphan-uuid", "stray.md"),
		note("plans", "README.md"),
	}
	m.setJobs(nil) // seeds the derived maps (including artifact counts)
	m.jobIDToTitle = map[string]string{"job-abc": "add pi vim"}
	m.jobIDToFile = map[string]string{"job-abc": "01-job.md"}
	m.jobFileToID = map[string]string{"01-job.md": "job-abc"}
	return m, ws
}

// buildExpanded renders the tree twice: the first pass fires the default-collapse
// seeds for .artifacts-style nodes, the second renders everything open.
func buildExpanded(m *Model) {
	m.BuildDisplayTree()
	m.collapsedNodes = map[string]bool{}
	m.BuildDisplayTree()
}

func findDirNode(m *Model, pathSuffix string) *DisplayNode {
	for _, n := range m.displayNodes {
		if n.Item != nil && n.Item.IsDir && strings.HasSuffix(n.Item.Path, pathSuffix) {
			return n
		}
	}
	return nil
}

// The standalone .artifacts node belongs INSIDE its plan. It used to be emitted
// at a hardcoded ws.Depth+2 — correct for a top-level group, one level too
// shallow for a plan — so it rendered as a sibling of the plan it belongs to.
func TestArtifactsNodeNestsUnderItsPlan(t *testing.T) {
	m, _ := newPlanArtifactModel(t)
	buildExpanded(m)

	plan := findDirNode(m, "/plans/demo")
	if plan == nil {
		t.Fatal("no plan node in tree")
	}
	artifacts := findDirNode(m, "/plans/demo/.artifacts")
	if artifacts == nil {
		t.Fatal("no .artifacts node in tree")
	}
	if artifacts.Depth != plan.Depth+1 {
		t.Errorf(".artifacts depth = %d, want plan depth + 1 = %d", artifacts.Depth, plan.Depth+1)
	}
}

// A job's whole artifact subtree nests under the job row: its own files AND any
// nested dirs (workflows/…), which previously stayed behind in the standalone
// .artifacts node. Only dirs owned by no job remain there.
func TestJobArtifactSubtreeNestsUnderJobRow(t *testing.T) {
	m, _ := newPlanArtifactModel(t)
	buildExpanded(m)

	nested := findDirNode(m, "/.artifacts-nested/01-job.md")
	if nested == nil {
		t.Fatal("job row has no nested artifacts node")
	}
	if nested.ChildCount != 2 {
		t.Errorf("nested artifacts ChildCount = %d, want 2 (briefing.xml + workflows/wf.md)", nested.ChildCount)
	}

	// Everything the job owns renders below the nested node and above the next
	// row at or above its depth.
	var owned []string
	for i, n := range m.displayNodes {
		if n != nested {
			continue
		}
		for _, sub := range m.displayNodes[i+1:] {
			if sub.Item == nil || sub.Depth <= nested.Depth {
				break
			}
			owned = append(owned, sub.Item.Name)
		}
		break
	}
	joined := strings.Join(owned, " ")
	for _, want := range []string{"workflows", "wf.md", "briefing.xml"} {
		if !strings.Contains(joined, want) {
			t.Errorf("nested artifacts missing %q (got: %s)", want, joined)
		}
	}

	// The standalone .artifacts node keeps only the orphan.
	artifacts := findDirNode(m, "/plans/demo/.artifacts")
	if artifacts == nil {
		t.Fatal("no .artifacts node in tree")
	}
	if artifacts.ChildCount != 1 {
		t.Errorf(".artifacts ChildCount = %d, want 1 (the orphan dir only)", artifacts.ChildCount)
	}
	// The job's dirs render exactly once — nested under the job, never also as
	// a standalone child of .artifacts.
	workflowRows := 0
	for _, n := range m.displayNodes {
		if n.Item != nil && n.Item.IsDir && strings.HasSuffix(n.Item.Path, "/.artifacts/job-abc/workflows") {
			workflowRows++
		}
	}
	if workflowRows != 1 {
		t.Errorf("workflows dir rendered %d times, want exactly 1 (nested under its job)", workflowRows)
	}
	if findDirNode(m, "/.artifacts/orphan-uuid") == nil {
		t.Error("orphaned artifact dir disappeared from the standalone .artifacts node")
	}
}

// The artifact-count badge counts the job's whole subtree, matching what the
// nested node expands to.
func TestArtifactBadgeCountsWholeSubtree(t *testing.T) {
	m, _ := newPlanArtifactModel(t)
	buildExpanded(m)

	for _, n := range m.displayNodes {
		if n.IsNote() && n.Item.Name == "01-job.md" {
			if got := m.artifactCountForNote(n); got != 2 {
				t.Errorf("artifact badge = %d, want 2", got)
			}
			return
		}
	}
	t.Fatal("job note row not found")
}

// Files sitting directly in plans/ used to render as a SECOND top-level "plans"
// group beside the real one. They now live inside the plans container.
func TestPlansRootFilesStayInsideTheContainer(t *testing.T) {
	m, _ := newPlanArtifactModel(t)
	buildExpanded(m)

	var plansRows []*DisplayNode
	for _, n := range m.displayNodes {
		if n.Item != nil && n.Item.IsDir && n.Item.Name == "plans" {
			plansRows = append(plansRows, n)
		}
	}
	if len(plansRows) != 1 {
		t.Fatalf("found %d %q rows, want exactly 1", len(plansRows), "plans")
	}

	var loose *DisplayNode
	for _, n := range m.displayNodes {
		if n.IsNote() && n.Item.Name == "README.md" {
			loose = n
		}
	}
	if loose == nil {
		t.Fatal("loose plans/ file was dropped from the tree")
	}
	if loose.Depth != plansRows[0].Depth+1 {
		t.Errorf("loose plans/ file depth = %d, want inside the plans container (%d)", loose.Depth, plansRows[0].Depth+1)
	}
}

// A "group by" axis buckets the job rows but must not evict a job's artifacts
// back into the standalone .artifacts node — they stay nested under their job.
func TestArtifactsNestUnderJobRowWithGroupBy(t *testing.T) {
	m, _ := newPlanArtifactModel(t)
	m.groupBy = "date"
	buildExpanded(m)

	nested := findDirNode(m, "/.artifacts-nested/01-job.md")
	if nested == nil {
		t.Fatal("group-by layout dropped the job's nested artifacts node")
	}
	if nested.ChildCount != 2 {
		t.Errorf("nested artifacts ChildCount = %d, want 2", nested.ChildCount)
	}
	artifacts := findDirNode(m, "/plans/demo/.artifacts")
	if artifacts != nil && artifacts.ChildCount != 1 {
		t.Errorf(".artifacts ChildCount = %d under group-by, want 1 (the orphan only)", artifacts.ChildCount)
	}
}

// The standalone .artifacts dirs are named after the job's markdown file — the
// name flow's job table shows — rather than the job title, and sort into job
// order (01-, 02-, …) instead of job-ID order.
func TestArtifactDirsLabeledAndSortedByJobFilename(t *testing.T) {
	m, _ := newPlanArtifactModel(t)
	// No job rows in this group, so both dirs stay in the standalone node.
	m.jobFileToID = map[string]string{}
	m.jobIDToFile = map[string]string{
		"job-abc":     "02-job.md",
		"orphan-uuid": "01-other.md",
	}
	m.jobIDToTitle = map[string]string{"job-abc": "add pi vim"}
	buildExpanded(m)

	var labels []string
	for _, n := range m.displayNodes {
		if n.Item == nil || !n.Item.IsDir {
			continue
		}
		if !strings.Contains(n.Item.Path, "/.artifacts/") || strings.Count(n.Item.Path, "/.artifacts/") != 1 {
			continue
		}
		rel := n.Item.Path[strings.Index(n.Item.Path, "/.artifacts/")+len("/.artifacts/"):]
		if strings.Contains(rel, "/") {
			continue // nested dir, labeled by its own segment
		}
		labels = append(labels, m.getNodeRenderInfo(n).name)
	}
	want := []string{"01-other.md", "02-job.md"}
	if strings.Join(labels, ",") != strings.Join(want, ",") {
		t.Errorf("artifact dir labels = %v, want %v (job filenames, in job order)", labels, want)
	}
}

// Folding a group-by bucket hides its jobs, so their artifacts must not
// resurface in the standalone .artifacts node.
func TestCollapsedBucketDoesNotLeakArtifacts(t *testing.T) {
	m, _ := newPlanArtifactModel(t)
	m.groupBy = "date"
	buildExpanded(m)

	// Fold every synthetic bucket.
	for _, n := range m.displayNodes {
		if n.Item != nil && n.Item.IsDir && strings.Contains(n.Item.Path, ".synthetic-") {
			m.collapsedNodes[n.NodeID()] = true
		}
	}
	m.BuildDisplayTree()

	artifacts := findDirNode(m, "/plans/demo/.artifacts")
	if artifacts == nil {
		t.Fatal("no .artifacts node in tree")
	}
	if artifacts.ChildCount != 1 {
		t.Errorf(".artifacts ChildCount = %d with every bucket folded, want 1 (the orphan only)", artifacts.ChildCount)
	}
}
