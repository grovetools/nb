package views

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the keybindings needed for the view component.
// Field names match keymap.Base for consistency.
type KeyMap struct {
	Up           key.Binding
	Down         key.Binding
	Left         key.Binding // Used for closing folds
	Right        key.Binding // Used for opening folds
	PageUp       key.Binding
	PageDown     key.Binding
	Top          key.Binding // gg sequence
	Bottom       key.Binding // G
	FoldOpen     key.Binding // zo
	FoldClose    key.Binding // zc
	FoldToggle   key.Binding // za
	FoldOpenAll  key.Binding // zR
	FoldCloseAll key.Binding // zM
	ToggleSelect key.Binding
	SelectNone   key.Binding
}
