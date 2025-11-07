package models

import (
	"testing"
	"time"
)

func TestNoteTypeValidation(t *testing.T) {
	tests := []struct {
		noteType NoteType
		isValid  bool
	}{
		{"current", true},
		{"quick", true},
		{"llm", true},
		{"learn", true},
		{"daily", true},
		{"issues", true},
		{"architecture", true},
		{"todos", true},
		{"blog", true},
		{"prompts", true},
		{NoteType("invalid"), false},
		{NoteType(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.noteType), func(t *testing.T) {
			valid := isValidNoteType(tt.noteType)
			if valid != tt.isValid {
				t.Errorf("Expected isValid %v for note type %s", tt.isValid, tt.noteType)
			}
		})
	}
}

func TestNoteFields(t *testing.T) {
	note := &Note{
		Path:       "/path/to/note.md",
		Title:      "Test Note",
		Type:       "current",
		Content:    "This is test content",
		CreatedAt:  time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		ModifiedAt: time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC),
		WordCount:  4,
		HasTodos:   true,
		IsArchived: false,
		Workspace:  "test-workspace",
		Branch:     "main",
	}

	// Test basic fields
	if note.Title != "Test Note" {
		t.Errorf("Expected title 'Test Note', got %s", note.Title)
	}

	if note.Type != "current" {
		t.Errorf("Expected type 'current', got %s", note.Type)
	}

	if note.Path != "/path/to/note.md" {
		t.Errorf("Expected path '/path/to/note.md', got %s", note.Path)
	}
}

func TestNoteWithMetadata(t *testing.T) {
	now := time.Now()
	note := &Note{
		Path:       "/test/note.md",
		Title:      "Metadata Test",
		Type:       "todos",
		CreatedAt:  now,
		ModifiedAt: now.Add(24 * time.Hour),
		WordCount:  100,
		HasTodos:   true,
		IsArchived: false,
		Workspace:  "project",
		Branch:     "feature/test",
	}

	// Test metadata fields
	if note.WordCount != 100 {
		t.Errorf("Expected word count 100, got %d", note.WordCount)
	}

	if !note.HasTodos {
		t.Error("Expected HasTodos to be true")
	}

	if note.IsArchived {
		t.Error("Expected IsArchived to be false")
	}

	if note.Workspace != "project" {
		t.Errorf("Expected workspace 'project', got %s", note.Workspace)
	}

	if note.Branch != "feature/test" {
		t.Errorf("Expected branch 'feature/test', got %s", note.Branch)
	}
}

func TestEmptyNote(t *testing.T) {
	note := &Note{}

	// Test defaults
	if note.Type != "" {
		t.Errorf("Expected empty note type, got %s", note.Type)
	}

	if note.WordCount != 0 {
		t.Errorf("Expected word count 0, got %d", note.WordCount)
	}

	if note.HasTodos {
		t.Error("Expected HasTodos to be false by default")
	}

	if note.IsArchived {
		t.Error("Expected IsArchived to be false by default")
	}
}

// Helper functions for testing
func isValidNoteType(nt NoteType) bool {
	validTypes := []NoteType{
		"current",
		"quick",
		"llm",
		"learn",
		"daily",
		"issues",
		"architecture",
		"todos",
		"blog",
		"prompts",
	}

	for _, valid := range validTypes {
		if nt == valid {
			return true
		}
	}
	return false
}
