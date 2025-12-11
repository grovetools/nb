package sync

import "time"

// Provider defines the interface for a source of syncable items (e.g., GitHub).
type Provider interface {
	// Name returns the provider's name (e.g., "github").
	Name() string
	// Sync fetches all relevant items from the remote.
	Sync(config map[string]string, repoPath string) ([]*Item, error)
	// CreateItem creates a new item on the remote and returns the created item.
	CreateItem(item *Item, repoPath string) (*Item, error)
	// UpdateItem pushes changes for a single item to the remote and returns the updated item.
	UpdateItem(item *Item, repoPath string) (*Item, error)
	// AddComment posts a new comment to an item.
	AddComment(itemType, itemID, body, repoPath string) error
	// GetItem fetches a single item from the remote.
	GetItem(itemType, itemID, repoPath string) (*Item, error)
}

// Comment represents a single comment on a syncable item.
type Comment struct {
	ID        string
	Body      string
	Author    string
	CreatedAt time.Time
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
	Comments  []*Comment
}

// Report summarizes the results of a sync operation.
type Report struct {
	Provider  string
	Created   int
	Updated   int
	Unchanged int
	Failed    int
	Errors    []string // Detailed error messages
}
