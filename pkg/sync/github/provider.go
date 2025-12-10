package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/mattsolo1/grove-notebook/pkg/sync"
)

// GitHubProvider implements the sync.Provider interface for GitHub.
type GitHubProvider struct{}

// NewProvider creates a new GitHubProvider.
func NewProvider() *GitHubProvider {
	return &GitHubProvider{}
}

// Name returns the name of the provider.
func (p *GitHubProvider) Name() string {
	return "github"
}

// Sync fetches issues and pull requests from a GitHub repository.
func (p *GitHubProvider) Sync(config map[string]string, repoPath string) ([]*sync.Item, error) {
	var allItems []*sync.Item

	// Check if gh cli is installed
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("gh command not found in PATH, please install the GitHub CLI")
	}

	// Sync issues if configured
	if _, ok := config["issues_type"]; ok {
		issueItems, err := p.fetchItems("issue", repoPath)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch issues: %w", err)
		}
		allItems = append(allItems, issueItems...)
	}

	// Sync pull requests if configured
	if _, ok := config["prs_type"]; ok {
		prItems, err := p.fetchItems("pr", repoPath)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch pull requests: %w", err)
		}
		allItems = append(allItems, prItems...)
	}

	return allItems, nil
}

// UpdateItem pushes changes for a single item to GitHub.
func (p *GitHubProvider) UpdateItem(item *sync.Item, repoPath string) (*sync.Item, error) {
	itemType := item.Type
	if itemType == "pull_request" {
		itemType = "pr" // gh cli uses 'pr'
	}

	// 1. Update title and body
	editArgs := []string{itemType, "edit", item.ID, "--title", item.Title, "--body", item.Body}
	cmd := exec.Command("gh", editArgs...)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("gh %s edit failed: %w\n%s", itemType, err, string(output))
	}

	// 2. Update state if necessary
	// We need to fetch the current remote state to see if a state change is needed.
	remoteItem, err := p.fetchSingleItem(itemType, item.ID, repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote item after edit: %w", err)
	}

	if remoteItem.State != item.State {
		var stateCmd *exec.Cmd
		switch strings.ToLower(item.State) {
		case "open":
			stateArgs := []string{itemType, "reopen", item.ID}
			stateCmd = exec.Command("gh", stateArgs...)
		case "closed", "merged":
			stateArgs := []string{itemType, "close", item.ID}
			stateCmd = exec.Command("gh", stateArgs...)
		}

		if stateCmd != nil {
			stateCmd.Dir = repoPath
			if output, err := stateCmd.CombinedOutput(); err != nil {
				return nil, fmt.Errorf("gh %s state change failed: %w\n%s", itemType, err, string(output))
			}
		}
	}

	// 3. Re-fetch the item to get the final state and new UpdatedAt timestamp
	return p.fetchSingleItem(itemType, item.ID, repoPath)
}

// fetchSingleItem fetches a single issue or PR from GitHub.
func (p *GitHubProvider) fetchSingleItem(itemType, itemID, repoPath string) (*sync.Item, error) {
	cmdArgs := []string{itemType, "view", itemID, "--json", "id,number,title,body,state,url,updatedAt,labels,assignees,milestone"}
	cmd := exec.Command("gh", cmdArgs...)
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh %s view command failed: %w", itemType, err)
	}

	var ghItem ghItem
	if err := json.Unmarshal(output, &ghItem); err != nil {
		return nil, fmt.Errorf("failed to parse gh JSON output for single item: %w", err)
	}

	return p.ghItemToSyncItem(&ghItem, itemType), nil
}

// ghItemToSyncItem converts a ghItem to a sync.Item.
func (p *GitHubProvider) ghItemToSyncItem(item *ghItem, itemType string) *sync.Item {
	var labels []string
	for _, label := range item.Labels {
		labels = append(labels, label.Name)
	}

	var assignees []string
	for _, assignee := range item.Assignees {
		assignees = append(assignees, assignee.Login)
	}

	var milestone string
	if item.Milestone != nil {
		milestone = item.Milestone.Title
	}

	return &sync.Item{
		ID:        fmt.Sprintf("%d", item.Number),
		Type:      itemType,
		Title:     item.Title,
		Body:      item.Body,
		State:     strings.ToLower(item.State),
		URL:       item.URL,
		Labels:    labels,
		Assignees: assignees,
		Milestone: milestone,
		UpdatedAt: item.UpdatedAt,
	}
}

// ghItem represents the JSON structure returned by 'gh ... list --json'.
type ghItem struct {
	ID        string    `json:"id"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updatedAt"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Assignees []struct {
		Login string `json:"login"`
	} `json:"assignees"`
	Milestone *struct {
		Title string `json:"title"`
	} `json:"milestone"`
}

// fetchItems executes the gh command to get issues or PRs.
func (p *GitHubProvider) fetchItems(itemType string, repoPath string) ([]*sync.Item, error) {
	cmdArgs := []string{itemType, "list", "--state", "all", "--limit", "200", "--json", "id,number,title,body,state,url,updatedAt,labels,assignees,milestone"}
	cmd := exec.Command("gh", cmdArgs...)
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh command failed: %w", err)
	}

	var ghItems []ghItem
	if err := json.Unmarshal(output, &ghItems); err != nil {
		return nil, fmt.Errorf("failed to parse gh JSON output: %w", err)
	}

	var syncItems []*sync.Item
	for i := range ghItems {
		syncItems = append(syncItems, p.ghItemToSyncItem(&ghItems[i], itemType))
	}

	return syncItems, nil
}
