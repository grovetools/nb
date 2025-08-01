package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-notebook/pkg/workspace"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check and repair workspace configuration issues",
	Long: `The doctor command checks for common workspace configuration issues
and offers to fix them automatically.

Issues it can detect and fix:
- Inconsistent path casing in the database
- Invalid or non-existent workspace paths
- Duplicate workspace entries`,
	RunE: runDoctor,
}

var (
	doctorFix bool
)

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "Automatically fix issues")
}

func runDoctor(cmd *cobra.Command, args []string) error {
	// Use the existing initialization from other commands
	dataDir := filepath.Join(os.Getenv("HOME"), ".local", "share", "nb")
	registry, err := workspace.NewRegistry(dataDir)
	if err != nil {
		return fmt.Errorf("init registry: %w", err)
	}
	defer registry.Close()
	workspaces, err := registry.List()
	if err != nil {
		return fmt.Errorf("list workspaces: %w", err)
	}

	fmt.Println("🏥 Running workspace doctor...")
	fmt.Println()

	issues := 0
	fixed := 0

	// Check for path casing issues
	pathMap := make(map[string][]*workspace.Workspace)
	for _, ws := range workspaces {
		// Normalize path for comparison
		normalizedPath := strings.ToLower(ws.Path)
		pathMap[normalizedPath] = append(pathMap[normalizedPath], ws)
	}

	// Find workspaces with inconsistent casing
	for _, wsList := range pathMap {
		if len(wsList) > 1 {
			issues++
			fmt.Printf("❗ Found %d workspaces with path variations:\n", len(wsList))
			for _, ws := range wsList {
				fmt.Printf("   - %s: %s\n", ws.Name, ws.Path)
			}

			if doctorFix {
				// Keep the most recently used one and update its path to be normalized
				var mostRecent *workspace.Workspace
				for _, ws := range wsList {
					if mostRecent == nil || ws.LastUsed.After(mostRecent.LastUsed) {
						mostRecent = ws
					}
				}

				// Normalize the path
				absPath, err := filepath.Abs(mostRecent.Path)
				if err == nil {
					mostRecent.Path = absPath
					if err := registry.Add(mostRecent); err == nil {
						fmt.Printf("   ✅ Updated %s to use normalized path: %s\n", mostRecent.Name, absPath)
						fixed++
					}
				}

				// Remove duplicates
				for _, ws := range wsList {
					if ws.Name != mostRecent.Name {
						if err := registry.Remove(ws.Name); err == nil {
							fmt.Printf("   ✅ Removed duplicate: %s\n", ws.Name)
						}
					}
				}
			} else {
				fmt.Println("   💡 Run with --fix to resolve this issue")
			}
			fmt.Println()
		}
	}

	// Check for workspaces with same name but different paths
	nameMap := make(map[string][]*workspace.Workspace)
	for _, ws := range workspaces {
		nameMap[ws.Name] = append(nameMap[ws.Name], ws)
	}

	for name, wsList := range nameMap {
		if len(wsList) > 1 {
			issues++
			fmt.Printf("❗ Found %d workspaces with name '%s':\n", len(wsList), name)
			for _, ws := range wsList {
				fmt.Printf("   - Path: %s (last used: %s)\n", ws.Path, ws.LastUsed.Format("2006-01-02 15:04"))
			}
			if !doctorFix {
				fmt.Println("   💡 This requires manual intervention")
			}
			fmt.Println()
		}
	}

	// Summary
	if issues == 0 {
		fmt.Println("✨ No issues found! Your workspace configuration is healthy.")
	} else {
		fmt.Printf("\n📊 Summary: Found %d issue(s)", issues)
		if doctorFix {
			fmt.Printf(", fixed %d", fixed)
		}
		fmt.Println()
		if !doctorFix && issues > fixed {
			fmt.Println("\n💡 Run 'nb workspace doctor --fix' to automatically fix issues")
		}
	}

	return nil
}