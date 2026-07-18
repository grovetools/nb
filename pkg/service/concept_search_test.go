package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// conceptSearchFixture builds a tempdir vault of two fake concepts and returns
// their search dirs:
//
//   - rate-limiter: "widget" and regex metacharacters only in a body file,
//     "throttling"/"gateway" only in the manifest description, "downstream"
//     only in overview.md.
//   - auth: "Widget" in the manifest title, "zebra" only in a non-overview
//     file, "gateway" in a body file.
func conceptSearchFixture(t *testing.T) []conceptSearchDir {
	t.Helper()
	root := t.TempDir()

	write := func(id string, files map[string]string) conceptSearchDir {
		dir := filepath.Join(root, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
		return conceptSearchDir{Path: dir, ID: id, Workspace: "testws"}
	}

	rateLimiter := write("rate-limiter", map[string]string{
		"concept-manifest.yml": "id: rate-limiter\ntitle: Rate Limiter\ndescription: Token bucket throttling for the API gateway\n",
		"overview.md":          "## Role\nThe rate limiter protects downstream services.\nBuckets refill every second.\n",
		"notes.md":             "Implementation notes about widget grease.\nliteral chars: price (usd) and a.b*c here\n",
	})
	auth := write("auth", map[string]string{
		"concept-manifest.yml": "id: auth\ntitle: Authentication Widget\ndescription: Session tokens and login flows\n",
		"overview.md":          "## Role\nHandles sessions.\n",
		"extra.md":             "deep file mentions gateway too\nzebra appears only here\naxbyc regex trap\n",
	})

	return []conceptSearchDir{rateLimiter, auth}
}

func searchFixture(t *testing.T, dirs []conceptSearchDir, query string, opts ConceptSearchOptions) []ConceptSearchResult {
	t.Helper()
	results, err := searchConceptDirs(dirs, query, opts)
	if err != nil {
		t.Fatalf("searchConceptDirs(%q, %+v): %v", query, opts, err)
	}
	return results
}

func resultIDs(results []ConceptSearchResult) []string {
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.ConceptID)
	}
	return ids
}

func TestConceptSearchMultiTokenOR(t *testing.T) {
	dirs := conceptSearchFixture(t)

	// Each token matches a different concept — OR semantics return both.
	results := searchFixture(t, dirs, "buckets zebra", ConceptSearchOptions{})
	if len(results) != 2 {
		t.Fatalf("expected 2 concepts for OR query, got %v", resultIDs(results))
	}
	for _, r := range results {
		if r.Score <= 0 {
			t.Errorf("concept %s has non-positive score %v", r.ConceptID, r.Score)
		}
	}

	// Full coverage must outscore partial coverage of the same concept.
	full := searchFixture(t, dirs, "buckets downstream", ConceptSearchOptions{})
	partial := searchFixture(t, dirs, "buckets qqqqnomatch", ConceptSearchOptions{})
	if len(full) != 1 || full[0].ConceptID != "rate-limiter" {
		t.Fatalf("expected only rate-limiter for full query, got %v", resultIDs(full))
	}
	if len(partial) != 1 || partial[0].ConceptID != "rate-limiter" {
		t.Fatalf("expected only rate-limiter for partial query, got %v", resultIDs(partial))
	}
	if full[0].Score <= partial[0].Score {
		t.Errorf("full coverage score %v should beat partial coverage score %v", full[0].Score, partial[0].Score)
	}

	if empty := searchFixture(t, dirs, "qqqqnomatch", ConceptSearchOptions{}); len(empty) != 0 {
		t.Errorf("expected no results, got %v", resultIDs(empty))
	}
}

func TestConceptSearchTitleBeatsBody(t *testing.T) {
	dirs := conceptSearchFixture(t)

	// "widget": auth has it in the manifest title, rate-limiter only in a body
	// file — the title hit must rank first.
	results := searchFixture(t, dirs, "widget", ConceptSearchOptions{})
	if len(results) != 2 {
		t.Fatalf("expected both concepts, got %v", resultIDs(results))
	}
	if results[0].ConceptID != "auth" {
		t.Errorf("title hit should rank first, got order %v", resultIDs(results))
	}
	if results[0].Title != "Authentication Widget" || results[0].Description != "Session tokens and login flows" {
		t.Errorf("manifest metadata not populated: %+v", results[0])
	}
}

func TestConceptSearchInScoping(t *testing.T) {
	dirs := conceptSearchFixture(t)

	cases := []struct {
		query string
		in    string
		want  []string
	}{
		// only in auth/extra.md (a non-overview body file)
		{"zebra", ConceptSearchInAll, []string{"auth"}},
		{"zebra", ConceptSearchInOverview, nil},
		{"zebra", ConceptSearchInRole, nil},
		// only in rate-limiter's manifest description
		{"throttling", ConceptSearchInAll, []string{"rate-limiter"}},
		{"throttling", ConceptSearchInOverview, nil},
		{"throttling", ConceptSearchInRole, []string{"rate-limiter"}},
		// only in rate-limiter/overview.md
		{"downstream", ConceptSearchInAll, []string{"rate-limiter"}},
		{"downstream", ConceptSearchInOverview, []string{"rate-limiter"}},
		{"downstream", ConceptSearchInRole, []string{"rate-limiter"}},
	}
	for _, tc := range cases {
		results := searchFixture(t, dirs, tc.query, ConceptSearchOptions{In: tc.in})
		got := resultIDs(results)
		if len(got) != len(tc.want) {
			t.Errorf("query %q in %q: got %v, want %v", tc.query, tc.in, got, tc.want)
			continue
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("query %q in %q: got %v, want %v", tc.query, tc.in, got, tc.want)
			}
		}
	}

	if _, err := searchConceptDirs(dirs, "x", ConceptSearchOptions{In: "bogus"}); err == nil {
		t.Error("expected error for invalid scope")
	}
}

func TestConceptSearchLimit(t *testing.T) {
	dirs := conceptSearchFixture(t)

	// "gateway": rate-limiter hits the manifest description (weight 3), auth
	// hits a plain body file (weight 1) — both match, description ranks first.
	all := searchFixture(t, dirs, "gateway", ConceptSearchOptions{})
	if len(all) != 2 {
		t.Fatalf("expected 2 concepts, got %v", resultIDs(all))
	}
	limited := searchFixture(t, dirs, "gateway", ConceptSearchOptions{Limit: 1})
	if len(limited) != 1 || limited[0].ConceptID != "rate-limiter" {
		t.Errorf("limit should keep the top-ranked concept, got %v", resultIDs(limited))
	}
}

func TestConceptSearchUnifiedJSONSchema(t *testing.T) {
	dirs := conceptSearchFixture(t)

	results := searchFixture(t, dirs, "downstream", ConceptSearchOptions{})
	if len(results) != 1 || len(results[0].Files) != 1 {
		t.Fatalf("unexpected results: %+v", results)
	}

	data, err := json.Marshal(results)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"concept_id"`, `"workspace"`, `"title"`, `"description"`, `"score"`, `"files"`, `"file_path"`, `"matches"`, `"line"`, `"text"`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("JSON missing key %s: %s", key, data)
		}
	}

	// files-only mode reuses the same struct with matches omitted.
	results[0].Files[0].Matches = nil
	data, err = json.Marshal(results)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), `"matches"`) {
		t.Errorf("nil matches should be omitted from JSON: %s", data)
	}
	if !strings.Contains(string(data), `"file_path"`) {
		t.Errorf("files-only JSON should keep file entries: %s", data)
	}
}

func TestConceptSearchRegexMetacharactersAreLiteral(t *testing.T) {
	dirs := conceptSearchFixture(t)

	// "(usd)" as a regex is a capture group; fixed-string search must match
	// the literal parentheses in rate-limiter/notes.md.
	results := searchFixture(t, dirs, "price (usd)", ConceptSearchOptions{})
	if len(results) != 1 || results[0].ConceptID != "rate-limiter" {
		t.Fatalf("expected literal match in rate-limiter, got %v", resultIDs(results))
	}

	// "a.b" as a regex would also match "axb"-style text in auth/extra.md;
	// literally it only occurs in rate-limiter's "a.b*c".
	results = searchFixture(t, dirs, "a.b", ConceptSearchOptions{})
	if len(results) != 1 || results[0].ConceptID != "rate-limiter" {
		t.Fatalf("dot must not act as a regex wildcard, got %v", resultIDs(results))
	}
}
