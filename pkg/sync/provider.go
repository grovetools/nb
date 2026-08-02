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

// PRState is a read-only observation of a pull request's current state.
type PRState struct {
	State     string // "open", "closed", "merged", "draft"
	UpdatedAt time.Time
}

// PRStateFetcher is an OPTIONAL provider capability: read-only lookup of a
// pull request by its URL. It is deliberately not part of Provider — a
// provider that cannot do it simply does not implement it, and the `prs:`
// refresh then leaves every entry untouched rather than erroring.
//
// Read-only by construction: there is no counterpart that writes to the forge.
type PRStateFetcher interface {
	// FetchPRState returns the current state of the PR at url. A provider
	// that is installed but unreachable (offline, unauthenticated) returns an
	// error; callers treat that as "unknown" and leave the entry alone.
	FetchPRState(url string) (*PRState, error)
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
	ID        string // The unique ID on the remote platform (e.g., issue number "123").
	Type      string // "issue" or "pull_request".
	Title     string
	Body      string
	State     string // "open", "closed", "merged", etc.
	URL       string
	Labels    []string
	Assignees []string
	Milestone string
	UpdatedAt time.Time
	Comments  []*Comment

	// HeadBranch is the PR's source branch, empty for issues and for
	// providers that do not report it. It is the only signal the mirrored-PR
	// note uses to find the ticket it belongs to (see planlink.go).
	HeadBranch string
}

// Report summarizes the results of a sync operation.
type Report struct {
	Provider  string
	Created   int
	Updated   int
	Unchanged int
	Failed    int
	Errors    []string // Detailed error messages

	// PRs summarizes the read-only `prs:` freshness pass. Zero-valued when the
	// provider cannot read PR state.
	PRs PRRefreshReport
}
