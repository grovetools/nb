package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/grovetools/nb/pkg/service"
)

func executeConceptSearch(t *testing.T, runner conceptSearchRunner, args ...string) (string, error) {
	t.Helper()
	cmd := newConceptSearchCmdWithRunner(runner)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return strings.TrimSpace(out.String()), err
}

func TestConceptSearchCommandCompactEmptySchema(t *testing.T) {
	out, err := executeConceptSearch(t, func(_ string, opts service.ConceptSearchOptions, _ bool) (service.ConceptSearchPage, error) {
		if opts.Limit != service.ConceptCompactMaxResults {
			t.Fatalf("compact default limit = %d, want %d", opts.Limit, service.ConceptCompactMaxResults)
		}
		return service.ConceptSearchPage{Results: []service.ConceptSearchResult{}}, nil
	}, "nothing", "--compact")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out)
	}
	if got["schema_version"] != float64(1) || got["omitted"] != float64(0) {
		t.Fatalf("unexpected compact envelope: %v", got)
	}
	results, ok := got["results"].([]any)
	if !ok || len(results) != 0 {
		t.Fatalf("compact empty results must be [], got %#v", got["results"])
	}
}

func TestConceptSearchCommandCompactPrecedesJSON(t *testing.T) {
	out, err := executeConceptSearch(t, func(_ string, opts service.ConceptSearchOptions, _ bool) (service.ConceptSearchPage, error) {
		if opts.MinCoverage != 0.5 || opts.Limit != 1 || opts.In != service.ConceptSearchInRole {
			t.Fatalf("options not propagated: %+v", opts)
		}
		return service.ConceptSearchPage{
			Results:       []service.ConceptSearchResult{{ConceptID: "auth", Workspace: "core", Title: "Auth", Description: "Sessions", Score: 3}},
			EligibleTotal: 2,
		}, nil
	}, "session", "--compact", "--json", "--in", "role", "--limit", "1", "--min-coverage", "0.5")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, `{"schema_version":1`) || strings.HasPrefix(out, "[") {
		t.Fatalf("--compact must take precedence over --json: %s", out)
	}
	if !strings.Contains(out, `"concept":"core:auth"`) || !strings.Contains(out, `"omitted":1`) {
		t.Fatalf("compact identity/omitted missing: %s", out)
	}
}

func TestConceptSearchCommandLegacyJSONRemainsArray(t *testing.T) {
	out, err := executeConceptSearch(t, func(_ string, _ service.ConceptSearchOptions, _ bool) (service.ConceptSearchPage, error) {
		return service.ConceptSearchPage{Results: []service.ConceptSearchResult{{
			ConceptID: "auth", Workspace: "core", Title: "Auth", Description: "Sessions", Score: 3,
			Files: []service.ConceptFileMatch{{FilePath: "/tmp/overview.md", Score: 2}},
		}}}, nil
	}, "session", "--json", "--files-only")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "[") || strings.Contains(out, `"schema_version"`) {
		t.Fatalf("legacy JSON shape changed: %s", out)
	}
	if !strings.Contains(out, `"file_path":"/tmp/overview.md"`) || strings.Contains(out, `"matches"`) {
		t.Fatalf("legacy files-only schema changed: %s", out)
	}
}
