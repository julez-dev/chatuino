package save

import (
	"io"
	"reflect"
	"strings"

	"github.com/charmbracelet/bubbles/key"
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

	// Chat Binds
	InsertMode   key.Binding `yaml:"insert_mode"`
	InspectMode  key.Binding `yaml:"inspect_mode"`
	ChatPopUp    key.Binding `yaml:"chat_pop_up"`
	ChannelPopUp key.Binding `yaml:"channel_pop_up"`
	GoToTop      key.Binding `yaml:"go_to_top"`
	GoToBottom   key.Binding `yaml:"go_to_bottom"`
	DumpChat     key.Binding `yaml:"dump_chat"`
	QuickTimeout key.Binding `yaml:"quick_timeout"`
	CopyMessage  key.Binding `yaml:"copy_message"`
	SearchMode   key.Binding `yaml:"search_mode"`
	QuickSent    key.Binding `yaml:"quick_sent"`

	// Unban Request
	UnbanRequestMode key.Binding `yaml:"unban_request_mode"`
	PrevPage         key.Binding `yaml:"prev_page"`
	NextPage         key.Binding `yaml:"next_page"`
	PrevFilter       key.Binding `yaml:"prev_filter"`
	NextFilter       key.Binding `yaml:"next_filter"`
	Deny             key.Binding `yaml:"deny"`
	Approve          key.Binding `yaml:"approve"`

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
			key.WithKeys("ctrl+t"),
			key.WithHelp("ctrl+t", "open new tab/add account"),
		),
		Remove: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "remove"),
		),
		CloseTab: key.NewBinding(
			key.WithKeys("ctrl+q", "ctrl+w"),
			key.WithHelp("ctrl+q/ctrl+w", "close current tab"),
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
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "user inspect mode"),
		),
		ChatPopUp: key.NewBinding(
			key.WithKeys("ctrl+p"),
			key.WithHelp("ctrl+p", "twitch chat browser pop up"),
		),
		ChannelPopUp: key.NewBinding(
			key.WithKeys("ctrl+b"),
			key.WithHelp("ctrl+b", "twitch channel pop up"),
		),
		MarkLeader: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "mark account as main account"),
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
			key.WithKeys("alt+t"),
			key.WithHelp("alt+t", "quick timeout"),
		),
		DumpChat: key.NewBinding(
			key.WithKeys("ctrl+alt+c"),
			key.WithHelp("ctrl+alt+c", "dump chat"),
		),
		PrevPage: key.NewBinding(
			key.WithKeys("pgup", "left", "h"),
			key.WithHelp("pgup/left/h", "previous page"),
		),
		NextPage: key.NewBinding(
			key.WithKeys("pgdown", "right", "l"),
			key.WithHelp("pgdown/right/l", "next page"),
		),
		Approve: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "approve unban request"),
		),
		Deny: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "deny unban request"),
		),
		NextFilter: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", "next filter"),
		),
		PrevFilter: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[", "previous filter"),
		),
		CopyMessage: key.NewBinding(
			key.WithKeys("alt+c"),
			key.WithHelp("alt+c", "copy selected message"),
		),
		UnbanRequestMode: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "open unban request mode in current channel"),
		),
		SearchMode: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "start search mode in chat window"),
		),
		QuickSent: key.NewBinding(
			key.WithKeys("alt+enter"),
			key.WithHelp("alt+enter", "send message but stay in insert mode"),
		),
	}
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
