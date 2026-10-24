package save

import (
	"io"
	"reflect"
	"strings"

	"charm.land/bubbles/v2/key"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

const (
	keyMapFileName = "keymap.yaml"
)

var (
	_ yaml.Marshaler   = (*KeyMap)(nil)
	_ yaml.Unmarshaler = (*KeyMap)(nil)
)

type KeyMap struct {
	// General
	Up      key.Binding `yaml:"up"`
	Down    key.Binding `yaml:"down"`
	Escape  key.Binding `yaml:"escape"`
	Confirm key.Binding `yaml:"confirm"`
	Help    key.Binding `yaml:"help"`

	// App Binds
	Quit       key.Binding `yaml:"quit"`
	Create     key.Binding `yaml:"create"`
	Remove     key.Binding `yaml:"remove"`
	CloseTab   key.Binding `yaml:"close_tab"`
	DumpScreen key.Binding `yaml:"dump_screen"` // used by lists, and join input type switch

	// Tab Binds
	Next     key.Binding `yaml:"next"`
	Previous key.Binding `yaml:"previous"`

	// Pane controls (active only in pane mode, except PaneMode itself).
	PaneMode         key.Binding `yaml:"pane_mode"`
	PaneFocusLeft    key.Binding `yaml:"pane_focus_left"`
	PaneFocusDown    key.Binding `yaml:"pane_focus_down"`
	PaneFocusUp      key.Binding `yaml:"pane_focus_up"`
	PaneFocusRight   key.Binding `yaml:"pane_focus_right"`
	PaneSwapLeft     key.Binding `yaml:"pane_swap_left"`
	PaneSwapDown     key.Binding `yaml:"pane_swap_down"`
	PaneSwapUp       key.Binding `yaml:"pane_swap_up"`
	PaneSwapRight    key.Binding `yaml:"pane_swap_right"`
	PaneSplitColumns key.Binding `yaml:"pane_split_columns"`
	PaneSplitRows    key.Binding `yaml:"pane_split_rows"`
	PaneNarrow       key.Binding `yaml:"pane_narrow"`
	PaneWiden        key.Binding `yaml:"pane_widen"`
	PaneShorten      key.Binding `yaml:"pane_shorten"`
	PaneTaller       key.Binding `yaml:"pane_taller"`
	PaneClose        key.Binding `yaml:"pane_close"`
	PaneZoom         key.Binding `yaml:"pane_zoom"`
	PaneNext         key.Binding `yaml:"pane_next"`
	PanePrevious     key.Binding `yaml:"pane_previous"`

	// Quick Join
	QuickJoin key.Binding `yaml:"quick_join"`

	// Suggestion Binds (chat composer and join channel input)
	AcceptSuggestion key.Binding `yaml:"accept_suggestion"`
	NextSuggestion   key.Binding `yaml:"next_suggestion"`
	PrevSuggestion   key.Binding `yaml:"prev_suggestion"`

	// Chat Binds
	InsertMode      key.Binding `yaml:"insert_mode"`
	InspectMode     key.Binding `yaml:"inspect_mode"`
	ChatPopUp       key.Binding `yaml:"chat_pop_up"`
	ChannelPopUp    key.Binding `yaml:"channel_pop_up"`
	GoToTop         key.Binding `yaml:"go_to_top"`
	GoToBottom      key.Binding `yaml:"go_to_bottom"`
	PageUp          key.Binding `yaml:"page_up"`
	PageDown        key.Binding `yaml:"page_down"`
	DumpChat        key.Binding `yaml:"dump_chat"`
	QuickTimeout    key.Binding `yaml:"quick_timeout"`
	CopyMessage     key.Binding `yaml:"copy_message"`
	CopyToClipboard key.Binding `yaml:"copy_to_clipboard"`
	ReplyMessage    key.Binding `yaml:"reply_message"`
	SearchMode      key.Binding `yaml:"search_mode"`
	QuickSent       key.Binding `yaml:"quick_sent"`

	// Account Binds
	MarkLeader key.Binding `yaml:"mark_leader"`
}

func (c *KeyMap) MarshalYAML() (interface{}, error) {
	data := map[string][]string{}

	for i := 0; i < reflect.ValueOf(c).Elem().NumField(); i++ {
		field := reflect.TypeOf(c).Elem().Field(i)
		value := reflect.ValueOf(c).Elem().Field(i)

		if value.IsZero() {
			continue
		}

		fieldName := field.Tag.Get("yaml")
		if fieldName == "" {
			fieldName = field.Name
		}

		data[fieldName] = value.Interface().(key.Binding).Keys()
	}

	return data, nil
}

func (c *KeyMap) UnmarshalYAML(value *yaml.Node) error {
	target := map[string][]string{}
	if err := value.Decode(&target); err != nil {
		return err
	}

	val := reflect.ValueOf(c).Elem()

	for targetField, binds := range target {
		for i := 0; i < val.NumField(); i++ {
			fieldName := val.Type().Field(i).Tag.Get("yaml")
			if fieldName == "" {
				fieldName = val.Type().Field(i).Name
			}

			if fieldName == targetField {
				keyBind := reflect.ValueOf(c).Elem().Field(i).Interface().(key.Binding)
				keyBind.SetKeys(binds...)
				keyBind.SetHelp(strings.Join(binds, "/"), keyBind.Help().Desc) // overwrite help with old description but new keys
				reflect.ValueOf(c).Elem().Field(i).Set(reflect.ValueOf(keyBind))
			}
		}
	}

	return nil
}

func paneBinding(description string, keys ...string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(strings.Join(keys, "/"), description))
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
			key.WithKeys("?", "f1"),
			key.WithHelp("?/f1", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c", "q"),
			key.WithHelp("ctrl+c/q", "quit"),
		),
		Create: key.NewBinding(
			key.WithKeys("ctrl+t"),
			key.WithHelp("ctrl+t", "new tab / pane"),
		),
		Remove: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "remove account"),
		),
		CloseTab: key.NewBinding(
			key.WithKeys("ctrl+q", "ctrl+w", "alt+w"),
			key.WithHelp("ctrl+q/ctrl+w/alt+w", "close current tab"),
		),
		DumpScreen: key.NewBinding(
			key.WithKeys("ctrl+alt+d"),
			key.WithHelp("ctrl+alt+d", "dump screen"),
		),
		PaneMode:         paneBinding("pane controls", "f2"),
		PaneFocusLeft:    paneBinding("focus pane left", "left", "h"),
		PaneFocusDown:    paneBinding("focus pane down", "down", "j"),
		PaneFocusUp:      paneBinding("focus pane up", "up", "k"),
		PaneFocusRight:   paneBinding("focus pane right", "right", "l"),
		PaneSwapLeft:     paneBinding("move pane left", "H"),
		PaneSwapDown:     paneBinding("move pane down", "J"),
		PaneSwapUp:       paneBinding("move pane up", "K"),
		PaneSwapRight:    paneBinding("move pane right", "L"),
		PaneSplitColumns: paneBinding("split left/right", "v"),
		PaneSplitRows:    paneBinding("split top/bottom", "s"),
		PaneNarrow:       paneBinding("decrease pane width", "<"),
		PaneWiden:        paneBinding("increase pane width", ">"),
		PaneShorten:      paneBinding("decrease pane height", "-"),
		PaneTaller:       paneBinding("increase pane height", "+"),
		PaneClose:        paneBinding("hide pane (keep tab)", "x"),
		PaneZoom:         paneBinding("zoom pane", "z"),
		PaneNext:         paneBinding("next pane", "tab"),
		PanePrevious:     paneBinding("previous pane", "shift+tab"),
		QuickJoin: key.NewBinding(
			key.WithKeys("ctrl+j"),
			key.WithHelp("ctrl+j", "quick join channel"),
		),
		AcceptSuggestion: key.NewBinding(
			key.WithKeys("tab", "ctrl+space", "right"),
			key.WithHelp("tab/ctrl+space/→", "accept suggestion"),
		),
		NextSuggestion: key.NewBinding(
			key.WithKeys("down", "ctrl+n"),
			key.WithHelp("↓/ctrl+n", "next suggestion"),
		),
		PrevSuggestion: key.NewBinding(
			key.WithKeys("up", "ctrl+p"),
			key.WithHelp("↑/ctrl+p", "previous suggestion"),
		),
		Next: key.NewBinding(
			key.WithKeys("tab", "]", "ctrl+n", "alt+l"),
			key.WithHelp("tab/]/ctrl+n/alt+l", "next tab"),
		),
		Previous: key.NewBinding(
			key.WithKeys("shift+tab", "[", "alt+h"),
			key.WithHelp("shift+tab/[/alt+h", "previous tab"),
		),
		InsertMode: key.NewBinding(
			key.WithKeys("i", "I", "enter"),
			key.WithHelp("i/enter", "insert mode"),
		),
		InspectMode: key.NewBinding(
			key.WithKeys("ctrl+l", "u"),
			key.WithHelp("ctrl+l/u", "user inspect mode"),
		),
		ChatPopUp: key.NewBinding(
			key.WithKeys("ctrl+p", "alt+p"),
			key.WithHelp("ctrl+p/alt+p", "twitch chat browser pop up"),
		),
		ChannelPopUp: key.NewBinding(
			key.WithKeys("ctrl+b", "alt+b"),
			key.WithHelp("ctrl+b/alt+b", "twitch channel pop up"),
		),
		MarkLeader: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "mark account as main account"),
		),
		GoToTop: key.NewBinding(
			key.WithKeys("t", "g", "home"),
			key.WithHelp("t/g/home", "go to top"),
		),
		GoToBottom: key.NewBinding(
			key.WithKeys("b", "G", "end"),
			key.WithHelp("b/G/end", "go to bottom"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+u"),
			key.WithHelp("pgup/ctrl+u", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+d"),
			key.WithHelp("pgdown/ctrl+d", "page down"),
		),
		QuickTimeout: key.NewBinding(
			key.WithKeys("alt+t"),
			key.WithHelp("alt+t", "quick timeout"),
		),
		DumpChat: key.NewBinding(
			key.WithKeys("ctrl+alt+c"),
			key.WithHelp("ctrl+alt+c", "dump chat"),
		),
		CopyMessage: key.NewBinding(
			key.WithKeys("alt+c", "y"),
			key.WithHelp("alt+c/y", "copy selected message to input"),
		),
		CopyToClipboard: key.NewBinding(
			key.WithKeys("Y", "alt+y"),
			key.WithHelp("Y/alt+y", "copy selected message to clipboard"),
		),
		ReplyMessage: key.NewBinding(
			key.WithKeys("alt+r", "r"),
			key.WithHelp("alt+r/r", "reply to selected message"),
		),
		SearchMode: key.NewBinding(
			key.WithKeys("/", "ctrl+f"),
			key.WithHelp("/ / ctrl+f", "start search mode in chat window"),
		),
		QuickSent: key.NewBinding(
			key.WithKeys("alt+enter", "ctrl+enter"),
			key.WithHelp("alt+enter/ctrl+enter", "send message but stay in insert mode"),
		),
	}
}

// CreateReadKeyMap reads the keymap file, creating a default one when it does
// not exist yet. Existing files are never migrated or rewritten: fields that
// are present keep exactly the keys they list, and only fields added in newer
// versions fall back to their defaults.
//
// Deleting keymap.yaml is the supported way to pick up new default aliases.
func CreateReadKeyMap() (KeyMap, error) {
	f, err := openCreateConfigFile(afero.NewOsFs(), keyMapFileName)
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
		b, err := yaml.Marshal(&m)
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
	readableMap := BuildDefaultKeyMap()
	if err := yaml.Unmarshal(b, &readableMap); err != nil {
		return KeyMap{}, err
	}

	return readableMap, nil
}
