package sync

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/grovetools/nb/pkg/frontmatter"
	"github.com/grovetools/nb/pkg/models"
)

// fakeProvider is a Provider that does nothing. It deliberately does NOT
// implement PRStateFetcher, so it stands in for a provider that cannot read PR
// state (the degradation path).
type fakeProvider struct{ name string }

func (p *fakeProvider) Name() string { return p.name }
func (p *fakeProvider) Sync(map[string]string, string) ([]*Item, error) {
	return nil, nil
}
func (p *fakeProvider) CreateItem(*Item, string) (*Item, error) { return nil, nil }
func (p *fakeProvider) UpdateItem(*Item, string) (*Item, error) { return nil, nil }
func (p *fakeProvider) AddComment(_, _, _, _ string) error      { return nil }
func (p *fakeProvider) GetItem(_, _, _ string) (*Item, error)   { return nil, nil }

// fetchingProvider adds the optional read-only PR-state capability.
type fetchingProvider struct {
	fakeProvider
	states map[string]*PRState
	errs   map[string]error
	calls  int
}

func (p *fetchingProvider) FetchPRState(url string) (*PRState, error) {
	p.calls++
	if err, ok := p.errs[url]; ok {
		return nil, err
	}
	if st, ok := p.states[url]; ok {
		return st, nil
	}
	return nil, fmt.Errorf("no such PR: %s", url)
}

// syncerWithFixtureWriter returns a Syncer that writes notes directly, so the
// refresh pass can run against fixture files without a service.
func syncerWithFixtureWriter() *Syncer {
	return &Syncer{
		logger: testLogger(),
		writeNote: func(path string, fm *frontmatter.Frontmatter, body string) error {
			return os.WriteFile(path, []byte(frontmatter.BuildContent(fm, body)), 0o644)
		},
	}
}

func ticketWithPRs(t *testing.T, dir, name, prsBlock string) *models.Note {
	t.Helper()
	return fixtureNote(t, dir, name, `id: `+strings.TrimSuffix(name, ".md")+`
title: Ticket
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
plan_ref: plans/hosted-git-and-prs
`+prsBlock)
}

func readPRs(t *testing.T, path string) []frontmatter.PRRef {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	fm, _, err := frontmatter.Parse(string(content))
	if err != nil {
		t.Fatalf("parse note: %v", err)
	}
	return fm.PRs.Entries
}

func TestRefreshPRStatesUpdatesStateAndTimestamp(t *testing.T) {
	dir := t.TempDir()
	note := ticketWithPRs(t, dir, "ticket.md", `prs:
  - repo: flow
    provider: github
    url: https://github.com/grovetools/flow/pull/12
    state: open
    updated_at: 2026-01-01T00:00:00Z`)

	observed := time.Date(2026, 8, 2, 10, 30, 0, 0, time.UTC)
	provider := &fetchingProvider{
		fakeProvider: fakeProvider{name: "github"},
		states: map[string]*PRState{
			"https://github.com/grovetools/flow/pull/12": {State: "merged", UpdatedAt: observed},
		},
	}

	s := syncerWithFixtureWriter()
	report := s.refreshPRStates([]*models.Note{note}, provider)

	if report.NotesScanned != 1 || report.NotesUpdated != 1 || report.EntriesFresh != 1 {
		t.Errorf("report = %+v, want 1 scanned / 1 updated / 1 fresh", report)
	}

	entries := readPRs(t, note.Path)
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (refresh must never add or drop entries)", len(entries))
	}
	if entries[0].State != "merged" {
		t.Errorf("state = %q, want merged", entries[0].State)
	}
	if entries[0].UpdatedAt != frontmatter.FormatTimestamp(observed) {
		t.Errorf("updated_at = %q, want %q", entries[0].UpdatedAt, frontmatter.FormatTimestamp(observed))
	}
	// Identity fields are not the refresh's business.
	if entries[0].Repo != "flow" || entries[0].URL != "https://github.com/grovetools/flow/pull/12" {
		t.Errorf("refresh altered identity fields: %+v", entries[0])
	}
}

// TestRefreshPRStatesLeavesEntriesUntouchedWhenProviderCannotRead is the
// degradation requirement: no fetcher capability ⇒ nothing changes, no error.
func TestRefreshPRStatesLeavesEntriesUntouchedWhenProviderCannotRead(t *testing.T) {
	dir := t.TempDir()
	note := ticketWithPRs(t, dir, "ticket.md", `prs:
  - repo: flow
    url: https://github.com/grovetools/flow/pull/12
    state: open`)

	before, err := os.ReadFile(note.Path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	s := syncerWithFixtureWriter()
	report := s.refreshPRStates([]*models.Note{note}, &fakeProvider{name: "github"})

	if report != (PRRefreshReport{}) {
		t.Errorf("report = %+v, want zero value", report)
	}
	after, err := os.ReadFile(note.Path)
	if err != nil {
		t.Fatalf("re-read fixture: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("note was rewritten despite the provider having no read capability:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestRefreshPRStatesLeavesUnresolvableEntriesAlone covers gh being present but
// unable to answer (offline, unauthenticated, PR in another forge). An unknown
// state must stay unknown — never coerced to something healthier-looking (D4).
func TestRefreshPRStatesLeavesUnresolvableEntriesAlone(t *testing.T) {
	dir := t.TempDir()
	note := ticketWithPRs(t, dir, "ticket.md", `prs:
  - repo: flow
    provider: github
    url: https://github.com/grovetools/flow/pull/12
    state: open
  - repo: core
    provider: forgejo
    url: https://forge.example.com/grove/core/pulls/3
    state: ""`)

	provider := &fetchingProvider{
		fakeProvider: fakeProvider{name: "github"},
		errs: map[string]error{
			"https://github.com/grovetools/flow/pull/12": fmt.Errorf("offline"),
		},
	}

	s := syncerWithFixtureWriter()
	report := s.refreshPRStates([]*models.Note{note}, provider)

	if report.NotesUpdated != 0 || report.EntriesFresh != 0 {
		t.Errorf("report = %+v, want nothing updated", report)
	}
	if report.EntriesUnknown != 1 {
		t.Errorf("EntriesUnknown = %d, want 1 (only the github entry is attempted)", report.EntriesUnknown)
	}

	entries := readPRs(t, note.Path)
	if entries[0].State != "open" {
		t.Errorf("github entry state = %q, want the original %q", entries[0].State, "open")
	}
	if entries[1].State != "" {
		t.Errorf("forgejo entry state = %q, want to stay unknown", entries[1].State)
	}
	// The forgejo entry belongs to another forge and must not be fetched.
	if provider.calls != 1 {
		t.Errorf("FetchPRState called %d times, want 1 (entries of other providers are skipped)", provider.calls)
	}
}

// TestRefreshPRStatesSkipsNotesWithoutPRs pins that the pass is inert for the
// overwhelming majority of notes, which carry no `prs:` at all.
func TestRefreshPRStatesSkipsNotesWithoutPRs(t *testing.T) {
	dir := t.TempDir()
	plain := fixtureNote(t, dir, "plain.md", `id: plain
title: Plain
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00`)

	provider := &fetchingProvider{fakeProvider: fakeProvider{name: "github"}}
	s := syncerWithFixtureWriter()
	report := s.refreshPRStates([]*models.Note{plain}, provider)

	if report.NotesScanned != 0 {
		t.Errorf("NotesScanned = %d, want 0", report.NotesScanned)
	}
	if provider.calls != 0 {
		t.Errorf("FetchPRState called %d times, want 0", provider.calls)
	}
}

// TestRefreshPRStatesIsIdempotent: a second pass over already-fresh entries
// must not rewrite the file, so sync does not churn note mtimes.
func TestRefreshPRStatesIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	observed := time.Date(2026, 8, 2, 10, 30, 0, 0, time.UTC)
	note := ticketWithPRs(t, dir, "ticket.md", `prs:
  - repo: flow
    provider: github
    url: https://github.com/grovetools/flow/pull/12
    state: merged
    updated_at: `+frontmatter.FormatTimestamp(observed))

	provider := &fetchingProvider{
		fakeProvider: fakeProvider{name: "github"},
		states: map[string]*PRState{
			"https://github.com/grovetools/flow/pull/12": {State: "merged", UpdatedAt: observed},
		},
	}

	s := syncerWithFixtureWriter()
	report := s.refreshPRStates([]*models.Note{note}, provider)

	if report.NotesUpdated != 0 || report.EntriesFresh != 0 {
		t.Errorf("report = %+v, want no writes when nothing changed", report)
	}
}

// TestRefreshPRStatesDeduplicatesByURL: two notes referencing one PR cost one
// read, not two.
func TestRefreshPRStatesDeduplicatesByURL(t *testing.T) {
	dir := t.TempDir()
	url := "https://github.com/grovetools/flow/pull/12"
	block := `prs:
  - repo: flow
    provider: github
    url: ` + url + `
    state: open`

	notes := []*models.Note{
		ticketWithPRs(t, dir, "one.md", block),
		ticketWithPRs(t, dir, "two.md", block),
	}

	provider := &fetchingProvider{
		fakeProvider: fakeProvider{name: "github"},
		states:       map[string]*PRState{url: {State: "merged", UpdatedAt: time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)}},
	}

	s := syncerWithFixtureWriter()
	report := s.refreshPRStates(notes, provider)

	if report.NotesUpdated != 2 {
		t.Errorf("NotesUpdated = %d, want 2", report.NotesUpdated)
	}
	if provider.calls != 1 {
		t.Errorf("FetchPRState called %d times, want 1 (one read per distinct URL)", provider.calls)
	}
}

// TestRefreshPRStatesPreservesUnknownKeys: the refresh rewrites the note, so it
// must not become a vector for destroying frontmatter nb does not model.
func TestRefreshPRStatesPreservesUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	note := fixtureNote(t, dir, "ticket.md", `id: t
title: Ticket
aliases: []
tags: []
created: 2023-01-01 10:00:00
modified: 2023-01-01 10:00:00
custom_field: keep-me
prs:
  - repo: flow
    provider: github
    url: https://github.com/grovetools/flow/pull/12
    state: open
    reviewer_note: also-keep-me`)

	provider := &fetchingProvider{
		fakeProvider: fakeProvider{name: "github"},
		states: map[string]*PRState{
			"https://github.com/grovetools/flow/pull/12": {State: "merged", UpdatedAt: time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)},
		},
	}

	s := syncerWithFixtureWriter()
	if report := s.refreshPRStates([]*models.Note{note}, provider); report.NotesUpdated != 1 {
		t.Fatalf("report = %+v, want the note rewritten", report)
	}

	content, err := os.ReadFile(note.Path)
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	for _, want := range []string{"custom_field: keep-me", "reviewer_note: also-keep-me", "state: merged"} {
		if !strings.Contains(string(content), want) {
			t.Errorf("refreshed note lost %q:\n%s", want, content)
		}
	}
}
