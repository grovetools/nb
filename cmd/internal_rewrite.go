package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/grovetools/core/pkg/models"
	"github.com/spf13/cobra"

	"github.com/grovetools/nb/pkg/notedoc"
	"github.com/grovetools/nb/pkg/service"
)

// `internal rewrite-note` is the in-place refresh seam for GENERATED notes,
// the counterpart to `internal update-note` (append-only) and `internal
// update-frontmatter` (one modelled string field).
//
// A generator that owns part of a note — the review packet is the first —
// needs to rewrite that part repeatedly without ever disturbing what a human
// wrote underneath, and to carry a structured payload that nobody should have
// to parse out of prose. Both halves are nb's existing conventions: the sync
// marker's ownership split (nb/pkg/sync has rebuilt bodies around it since
// remote sync existed) and the frontmatter Extra map's verbatim passthrough of
// unmodelled keys. notedoc holds the rules; this command is their CLI seam, so
// generators outside nb keep going through nb instead of writing notebook
// files behind its back.
//
// The write is skipped entirely when the rebuilt content is byte-identical to
// what is already on disk. That makes a refresh genuinely idempotent — no
// rewritten mtime, no note event, and nothing for the notebook syncer to
// replicate — which is what lets a caller refresh on every review without
// generating churn.

// rewriteNotePayload is the stdin document. body_above_marker replaces the
// machine-owned region of the body; frontmatter sets (or, for a null value,
// deletes) unmodelled frontmatter keys.
type rewriteNotePayload struct {
	BodyAboveMarker string         `json:"body_above_marker"`
	Frontmatter     map[string]any `json:"frontmatter,omitempty"`
}

// newRewriteNoteCmd creates the 'internal rewrite-note' command.
func newRewriteNoteCmd(_ **service.Service) *cobra.Command {
	var notePath string

	cmd := &cobra.Command{
		Use:   "rewrite-note",
		Short: "Rewrites a generated note's machine-owned region from a JSON payload on stdin",
		Long: `Replaces the part of a note ABOVE nb's sync marker, and sets unmodelled
frontmatter keys, from a JSON payload read on stdin:

  {"body_above_marker": "...", "frontmatter": {"some_key": {...}}}

Everything from the marker down is preserved byte for byte. A note with no
marker keeps its whole existing body BELOW a marker inserted at rewrite time.
A null frontmatter value deletes that key. The file is left untouched when the
result is byte-identical to what is already there.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if notePath == "" {
				return fmt.Errorf("--path is required")
			}

			raw, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("read payload from stdin: %w", err)
			}
			payload, err := decodeRewritePayload(raw)
			if err != nil {
				return err
			}

			content, err := os.ReadFile(notePath)
			if err != nil {
				return fmt.Errorf("failed to read note file: %w", err)
			}

			updated, err := notedoc.Rewrite(string(content), payload.BodyAboveMarker, payload.Frontmatter)
			if err != nil {
				return err
			}

			if updated == string(content) {
				internalUlog.Info("Note unchanged").
					Field("path", notePath).
					Pretty(fmt.Sprintf("* Unchanged: %s", notePath)).
					PrettyOnly().
					Emit()
				return nil
			}

			if err := os.WriteFile(notePath, []byte(updated), 0o644); err != nil {
				return fmt.Errorf("failed to write note file: %w", err)
			}

			ws, _, noteType := service.GetNoteMetadata(notePath)
			service.EmitNoteEvent(models.NoteEvent{
				Event:     models.NoteEventUpdated,
				Workspace: ws,
				NoteType:  noteType,
				Path:      notePath,
			})

			internalUlog.Success("Note rewritten").
				Field("path", notePath).
				Pretty(fmt.Sprintf("* Rewrote: %s", notePath)).
				PrettyOnly().
				Emit()
			return nil
		},
	}

	cmd.Flags().StringVar(&notePath, "path", "", "The absolute path to the note file to rewrite")
	_ = cmd.MarkFlagRequired("path")

	return cmd
}

// decodeRewritePayload parses the stdin document, normalizing JSON numbers so
// an integer survives the trip into YAML as an integer. encoding/json decodes
// every number into float64 by default, which would turn a schema version of 1
// into `1` or `1.0` depending on the YAML encoder's mood — a difference that
// would make an otherwise unchanged refresh look changed forever.
func decodeRewritePayload(raw []byte) (rewriteNotePayload, error) {
	var payload rewriteNotePayload
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		return rewriteNotePayload{}, fmt.Errorf("parse rewrite payload: %w", err)
	}
	for key, value := range payload.Frontmatter {
		payload.Frontmatter[key] = normalizeJSONNumbers(value)
	}
	return payload, nil
}

// normalizeJSONNumbers walks a decoded JSON value converting json.Number into
// int64 where it is integral and float64 otherwise.
func normalizeJSONNumbers(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	case map[string]any:
		for k, sub := range t {
			t[k] = normalizeJSONNumbers(sub)
		}
		return t
	case []any:
		for i, sub := range t {
			t[i] = normalizeJSONNumbers(sub)
		}
		return t
	default:
		return v
	}
}
