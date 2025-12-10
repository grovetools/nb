package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
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
	for _, item := range ghItems {
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

		syncItem := &sync.Item{
			ID:        fmt.Sprintf("%d", item.Number),
			Type:      itemType,
			Title:     item.Title,
			Body:      item.Body,
			State:     item.State,
			URL:       item.URL,
			Labels:    labels,
			Assignees: assignees,
			Milestone: milestone,
			UpdatedAt: item.UpdatedAt,
		}
		syncItems = append(syncItems, syncItem)
	}

	return syncItems, nil
}
