package views

import (
	"fmt"
	"strings"
	"testing"
	"time"

	coreconfig "github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/workspace"

	"github.com/grovetools/nb/pkg/models"
	"github.com/grovetools/nb/pkg/service"
)

type dirInfo struct {
	group   string
	display string
	depth   int
	prefix  string
}

// newArtifactTestModel returns a Model with the maps that addArtifactSubgroup
// touches initialized, so it can be driven in isolation.
func newArtifactTestModel() *Model {
	return &Model{
		collapsedNodes: make(map[string]bool),
		seededCollapse: make(map[string]bool),
		jobIDToTitle:   make(map[string]string),
		jobFileToID:    make(map[string]string),
		showArtifacts:  true,
		service:        &service.Service{NoteTypes: map[string]*coreconfig.NoteTypeConfig{}},
	}
}

func mkArtifactNote(group, title string) *models.Note {
	return &models.Note{
		Path:      "/ws/plans/demo/.artifacts/" + group + "/" + title,
		Title:     title,
		Group:     group,
		Workspace: "demo",
		CreatedAt: time.Now(),
	}
}

// TestArtifactSubgroupNestsDeepDirs verifies that a nested .artifacts layout
// (test/ -> workflows/ -> wf_x/ -> agents/) renders as a properly nested,
// indented tree: the intermediate "workflows/" node is present, deeper dirs are
// indented as children of their parent, and each dir node is labeled by its LAST
// path segment (the top job-dir segment resolving via jobIDToTitle).
func TestArtifactSubgroupNestsDeepDirs(t *testing.T) {
	m := newArtifactTestModel()
	// The top job-dir segment is an opaque job ID; register a friendly title.
	m.jobIDToTitle["test-2b3a8ac9"] = "test"

	ws := &workspace.WorkspaceNode{Name: "demo", Path: "/ws", Depth: 0}

	// Synthetic layout matching the bug report's on-disk shape: a single clean
	// chain test/ -> workflows/ -> wf_abc/ -> agents/ with files at two depths.
	artifactJobs := map[string][]*models.Note{
		"test-2b3a8ac9/workflows/wf_abc/agents": {
			mkArtifactNote("test-2b3a8ac9/workflows/wf_abc/agents", "agent-1.md"),
			mkArtifactNote("test-2b3a8ac9/workflows/wf_abc/agents", "agent-2.md"),
		},
		"test-2b3a8ac9/workflows/wf_abc": {
			mkArtifactNote("test-2b3a8ac9/workflows/wf_abc", "run.md"),
		},
	}

	// Force expansion (bypass default-collapse) by passing hasSearchFilter=true.
	var nodes []*DisplayNode
	m.addArtifactSubgroup(
		&nodes,
		ws,
		"├ ", // group prefix
		artifactJobs,
		true, // hasSearchFilter -> expand everything
		map[string]string{"demo": "/ws"},
		"/ws/plans/demo", // parentPath
		"demo",           // parentName
		"plans/demo",     // parentGroup
	)

	// Collect the directory (group) nodes with their derived display name, depth,
	// and full Group metadata.
	var dirs []dirInfo
	for _, n := range nodes {
		if n.Item == nil || !n.Item.IsDir {
			continue
		}
		info := m.getNodeRenderInfo(n)
		grp, _ := n.Item.Metadata["Group"].(string)
		dirs = append(dirs, dirInfo{group: grp, display: info.name, depth: n.Depth, prefix: n.Prefix})
	}

	// Expected dir nodes, in order, with their last-segment labels.
	// .artifacts (parent) -> test (job dir) -> workflows -> wf_abc -> agents
	wantGroups := []string{
		"plans/demo/.artifacts",
		"plans/demo/.artifacts/test-2b3a8ac9",
		"plans/demo/.artifacts/test-2b3a8ac9/workflows",
		"plans/demo/.artifacts/test-2b3a8ac9/workflows/wf_abc",
		"plans/demo/.artifacts/test-2b3a8ac9/workflows/wf_abc/agents",
	}
	wantLabels := []string{".artifacts", "test", "workflows", "wf_abc", "agents"}

	if len(dirs) != len(wantGroups) {
		t.Fatalf("expected %d dir nodes, got %d:\n%s", len(wantGroups), len(dirs), dumpDirs(dirs))
	}

	for i := range wantGroups {
		if dirs[i].group != wantGroups[i] {
			t.Errorf("dir[%d] Group = %q, want %q", i, dirs[i].group, wantGroups[i])
		}
		if dirs[i].display != wantLabels[i] {
			t.Errorf("dir[%d] display = %q, want %q (last-segment label)", i, dirs[i].display, wantLabels[i])
		}
	}

	// The intermediate "workflows/" node must be present and labeled by its last
	// segment, NOT dropped and NOT carrying a raw job-id-prefixed full path.
	var foundWorkflows bool
	for _, d := range dirs {
		if d.display == "workflows" {
			foundWorkflows = true
			if strings.Contains(d.display, "/") || strings.Contains(d.display, "test-2b3a8ac9") {
				t.Errorf("workflows node label leaked full/raw path: %q", d.display)
			}
		}
	}
	if !foundWorkflows {
		t.Fatalf("intermediate 'workflows/' node was dropped:\n%s", dumpDirs(dirs))
	}

	// Deeper dirs must be indented as children: each successive dir node's depth
	// strictly increases down the chain (parent .artifacts depth+1, +2, ...).
	for i := 1; i < len(dirs); i++ {
		if dirs[i].depth <= dirs[i-1].depth {
			t.Errorf("dir[%d] (%q) depth %d not deeper than parent dir[%d] (%q) depth %d",
				i, dirs[i].display, dirs[i].depth, i-1, dirs[i-1].display, dirs[i-1].depth)
		}
		// Indentation prefix must grow too.
		if len(dirs[i].prefix) <= len(dirs[i-1].prefix) {
			t.Errorf("dir[%d] (%q) prefix not indented deeper than parent dir[%d] (%q)",
				i, dirs[i].display, i-1, dirs[i-1].display)
		}
	}

	// Sanity: the deepest agents/ dir must carry its two artifact files, and the
	// wf_abc/ dir must carry its single file.
	var agentFiles, wfFiles int
	for _, n := range nodes {
		if n.Item == nil || n.Item.IsDir {
			continue
		}
		switch n.Item.Name {
		case "agent-1.md", "agent-2.md":
			agentFiles++
		case "run.md":
			wfFiles++
		}
	}
	if agentFiles != 2 {
		t.Errorf("expected 2 agent files rendered, got %d", agentFiles)
	}
	if wfFiles != 1 {
		t.Errorf("expected 1 wf_abc file rendered, got %d", wfFiles)
	}
}

func dumpDirs(dirs []dirInfo) string {
	var b strings.Builder
	for _, d := range dirs {
		fmt.Fprintf(&b, "  depth=%d display=%q group=%q\n", d.depth, d.display, d.group)
	}
	return b.String()
}
