package keybind

import "github.com/charmbracelet/bubbles/key"

type KeyMap struct {
	// General
	Up      key.Binding
	Down    key.Binding
	Escape  key.Binding
	Confirm key.Binding
	Help    key.Binding

	// App Binds
	Quit       key.Binding
	Create     key.Binding
	Remove     key.Binding
	CloseTab   key.Binding
	DumpScreen key.Binding

	// Tab Binds
	Next     key.Binding
	Previous key.Binding

	// Chat Binds
	InsertMode  key.Binding
	InspectMode key.Binding
	ChatPopUp   key.Binding
	GoToTop     key.Binding
	GoToBottom  key.Binding
	DumpChat    key.Binding

	// Account Binds
	MarkLeader key.Binding
}

func BuildDefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "escape"),
		),
		Confirm: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Create: key.NewBinding(
			key.WithKeys("f1"),
			key.WithHelp("f1", "create"),
		),
		Remove: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "remove"),
		),
		CloseTab: key.NewBinding(
			key.WithKeys("q", "ctrl+w"),
			key.WithHelp("q/ctrl+w", "Close Tab"),
		),
		DumpScreen: key.NewBinding(
			key.WithKeys("ctrl+alt+d"),
			key.WithHelp("ctrl+alt+d", "dump screen"),
		),
		Next: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next item"),
		),
		Previous: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "previous item"),
		),
		InsertMode: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "insert mode"),
		),
		InspectMode: key.NewBinding(
			key.WithKeys("u"),
			key.WithHelp("u", "inspect mode"),
		),
		ChatPopUp: key.NewBinding(
			key.WithKeys("p", "c"),
			key.WithHelp("p/c", "twitch chat browser pop up/channel"),
		),
		MarkLeader: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "mark account as leader"),
		),
		GoToTop: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "go to top"),
		),
		GoToBottom: key.NewBinding(
			key.WithKeys("b"),
			key.WithHelp("b", "go to bottom"),
		),
		DumpChat: key.NewBinding(
			key.WithKeys("ctrl+alt+c"),
			key.WithHelp("ctrl+alt+c", "dump chat"),
		),
	}
}
