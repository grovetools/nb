// Package index implements the Phase-1 vault indexer for nb: wikilink
// parsing, backlink and tag inversion, Obsidian-style link resolution, and a
// tokenized full-text search over a set of markdown roots.
//
// The index is a rebuildable cache over the vault on disk — disk stays truth.
// Callers hand Build a set of roots to scan; the package depends only on the
// standard library and nb/pkg/frontmatter, and never imports the workspace
// locator (callers wire locator → roots at the edge).
package index

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Root is a directory tree to scan for markdown files. Workspace labels every
// doc found under Dir and is used for same-workspace tie-breaking during link
// resolution.
type Root struct {
	Dir       string
	Workspace string
}

// Heading is a markdown ATX heading within a doc.
type Heading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
	Line  int    `json:"line"`
}

// Link is an outgoing wikilink or markdown link from a doc.
type Link struct {
	SourcePath   string `json:"source_path"`
	RawTarget    string `json:"raw_target"`        // target as written, without #heading/|display parts
	Heading      string `json:"heading,omitempty"` // optional #heading fragment
	Display      string `json:"display,omitempty"` // optional |display text
	Line         int    `json:"line"`
	ResolvedPath string `json:"resolved_path,omitempty"` // "" if unresolved

	wiki  bool // wikilink vs plain markdown link
	embed bool // ![[...]] embed form
}

// Doc is one indexed markdown file.
type Doc struct {
	Path      string    `json:"path"`
	Workspace string    `json:"workspace"`
	NoteType  string    `json:"note_type,omitempty"`
	ID        string    `json:"id,omitempty"`
	Title     string    `json:"title"`
	Aliases   []string  `json:"aliases,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Headings  []Heading `json:"headings,omitempty"`
	Links     []Link    `json:"links,omitempty"`
	PlanRef   string    `json:"plan_ref,omitempty"`
	Archived  bool      `json:"archived,omitempty"`
	ModTime   time.Time `json:"mod_time"`

	bodyStart int // 0-based line index where the body begins (after frontmatter)
}

// Index holds the built vault index. It is safe for concurrent reads after
// Build returns; Build itself must not race with readers.
type Index struct {
	docs       map[string]*Doc   // abs path -> doc
	backlinks  map[string][]Link // abs path -> incoming resolved links
	unresolved []Link
	tagDocs    map[string][]*Doc // tag -> docs
	byStem     map[string][]*Doc // lower filename stem -> docs
	byID       map[string][]*Doc // lower frontmatter id -> docs
	byAlias    map[string][]*Doc // lower alias -> docs
	byTitle    map[string][]*Doc // lower title -> docs
	postings   map[string]map[string]*postingCount
	bodyLines  map[string][]string // abs path -> raw file lines (for snippets)
}

// New creates an empty Index. Call Build before querying.
func New() *Index {
	return &Index{
		docs:      make(map[string]*Doc),
		backlinks: make(map[string][]Link),
		tagDocs:   make(map[string][]*Doc),
		byStem:    make(map[string][]*Doc),
		byID:      make(map[string][]*Doc),
		byAlias:   make(map[string][]*Doc),
		byTitle:   make(map[string][]*Doc),
		postings:  make(map[string]map[string]*postingCount),
		bodyLines: make(map[string][]string),
	}
}

// Build scans every root for *.md files, parses them, resolves links, and
// populates the backlink/tag/full-text inversions. Roots that do not exist
// are skipped; a file seen under two overlapping roots is indexed once (first
// root wins). Files under a .archive directory are included and flagged.
func (ix *Index) Build(roots []Root) error {
	for _, root := range roots {
		absRoot, err := filepath.Abs(root.Dir)
		if err != nil {
			continue
		}
		if info, err := os.Stat(absRoot); err != nil || !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip unreadable entries
			}
			if d.IsDir() {
				// Skip hidden dirs except .archive (archived notes stay indexed).
				name := d.Name()
				if strings.HasPrefix(name, ".") && name != ".archive" && path != absRoot {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.EqualFold(filepath.Ext(path), ".md") {
				return nil
			}
			if _, seen := ix.docs[path]; seen {
				return nil
			}
			doc, lines, perr := parseFile(path, absRoot, root.Workspace)
			if perr != nil {
				return nil // unreadable file: skip, disk stays truth
			}
			ix.docs[path] = doc
			ix.bodyLines[path] = lines
			return nil
		})
		if err != nil {
			return err
		}
	}

	ix.buildLookups()
	ix.resolveLinks()
	ix.buildInversions()
	return nil
}

// Doc returns the indexed doc for an absolute path.
func (ix *Index) Doc(path string) (*Doc, bool) {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	d, ok := ix.docs[path]
	return d, ok
}

// Docs returns all indexed docs sorted by path.
func (ix *Index) Docs() []*Doc {
	out := make([]*Doc, 0, len(ix.docs))
	for _, d := range ix.docs {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Backlinks returns the links whose ResolvedPath is the given doc path.
func (ix *Index) Backlinks(path string) []Link {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return ix.backlinks[path]
}

// Unresolved returns every wikilink in the vault that resolved to no doc.
func (ix *Index) Unresolved() []Link {
	return ix.unresolved
}

// Tags returns tag -> doc count over the whole vault.
func (ix *Index) Tags() map[string]int {
	out := make(map[string]int, len(ix.tagDocs))
	for tag, docs := range ix.tagDocs {
		out[tag] = len(docs)
	}
	return out
}

// DocsByTag returns the docs carrying the given tag, sorted by path.
func (ix *Index) DocsByTag(tag string) []*Doc {
	return ix.tagDocs[tag]
}

// Resolve resolves a wikilink-style target to candidate docs, Obsidian-style:
// filename stem match first (path-suffix match when the target contains a
// slash), then frontmatter id, then aliases, then exact title — all
// case-insensitive. Candidates within each tier are sorted by path.
func (ix *Index) Resolve(target string) []*Doc {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	lower := strings.ToLower(target)

	var stemMatches []*Doc
	if strings.Contains(lower, "/") {
		// Path-qualified target: match docs whose extension-less path ends
		// with the target.
		suffix := "/" + strings.TrimSuffix(strings.TrimPrefix(lower, "/"), ".md")
		for _, d := range ix.docs {
			p := strings.ToLower(strings.TrimSuffix(d.Path, filepath.Ext(d.Path)))
			if strings.HasSuffix(p, suffix) {
				stemMatches = append(stemMatches, d)
			}
		}
		sort.Slice(stemMatches, func(i, j int) bool { return stemMatches[i].Path < stemMatches[j].Path })
	} else {
		stemMatches = ix.byStem[strings.TrimSuffix(lower, ".md")]
	}
	if len(stemMatches) > 0 {
		return stemMatches
	}
	if m := ix.byID[lower]; len(m) > 0 {
		return m
	}
	if m := ix.byAlias[lower]; len(m) > 0 {
		return m
	}
	return ix.byTitle[lower]
}

func (ix *Index) buildLookups() {
	for _, d := range ix.docs {
		stem := strings.ToLower(strings.TrimSuffix(filepath.Base(d.Path), filepath.Ext(d.Path)))
		ix.byStem[stem] = append(ix.byStem[stem], d)
		if d.ID != "" {
			ix.byID[strings.ToLower(d.ID)] = append(ix.byID[strings.ToLower(d.ID)], d)
		}
		for _, a := range d.Aliases {
			if a != "" {
				ix.byAlias[strings.ToLower(a)] = append(ix.byAlias[strings.ToLower(a)], d)
			}
		}
		if d.Title != "" {
			ix.byTitle[strings.ToLower(d.Title)] = append(ix.byTitle[strings.ToLower(d.Title)], d)
		}
	}
	for _, m := range []map[string][]*Doc{ix.byStem, ix.byID, ix.byAlias, ix.byTitle} {
		for _, docs := range m {
			sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
		}
	}
}

// resolveLinks fills in ResolvedPath on every parsed link. Wikilinks that
// resolve to nothing are kept (they feed Unresolved); markdown links that do
// not land on an indexed vault file are dropped entirely.
func (ix *Index) resolveLinks() {
	for _, d := range ix.docs {
		kept := d.Links[:0]
		for _, l := range d.Links {
			if l.wiki {
				if target := ix.pickResolution(l.RawTarget, d.Workspace); target != nil {
					l.ResolvedPath = target.Path
				}
				kept = append(kept, l)
				continue
			}
			// Markdown link: resolve relative to the source file's directory.
			if target := ix.resolveMarkdownHref(d.Path, l.RawTarget); target != nil {
				l.ResolvedPath = target.Path
				kept = append(kept, l)
			}
		}
		d.Links = kept
	}
}

// pickResolution picks one doc for a wikilink target, preferring docs in the
// source's workspace when the match is ambiguous.
func (ix *Index) pickResolution(target, sourceWorkspace string) *Doc {
	candidates := ix.Resolve(target)
	if len(candidates) == 0 {
		return nil
	}
	for _, c := range candidates {
		if c.Workspace == sourceWorkspace {
			return c
		}
	}
	return candidates[0]
}

func (ix *Index) resolveMarkdownHref(sourcePath, href string) *Doc {
	if href == "" {
		return nil
	}
	href = pathUnescape(href)
	var abs string
	if filepath.IsAbs(href) {
		abs = filepath.Clean(href)
	} else {
		abs = filepath.Clean(filepath.Join(filepath.Dir(sourcePath), href))
	}
	if d, ok := ix.docs[abs]; ok {
		return d
	}
	return nil
}

func (ix *Index) buildInversions() {
	for _, d := range ix.docs {
		for _, l := range d.Links {
			if l.ResolvedPath != "" {
				ix.backlinks[l.ResolvedPath] = append(ix.backlinks[l.ResolvedPath], l)
			} else if l.wiki {
				ix.unresolved = append(ix.unresolved, l)
			}
		}
		for _, tag := range d.Tags {
			ix.tagDocs[tag] = append(ix.tagDocs[tag], d)
		}
	}
	for _, links := range ix.backlinks {
		sort.Slice(links, func(i, j int) bool {
			if links[i].SourcePath != links[j].SourcePath {
				return links[i].SourcePath < links[j].SourcePath
			}
			return links[i].Line < links[j].Line
		})
	}
	sort.Slice(ix.unresolved, func(i, j int) bool {
		if ix.unresolved[i].SourcePath != ix.unresolved[j].SourcePath {
			return ix.unresolved[i].SourcePath < ix.unresolved[j].SourcePath
		}
		return ix.unresolved[i].Line < ix.unresolved[j].Line
	})
	for _, docs := range ix.tagDocs {
		sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	}
	ix.buildPostings()
}

// pathUnescape decodes %XX escapes in markdown hrefs (e.g. %20 for space)
// without pulling in net/url semantics for '+'.
func pathUnescape(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			hi, ok1 := unhex(s[i+1])
			lo, ok2 := unhex(s[i+2])
			if ok1 && ok2 {
				b.WriteByte(hi<<4 | lo)
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func unhex(c byte) (byte, bool) {
	switch {
	case '0' <= c && c <= '9':
		return c - '0', true
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, true
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
