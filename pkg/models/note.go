package models

import "time"

// NoteType represents the type of note
type NoteType string

const (
	NoteTypeCurrent      NoteType = "current"
	NoteTypeLLM          NoteType = "llm"
	NoteTypeLearn        NoteType = "learn"
	NoteTypeDaily        NoteType = "daily"
	NoteTypeIssues       NoteType = "issues"
	NoteTypeArchitecture NoteType = "architecture"
	NoteTypeTodos        NoteType = "todos"
	NoteTypeQuick        NoteType = "quick"
	NoteTypeBlog         NoteType = "blog"
	NoteTypePrompts      NoteType = "prompts"
)

// Note represents a note file
type Note struct {
	Path       string    `json:"path"`
	Title      string    `json:"title"`
	Type       NoteType  `json:"type"`
	Content    string    `json:"content,omitempty"`
	Workspace  string    `json:"workspace"`
	Branch     string    `json:"branch,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	ModifiedAt time.Time `json:"modified_at"`
	WordCount  int       `json:"word_count"`
	HasTodos   bool      `json:"has_todos"`
	IsArchived bool      `json:"is_archived"`

	// Frontmatter fields
	ID         string   `json:"id"`
	Aliases    []string `json:"aliases"`
	Tags       []string `json:"tags"`
	Repository string   `json:"repository,omitempty"`
}
