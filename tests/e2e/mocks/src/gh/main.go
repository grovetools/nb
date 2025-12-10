package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// ghItem represents the JSON structure used in our mock data files.
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

// main is a mock of the 'gh' CLI for testing 'nb sync'.
// It reads the GH_MOCK_STATE_DIR environment variable to locate JSON data files.
// Based on the command-line arguments (e.g., "issue list"), it prints the
// content of the corresponding JSON file (issues.json or prs.json) to stdout.
func main() {
	stateDir := os.Getenv("GH_MOCK_STATE_DIR")
	if stateDir == "" {
		fmt.Fprintln(os.Stderr, "mock gh: GH_MOCK_STATE_DIR not set")
		os.Exit(1)
	}

	args := os.Args[1:]

	switch {
	case len(args) > 1 && args[1] == "list":
		handleList(args, stateDir)
	case len(args) > 1 && args[1] == "view":
		handleView(args, stateDir)
	case len(args) > 1 && args[1] == "edit":
		handleEdit(args, stateDir)
	case len(args) > 1 && (args[1] == "reopen" || args[1] == "close"):
		handleStateChange(args, stateDir)
	default:
		fmt.Fprintf(os.Stderr, "mock gh: unhandled command %v\n", args)
		os.Exit(1)
	}
}

func handleList(args []string, stateDir string) {
	var jsonFile string
	if args[0] == "issue" {
		jsonFile = "issues.json"
	} else if args[0] == "pr" {
		jsonFile = "prs.json"
	} else {
		fmt.Fprintf(os.Stderr, "mock gh: unhandled list command %v\n", args)
		os.Exit(1)
	}

	jsonPath := filepath.Join(stateDir, jsonFile)
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mock gh: failed to read %s: %v\n", jsonPath, err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func handleView(args []string, stateDir string) {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "mock gh: view command needs an ID")
		os.Exit(1)
	}
	itemType := args[0]
	itemID := args[2]

	var jsonFile string
	if itemType == "issue" {
		jsonFile = "issues.json"
	} else if itemType == "pr" {
		jsonFile = "prs.json"
	} else {
		fmt.Fprintf(os.Stderr, "mock gh: unhandled view command %v\n", args)
		os.Exit(1)
	}

	jsonPath := filepath.Join(stateDir, jsonFile)
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mock gh: failed to read %s: %v\n", jsonPath, err)
		os.Exit(1)
	}

	var items []ghItem
	if err := json.Unmarshal(data, &items); err != nil {
		fmt.Fprintf(os.Stderr, "mock gh: failed to unmarshal %s: %v\n", jsonFile, err)
		os.Exit(1)
	}

	id, _ := strconv.Atoi(itemID)
	for _, item := range items {
		if item.Number == id {
			output, _ := json.Marshal(item)
			fmt.Println(string(output))
			return
		}
	}
	fmt.Fprintf(os.Stderr, "mock gh: item %s not found in %s\n", itemID, jsonFile)
	os.Exit(1)
}

func handleEdit(args []string, stateDir string) {
	itemType := args[0]
	itemID := args[2]

	var title, body string
	for i := 3; i < len(args); i++ {
		if args[i] == "--title" && i+1 < len(args) {
			title = args[i+1]
			i++
		}
		if args[i] == "--body" && i+1 < len(args) {
			body = args[i+1]
			i++
		}
	}

	updateItem(itemType, itemID, stateDir, func(item *ghItem) {
		if title != "" {
			item.Title = title
		}
		if body != "" {
			item.Body = body
		}
	})
}

func handleStateChange(args []string, stateDir string) {
	itemType := args[0]
	newState := args[1]
	itemID := args[2]

	updateItem(itemType, itemID, stateDir, func(item *ghItem) {
		if newState == "reopen" {
			item.State = "OPEN"
		} else if newState == "close" {
			item.State = "CLOSED"
		}
	})
}

func updateItem(itemType, itemID, stateDir string, updater func(*ghItem)) {
	var jsonFile string
	if itemType == "issue" {
		jsonFile = "issues.json"
	} else if itemType == "pr" {
		jsonFile = "prs.json"
	} else {
		return
	}

	jsonPath := filepath.Join(stateDir, jsonFile)
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return
	}

	var items []ghItem
	if err := json.Unmarshal(data, &items); err != nil {
		return
	}

	id, _ := strconv.Atoi(itemID)
	found := false
	for i := range items {
		if items[i].Number == id {
			updater(&items[i])
			items[i].UpdatedAt = time.Now()
			found = true
			break
		}
	}

	if found {
		newData, _ := json.MarshalIndent(items, "", "\t")
		os.WriteFile(jsonPath, newData, 0644)
	}
}
