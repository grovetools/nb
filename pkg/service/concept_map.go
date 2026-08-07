package service

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
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

// ConceptMapDetailDir is the folder inside a concept that holds its federated
// LikeC4 detail files. Attaching a concept to a map adds this folder to the
// map config's include.paths, so the concept can decompose "its" element
// without touching the map's own sources.
const ConceptMapDetailDir = "likec4"

// ConceptMapAttachment reports the outcome of an attach/detach pass over a
// map's likec4.config.json.
type ConceptMapAttachment struct {
	MapDir     string `json:"mapDir"`
	ConceptDir string `json:"conceptDir"`
	// DetailDir is the concept's likec4/ folder, as an absolute-or-caller-
	// relative path (the same form the caller passed for the concept).
	DetailDir string `json:"detailDir"`
	// Path is DetailDir expressed relative to the map's likec4.config.json
	// folder — exactly the string that lives in include.paths.
	Path string `json:"path"`
	// Changed reports whether the config was rewritten; a repeated attach or a
	// detach of something that was never attached leaves it false.
	Changed bool `json:"changed"`
	// CreatedDetailDir reports that attach had to create the concept's likec4/
	// folder.
	CreatedDetailDir bool     `json:"createdDetailDir"`
	IncludePaths     []string `json:"includePaths"`
}

// ConceptMapInfo is a concept that carries a LikeC4 project.
type ConceptMapInfo struct {
	ConceptInfo
	MapDir string `json:"mapDir"`
}

// ListConceptMaps returns every concept across the known workspaces that
// carries a LikeC4 project (i.e. a likec4.config.json), sorted by workspace
// then id. It is what lets `attach`/`detach` default to the sole map.
func (s *Service) ListConceptMaps() ([]ConceptMapInfo, error) {
	concepts, err := s.ListAllConcepts()
	if err != nil {
		return nil, err
	}
	var maps []ConceptMapInfo
	for _, concept := range concepts {
		info, err := os.Stat(filepath.Join(concept.Path, conceptMapConfigFile))
		if err != nil || info.IsDir() {
			continue
		}
		maps = append(maps, ConceptMapInfo{ConceptInfo: concept, MapDir: concept.Path})
	}
	sort.Slice(maps, func(i, j int) bool {
		if maps[i].Workspace != maps[j].Workspace {
			return maps[i].Workspace < maps[j].Workspace
		}
		return maps[i].ID < maps[j].ID
	})
	return maps, nil
}

// AttachConceptToMap adds the concept's likec4/ detail folder to the map's
// include.paths, creating the folder when missing. The config is merge-
// rewritten: every user key (and every other include key) survives. Attaching
// an already-attached concept is a no-op.
func (s *Service) AttachConceptToMap(mapDir, conceptDir string) (*ConceptMapAttachment, error) {
	return attachConceptToMap(mapDir, conceptDir)
}

// DetachConceptFromMap removes the concept's likec4/ detail folder from the
// map's include.paths, leaving every other entry — and every user key — in
// place. Detaching something that was never attached is a no-op.
func (s *Service) DetachConceptFromMap(mapDir, conceptDir string) (*ConceptMapAttachment, error) {
	return detachConceptFromMap(mapDir, conceptDir)
}

func attachConceptToMap(mapDir, conceptDir string) (*ConceptMapAttachment, error) {
	result, err := newConceptMapAttachment(mapDir, conceptDir)
	if err != nil {
		return nil, err
	}

	switch info, err := os.Stat(result.DetailDir); {
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(result.DetailDir, 0o755); err != nil {
			return nil, fmt.Errorf("create concept detail directory: %w", err)
		}
		result.CreatedDetailDir = true
	case err != nil:
		return nil, fmt.Errorf("stat concept detail directory: %w", err)
	case !info.IsDir():
		return nil, fmt.Errorf("%s exists and is not a directory", result.DetailDir)
	}

	config, err := loadConceptMapConfig(mapDir)
	if err != nil {
		return nil, err
	}
	paths, err := conceptMapIncludePaths(config)
	if err != nil {
		return nil, err
	}
	for _, existing := range paths {
		if conceptMapPathsEqual(existing, result.Path) {
			result.IncludePaths = paths
			return result, nil
		}
	}

	paths = append(paths, result.Path)
	if err := writeConceptMapIncludePaths(mapDir, config, paths); err != nil {
		return nil, err
	}
	result.Changed = true
	result.IncludePaths = paths
	return result, nil
}

func detachConceptFromMap(mapDir, conceptDir string) (*ConceptMapAttachment, error) {
	result, err := newConceptMapAttachment(mapDir, conceptDir)
	if err != nil {
		return nil, err
	}

	config, err := loadConceptMapConfig(mapDir)
	if err != nil {
		return nil, err
	}
	paths, err := conceptMapIncludePaths(config)
	if err != nil {
		return nil, err
	}
	kept := make([]string, 0, len(paths))
	for _, existing := range paths {
		if conceptMapPathsEqual(existing, result.Path) {
			continue
		}
		kept = append(kept, existing)
	}
	result.IncludePaths = kept
	if len(kept) == len(paths) {
		return result, nil
	}

	if err := writeConceptMapIncludePaths(mapDir, config, kept); err != nil {
		return nil, err
	}
	result.Changed = true
	return result, nil
}

// newConceptMapAttachment resolves the two directories and computes the
// include.paths entry: the concept's likec4/ folder relative to the folder
// holding the map's likec4.config.json.
func newConceptMapAttachment(mapDir, conceptDir string) (*ConceptMapAttachment, error) {
	mapAbs, err := conceptMapRealPath(mapDir)
	if err != nil {
		return nil, err
	}
	conceptAbs, err := conceptMapRealPath(conceptDir)
	if err != nil {
		return nil, err
	}
	if mapAbs == conceptAbs {
		return nil, fmt.Errorf("cannot attach a concept map to itself (%s)", mapDir)
	}

	rel, err := filepath.Rel(mapAbs, filepath.Join(conceptAbs, ConceptMapDetailDir))
	if err != nil {
		return nil, fmt.Errorf("compute path from map %s to concept %s: %w", mapDir, conceptDir, err)
	}
	return &ConceptMapAttachment{
		MapDir:     mapDir,
		ConceptDir: conceptDir,
		DetailDir:  filepath.Join(conceptDir, ConceptMapDetailDir),
		Path:       filepath.ToSlash(rel),
	}, nil
}

// conceptMapRealPath makes a directory absolute and resolves symlinks so that
// the relative path between a map and a concept is computed over the same
// physical tree LikeC4 will walk (macOS /var -> /private/var, notebooks
// reached through symlinked workspace roots).
func conceptMapRealPath(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", dir, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", dir, err)
	}
	return resolved, nil
}

// loadConceptMapConfig reads the map's likec4.config.json as a generic object
// so a rewrite can preserve every key it does not own. Unlike the scaffold,
// attach/detach refuse to touch an unparseable config.
func loadConceptMapConfig(mapDir string) (map[string]any, error) {
	path := filepath.Join(mapDir, conceptMapConfigFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%s has no %s; run 'nb concept map update' to scaffold one", mapDir, conceptMapConfigFile)
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", conceptMapConfigFile, err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON (%v); fix it and re-run", path, err)
	}
	if config == nil {
		config = map[string]any{}
	}
	return config, nil
}

// conceptMapIncludePaths reads include.paths out of a parsed config. A missing
// include block is an empty list; a malformed one is an error rather than
// something to silently overwrite.
func conceptMapIncludePaths(config map[string]any) ([]string, error) {
	raw, ok := config["include"]
	if !ok || raw == nil {
		return nil, nil
	}
	include, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: include must be an object", conceptMapConfigFile)
	}
	rawPaths, ok := include["paths"]
	if !ok || rawPaths == nil {
		return nil, nil
	}
	list, ok := rawPaths.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: include.paths must be an array of strings", conceptMapConfigFile)
	}
	paths := make([]string, 0, len(list))
	for _, item := range list {
		path, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s: include.paths must contain only strings (found %T)", conceptMapConfigFile, item)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// writeConceptMapIncludePaths rewrites the config with a new include.paths,
// preserving every other top-level key and every other key inside include.
func writeConceptMapIncludePaths(mapDir string, config map[string]any, paths []string) error {
	include, _ := config["include"].(map[string]any)
	if include == nil {
		include = map[string]any{}
	}
	list := make([]any, 0, len(paths))
	for _, path := range paths {
		list = append(list, path)
	}
	include["paths"] = list
	config["include"] = include

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", conceptMapConfigFile, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(mapDir, conceptMapConfigFile), data, 0o644); err != nil {
		return fmt.Errorf("rewrite %s: %w", conceptMapConfigFile, err)
	}
	return nil
}

// conceptMapPathsEqual compares two include.paths entries as paths, so that
// "./x/y", "x/y" and "x//y" all count as the same attachment.
func conceptMapPathsEqual(a, b string) bool {
	return path.Clean(filepath.ToSlash(a)) == path.Clean(filepath.ToSlash(b))
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
