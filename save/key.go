package save

import (
	"io"

	"github.com/charmbracelet/bubbles/key"
	"gopkg.in/yaml.v3"
)

const (
	keyMapFileName = "keymap.yaml"
)

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
	InsertMode   key.Binding
	InspectMode  key.Binding
	ChatPopUp    key.Binding
	GoToTop      key.Binding
	GoToBottom   key.Binding
	DumpChat     key.Binding
	QuickTimeout key.Binding
	CopyMessage  key.Binding

	// Unban Request
	PrevPage   key.Binding
	NextPage   key.Binding
	PrevFilter key.Binding
	NextFilter key.Binding
	Deny       key.Binding
	Approve    key.Binding

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
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
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
		QuickTimeout: key.NewBinding(
			key.WithKeys("ctrl+t"),
			key.WithHelp("ctrl+t", "quick timeout"),
		),
		DumpChat: key.NewBinding(
			key.WithKeys("ctrl+alt+c"),
			key.WithHelp("ctrl+alt+c", "dump chat"),
		),
		PrevPage:    key.NewBinding(key.WithKeys("pgup", "left", "h")),
		NextPage:    key.NewBinding(key.WithKeys("pgdown", "right", "l")),
		Approve:     key.NewBinding(key.WithKeys("a")),
		Deny:        key.NewBinding(key.WithKeys("d")),
		NextFilter:  key.NewBinding(key.WithKeys("]")),
		PrevFilter:  key.NewBinding(key.WithKeys("[")),
		CopyMessage: key.NewBinding(key.WithKeys("alt+c")),
	}
}

func (k KeyMap) saveRepresentation() saveableKeyMap {
	return saveableKeyMap{
		Up:           k.Up.Keys(),
		Down:         k.Down.Keys(),
		Escape:       k.Escape.Keys(),
		Confirm:      k.Confirm.Keys(),
		Help:         k.Help.Keys(),
		Quit:         k.Quit.Keys(),
		Create:       k.Create.Keys(),
		Remove:       k.Remove.Keys(),
		CloseTab:     k.CloseTab.Keys(),
		DumpScreen:   k.DumpScreen.Keys(),
		Next:         k.Next.Keys(),
		Previous:     k.Previous.Keys(),
		InsertMode:   k.InsertMode.Keys(),
		InspectMode:  k.InspectMode.Keys(),
		ChatPopUp:    k.ChatPopUp.Keys(),
		GoToTop:      k.GoToTop.Keys(),
		GoToBottom:   k.GoToBottom.Keys(),
		DumpChat:     k.DumpChat.Keys(),
		MarkLeader:   k.MarkLeader.Keys(),
		QuickTimeout: k.QuickTimeout.Keys(),
		PrevPage:     k.PrevPage.Keys(),
		NextPage:     k.NextPage.Keys(),
		Accept:       k.Approve.Keys(),
		Deny:         k.Deny.Keys(),
		PrevFilter:   k.PrevFilter.Keys(),
		NextFilter:   k.NextFilter.Keys(),
		CopyMessage:  k.CopyMessage.Keys(),
	}
}

type saveableKeyMap struct {
	Up      []string `yaml:"up"`
	Down    []string `yaml:"down"`
	Escape  []string `yaml:"escape"`
	Confirm []string `yaml:"confirm"`
	Help    []string `yaml:"help"`

	// App Binds
	Quit       []string `yaml:"quit"`
	Create     []string `yaml:"create"`
	Remove     []string `yaml:"remove"`
	CloseTab   []string `yaml:"close_tab"`
	DumpScreen []string `yaml:"dump_screen"`

	// Tab Binds
	Next     []string `yaml:"next"`
	Previous []string `yaml:"previous"`

	// Chat Binds
	InsertMode   []string `yaml:"insert_mode"`
	InspectMode  []string `yaml:"inspect_mode"`
	ChatPopUp    []string `yaml:"chat_pop_up"`
	GoToTop      []string `yaml:"go_to_top"`
	GoToBottom   []string `yaml:"go_to_bottom"`
	DumpChat     []string `yaml:"dump_chat"`
	QuickTimeout []string `yaml:"quick_timeout"`
	CopyMessage  []string `yaml:"copy_message"`

	// Unban Request
	PrevPage   []string `yaml:"prev_page"`
	NextPage   []string `yaml:"next_page"`
	Deny       []string `yaml:"deny_request"`
	Accept     []string `yaml:"approve_request"`
	PrevFilter []string `yaml:"prev_filter"`
	NextFilter []string `yaml:"next_filter"`

	// Account Binds
	MarkLeader []string `yaml:"mark_leader"`
}

func setIfNotEmpty(b *key.Binding, keys []string) {
	if len(keys) > 0 {
		b.SetKeys(keys...)
	}
}

func (s saveableKeyMap) keyMap() KeyMap {
	m := BuildDefaultKeyMap() // For loading help texts

	setIfNotEmpty(&m.Up, s.Up)
	setIfNotEmpty(&m.Down, s.Down)
	setIfNotEmpty(&m.Escape, s.Escape)
	setIfNotEmpty(&m.Confirm, s.Confirm)
	setIfNotEmpty(&m.Help, s.Help)
	setIfNotEmpty(&m.Quit, s.Quit)
	setIfNotEmpty(&m.Create, s.Create)
	setIfNotEmpty(&m.Remove, s.Remove)
	setIfNotEmpty(&m.CloseTab, s.CloseTab)
	setIfNotEmpty(&m.DumpScreen, s.DumpScreen)
	setIfNotEmpty(&m.Next, s.Next)
	setIfNotEmpty(&m.Previous, s.Previous)
	setIfNotEmpty(&m.InsertMode, s.InsertMode)
	setIfNotEmpty(&m.InspectMode, s.InspectMode)
	setIfNotEmpty(&m.ChatPopUp, s.ChatPopUp)
	setIfNotEmpty(&m.GoToTop, s.GoToTop)
	setIfNotEmpty(&m.GoToBottom, s.GoToBottom)
	setIfNotEmpty(&m.DumpChat, s.DumpChat)
	setIfNotEmpty(&m.MarkLeader, s.MarkLeader)
	setIfNotEmpty(&m.QuickTimeout, s.QuickTimeout)
	setIfNotEmpty(&m.PrevPage, s.PrevPage)
	setIfNotEmpty(&m.NextPage, s.NextPage)
	setIfNotEmpty(&m.Approve, s.Accept)
	setIfNotEmpty(&m.Deny, s.Deny)
	setIfNotEmpty(&m.PrevFilter, s.PrevFilter)
	setIfNotEmpty(&m.NextFilter, s.NextFilter)
	setIfNotEmpty(&m.CopyMessage, s.CopyMessage)

	return m
}

func CreateReadKeyMap() (KeyMap, error) {
	f, err := openCreateConfigFile(keyMapFileName)

	if err != nil {
		return KeyMap{}, err
	}

	defer f.Close()

	stat, err := f.Stat()

	if err != nil {
		return KeyMap{}, err
	}

	// Config was empty, return default config and write a default one to disk
	if stat.Size() == 0 {
		m := BuildDefaultKeyMap()
		saveableMap := m.saveRepresentation()

		b, err := yaml.Marshal(saveableMap)

		if err != nil {
			return KeyMap{}, err
		}

		if _, err := f.Write(b); err != nil {
			return KeyMap{}, err
		}

		return m, nil
	}

	b, err := io.ReadAll(f)

	if err != nil {
		return KeyMap{}, err
	}

	// Config was not empty, read it and return it
	var readableMap saveableKeyMap
	if err := yaml.Unmarshal(b, &readableMap); err != nil {
		return KeyMap{}, err
	}

	return readableMap.keyMap(), nil
}
