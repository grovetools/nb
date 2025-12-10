package sync

import "time"

// Provider defines the interface for a source of syncable items (e.g., GitHub).
type Provider interface {
	Name() string
	Sync(config map[string]string, repoPath string) ([]*Item, error)
}

// Item represents a generic syncable entity.
type Item struct {
	ID        string    // The unique ID on the remote platform (e.g., issue number "123").
	Type      string    // "issue" or "pull_request".
	Title     string
	Body      string
	State     string // "open", "closed", "merged", etc.
	URL       string
	Labels    []string
	UpdatedAt time.Time
}

// Report summarizes the results of a sync operation.
type Report struct {
	Provider  string
	Created   int
	Updated   int
	Unchanged int
	Failed    int
}
