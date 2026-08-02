package sync

import (
	"os"

	"github.com/sirupsen/logrus"

	"github.com/grovetools/nb/pkg/frontmatter"
	"github.com/grovetools/nb/pkg/models"
)

// PRRefreshReport summarizes one read-only `prs:` freshness pass.
type PRRefreshReport struct {
	NotesScanned   int // notes carrying at least one `prs:` entry
	NotesUpdated   int // notes whose file was rewritten
	EntriesFresh   int // entries whose state/updated_at changed
	EntriesUnknown int // entries the provider could not resolve — left untouched
}

// refreshPRStates refreshes the `state`/`updated_at` of `prs:` entries already
// present on local notes. It is freshness ONLY:
//
//   - it never adds, removes or reorders entries — publish is deferred, so
//     nothing in this wave decides which PRs a ticket has;
//   - it never moves a note between directories and touches no transition
//     machinery (STATE.md D5) — directory-as-state is untouched;
//   - it never writes to the forge; the provider capability it uses is
//     read-only by construction;
//   - an unresolvable entry is left exactly as it was. An unknown state is
//     reported as unknown, never rewritten to a healthier-looking one (D4).
//
// When the provider does not implement PRStateFetcher — or `gh` is missing or
// offline — every entry is left untouched and the pass logs once at debug
// level rather than once per entry. Silence is the required behavior.
func (s *Syncer) refreshPRStates(notes []*models.Note, provider Provider) PRRefreshReport {
	var report PRRefreshReport

	fetcher, ok := provider.(PRStateFetcher)
	if !ok {
		s.logger.WithField("provider", provider.Name()).
			Debug("Provider cannot read PR state; leaving prs: entries untouched")
		return report
	}

	// One observation per URL per pass: a plan's tickets and its mirrored PR
	// notes routinely reference the same PR.
	observed := make(map[string]*PRState)
	unresolved := make(map[string]bool)

	for _, note := range notes {
		if note == nil {
			continue
		}
		content, err := os.ReadFile(note.Path)
		if err != nil {
			continue
		}
		fm, body, err := frontmatter.Parse(string(content))
		if err != nil || fm == nil || len(fm.PRs.Entries) == 0 {
			continue
		}
		report.NotesScanned++

		changed := false
		for i := range fm.PRs.Entries {
			entry := &fm.PRs.Entries[i]
			if entry.URL == "" || !s.providerOwns(provider, entry) {
				continue
			}

			state, seen := observed[entry.URL]
			if !seen && !unresolved[entry.URL] {
				fetched, err := fetcher.FetchPRState(entry.URL)
				if err != nil {
					unresolved[entry.URL] = true
					s.logger.WithFields(logrus.Fields{
						"url":   entry.URL,
						"error": err.Error(),
					}).Debug("PR state unavailable; leaving entry untouched")
				} else {
					observed[entry.URL] = fetched
					state = fetched
				}
			}
			if state == nil {
				report.EntriesUnknown++
				continue
			}

			updatedAt := frontmatter.FormatTimestamp(state.UpdatedAt)
			if entry.State == state.State && entry.UpdatedAt == updatedAt {
				continue
			}
			entry.State = state.State
			entry.UpdatedAt = updatedAt
			changed = true
			report.EntriesFresh++
		}

		if !changed {
			continue
		}
		// Write through the frontmatter serializer so unknown keys and the
		// body survive; the note stays exactly where it is on disk.
		if err := s.persistNote(note.Path, fm, body); err != nil {
			s.logger.WithFields(logrus.Fields{
				"note_path": note.Path,
				"error":     err.Error(),
			}).Warn("Failed to write refreshed prs: states")
			continue
		}
		report.NotesUpdated++
	}

	if report.NotesScanned > 0 {
		s.logger.WithFields(logrus.Fields{
			"provider":        provider.Name(),
			"notes_scanned":   report.NotesScanned,
			"notes_updated":   report.NotesUpdated,
			"entries_fresh":   report.EntriesFresh,
			"entries_unknown": report.EntriesUnknown,
		}).Debug("prs: freshness pass complete")
	}

	return report
}

// providerOwns reports whether this provider should attempt to resolve an
// entry. An entry naming a different forge is not this provider's business; an
// entry with no provider is attempted, since a hand-written entry commonly
// omits it and a failed read is harmless.
func (s *Syncer) providerOwns(provider Provider, entry *frontmatter.PRRef) bool {
	return entry.Provider == "" || entry.Provider == provider.Name()
}
