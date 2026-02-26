package syncthing

import (
	"fmt"
	"os/exec"
	"strings"
)

// Client wraps the local syncthing cli for programmatically configuring folders and devices.
type Client struct{}

// NewClient returns a new Syncthing client.
func NewClient() *Client {
	return &Client{}
}

// IsInstalled checks if the syncthing binary is available in the user's PATH.
func (c *Client) IsInstalled() bool {
	_, err := exec.LookPath("syncthing")
	return err == nil
}

// AddFolder registers a new folder in Syncthing.
func (c *Client) AddFolder(id, path string) error {
	cmd := exec.Command("syncthing", "cli", "config", "folders", "add", "--id", id, "--path", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := string(out)
		// Ignore error if the folder already exists
		if strings.Contains(outStr, "already exists") {
			return nil
		}
		return fmt.Errorf("failed to add syncthing folder: %s (err: %w)", outStr, err)
	}
	return nil
}

// ShareFolderWithDevice assigns a device ID to a specific folder.
func (c *Client) ShareFolderWithDevice(folderID, deviceID string) error {
	cmd := exec.Command("syncthing", "cli", "config", "folders", folderID, "devices", "add", "--device-id", deviceID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := string(out)
		// Ignore error if the device is already attached to the folder
		if strings.Contains(outStr, "already exists") {
			return nil
		}
		return fmt.Errorf("failed to share folder with device %s: %s (err: %w)", deviceID, outStr, err)
	}
	return nil
}
