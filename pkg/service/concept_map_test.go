package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
