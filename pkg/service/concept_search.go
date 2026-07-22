package service

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	coreworkspace "github.com/grovetools/core/pkg/workspace"
	"gopkg.in/yaml.v3"
)

// Search scopes for ConceptSearchOptions.In.
const (
	ConceptSearchInRole     = "role"
	ConceptSearchInOverview = "overview"
	ConceptSearchInAll      = "all"
)

// Scoring weights: manifest title beats description beats overview lines beats
// other files. Constants are deliberately coarse — ordering guarantees matter,
// exact values do not.
const (
	conceptWeightTitle          = 5.0
	conceptWeightDescription    = 3.0
	conceptWeightOverview       = 2.0
	conceptWeightOther          = 1.0
	conceptPhraseBonus          = 10.0
	conceptSnippetMaxRunes      = 200
	ConceptCompactMaxResults    = 10
	ConceptCompactSchemaVersion = 1
)

// ConceptSearchOptions controls scope and result count for concept search.
type ConceptSearchOptions struct {
	// In is the search scope: "role" (manifest title/description + only the
	// ## Role section of overview.md), "overview" (all of overview.md), or
	// "all" (every file in concept dirs). Empty means "all". An overview with
	// no Role section contributes no prose hits to the role scope.
	In string
	// Limit caps the number of ranked concepts returned; 0 = unlimited.
	Limit int
	// MinCoverage excludes results matching less than this fraction of the
	// content query tokens. It must be between 0 and 1; 0 disables the gate.
	MinCoverage float64
}

// ConceptSearchMatch represents a single line match within a file.
type ConceptSearchMatch struct {
	LineNumber int    `json:"line"`
	Text       string `json:"text"`
}

// ConceptFileMatch represents the matches within one file of a concept.
type ConceptFileMatch struct {
	FilePath string               `json:"file_path"`
	Score    float64              `json:"score"`
	Matches  []ConceptSearchMatch `json:"matches,omitempty"`
}

// ConceptSearchResult is one ranked concept with its matching files.
type ConceptSearchResult struct {
	ConceptID   string             `json:"concept_id"`
	Workspace   string             `json:"workspace"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Score       float64            `json:"score"`
	Files       []ConceptFileMatch `json:"files"`
}

// ConceptSearchPage preserves the eligible result total before presentation
// limiting. Legacy search methods return Results only.
type ConceptSearchPage struct {
	Results       []ConceptSearchResult
	EligibleTotal int
}

// CompactConceptSearchResult is the bounded machine-facing concept shape.
type CompactConceptSearchResult struct {
	Concept string  `json:"concept"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// CompactConceptSearchEnvelope is emitted by `nb concept search --compact`.
type CompactConceptSearchEnvelope struct {
	SchemaVersion int                          `json:"schema_version"`
	Results       []CompactConceptSearchResult `json:"results"`
	Omitted       int                          `json:"omitted"`
}

// CompactConceptSearch converts a search page to the versioned, bounded CLI
// schema. It intentionally contains no filesystem paths.
func CompactConceptSearch(page ConceptSearchPage) CompactConceptSearchEnvelope {
	results := make([]CompactConceptSearchResult, 0, len(page.Results))
	for _, result := range page.Results {
		results = append(results, CompactConceptSearchResult{
			Concept: result.Workspace + ":" + result.ConceptID,
			Title:   truncateRunes(result.Title, conceptSnippetMaxRunes),
			Snippet: truncateRunes(result.Description, conceptSnippetMaxRunes),
			Score:   result.Score,
		})
	}
	omitted := page.EligibleTotal - len(page.Results)
	if omitted < 0 {
		omitted = 0
	}
	return CompactConceptSearchEnvelope{
		SchemaVersion: ConceptCompactSchemaVersion,
		Results:       results,
		Omitted:       omitted,
	}
}

// conceptManifestMeta is the shared minimal parse of concept-manifest.yml.
type conceptManifestMeta struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
}

// conceptSearchDir is one concept directory in the search corpus.
type conceptSearchDir struct {
	Path      string
	ID        string
	Workspace string
}

// SearchConcepts searches concept files across all known workspaces.
func (s *Service) SearchConcepts(query string, opts ConceptSearchOptions) ([]ConceptSearchResult, error) {
	page, err := s.SearchConceptsPage(query, opts)
	return page.Results, err
}

// SearchConceptsPage searches all known workspaces and preserves the eligible
// total before Limit is applied.
func (s *Service) SearchConceptsPage(query string, opts ConceptSearchOptions) (ConceptSearchPage, error) {
	return searchConceptDirsPage(s.conceptDirsForWorkspaces(s.workspaceProvider.All()), query, opts)
}

// SearchEcosystemConcepts searches concepts within the current ecosystem only.
func (s *Service) SearchEcosystemConcepts(ctx *WorkspaceContext, query string, opts ConceptSearchOptions) ([]ConceptSearchResult, error) {
	page, err := s.SearchEcosystemConceptsPage(ctx, query, opts)
	return page.Results, err
}

// SearchEcosystemConceptsPage searches the current ecosystem and preserves the
// eligible total before Limit is applied.
func (s *Service) SearchEcosystemConceptsPage(ctx *WorkspaceContext, query string, opts ConceptSearchOptions) (ConceptSearchPage, error) {
	currentWs := ctx.CurrentWorkspace
	if currentWs == nil {
		return ConceptSearchPage{}, fmt.Errorf("no current workspace context")
	}

	var ecosystemRootPath string
	switch currentWs.Kind {
	case coreworkspace.KindEcosystemRoot:
		ecosystemRootPath = currentWs.Path
	case coreworkspace.KindEcosystemWorktree,
		coreworkspace.KindEcosystemSubProject,
		coreworkspace.KindEcosystemSubProjectWorktree,
		coreworkspace.KindEcosystemWorktreeSubProject,
		coreworkspace.KindEcosystemWorktreeSubProjectWorktree:
		if currentWs.RootEcosystemPath != "" {
			ecosystemRootPath = currentWs.RootEcosystemPath
		} else if currentWs.ParentEcosystemPath != "" {
			ecosystemRootPath = currentWs.ParentEcosystemPath
		}
	default:
		// Not in an ecosystem - search current workspace only
	}

	if ecosystemRootPath == "" {
		dirs := s.conceptDirsForWorkspaces([]*coreworkspace.WorkspaceNode{currentWs})
		return searchConceptDirsPage(dirs, query, opts)
	}

	// Normalize for case-insensitive comparison (macOS)
	ecosystemRootPath = strings.ToLower(filepath.Clean(ecosystemRootPath))

	var ecosystemWorkspaces []*coreworkspace.WorkspaceNode
	for _, ws := range s.workspaceProvider.All() {
		wsPath := strings.ToLower(filepath.Clean(ws.Path))
		wsRootEco := strings.ToLower(filepath.Clean(ws.RootEcosystemPath))
		wsParentEco := strings.ToLower(filepath.Clean(ws.ParentEcosystemPath))

		if wsPath == ecosystemRootPath || wsRootEco == ecosystemRootPath || wsParentEco == ecosystemRootPath {
			ecosystemWorkspaces = append(ecosystemWorkspaces, ws)
		}
	}

	return searchConceptDirsPage(s.conceptDirsForWorkspaces(ecosystemWorkspaces), query, opts)
}

// conceptDirsForWorkspaces collects the concept directories (with owning
// workspace names) for the given workspaces, deduplicating shared notebooks.
func (s *Service) conceptDirsForWorkspaces(workspaces []*coreworkspace.WorkspaceNode) []conceptSearchDir {
	var dirs []conceptSearchDir
	seenPaths := make(map[string]bool)

	for _, ws := range workspaces {
		notebookContext, err := s.findNotebookContextNode(ws)
		if err != nil {
			continue
		}

		conceptsDir, err := s.notebookLocator.GetNotesDir(notebookContext, "concepts")
		if err != nil {
			continue
		}

		if seenPaths[conceptsDir] {
			continue
		}
		seenPaths[conceptsDir] = true

		entries, err := os.ReadDir(conceptsDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dirs = append(dirs, conceptSearchDir{
				Path:      filepath.Join(conceptsDir, entry.Name()),
				ID:        entry.Name(),
				Workspace: notebookContext.Name,
			})
		}
	}

	return dirs
}

// conceptAccum accumulates per-concept match state during a search.
type conceptAccum struct {
	dir         conceptSearchDir
	meta        conceptManifestMeta
	tokens      map[string]bool // query tokens this concept matched anywhere
	titleTokens int             // distinct tokens matching the manifest title
	descTokens  int             // distinct tokens matching the manifest description
	files       map[string]map[int]*conceptLineHit
	phrase      bool // full query matched as an exact substring somewhere
}

type conceptLineHit struct {
	text string
}

// searchConceptDirs is the legacy slice-returning search helper.
func searchConceptDirs(dirs []conceptSearchDir, query string, opts ConceptSearchOptions) ([]ConceptSearchResult, error) {
	page, err := searchConceptDirsPage(dirs, query, opts)
	return page.Results, err
}

// searchConceptDirsPage runs an OR-token, fixed-string, case-insensitive
// search and preserves the eligible total before presentation limiting.
func searchConceptDirsPage(dirs []conceptSearchDir, query string, opts ConceptSearchOptions) (ConceptSearchPage, error) {
	scope := opts.In
	if scope == "" {
		scope = ConceptSearchInAll
	}
	switch scope {
	case ConceptSearchInRole, ConceptSearchInOverview, ConceptSearchInAll:
	default:
		return ConceptSearchPage{}, fmt.Errorf("invalid search scope %q (want role, overview, or all)", opts.In)
	}
	if opts.MinCoverage < 0 || opts.MinCoverage > 1 {
		return ConceptSearchPage{}, fmt.Errorf("min coverage must be between 0 and 1")
	}

	tokens := tokenizeConceptQuery(query)
	if len(tokens) == 0 || len(dirs) == 0 {
		return ConceptSearchPage{Results: []ConceptSearchResult{}}, nil
	}
	phrase := strings.ToLower(strings.TrimSpace(query))
	multiToken := len(tokens) > 1

	accums := make(map[string]*conceptAccum, len(dirs))
	searchPaths := make([]string, 0, len(dirs))
	for _, d := range dirs {
		accums[d.Path] = &conceptAccum{
			dir:    d,
			meta:   readConceptManifestMeta(filepath.Join(d.Path, "concept-manifest.yml")),
			tokens: make(map[string]bool),
			files:  make(map[string]map[int]*conceptLineHit),
		}
		searchPaths = append(searchPaths, d.Path)
	}

	// Manifest title/description matching happens in-process (rg cannot tell
	// which yaml field a hit landed in); excluded from the overview scope.
	if scope != ConceptSearchInOverview {
		for _, a := range accums {
			titleLower := strings.ToLower(a.meta.Title)
			descLower := strings.ToLower(a.meta.Description)
			for _, t := range tokens {
				if strings.Contains(titleLower, t) {
					a.titleTokens++
					a.tokens[t] = true
				}
				if strings.Contains(descLower, t) {
					a.descTokens++
					a.tokens[t] = true
				}
			}
			if multiToken && (strings.Contains(titleLower, phrase) || strings.Contains(descLower, phrase)) {
				a.phrase = true
			}
		}
	}

	if scope == ConceptSearchInRole {
		// Role scope is intentionally in-memory: grep cannot constrain matches
		// to one Markdown section. Missing Role headings contribute no prose.
		for _, a := range accums {
			overviewPath := filepath.Join(a.dir.Path, "overview.md")
			content, err := os.ReadFile(overviewPath)
			if err != nil {
				continue
			}
			role, startLine := extractRoleSection(string(content))
			if role == "" {
				continue
			}
			for offset, line := range strings.Split(role, "\n") {
				trimmed := strings.TrimSpace(line)
				lower := strings.ToLower(trimmed)
				for _, token := range tokens {
					if !strings.Contains(lower, token) {
						continue
					}
					fileHits := a.files[overviewPath]
					if fileHits == nil {
						fileHits = make(map[int]*conceptLineHit)
						a.files[overviewPath] = fileHits
					}
					lineNum := startLine + offset
					if fileHits[lineNum] == nil {
						fileHits[lineNum] = &conceptLineHit{text: truncateSnippet(trimmed)}
					}
					a.tokens[token] = true
				}
				if multiToken && strings.Contains(lower, phrase) {
					a.phrase = true
				}
			}
		}
	} else {
		overviewOnly := scope == ConceptSearchInOverview
		for _, token := range tokens {
			lines, err := runConceptTokenSearch(token, searchPaths, overviewOnly)
			if err != nil {
				return ConceptSearchPage{}, err
			}
			for _, line := range lines {
				filePath, lineNum, text, ok := parseGrepLine(line)
				if !ok {
					continue
				}

				// Title/description lines of the manifest are scored in-process
				// above; skip them here to avoid double counting.
				if filepath.Base(filePath) == "concept-manifest.yml" {
					lower := strings.ToLower(strings.TrimSpace(text))
					if strings.HasPrefix(lower, "title:") || strings.HasPrefix(lower, "description:") {
						continue
					}
				}

				var a *conceptAccum
				for dirPath, cand := range accums {
					if strings.HasPrefix(filePath, dirPath+string(os.PathSeparator)) {
						a = cand
						break
					}
				}
				if a == nil {
					continue
				}

				trimmed := strings.TrimSpace(text)
				if multiToken && strings.Contains(strings.ToLower(trimmed), phrase) {
					a.phrase = true
				}
				fileHits := a.files[filePath]
				if fileHits == nil {
					fileHits = make(map[int]*conceptLineHit)
					a.files[filePath] = fileHits
				}
				if fileHits[lineNum] == nil {
					fileHits[lineNum] = &conceptLineHit{text: truncateSnippet(trimmed)}
				}
				a.tokens[token] = true
			}
		}
	}

	totalTokens := float64(len(tokens))
	results := make([]ConceptSearchResult, 0, len(dirs))
	for _, d := range dirs {
		a := accums[d.Path]
		if len(a.tokens) == 0 {
			continue
		}

		coverage := float64(len(a.tokens)) / totalTokens
		if coverage < opts.MinCoverage {
			continue
		}
		sum := 0.0
		if a.titleTokens > 0 {
			sum += conceptWeightTitle * (1 + math.Log(1+float64(a.titleTokens)))
		}
		if a.descTokens > 0 {
			sum += conceptWeightDescription * (1 + math.Log(1+float64(a.descTokens)))
		}

		filePaths := make([]string, 0, len(a.files))
		for fp := range a.files {
			filePaths = append(filePaths, fp)
		}
		sort.Strings(filePaths)

		files := make([]ConceptFileMatch, 0, len(filePaths))
		for _, fp := range filePaths {
			lineHits := a.files[fp]
			base := conceptWeightOther
			if filepath.Base(fp) == "overview.md" {
				base = conceptWeightOverview
			}
			fileScore := base * (1 + math.Log(1+float64(len(lineHits))))
			sum += fileScore

			lineNums := make([]int, 0, len(lineHits))
			for ln := range lineHits {
				lineNums = append(lineNums, ln)
			}
			sort.Ints(lineNums)
			matches := make([]ConceptSearchMatch, 0, len(lineNums))
			for _, ln := range lineNums {
				matches = append(matches, ConceptSearchMatch{LineNumber: ln, Text: lineHits[ln].text})
			}

			files = append(files, ConceptFileMatch{
				FilePath: fp,
				Score:    roundScore(fileScore),
				Matches:  matches,
			})
		}
		sort.SliceStable(files, func(i, j int) bool {
			if files[i].Score != files[j].Score {
				return files[i].Score > files[j].Score
			}
			return files[i].FilePath < files[j].FilePath
		})

		if a.phrase {
			sum += conceptPhraseBonus
		}

		results = append(results, ConceptSearchResult{
			ConceptID:   d.ID,
			Workspace:   d.Workspace,
			Title:       a.meta.Title,
			Description: a.meta.Description,
			Score:       roundScore(coverage * sum),
			Files:       files,
		})
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].ConceptID < results[j].ConceptID
	})

	eligibleTotal := len(results)
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}
	return ConceptSearchPage{Results: results, EligibleTotal: eligibleTotal}, nil
}

// runConceptTokenSearch runs one fixed-string, case-insensitive token search
// over the given directories and returns raw "file:line:text" output lines.
func runConceptTokenSearch(token string, dirs []string, overviewOnly bool) ([]string, error) {
	var cmd *exec.Cmd
	if rgPath, err := exec.LookPath("rg"); err == nil {
		args := []string{"-n", "--ignore-case", "--fixed-strings"}
		if overviewOnly {
			args = append(args, "--glob", "overview.md")
		}
		args = append(args, "--", token)
		args = append(args, dirs...)
		cmd = exec.Command(rgPath, args...)
	} else if grepPath, err := exec.LookPath("grep"); err == nil {
		args := []string{"-rniF"}
		if overviewOnly {
			args = append(args, "--include=overview.md")
		}
		args = append(args, "-e", token)
		args = append(args, dirs...)
		cmd = exec.Command(grepPath, args...)
	} else {
		return nil, fmt.Errorf("neither 'rg' nor 'grep' found in PATH")
	}

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Exit code 1 means no matches found
			if exitErr.ExitCode() == 1 {
				return nil, nil
			}
			return nil, fmt.Errorf("search command failed: %w, stderr: %s", err, exitErr.Stderr)
		}
		return nil, fmt.Errorf("search command failed: %w", err)
	}

	return strings.Split(string(output), "\n"), nil
}

// parseGrepLine splits one "path:linenum:content" output line.
func parseGrepLine(line string) (filePath string, lineNum int, text string, ok bool) {
	if strings.TrimSpace(line) == "" {
		return "", 0, "", false
	}
	firstColon := strings.Index(line, ":")
	if firstColon == -1 {
		return "", 0, "", false
	}
	rest := line[firstColon+1:]
	secondColon := strings.Index(rest, ":")
	if secondColon == -1 {
		return "", 0, "", false
	}
	if _, err := fmt.Sscanf(rest[:secondColon], "%d", &lineNum); err != nil {
		return "", 0, "", false
	}
	return line[:firstColon], lineNum, rest[secondColon+1:], true
}

// extractRoleSection returns only the prose beneath a level-two Role heading,
// plus the 1-based line number of the first returned line. It stops at the next
// level-one or level-two heading. Missing Role headings return no prose.
func extractRoleSection(content string) (string, int) {
	lines := strings.Split(content, "\n")
	start := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if start < 0 {
			if strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ") {
				heading := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(strings.TrimPrefix(trimmed, "##")), "#"))
				if strings.EqualFold(heading, "role") {
					start = i + 1
				}
			}
			continue
		}
		if strings.HasPrefix(trimmed, "# ") || (strings.HasPrefix(trimmed, "## ") && !strings.HasPrefix(trimmed, "### ")) {
			return strings.Join(lines[start:i], "\n"), start + 1
		}
	}
	if start >= 0 {
		return strings.Join(lines[start:], "\n"), start + 1
	}
	return "", 0
}

var conceptStopwords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {}, "be": {},
	"by": {}, "do": {}, "does": {}, "for": {}, "from": {}, "how": {}, "in": {},
	"is": {}, "it": {}, "of": {}, "on": {}, "or": {}, "that": {}, "the": {},
	"this": {}, "to": {}, "what": {}, "when": {}, "where": {}, "which": {},
	"who": {}, "why": {}, "with": {},
}

// tokenizeConceptQuery lowercases and splits a query on whitespace,
// deduplicating tokens while preserving order. Stopwords are removed only if
// at least one content token remains, so an all-stopword query still searches.
func tokenizeConceptQuery(query string) []string {
	fields := strings.Fields(strings.ToLower(query))
	seen := make(map[string]bool, len(fields))
	all := make([]string, 0, len(fields))
	content := make([]string, 0, len(fields))
	for _, f := range fields {
		if seen[f] {
			continue
		}
		seen[f] = true
		all = append(all, f)
		if _, stop := conceptStopwords[f]; !stop {
			content = append(content, f)
		}
	}
	if len(content) > 0 {
		return content
	}
	return all
}

// readConceptManifestMeta parses a concept-manifest.yml, tolerating absence
// or invalid yaml (search still covers the concept's files).
func readConceptManifestMeta(path string) conceptManifestMeta {
	var meta conceptManifestMeta
	data, err := os.ReadFile(path)
	if err != nil {
		return meta
	}
	_ = yaml.Unmarshal(data, &meta)
	return meta
}

func truncateSnippet(s string) string {
	return truncateRunes(s, conceptSnippetMaxRunes)
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

func roundScore(f float64) float64 {
	return math.Round(f*100) / 100
}
