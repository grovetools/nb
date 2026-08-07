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
