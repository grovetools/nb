package index

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/grovetools/nb/pkg/frontmatter"
)

var (
	fenceRe      = regexp.MustCompile("^\\s{0,3}(`{3,}|~{3,})")
	inlineCodeRe = regexp.MustCompile("`[^`]*`")
	headingRe    = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	wikilinkRe   = regexp.MustCompile(`(!?)\[\[([^\[\]]+)\]\]`)
	// [text](href) — href up to the first closing paren. Non-.md hrefs
	// (images, binaries) are dropped in parseMarkdownHref.
	mdLinkRe = regexp.MustCompile(`\[[^\]]*\]\(([^()\s]+)\)`)
	// Inline #tag: preceded by start-of-line, whitespace or '('; tag chars are
	// unicode letters/digits plus _ / -. A heading marker never matches
	// because '# ' has a space right after the hash.
	inlineTagRe = regexp.MustCompile(`(?:^|[\s(])#([\p{L}\p{N}_][\p{L}\p{N}_/-]*)`)
	nonDigitRe  = regexp.MustCompile(`[^\d/]`)
)

// parseFile reads and parses one markdown file into a Doc. Returned lines are
// the raw file lines, used later for search snippets.
func parseFile(path, root, workspace string) (*Doc, []string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	content := string(raw)

	fm, body, fmErr := frontmatter.Parse(content)
	if fmErr != nil {
		// Malformed frontmatter: index the whole file as body.
		fm, body = nil, content
	}
	// Line offset of the body within the file (frontmatter lines precede it).
	offset := strings.Count(content[:len(content)-len(body)], "\n")

	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	relDir := filepath.Dir(rel)
	if relDir == "." {
		relDir = ""
	}
	relDir = filepath.ToSlash(relDir)

	doc := &Doc{
		Path:      path,
		Workspace: workspace,
		NoteType:  firstSegment(relDir),
		Archived:  isArchivedPath(rel),
		ModTime:   info.ModTime(),
		bodyStart: offset,
	}
	if fm != nil {
		doc.ID = fm.ID
		doc.Title = fm.Title
		doc.Aliases = fm.Aliases
		doc.PlanRef = fm.PlanRef
		doc.PlanJob = fm.PlanJob
		// A canonical frontmatter name resolves links to notes with generic
		// filenames (skills/<name>/SKILL.md is wikilinked as [[<name>]]).
		if fm.Name != "" {
			doc.Aliases = append(append([]string{}, doc.Aliases...), fm.Name)
		}
	}

	inlineTags := parseBody(doc, body, offset)

	if doc.Title == "" {
		for _, h := range doc.Headings {
			if h.Level == 1 {
				doc.Title = h.Text
				break
			}
		}
	}
	if doc.Title == "" {
		doc.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	var fmTags []string
	if fm != nil {
		fmTags = fm.Tags
	}
	pathTags := frontmatter.ExtractPathTags(strings.TrimPrefix(nonHiddenDirPath(relDir), "/"))
	doc.Tags = frontmatter.MergeTags(fmTags, pathTags, inlineTags)

	return doc, strings.Split(content, "\n"), nil
}

// parseBody scans the body for headings, wikilinks, markdown links and inline
// tags, excluding fenced code blocks and inline code spans. It appends to
// doc.Headings/doc.Links and returns the inline tags found.
func parseBody(doc *Doc, body string, lineOffset int) []string {
	var inlineTags []string
	seenTags := map[string]bool{}
	inFence := false

	for i, line := range strings.Split(body, "\n") {
		lineNo := lineOffset + i + 1

		if fenceRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := headingRe.FindStringSubmatch(line); m != nil {
			doc.Headings = append(doc.Headings, Heading{Level: len(m[1]), Text: m[2], Line: lineNo})
		}

		scannable := inlineCodeRe.ReplaceAllString(line, "")

		for _, m := range wikilinkRe.FindAllStringSubmatch(scannable, -1) {
			if l, ok := parseWikilink(m[2], m[1] == "!"); ok {
				l.SourcePath = doc.Path
				l.Line = lineNo
				doc.Links = append(doc.Links, l)
			}
		}
		// Strip wikilinks before scanning markdown links and tags so
		// [[a|b]] and [[tag#x]] pieces are not re-matched.
		scannable = wikilinkRe.ReplaceAllString(scannable, "")

		for _, m := range mdLinkRe.FindAllStringSubmatch(scannable, -1) {
			if l, ok := parseMarkdownHref(m[1]); ok {
				l.SourcePath = doc.Path
				l.Line = lineNo
				doc.Links = append(doc.Links, l)
			}
		}

		for _, m := range inlineTagRe.FindAllStringSubmatch(scannable, -1) {
			tag := strings.Trim(m[1], "/-")
			// Obsidian rule: a tag needs at least one non-numeric character.
			if tag == "" || !nonDigitRe.MatchString(tag) || seenTags[tag] {
				continue
			}
			seenTags[tag] = true
			inlineTags = append(inlineTags, tag)
		}
	}
	return inlineTags
}

// parseWikilink splits the inner text of [[...]] into target, #heading and
// |display parts.
func parseWikilink(inner string, embed bool) (Link, bool) {
	l := Link{wiki: true, embed: embed}
	target := inner
	if idx := strings.Index(target, "|"); idx >= 0 {
		l.Display = strings.TrimSpace(target[idx+1:])
		target = target[:idx]
	}
	if idx := strings.Index(target, "#"); idx >= 0 {
		l.Heading = strings.TrimSpace(target[idx+1:])
		target = target[:idx]
	}
	l.RawTarget = strings.TrimSpace(target)
	if l.RawTarget == "" {
		// [[#heading]] self-references are out of scope for Phase 1.
		return Link{}, false
	}
	return l, true
}

// parseMarkdownHref keeps only hrefs that can point into the vault: relative
// (or absolute) file paths, not URLs or pure anchors. Resolution against the
// index happens later; unresolvable markdown links are dropped there.
func parseMarkdownHref(href string) (Link, bool) {
	if href == "" || strings.HasPrefix(href, "#") {
		return Link{}, false
	}
	if strings.Contains(href, "://") || strings.HasPrefix(href, "mailto:") {
		return Link{}, false
	}
	l := Link{}
	if idx := strings.Index(href, "#"); idx >= 0 {
		l.Heading = href[idx+1:]
		href = href[:idx]
	}
	if href == "" || !strings.EqualFold(filepath.Ext(href), ".md") {
		return Link{}, false
	}
	l.RawTarget = href
	return l, true
}

func firstSegment(relDir string) string {
	if relDir == "" {
		return ""
	}
	seg := relDir
	if idx := strings.Index(seg, "/"); idx >= 0 {
		seg = seg[:idx]
	}
	return seg
}

func isArchivedPath(rel string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if seg == ".archive" {
			return true
		}
	}
	return false
}

// nonHiddenDirPath drops dot-directories (.archive) from a relative dir path
// so they do not become path tags.
func nonHiddenDirPath(relDir string) string {
	if relDir == "" {
		return ""
	}
	var kept []string
	for _, seg := range strings.Split(relDir, "/") {
		if seg != "" && !strings.HasPrefix(seg, ".") {
			kept = append(kept, seg)
		}
	}
	return strings.Join(kept, "/")
}
