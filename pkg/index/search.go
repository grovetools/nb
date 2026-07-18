package index

import (
	"sort"
	"strings"
	"unicode"
)

// SearchOptions tunes Search.
type SearchOptions struct {
	Limit     int    // max hits; <= 0 means no limit
	Workspace string // restrict hits to one workspace; "" means all
}

// Snippet is a matching line from a doc body.
type Snippet struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// SearchHit is one scored search result.
type SearchHit struct {
	Doc      *Doc      `json:"doc"`
	Score    float64   `json:"score"`
	Snippets []Snippet `json:"snippets,omitempty"`
}

const maxSnippetsPerHit = 3

type postingCount struct {
	title   int
	heading int
	body    int
}

// Search runs tokenized OR matching over title/headings/body. Hits are scored
// title 3x, heading 2x, body 1x, multiplied by the fraction of query tokens
// matched, and returned best-first with line-level snippets.
func (ix *Index) Search(query string, opt SearchOptions) []SearchHit {
	tokens := tokenize(query)
	if len(tokens) == 0 {
		return nil
	}
	// Dedupe query tokens so repeated words don't inflate the match fraction.
	uniq := make([]string, 0, len(tokens))
	seen := map[string]bool{}
	for _, t := range tokens {
		if !seen[t] {
			seen[t] = true
			uniq = append(uniq, t)
		}
	}

	type acc struct {
		score   float64
		matched int
	}
	perDoc := map[string]*acc{}
	for _, tok := range uniq {
		for path, pc := range ix.postings[tok] {
			a := perDoc[path]
			if a == nil {
				a = &acc{}
				perDoc[path] = a
			}
			a.score += float64(3*pc.title + 2*pc.heading + pc.body)
			a.matched++
		}
	}

	hits := make([]SearchHit, 0, len(perDoc))
	for path, a := range perDoc {
		doc := ix.docs[path]
		if doc == nil {
			continue
		}
		if opt.Workspace != "" && doc.Workspace != opt.Workspace {
			continue
		}
		hits = append(hits, SearchHit{
			Doc:      doc,
			Score:    a.score * float64(a.matched) / float64(len(uniq)),
			Snippets: ix.snippets(path, uniq),
		})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Doc.Path < hits[j].Doc.Path
	})
	if opt.Limit > 0 && len(hits) > opt.Limit {
		hits = hits[:opt.Limit]
	}
	return hits
}

// snippets returns up to maxSnippetsPerHit body lines of the doc containing
// any query token. Line numbers are file-relative (frontmatter included).
func (ix *Index) snippets(path string, tokens []string) []Snippet {
	var out []Snippet
	start := 0
	if d := ix.docs[path]; d != nil {
		start = d.bodyStart
	}
	for i, line := range ix.bodyLines[path][start:] {
		lineTokens := tokenize(line)
		match := false
		for _, lt := range lineTokens {
			for _, qt := range tokens {
				if lt == qt {
					match = true
					break
				}
			}
			if match {
				break
			}
		}
		if match {
			out = append(out, Snippet{Line: start + i + 1, Text: strings.TrimSpace(line)})
			if len(out) == maxSnippetsPerHit {
				break
			}
		}
	}
	return out
}

// buildPostings indexes lowercased word tokens per doc, weighted by where
// they appear (title vs heading vs body).
func (ix *Index) buildPostings() {
	for path, d := range ix.docs {
		add := func(text string, bump func(*postingCount)) {
			for _, tok := range tokenize(text) {
				m := ix.postings[tok]
				if m == nil {
					m = make(map[string]*postingCount)
					ix.postings[tok] = m
				}
				pc := m[path]
				if pc == nil {
					pc = &postingCount{}
					m[path] = pc
				}
				bump(pc)
			}
		}
		add(d.Title, func(pc *postingCount) { pc.title++ })
		for _, h := range d.Headings {
			add(h.Text, func(pc *postingCount) { pc.heading++ })
		}
		for _, line := range ix.bodyLines[path][d.bodyStart:] {
			// Heading lines are indexed via d.Headings at heading weight;
			// counting them as body too would double-score them.
			if headingRe.MatchString(line) {
				continue
			}
			add(line, func(pc *postingCount) { pc.body++ })
		}
	}
}

// tokenize lowercases and splits on any rune that is not a unicode letter or
// digit.
func tokenize(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}
