package service

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// ConceptMapBackendLikeC4 is the default (and currently only) concept map backend.
const ConceptMapBackendLikeC4 = "likec4"

// conceptMapLikeC4Version is the pinned likec4 devDependency written into a
// scaffolded map's package.json. Bumping it makes `nb concept map update`
// rewrite generated files on the next run.
const conceptMapLikeC4Version = "1.59.2"

//go:embed all:templates/concept-map
var conceptMapTemplatesFS embed.FS

const conceptMapTemplateRoot = "templates/concept-map"

// ConceptMapScaffold reports what a scaffold pass did to a concept directory.
type ConceptMapScaffold struct {
	Dir      string   `json:"mapDir"`
	Wrote    []string `json:"wrote"`
	Updated  []string `json:"updated"`
	Skipped  []string `json:"skipped"`
	Warnings []string `json:"warnings,omitempty"`
}

// ValidateConceptMapID rejects ids that LikeC4 cannot accept as a project
// name or that would misbehave as a concept directory name.
func ValidateConceptMapID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("concept map id is required")
	}
	if id == "default" {
		return fmt.Errorf("invalid concept map id %q: LikeC4 project names must not be \"default\"", id)
	}
	if strings.ContainsAny(id, ".@#") {
		return fmt.Errorf("invalid concept map id %q: LikeC4 project names cannot contain '.', '@', or '#'", id)
	}
	if strings.ContainsAny(id, "/\\ \t") {
		return fmt.Errorf("invalid concept map id %q: must be a single directory name without spaces or path separators", id)
	}
	return nil
}

// ScaffoldConceptMap writes the LikeC4 project scaffold into the concept
// directory dir. It is idempotent and safe to re-run:
//   - likec4.config.json is merge-rewritten: only the generated keys
//     ($schema, name, title) are owned by the scaffold; every other key the
//     user added (include, exclude, ...) is preserved. An unparseable config
//     is left untouched and reported as skipped with a warning;
//   - package.json is (re)written whenever its generated content changed
//     (e.g. a likec4 pin bump);
//   - the seed src/*.c4 files are only created when src/ contains no .c4
//     files at all (fresh scaffold) — a map whose content has evolved never
//     gets template files resurrected;
//   - .gitignore is only created when missing.
//
// An empty title falls back to the concept-manifest.yml title, then to id.
func (s *Service) ScaffoldConceptMap(dir, id, title string) (*ConceptMapScaffold, error) {
	return scaffoldConceptMap(dir, id, title)
}

// conceptMapFile is one scaffold entry; overwrite selects the
// rewrite-if-generated-content-changed behavior, seed marks the src/*.c4
// files that only exist to seed a brand-new map.
type conceptMapFile struct {
	relPath   string
	content   []byte
	overwrite bool
	seed      bool
}

func scaffoldConceptMap(dir, id, title string) (*ConceptMapScaffold, error) {
	if err := ValidateConceptMapID(id); err != nil {
		return nil, err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("concept directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("concept path %s is not a directory", dir)
	}
	if title == "" {
		title = conceptMapFallbackTitle(dir, id)
	}

	files, err := renderConceptMapFiles(id, title)
	if err != nil {
		return nil, err
	}

	srcHasC4, err := conceptMapSrcHasC4(dir)
	if err != nil {
		return nil, err
	}

	result := &ConceptMapScaffold{Dir: dir}
	for _, f := range files {
		if f.relPath == conceptMapConfigFile {
			if err := scaffoldConceptMapConfig(dir, f.content, result); err != nil {
				return nil, err
			}
			continue
		}
		if f.seed && srcHasC4 {
			// The map's .c4 content has evolved past the seed files; never
			// resurrect template content into a mature src/ tree.
			result.Skipped = append(result.Skipped, f.relPath)
			continue
		}
		path := filepath.Join(dir, f.relPath)
		existing, err := os.ReadFile(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, fmt.Errorf("create %s parent directory: %w", f.relPath, err)
			}
			if err := os.WriteFile(path, f.content, 0o644); err != nil {
				return nil, fmt.Errorf("write %s: %w", f.relPath, err)
			}
			result.Wrote = append(result.Wrote, f.relPath)
		case err != nil:
			return nil, fmt.Errorf("read %s: %w", f.relPath, err)
		case !f.overwrite || bytes.Equal(existing, f.content):
			result.Skipped = append(result.Skipped, f.relPath)
		default:
			if err := os.WriteFile(path, f.content, 0o644); err != nil {
				return nil, fmt.Errorf("rewrite %s: %w", f.relPath, err)
			}
			result.Updated = append(result.Updated, f.relPath)
		}
	}
	warnConceptMapIncludePaths(dir, result)
	return result, nil
}

// conceptMapConfigFile is the LikeC4 project config; it is merge-rewritten
// rather than template-owned because users legitimately extend it (e.g. an
// include block pointing at federated concept detail files).
const conceptMapConfigFile = "likec4.config.json"

// conceptMapConfigOwnedKeys are the likec4.config.json keys the scaffold
// generates and keeps refreshed; every other key belongs to the user.
var conceptMapConfigOwnedKeys = []string{"$schema", "name", "title"}

// scaffoldConceptMapConfig writes likec4.config.json. A missing file gets the
// rendered template verbatim. An existing file is parsed and only the owned
// keys are set/updated — all user keys survive — and the file is rewritten
// only when the merge actually changed something. An unparseable file is left
// untouched and reported as skipped with a warning.
func scaffoldConceptMapConfig(dir string, rendered []byte, result *ConceptMapScaffold) error {
	path := filepath.Join(dir, conceptMapConfigFile)
	existing, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, rendered, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", conceptMapConfigFile, err)
		}
		result.Wrote = append(result.Wrote, conceptMapConfigFile)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", conceptMapConfigFile, err)
	}

	var current map[string]any
	if err := json.Unmarshal(existing, &current); err != nil {
		result.Skipped = append(result.Skipped, conceptMapConfigFile)
		result.Warnings = append(result.Warnings, fmt.Sprintf(
			"%s is not valid JSON (%v); left untouched — fix it and re-run update", conceptMapConfigFile, err))
		return nil
	}
	var generated map[string]any
	if err := json.Unmarshal(rendered, &generated); err != nil {
		return fmt.Errorf("rendered %s template is not valid JSON: %w", conceptMapConfigFile, err)
	}

	merged := make(map[string]any, len(current)+len(conceptMapConfigOwnedKeys))
	for k, v := range current {
		merged[k] = v
	}
	for _, k := range conceptMapConfigOwnedKeys {
		merged[k] = generated[k]
	}
	if reflect.DeepEqual(merged, current) {
		result.Skipped = append(result.Skipped, conceptMapConfigFile)
		return nil
	}

	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal merged %s: %w", conceptMapConfigFile, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("rewrite %s: %w", conceptMapConfigFile, err)
	}
	result.Updated = append(result.Updated, conceptMapConfigFile)
	return nil
}

// conceptMapSrcHasC4 reports whether the map's src/ tree already contains any
// .c4 file. A missing src/ directory means a fresh scaffold.
func conceptMapSrcHasC4(dir string) (bool, error) {
	src := filepath.Join(dir, "src")
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("stat src directory: %w", err)
	}
	found := false
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".c4") {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("scan src directory: %w", err)
	}
	return found, nil
}

// warnConceptMapIncludePaths checks the on-disk (merged) config's
// include.paths entries and records a warning for every entry that does not
// resolve to an existing directory relative to the config's folder. It never
// fails the scaffold: an unreadable or unparseable config is simply skipped
// (the latter already carries its own warning).
func warnConceptMapIncludePaths(dir string, result *ConceptMapScaffold) {
	data, err := os.ReadFile(filepath.Join(dir, conceptMapConfigFile))
	if err != nil {
		return
	}
	var config struct {
		Include struct {
			Paths []string `json:"paths"`
		} `json:"include"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return
	}
	for _, p := range config.Include.Paths {
		resolved := p
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(dir, resolved)
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"%s include.paths entry %q does not resolve to a directory (%s)", conceptMapConfigFile, p, resolved))
		}
	}
}

// renderConceptMapFiles produces the full scaffold file set for a map.
func renderConceptMapFiles(id, title string) ([]conceptMapFile, error) {
	configContent, err := renderConceptMapTemplate("likec4.config.json.tmpl", id, title)
	if err != nil {
		return nil, err
	}
	packageContent, err := renderConceptMapTemplate("package.json.tmpl", id, title)
	if err != nil {
		return nil, err
	}
	spec, err := conceptMapTemplatesFS.ReadFile(conceptMapTemplateRoot + "/src/_spec.c4")
	if err != nil {
		return nil, fmt.Errorf("read embedded _spec.c4 template: %w", err)
	}
	model, err := conceptMapTemplatesFS.ReadFile(conceptMapTemplateRoot + "/src/model.c4")
	if err != nil {
		return nil, fmt.Errorf("read embedded model.c4 template: %w", err)
	}
	// Stored as "gitignore" so the template itself is not treated as a live
	// ignore file inside this repository.
	gitignore, err := conceptMapTemplatesFS.ReadFile(conceptMapTemplateRoot + "/gitignore")
	if err != nil {
		return nil, fmt.Errorf("read embedded gitignore template: %w", err)
	}

	return []conceptMapFile{
		{relPath: conceptMapConfigFile, content: configContent, overwrite: true},
		{relPath: "package.json", content: packageContent, overwrite: true},
		{relPath: filepath.Join("src", "_spec.c4"), content: spec, seed: true},
		{relPath: filepath.Join("src", "model.c4"), content: model, seed: true},
		{relPath: ".gitignore", content: gitignore},
	}, nil
}

// renderConceptMapTemplate renders one embedded .tmpl file. String values are
// pre-encoded as JSON so titles with quotes cannot corrupt the output.
func renderConceptMapTemplate(name, id, title string) ([]byte, error) {
	raw, err := conceptMapTemplatesFS.ReadFile(conceptMapTemplateRoot + "/" + name)
	if err != nil {
		return nil, fmt.Errorf("read embedded template %s: %w", name, err)
	}
	tmpl, err := template.New(name).Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", name, err)
	}
	data := map[string]string{
		"NameJSON":          jsonString(id),
		"TitleJSON":         jsonString(title),
		"PackageNameJSON":   jsonString(id + "-map"),
		"LikeC4VersionJSON": jsonString(conceptMapLikeC4Version),
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render template %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

func jsonString(s string) string {
	data, err := json.Marshal(s)
	if err != nil {
		// Marshalling a string cannot fail; keep the template renderable anyway.
		return `""`
	}
	return string(data)
}

// conceptMapFallbackTitle recovers a map title for scaffold-into-existing
// paths (`nb concept map update`): the concept manifest's title wins, then the
// id.
func conceptMapFallbackTitle(dir, id string) string {
	data, err := os.ReadFile(filepath.Join(dir, "concept-manifest.yml"))
	if err == nil {
		var manifest struct {
			Title string `yaml:"title"`
		}
		if yaml.Unmarshal(data, &manifest) == nil && strings.TrimSpace(manifest.Title) != "" {
			return manifest.Title
		}
	}
	return id
}
