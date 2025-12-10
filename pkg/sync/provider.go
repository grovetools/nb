package sync

import "time"

// Provider defines the interface for a source of syncable items (e.g., GitHub).
type Provider interface {
	// Name returns the provider's name (e.g., "github").
	Name() string
	// Sync fetches all relevant items from the remote.
	Sync(config map[string]string, repoPath string) ([]*Item, error)
	// UpdateItem pushes changes for a single item to the remote and returns the updated item.
	UpdateItem(item *Item, repoPath string) (*Item, error)
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
	Assignees []string
	Milestone string
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
