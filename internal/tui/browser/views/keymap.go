package views

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the keybindings needed for the view component.
type KeyMap struct {
	Up           key.Binding
	Down         key.Binding
	PageUp       key.Binding
	PageDown     key.Binding
	GoToTop      key.Binding
	GoToBottom   key.Binding
	Fold         key.Binding
	Unfold       key.Binding
	FoldPrefix   key.Binding
	ToggleSelect key.Binding
	SelectAll    key.Binding
	SelectNone   key.Binding
}
