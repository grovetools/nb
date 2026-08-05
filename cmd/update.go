package cmd

import (
	"encoding/json"
	"errors"
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
// contract: exactly {"id","path"} plus an optional "disposition", nothing
// else. External producers json-parse stdout and require a non-empty absolute
// path, so the receipt is the ONLY thing --json writes to stdout.
type noteReceipt struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Disposition string `json:"disposition,omitempty"`
}

// emitNoteReceipt finishes a structured create/update: a bare JSON receipt on
// stdout for --json (human/audit output demoted to the structured log so
// stdout stays machine-parseable), or the given pretty line otherwise. verb
// names the outcome for the audit log ("created", "updated", "already
// exists"); prettyLine is what a human sees without --json. disposition is
// included in the receipt when non-empty.
func emitNoteReceipt(cmd *cobra.Command, ulog *grovelogging.UnifiedLogger, note *models.Note, jsonOut bool, verb, prettyLine, disposition string) error {
	notePath := note.Path
	if abs, err := filepath.Abs(notePath); err == nil {
		notePath = abs
	}

	if jsonOut {
		data, err := json.Marshal(noteReceipt{ID: note.ID, Path: notePath, Disposition: disposition})
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

// errorEnvelope is the machine error envelope written to stdout when a
// structured update fails with a typed service error.
type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewUpdateCmd creates the 'update' command: the in-place half of the
// structured note contract. It rewrites an existing note's frontmatter and
// body under the same merge policy as structured creation — nb-owned identity
// preserved, modified refreshed, validated producer fields merged — and
// refuses paths outside the selected route.
func NewUpdateCmd(svc **service.Service, workspaceOverride *string) *cobra.Command {
	var (
		notePath          string
		jsonOut           bool
		frontmatterFile   string
		bodyFile          string
		globalNote        bool
		expectType        string
		expectIdempotency string
		expectFilename    string
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
  nb update --json --path /abs/note.md --frontmatter-file fm.json --body-file body.md
  nb update --json --path /abs/note.md -g --expect-type hn/clippings --expect-idempotency-key hn:12345 --expect-filename comments.md`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOut {
				prev := grovelogging.SwapGlobalOutput(os.Stderr)
				defer grovelogging.SwapGlobalOutput(prev)
			}

			if globalNote && *workspaceOverride != "" {
				return fmt.Errorf("-g/--global and -W/--workspace conflict")
			}

			s := *svc

			hasExpectFlags := expectType != "" || expectIdempotency != "" || expectFilename != ""

			if hasExpectFlags && (expectType == "" || expectIdempotency == "" || expectFilename == "") {
				return fmt.Errorf("--expect-type, --expect-idempotency-key, and --expect-filename must be provided together")
			}

			if hasExpectFlags {
				// Route- and identity-validated structured update.
				var wsCtx *service.WorkspaceContext
				var err error
				if globalNote {
					wsCtx, err = s.GetWorkspaceContext("global")
				} else if *workspaceOverride != "" {
					wsCtx, err = s.GetExplicitWorkspaceContext(*workspaceOverride)
				} else {
					return fmt.Errorf("structured update requires -g or -W to select a route")
				}
				if err != nil {
					return handleStructuredUpdateError(cmd, jsonOut, err)
				}

				var producer frontmatter.ProducerFields
				if frontmatterFile != "" {
					fields, loadErr := frontmatter.LoadProducerFields(frontmatterFile)
					if loadErr != nil {
						return loadErr
					}
					producer = fields
				}

				var body *string
				if bodyFile != "" {
					data, readErr := os.ReadFile(bodyFile)
					if readErr != nil {
						return fmt.Errorf("read body file: %w", readErr)
					}
					content := string(data)
					body = &content
				}

				expected := service.ExpectedNoteIdentity{
					TypePath:       models.NoteType(expectType),
					IdempotencyKey: expectIdempotency,
					Filename:       expectFilename,
				}

				note, err := s.UpdateStructuredNoteForContext(wsCtx, notePath, producer, body, expected)
				if err != nil {
					return handleStructuredUpdateError(cmd, jsonOut, err)
				}
				return emitNoteReceipt(cmd, updateUlog, note, jsonOut, "updated", "Updated:", "")
			}

			// Legacy unscoped update (backward compat).
			var producer frontmatter.ProducerFields
			if frontmatterFile != "" {
				fields, err := frontmatter.LoadProducerFields(frontmatterFile)
				if err != nil {
					return err
				}
				producer = fields
			}

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

			return emitNoteReceipt(cmd, updateUlog, note, jsonOut, "updated", "Updated:", "")
		},
	}

	cmd.Flags().StringVar(&notePath, "path", "", "Absolute path of the note to update")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print a machine receipt {\"id\",\"path\"} on stdout")
	cmd.Flags().StringVar(&frontmatterFile, "frontmatter-file", "", "JSON or YAML file of producer frontmatter fields to merge (nb-owned fields win)")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "Replace the note body with this file's content")
	cmd.Flags().BoolVarP(&globalNote, "global", "g", false, "Select the global route")
	cmd.Flags().StringVar(&expectType, "expect-type", "", "Expected note type path (required for structured update)")
	cmd.Flags().StringVar(&expectIdempotency, "expect-idempotency-key", "", "Expected idempotency key")
	cmd.Flags().StringVar(&expectFilename, "expect-filename", "", "Expected canonical filename")
	_ = cmd.MarkFlagRequired("path")

	return cmd
}

// handleStructuredUpdateError catches StructuredUpdateError, writes the JSON
// error envelope to stdout when --json is active, and returns the error for
// nonzero exit.
func handleStructuredUpdateError(cmd *cobra.Command, jsonOut bool, err error) error {
	var sue *service.StructuredUpdateError
	if jsonOut && errors.As(err, &sue) {
		data, marshalErr := json.Marshal(errorEnvelope{Error: errorDetail{Code: sue.Code, Message: sue.Message}})
		if marshalErr == nil {
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
		}
	}
	return err
}
