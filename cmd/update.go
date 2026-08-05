package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	grovelogging "github.com/grovetools/core/logging"
	"github.com/spf13/cobra"

	"github.com/grovetools/nb/pkg/frontmatter"
	"github.com/grovetools/nb/pkg/models"
	"github.com/grovetools/nb/pkg/service"
)

var updateUlog = grovelogging.NewUnifiedLogger("grove-notebook.cmd.update")

// noteReceipt is the machine receipt of the structured create/update
// contract: exactly {"id","path"}, nothing else. External producers (e.g. the
// pomodoro panel's note writer) json-parse stdout and require a non-empty
// absolute path, so the receipt is the ONLY thing --json writes to stdout.
type noteReceipt struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// emitNoteReceipt finishes a structured create/update: a bare JSON receipt on
// stdout for --json (human/audit output demoted to the structured log so
// stdout stays machine-parseable), or the given pretty line otherwise. verb
// names the outcome for the audit log ("created", "updated", "already
// exists"); prettyLine is what a human sees without --json.
func emitNoteReceipt(cmd *cobra.Command, ulog *grovelogging.UnifiedLogger, note *models.Note, jsonOut bool, verb, prettyLine string) error {
	// The contract promises an absolute path; the locator hands back absolute
	// paths already, so this is belt-and-braces for exotic cwd-relative ctxs.
	notePath := note.Path
	if abs, err := filepath.Abs(notePath); err == nil {
		notePath = abs
	}

	if jsonOut {
		data, err := json.Marshal(noteReceipt{ID: note.ID, Path: notePath})
		if err != nil {
			return fmt.Errorf("marshal receipt: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		ulog.Success("Note "+verb).
			Field("path", notePath).
			Field("id", note.ID).
			StructuredOnly().
			Emit()
		return nil
	}

	ulog.Success("Note "+verb).
		Field("path", notePath).
		Field("id", note.ID).
		Pretty(fmt.Sprintf("%s %s", prettyLine, notePath)).
		PrettyOnly().
		Emit()
	return nil
}

// NewUpdateCmd creates the 'update' command: the in-place half of the
// structured note contract (ticket
// 20260803-nb-structured-idempotent-note-create-update). It rewrites an
// existing note's frontmatter and body under the same merge policy as
// structured creation — nb-owned identity preserved, modified refreshed,
// validated producer fields merged — and refuses paths outside any notebook
// root nb knows about.
func NewUpdateCmd(svc **service.Service) *cobra.Command {
	var (
		notePath        string
		jsonOut         bool
		frontmatterFile string
		bodyFile        string
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update an existing note's frontmatter and body in place",
		Long: `Update an existing note in place.

Producer frontmatter fields are merged into the note's frontmatter: nb-owned
fields (id, created, modified, directory-derived type) are preserved by nb,
namespaced producer fields (e.g. pomodoro_*) replace their previous values,
and the modified timestamp is refreshed. When --body-file is given the note
body is replaced; otherwise it is left untouched.

Examples:
  nb update --path /abs/note.md --frontmatter-file fm.json
  nb update --json --path /abs/note.md --frontmatter-file fm.json --body-file body.md`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Receipt purity: same treatment as `nb new --json` — the
			// structured console echo defaults to stdout in non-TTY runs, so
			// park the logging global writer on stderr while the receipt
			// owns stdout.
			if jsonOut {
				prev := grovelogging.SwapGlobalOutput(os.Stderr)
				defer grovelogging.SwapGlobalOutput(prev)
			}

			s := *svc

			var producer frontmatter.ProducerFields
			if frontmatterFile != "" {
				fields, err := frontmatter.LoadProducerFields(frontmatterFile)
				if err != nil {
					return err
				}
				producer = fields
			}

			// nil means "keep the existing body"; only --body-file replaces it.
			var body *string
			if bodyFile != "" {
				data, err := os.ReadFile(bodyFile)
				if err != nil {
					return fmt.Errorf("read body file: %w", err)
				}
				content := string(data)
				body = &content
			}

			note, err := s.UpdateStructuredNote(notePath, producer, body)
			if err != nil {
				return err
			}

			return emitNoteReceipt(cmd, updateUlog, note, jsonOut, "updated", "Updated:")
		},
	}

	cmd.Flags().StringVar(&notePath, "path", "", "Absolute path of the note to update")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print a machine receipt {\"id\",\"path\"} on stdout")
	cmd.Flags().StringVar(&frontmatterFile, "frontmatter-file", "", "JSON or YAML file of producer frontmatter fields to merge (nb-owned fields win)")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "Replace the note body with this file's content")
	_ = cmd.MarkFlagRequired("path")

	return cmd
}
