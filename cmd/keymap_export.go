package cmd

import (
	"github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/nb/internal/tui/browser"
)

// BrowserKeymapInfo returns the keymap metadata for the nb browser TUI.
// Used by the grove keys registry generator to aggregate all TUI keybindings.
func BrowserKeymapInfo() keymap.TUIInfo {
	return browser.KeymapInfo()
}
