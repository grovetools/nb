package service

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var wantConceptMapFiles = []string{
	"likec4.config.json",
	"package.json",
	filepath.Join("src", "_spec.c4"),
	filepath.Join("src", "model.c4"),
	".gitignore",
}

func TestScaffoldConceptMapWritesExpectedFiles(t *testing.T) {
	dir := t.TempDir()

	result, err := scaffoldConceptMap(dir, "payments", "Payments Landscape")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Wrote, wantConceptMapFiles) {
		t.Fatalf("wrote = %v, want %v", result.Wrote, wantConceptMapFiles)
	}
	if len(result.Updated) != 0 || len(result.Skipped) != 0 {
		t.Fatalf("fresh scaffold must not update/skip: %+v", result)
	}

	var config map[string]string
	data, err := os.ReadFile(filepath.Join(dir, "likec4.config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("likec4.config.json is not valid JSON: %v\n%s", err, data)
	}
	if config["name"] != "payments" || config["title"] != "Payments Landscape" {
		t.Fatalf("likec4.config.json = %v", config)
	}
	if config["$schema"] != "https://likec4.dev/schemas/config.json" {
		t.Fatalf("likec4.config.json $schema = %q", config["$schema"])
	}

	var pkg struct {
		Name            string            `json:"name"`
		Private         bool              `json:"private"`
		Scripts         map[string]string `json:"scripts"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	data, err = os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("package.json is not valid JSON: %v\n%s", err, data)
	}
	if pkg.Name != "payments-map" || !pkg.Private {
		t.Fatalf("package.json = %+v", pkg)
	}
	if pkg.DevDependencies["likec4"] != conceptMapLikeC4Version {
		t.Fatalf("likec4 pin = %q, want %q", pkg.DevDependencies["likec4"], conceptMapLikeC4Version)
	}
	if pkg.Scripts["validate"] != "likec4 validate --json --no-layout" {
		t.Fatalf("validate script = %q", pkg.Scripts["validate"])
	}
}

func TestScaffoldConceptMapJSONEscapesTitle(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffoldConceptMap(dir, "payments", `He said "maps"`); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "likec4.config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]string
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("config with quoted title is not valid JSON: %v\n%s", err, data)
	}
	if config["title"] != `He said "maps"` {
		t.Fatalf("title = %q", config["title"])
	}
}

func TestScaffoldConceptMapUpdateIsIdempotentAndPreservesModel(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffoldConceptMap(dir, "payments", "Payments Landscape"); err != nil {
		t.Fatal(err)
	}

	// Simulate real life between new and update: the agent edited the model,
	// someone deleted .gitignore, and a generated file drifted.
	customModel := "model {\n  payments = system 'Payments'\n}\n"
	modelPath := filepath.Join(dir, "src", "model.c4")
	if err := os.WriteFile(modelPath, []byte(customModel), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := scaffoldConceptMap(dir, "payments", "Payments Landscape")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Wrote, []string{".gitignore"}) {
		t.Fatalf("wrote = %v, want [.gitignore]", result.Wrote)
	}
	if !reflect.DeepEqual(result.Updated, []string{"package.json"}) {
		t.Fatalf("updated = %v, want [package.json]", result.Updated)
	}
	got, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != customModel {
		t.Fatalf("update touched src/model.c4:\n%s", got)
	}

	// A second update over a converged tree changes nothing.
	result, err = scaffoldConceptMap(dir, "payments", "Payments Landscape")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Wrote) != 0 || len(result.Updated) != 0 {
		t.Fatalf("second update must be a no-op: %+v", result)
	}
	if !reflect.DeepEqual(result.Skipped, wantConceptMapFiles) {
		t.Fatalf("skipped = %v, want %v", result.Skipped, wantConceptMapFiles)
	}
}

func TestScaffoldConceptMapConfigMergePreservesUserKeys(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffoldConceptMap(dir, "payments", "Payments Landscape"); err != nil {
		t.Fatal(err)
	}

	// The user extended the config with an include block (federated concept
	// detail files) and an unknown key, and something drifted an owned key.
	includeDir := filepath.Join(dir, "..", "other-concept")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	userConfig := map[string]any{
		"$schema": "https://likec4.dev/schemas/config.json",
		"name":    "renamed-by-hand",
		"title":   "Stale Title",
		"include": map[string]any{
			"paths": []any{"../other-concept"},
		},
		"exclude":  map[string]any{"paths": []any{}},
		"x-custom": "kept verbatim",
	}
	data, err := json.MarshalIndent(userConfig, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "likec4.config.json")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := scaffoldConceptMap(dir, "payments", "Payments Landscape")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Updated, []string{"likec4.config.json"}) {
		t.Fatalf("updated = %v, want [likec4.config.json]", result.Updated)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", result.Warnings)
	}

	merged := readConceptMapConfig(t, configPath)
	// Owned keys refreshed.
	if merged["name"] != "payments" || merged["title"] != "Payments Landscape" {
		t.Fatalf("owned keys not refreshed: %v", merged)
	}
	// User keys preserved.
	if merged["x-custom"] != "kept verbatim" {
		t.Fatalf("unknown user key lost: %v", merged)
	}
	include, ok := merged["include"].(map[string]any)
	if !ok || !reflect.DeepEqual(include["paths"], []any{"../other-concept"}) {
		t.Fatalf("include block lost or mangled: %v", merged["include"])
	}
	if _, ok := merged["exclude"]; !ok {
		t.Fatalf("exclude block lost: %v", merged)
	}

	// A converged config is not rewritten again.
	result, err = scaffoldConceptMap(dir, "payments", "Payments Landscape")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Updated) != 0 || len(result.Wrote) != 0 {
		t.Fatalf("second update must be a no-op: %+v", result)
	}
}

func TestScaffoldConceptMapUnparseableConfigLeftUntouched(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffoldConceptMap(dir, "payments", "Payments Landscape"); err != nil {
		t.Fatal(err)
	}
	garbage := []byte("{ this is not json\n")
	configPath := filepath.Join(dir, "likec4.config.json")
	if err := os.WriteFile(configPath, garbage, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := scaffoldConceptMap(dir, "payments", "Payments Landscape")
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(result.Skipped, "likec4.config.json") {
		t.Fatalf("skipped = %v, want likec4.config.json skipped", result.Skipped)
	}
	if containsString(result.Updated, "likec4.config.json") || containsString(result.Wrote, "likec4.config.json") {
		t.Fatalf("unparseable config must not be rewritten: %+v", result)
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected a warning about the unparseable config, got none")
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(garbage) {
		t.Fatalf("unparseable config was modified:\n%s", got)
	}
}

func TestScaffoldConceptMapDoesNotResurrectSeedC4(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffoldConceptMap(dir, "payments", "Payments Landscape"); err != nil {
		t.Fatal(err)
	}

	// A mature map: model.c4 was replaced by per-subsystem files.
	if err := os.Remove(filepath.Join(dir, "src", "model.c4")); err != nil {
		t.Fatal(err)
	}
	subsystem := "model {\n  billing = system 'Billing'\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "src", "billing.c4"), []byte(subsystem), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := scaffoldConceptMap(dir, "payments", "Payments Landscape")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "src", "model.c4")); !os.IsNotExist(err) {
		t.Fatalf("update resurrected src/model.c4 (stat err = %v)", err)
	}
	if !containsString(result.Skipped, filepath.Join("src", "model.c4")) ||
		!containsString(result.Skipped, filepath.Join("src", "_spec.c4")) {
		t.Fatalf("seed .c4 files not reported as skipped: %v", result.Skipped)
	}
	if len(result.Wrote) != 0 {
		t.Fatalf("mature update must not write files: %v", result.Wrote)
	}
}

func TestScaffoldConceptMapSeedsC4OnFreshSrc(t *testing.T) {
	dir := t.TempDir()
	// src/ exists but holds no .c4 files yet: still a fresh scaffold.
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := scaffoldConceptMap(dir, "payments", "Payments Landscape")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Wrote, wantConceptMapFiles) {
		t.Fatalf("wrote = %v, want %v", result.Wrote, wantConceptMapFiles)
	}
	for _, name := range []string{"_spec.c4", "model.c4"} {
		if _, err := os.Stat(filepath.Join(dir, "src", name)); err != nil {
			t.Fatalf("fresh scaffold missing src/%s: %v", name, err)
		}
	}
}

func TestScaffoldConceptMapWarnsOnDeadIncludePaths(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffoldConceptMap(dir, "payments", "Payments Landscape"); err != nil {
		t.Fatal(err)
	}
	liveDir := filepath.Join(dir, "..", "live-concept")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{
		"$schema": "https://likec4.dev/schemas/config.json",
		"name":    "payments",
		"title":   "Payments Landscape",
		"include": map[string]any{
			"paths": []any{"../live-concept", "../missing-concept"},
		},
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "likec4.config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := scaffoldConceptMap(dir, "payments", "Payments Landscape")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one dead-include warning", result.Warnings)
	}
	if want := "../missing-concept"; !strings.Contains(result.Warnings[0], want) {
		t.Fatalf("warning %q does not mention %q", result.Warnings[0], want)
	}
}

// newConceptMapFederationFixture builds the notebook shape attach/detach runs
// against — a scaffolded map concept in one workspace and a plain concept in
// another — and returns their directories.
func newConceptMapFederationFixture(t *testing.T) (mapDir, conceptDir string) {
	t.Helper()
	root := t.TempDir()
	mapDir = filepath.Join(root, "workspaces", "grovetools", "concepts", "grove-architecture")
	conceptDir = filepath.Join(root, "workspaces", "tuimux", "concepts", "embeddable-panel-system")
	for _, dir := range []string{mapDir, conceptDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := scaffoldConceptMap(mapDir, "grove-architecture", "Grove Architecture"); err != nil {
		t.Fatal(err)
	}
	return mapDir, conceptDir
}

// wantFederationPath is the include.paths entry for the fixture's concept:
// out of the map's concept dir, its concepts dir and its workspace, then back
// down into the other workspace.
const wantFederationPath = "../../../tuimux/concepts/embeddable-panel-system/likec4"

func TestAttachConceptToMapComputesRelativePath(t *testing.T) {
	mapDir, conceptDir := newConceptMapFederationFixture(t)

	result, err := attachConceptToMap(mapDir, conceptDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != wantFederationPath {
		t.Fatalf("path = %q, want %q", result.Path, wantFederationPath)
	}
	if !result.Changed || !result.CreatedDetailDir {
		t.Fatalf("first attach must change the config and create likec4/: %+v", result)
	}
	if !reflect.DeepEqual(result.IncludePaths, []string{wantFederationPath}) {
		t.Fatalf("includePaths = %v", result.IncludePaths)
	}

	// The detail folder exists and the recorded path resolves back to it from
	// the config's folder — the contract LikeC4 itself relies on.
	info, err := os.Stat(filepath.Join(conceptDir, ConceptMapDetailDir))
	if err != nil || !info.IsDir() {
		t.Fatalf("concept likec4/ not created: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(mapDir, filepath.FromSlash(result.Path)))
	if err != nil {
		t.Fatalf("include path does not resolve from the map dir: %v", err)
	}
	wantResolved, err := filepath.EvalSymlinks(filepath.Join(conceptDir, ConceptMapDetailDir))
	if err != nil {
		t.Fatal(err)
	}
	if resolved != wantResolved {
		t.Fatalf("include path resolves to %s, want %s", resolved, wantResolved)
	}

	config := readConceptMapConfig(t, filepath.Join(mapDir, conceptMapConfigFile))
	include, ok := config["include"].(map[string]any)
	if !ok || !reflect.DeepEqual(include["paths"], []any{wantFederationPath}) {
		t.Fatalf("on-disk include block = %v", config["include"])
	}
	if config["name"] != "grove-architecture" {
		t.Fatalf("attach clobbered the generated keys: %v", config)
	}
}

func TestAttachConceptToMapIsIdempotent(t *testing.T) {
	mapDir, conceptDir := newConceptMapFederationFixture(t)

	if _, err := attachConceptToMap(mapDir, conceptDir); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(mapDir, conceptMapConfigFile))
	if err != nil {
		t.Fatal(err)
	}

	result, err := attachConceptToMap(mapDir, conceptDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed || result.CreatedDetailDir {
		t.Fatalf("second attach must be a no-op: %+v", result)
	}
	if !reflect.DeepEqual(result.IncludePaths, []string{wantFederationPath}) {
		t.Fatalf("includePaths = %v, want one entry", result.IncludePaths)
	}
	after, err := os.ReadFile(filepath.Join(mapDir, conceptMapConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("second attach rewrote the config:\n%s", after)
	}

	// An equivalent entry written by hand ("./x", trailing slash) is the same
	// attachment, not a second one.
	config := readConceptMapConfig(t, filepath.Join(mapDir, conceptMapConfigFile))
	config["include"] = map[string]any{"paths": []any{"./" + wantFederationPath + "/"}}
	writeConceptMapConfigForTest(t, mapDir, config)
	result, err = attachConceptToMap(mapDir, conceptDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed || len(result.IncludePaths) != 1 {
		t.Fatalf("equivalent path re-attached: %+v", result)
	}
}

func TestDetachConceptFromMapRemovesOnlyItsEntry(t *testing.T) {
	mapDir, conceptDir := newConceptMapFederationFixture(t)

	config := readConceptMapConfig(t, filepath.Join(mapDir, conceptMapConfigFile))
	config["include"] = map[string]any{"paths": []any{"../other-concept/likec4"}}
	writeConceptMapConfigForTest(t, mapDir, config)

	if _, err := attachConceptToMap(mapDir, conceptDir); err != nil {
		t.Fatal(err)
	}
	result, err := detachConceptFromMap(mapDir, conceptDir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed {
		t.Fatalf("detach of an attached concept must change the config: %+v", result)
	}
	if !reflect.DeepEqual(result.IncludePaths, []string{"../other-concept/likec4"}) {
		t.Fatalf("includePaths = %v, want the unrelated entry only", result.IncludePaths)
	}
	config = readConceptMapConfig(t, filepath.Join(mapDir, conceptMapConfigFile))
	include, ok := config["include"].(map[string]any)
	if !ok || !reflect.DeepEqual(include["paths"], []any{"../other-concept/likec4"}) {
		t.Fatalf("on-disk include block = %v", config["include"])
	}
	// The concept keeps its own detail files; only the map's reference went.
	if _, err := os.Stat(filepath.Join(conceptDir, ConceptMapDetailDir)); err != nil {
		t.Fatalf("detach removed the concept's likec4/ folder: %v", err)
	}

	// Detaching again changes nothing.
	result, err = detachConceptFromMap(mapDir, conceptDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed {
		t.Fatalf("second detach must be a no-op: %+v", result)
	}
}

func TestAttachDetachPreserveUserConfigKeys(t *testing.T) {
	mapDir, conceptDir := newConceptMapFederationFixture(t)

	config := readConceptMapConfig(t, filepath.Join(mapDir, conceptMapConfigFile))
	config["exclude"] = map[string]any{"paths": []any{"node_modules"}}
	config["x-custom"] = "kept verbatim"
	config["include"] = map[string]any{
		"maxDepth": float64(3),
		"paths":    []any{"../other-concept/likec4"},
	}
	writeConceptMapConfigForTest(t, mapDir, config)

	assertUserKeys := func(stage string) {
		t.Helper()
		got := readConceptMapConfig(t, filepath.Join(mapDir, conceptMapConfigFile))
		if got["x-custom"] != "kept verbatim" {
			t.Fatalf("%s: unknown user key lost: %v", stage, got)
		}
		exclude, ok := got["exclude"].(map[string]any)
		if !ok || !reflect.DeepEqual(exclude["paths"], []any{"node_modules"}) {
			t.Fatalf("%s: exclude block lost: %v", stage, got["exclude"])
		}
		include, ok := got["include"].(map[string]any)
		if !ok || include["maxDepth"] != float64(3) {
			t.Fatalf("%s: sibling include key lost: %v", stage, got["include"])
		}
		if got["$schema"] != "https://likec4.dev/schemas/config.json" ||
			got["name"] != "grove-architecture" || got["title"] != "Grove Architecture" {
			t.Fatalf("%s: generated keys mangled: %v", stage, got)
		}
	}

	if _, err := attachConceptToMap(mapDir, conceptDir); err != nil {
		t.Fatal(err)
	}
	assertUserKeys("after attach")
	if _, err := detachConceptFromMap(mapDir, conceptDir); err != nil {
		t.Fatal(err)
	}
	assertUserKeys("after detach")

	// And the scaffold's own merge-rewrite still agrees with what we wrote.
	result, err := scaffoldConceptMap(mapDir, "grove-architecture", "Grove Architecture")
	if err != nil {
		t.Fatal(err)
	}
	if containsString(result.Updated, conceptMapConfigFile) {
		t.Fatalf("update rewrote a config attach/detach had converged: %+v", result)
	}
	assertUserKeys("after update")
}

func TestAttachConceptToMapRejectsBadInput(t *testing.T) {
	mapDir, conceptDir := newConceptMapFederationFixture(t)

	if _, err := attachConceptToMap(mapDir, mapDir); err == nil {
		t.Fatal("attaching a map to itself must fail")
	}

	configPath := filepath.Join(mapDir, conceptMapConfigFile)
	garbage := []byte("{ this is not json\n")
	if err := os.WriteFile(configPath, garbage, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := attachConceptToMap(mapDir, conceptDir); err == nil {
		t.Fatal("attaching against an unparseable config must fail")
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, garbage) {
		t.Fatalf("unparseable config was modified:\n%s", got)
	}
}

// conceptMapFixtureConcepts is what ListAllConcepts would report for the
// federation fixture: the map concept and the contributor concept, each in its
// own workspace.
func conceptMapFixtureConcepts(mapDir, conceptDir string) []ConceptInfo {
	return []ConceptInfo{
		{ID: "grove-architecture", Title: "Grove Architecture", Path: mapDir, Workspace: "grovetools"},
		{ID: "embeddable-panel-system", Title: "Embeddable Panel System", Path: conceptDir, Workspace: "tuimux"},
	}
}

func TestConceptRefIndexReverseMapsIncludePaths(t *testing.T) {
	mapDir, conceptDir := newConceptMapFederationFixture(t)
	index := newConceptRefIndex(conceptMapFixtureConcepts(mapDir, conceptDir))

	detailDir := filepath.Join(conceptDir, ConceptMapDetailDir)
	cases := []struct {
		name string
		dir  string
		want string
	}{
		{"concept dir itself", conceptDir, "tuimux:embeddable-panel-system"},
		{"the concept's likec4/ folder", detailDir, "tuimux:embeddable-panel-system"},
		{"nested under likec4/", filepath.Join(detailDir, "views", "deep"), "tuimux:embeddable-panel-system"},
		{"the map's own dir", mapDir, "grovetools:grove-architecture"},
		{"the map's src tree", filepath.Join(mapDir, "src"), "grovetools:grove-architecture"},
		{"outside every concept", filepath.Join(mapDir, "..", "..", "..", "elsewhere", "likec4"), ""},
	}
	for _, tc := range cases {
		if got := index.lookup(tc.dir); got != tc.want {
			t.Errorf("%s: lookup(%s) = %q, want %q", tc.name, tc.dir, got, tc.want)
		}
	}

	// The include.paths entry attach writes reverse-maps back to the concept
	// it came from — the round trip the listing depends on.
	attachment, err := attachConceptToMap(mapDir, conceptDir)
	if err != nil {
		t.Fatal(err)
	}
	resolved := filepath.Join(mapDir, filepath.FromSlash(attachment.Path))
	if got := index.lookup(resolved); got != "tuimux:embeddable-panel-system" {
		t.Fatalf("attach path %q reverse-maps to %q", attachment.Path, got)
	}
}

func TestConceptRefIndexUnqualifiedWorkspace(t *testing.T) {
	dir := t.TempDir()
	index := newConceptRefIndex([]ConceptInfo{{ID: "orphan", Path: dir}})
	if got := index.lookup(filepath.Join(dir, ConceptMapDetailDir)); got != "orphan" {
		t.Fatalf("lookup = %q, want the bare id for a workspace-less concept", got)
	}
}

func TestConceptMapListingResolvesAndFlagsIncludes(t *testing.T) {
	mapDir, conceptDir := newConceptMapFederationFixture(t)
	if _, err := attachConceptToMap(mapDir, conceptDir); err != nil {
		t.Fatal(err)
	}
	detailDir := filepath.Join(conceptDir, ConceptMapDetailDir)
	if err := os.WriteFile(filepath.Join(detailDir, "panel-system.c4"), []byte("// detail\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Alongside the live attachment: an entry whose concept still exists but
	// whose likec4/ folder never will, and one pointing outside the notebook.
	config := readConceptMapConfig(t, filepath.Join(mapDir, conceptMapConfigFile))
	config["include"] = map[string]any{"paths": []any{
		wantFederationPath,
		"../../../tuimux/concepts/embeddable-panel-system/likec4-gone",
		"../../../../nowhere/likec4",
	}}
	writeConceptMapConfigForTest(t, mapDir, config)

	info := ConceptMapInfo{
		ConceptInfo: ConceptInfo{ID: "grove-architecture", Title: "Grove Architecture", Path: mapDir, Workspace: "grovetools"},
		MapDir:      mapDir,
	}
	index := newConceptRefIndex(conceptMapFixtureConcepts(mapDir, conceptDir))
	listing, err := conceptMapListing(info, index, ConceptMapListOptions{Federated: true})
	if err != nil {
		t.Fatal(err)
	}

	// The map's own seed model still counts as its src/ content.
	if listing.SrcFiles != 2 {
		t.Fatalf("srcFiles = %d, want the two seed .c4 files", listing.SrcFiles)
	}
	if len(listing.Includes) != 3 {
		t.Fatalf("includes = %+v, want 3 entries", listing.Includes)
	}

	live := listing.Includes[0]
	if live.Dead {
		t.Fatalf("live include flagged dead: %+v", live)
	}
	if live.Concept != "tuimux:embeddable-panel-system" {
		t.Fatalf("live include concept = %q", live.Concept)
	}
	if !reflect.DeepEqual(live.Files, []string{"panel-system.c4"}) {
		t.Fatalf("live include files = %v", live.Files)
	}

	// A dead entry still names the concept it was meant to pull from: that is
	// what tells you whether to re-attach or drop the line.
	detached := listing.Includes[1]
	if !detached.Dead || len(detached.Files) != 0 {
		t.Fatalf("missing detail folder must be dead with no files: %+v", detached)
	}
	if detached.Concept != "tuimux:embeddable-panel-system" {
		t.Fatalf("dead include concept = %q, want the owning concept", detached.Concept)
	}

	stray := listing.Includes[2]
	if !stray.Dead || stray.Concept != "" {
		t.Fatalf("out-of-notebook include = %+v, want dead and unattributed", stray)
	}

	if len(listing.Warnings) != 2 {
		t.Fatalf("warnings = %v, want one per dead include", listing.Warnings)
	}
	for i, want := range []string{"likec4-gone", "nowhere/likec4"} {
		if !strings.Contains(listing.Warnings[i], want) {
			t.Fatalf("warning %q does not mention %q", listing.Warnings[i], want)
		}
	}
	// The wording is the one `nb concept map update` already warns with.
	scaffold := &ConceptMapScaffold{}
	warnConceptMapIncludePaths(mapDir, scaffold)
	if !reflect.DeepEqual(scaffold.Warnings, listing.Warnings) {
		t.Fatalf("list warnings %v diverge from update warnings %v", listing.Warnings, scaffold.Warnings)
	}
}

func TestConceptMapListingWithoutIncludesOrFederation(t *testing.T) {
	mapDir, conceptDir := newConceptMapFederationFixture(t)
	info := ConceptMapInfo{
		ConceptInfo: ConceptInfo{ID: "grove-architecture", Path: mapDir, Workspace: "grovetools"},
		MapDir:      mapDir,
	}
	index := newConceptRefIndex(conceptMapFixtureConcepts(mapDir, conceptDir))

	listing, err := conceptMapListing(info, index, ConceptMapListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Includes) != 0 || len(listing.Warnings) != 0 {
		t.Fatalf("unattached map must have no includes or warnings: %+v", listing)
	}

	// Contributor files are federated-only detail; a plain listing skips them.
	if _, err := attachConceptToMap(mapDir, conceptDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conceptDir, ConceptMapDetailDir, "panel.c4"), []byte("// detail\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	listing, err = conceptMapListing(info, index, ConceptMapListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Includes) != 1 || listing.Includes[0].Files != nil {
		t.Fatalf("non-federated listing must not walk contributor files: %+v", listing.Includes)
	}
	if listing.Includes[0].Concept != "tuimux:embeddable-panel-system" {
		t.Fatalf("concept = %q", listing.Includes[0].Concept)
	}
}

func TestConceptMapC4FilesIsSortedAndRecursive(t *testing.T) {
	dir := t.TempDir()
	if got, err := conceptMapC4Files(filepath.Join(dir, "missing")); err != nil || got != nil {
		t.Fatalf("missing dir = %v, %v; want nil, nil", got, err)
	}
	for _, rel := range []string{"z.c4", "a.c4", "README.md", filepath.Join("views", "b.c4")} {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("// x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := conceptMapC4Files(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"a.c4", "views/b.c4", "z.c4"}) {
		t.Fatalf("files = %v", got)
	}
}

func TestConceptMapsInKeepsOnlyMapsSorted(t *testing.T) {
	mapDir, conceptDir := newConceptMapFederationFixture(t)
	second := filepath.Join(filepath.Dir(mapDir), "billing-map")
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := scaffoldConceptMap(second, "billing-map", "Billing"); err != nil {
		t.Fatal(err)
	}

	concepts := append(conceptMapFixtureConcepts(mapDir, conceptDir),
		ConceptInfo{ID: "billing-map", Title: "Billing", Path: second, Workspace: "grovetools"})
	maps := conceptMapsIn(concepts)
	got := make([]string, 0, len(maps))
	for _, m := range maps {
		got = append(got, m.Workspace+":"+m.ID)
	}
	if !reflect.DeepEqual(got, []string{"grovetools:billing-map", "grovetools:grove-architecture"}) {
		t.Fatalf("maps = %v, want the two scaffolded maps sorted by workspace then id", got)
	}
}

func writeConceptMapConfigForTest(t *testing.T, mapDir string, config map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mapDir, conceptMapConfigFile), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readConceptMapConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("%s is not valid JSON: %v\n%s", path, err, data)
	}
	return config
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestScaffoldConceptMapFallsBackToManifestTitle(t *testing.T) {
	dir := t.TempDir()
	manifest := "id: payments\ntitle: Payments Landscape\n"
	if err := os.WriteFile(filepath.Join(dir, "concept-manifest.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := scaffoldConceptMap(dir, "payments", ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "likec4.config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]string
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config["title"] != "Payments Landscape" {
		t.Fatalf("title = %q, want manifest title", config["title"])
	}
}

func TestValidateConceptMapID(t *testing.T) {
	valid := []string{"payments", "workspace-model", "auth2"}
	for _, id := range valid {
		if err := ValidateConceptMapID(id); err != nil {
			t.Errorf("ValidateConceptMapID(%q) = %v, want nil", id, err)
		}
	}
	invalid := []string{"", "default", "pay.ments", "pay@ments", "pay#ments", "pay ments", "pay/ments"}
	for _, id := range invalid {
		if err := ValidateConceptMapID(id); err == nil {
			t.Errorf("ValidateConceptMapID(%q) = nil, want error", id)
		}
	}
}
