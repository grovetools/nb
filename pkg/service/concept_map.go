package service

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	Dir     string   `json:"mapDir"`
	Wrote   []string `json:"wrote"`
	Updated []string `json:"updated"`
	Skipped []string `json:"skipped"`
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
//   - likec4.config.json and package.json are (re)written whenever their
//     generated content changed (e.g. a likec4 pin bump);
//   - src/*.c4 and .gitignore are only created when missing — existing files
//     are never touched.
//
// An empty title falls back to the concept-manifest.yml title, then to id.
func (s *Service) ScaffoldConceptMap(dir, id, title string) (*ConceptMapScaffold, error) {
	return scaffoldConceptMap(dir, id, title)
}

// conceptMapFile is one scaffold entry; overwrite selects the
// rewrite-if-generated-content-changed behavior.
type conceptMapFile struct {
	relPath   string
	content   []byte
	overwrite bool
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

	result := &ConceptMapScaffold{Dir: dir}
	for _, f := range files {
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
	return result, nil
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
		{relPath: "likec4.config.json", content: configContent, overwrite: true},
		{relPath: "package.json", content: packageContent, overwrite: true},
		{relPath: filepath.Join("src", "_spec.c4"), content: spec},
		{relPath: filepath.Join("src", "model.c4"), content: model},
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
