package models

import "time"

// NoteType represents the type of note
type NoteType string

// Note represents a note file
type Note struct {
	Path       string    `json:"path"`
	Title      string    `json:"title"`
	Type       NoteType  `json:"type"`          // Note type from frontmatter (chat, interactive_agent, etc.)
	Group      string    `json:"group"`         // Directory grouping (current, plans/name, etc.)
	Content    string    `json:"content,omitempty"`
	Workspace  string    `json:"workspace"`
	Branch     string    `json:"branch,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	ModifiedAt time.Time `json:"modified_at"`
	WordCount  int       `json:"word_count"`
	HasTodos   bool      `json:"has_todos"`
	IsArchived bool      `json:"is_archived"`
	IsArtifact bool      `json:"is_artifact,omitempty"`
	PlanRef    string    `json:"plan_ref,omitempty"`

	// Sync fields
	SyncProvider  string    `json:"sync_provider,omitempty"`
	SyncID        string    `json:"sync_id,omitempty"`
	SyncURL       string    `json:"sync_url,omitempty"`
	SyncState     string    `json:"sync_state,omitempty"`
	SyncUpdatedAt time.Time `json:"sync_updated_at,omitempty"`

	// Frontmatter fields
	ID         string   `json:"id"`
	Aliases    []string `json:"aliases"`
	Tags       []string `json:"tags"`
	Repository string   `json:"repository,omitempty"`
}
