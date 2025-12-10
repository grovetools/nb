package models

import "time"

// NoteType represents the type of note
type NoteType string

// RemoteMetadata represents sync metadata from remote sources
type RemoteMetadata struct {
	Provider  string    `json:"provider,omitempty"`
	ID        string    `json:"id,omitempty"`
	URL       string    `json:"url,omitempty"`
	State     string    `json:"state,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	Labels    []string  `json:"labels,omitempty"`
	Assignees []string  `json:"assignees,omitempty"`
	Milestone string    `json:"milestone,omitempty"`
}

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
	IsArtifact bool   `json:"is_artifact,omitempty"`
	PlanRef    string `json:"plan_ref,omitempty"`

	// Remote sync metadata
	Remote *RemoteMetadata `json:"remote,omitempty"`

	// Frontmatter fields
	ID         string   `json:"id"`
	Aliases    []string `json:"aliases"`
	Tags       []string `json:"tags"`
	Repository string   `json:"repository,omitempty"`
}
