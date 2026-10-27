package save

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/julez-dev/chatuino/command"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

const (
	settingsFileName = "settings.yaml"
	// themeSettingKey is the settings.yaml key holding the active theme id.
	themeSettingKey = "theme"
	maxGraphicScale = 4 // must not exceed kittyimg.MaxRows
)

// SettingsPath is where SettingsFromDisk reads the settings from. It is used in
// user-facing messages so an error can point at the file to edit.
func SettingsPath() string {
	return settingsFilePath(ConfigDir())
}

// settingsFilePath is the settings file inside a chatuino config directory.
func settingsFilePath(configDir string) string {
	return filepath.Join(configDir, settingsFileName)
}

type Settings struct {
	VerticalTabList bool `yaml:"vertical_tab_list"`
	// Theme is the id of the active theme. Empty means "nothing configured",
	// which keeps a legacy theme.yaml active or falls back to the default.
	Theme          string             `yaml:"theme"`
	Mouse          bool               `yaml:"mouse"` // capture mouse clicks/wheel; default: true
	Moderation     ModerationSettings `yaml:"moderation"`
	Chat           ChatSettings       `yaml:"chat"`
	CustomCommands []CustomCommand    `yaml:"custom_commands"`
	BlockSettings  BlockSettings      `yaml:"block_settings"`
	Security       SecuritySettings   `yaml:"security"`
}

type ModerationSettings struct {
	StoreChatLogs      bool     `yaml:"store_chat_logs"`
	LogsChannelInclude []string `yaml:"logs_channel_include"`
	LogsChannelExclude []string `yaml:"logs_channel_exclude"`
}

type ChatSettings struct {
	GraphicBadges              bool   `yaml:"graphic_badges"`
	GraphicEmotes              bool   `yaml:"graphic_emotes"`
	GraphicScale               int    `yaml:"graphic_scale"` // Height of graphic emotes/badges in terminal rows (1-4), default: 1
	DisableBadges              bool   `yaml:"disable_badges"`
	DisablePaddingWrappedLines bool   `yaml:"disable_padding_wrapped_lines"`
	SmoothScroll               bool   `yaml:"smooth_scroll"`
	DisableEmoteTooltip        bool   `yaml:"disable_emote_tooltip"`    // don't show the emote name when hovering a graphic emote with the mouse
	NewMessageIndicator        bool   `yaml:"new_message_indicator"`    // show a badge when messages arrive while scrolled up; default: true
	ReplyContext               bool   `yaml:"reply_context"`            // quote the message a reply answers above it; default: true
	TimeFormat                 string `yaml:"time_format"`              // Go time format string, default: "15:04:05"
	UserInspectTimeFormat      string `yaml:"user_inspect_time_format"` // Go time format string, default: "2006-01-02 15:04:05"
}

type BlockSettings struct {
	Users []string `yaml:"users"`
	Words []string `yaml:"words"`
}

type SecuritySettings struct {
	CheckLinks bool `yaml:"check_links"`
}

type CustomCommand struct {
	Trigger     string `yaml:"trigger"`
	Replacement string `yaml:"replacement"`
}

func BuildDefaultSettings() Settings {
	return Settings{
		Mouse: true,
		Moderation: ModerationSettings{
			StoreChatLogs: true,
		},
		Security: SecuritySettings{
			CheckLinks: true,
		},
		Chat: ChatSettings{
			TimeFormat:            "15:04:05",
			UserInspectTimeFormat: "2006-01-02 15:04:05",
			NewMessageIndicator:   true,
			ReplyContext:          true,
		},
	}
}

func (s Settings) validate() error {
	if len(s.Moderation.LogsChannelExclude) > 0 && len(s.Moderation.LogsChannelInclude) > 0 {
		return fmt.Errorf("cant't have both of logs_channel_include and logs_channel_exclude in settings.moderation")
	}

	for _, c := range s.CustomCommands {
		if len(c.Trigger) < 4 || !strings.HasPrefix(c.Trigger, "/") {
			return fmt.Errorf("custom command trigger %q must have at least 3 characters and start with a /", c.Trigger)
		}

		// combine CommandSuggestions and CustomCommands to check for collisions for custom commands
		predefinedCommands := append(command.CommandSuggestions[:], command.ModeratorSuggestions[:]...)

		if slices.Contains(predefinedCommands, c.Trigger) {
			return fmt.Errorf("custom command trigger %q is already a default command", c.Trigger)
		}
	}

	if s.Chat.GraphicScale < 0 || s.Chat.GraphicScale > maxGraphicScale {
		return fmt.Errorf("chat.graphic_scale must be between 1 and %d, got %d", maxGraphicScale, s.Chat.GraphicScale)
	}

	if slices.Contains(s.BlockSettings.Users, "") {
		return fmt.Errorf("block settings user entry can't be empty string")
	}

	if slices.Contains(s.BlockSettings.Words, "") {
		return fmt.Errorf("block settings word entry can't be empty string")
	}

	return nil
}

func (s Settings) BuildCustomSuggestionMap() map[string]string {
	m := make(map[string]string, len(s.CustomCommands))
	for _, c := range s.CustomCommands {
		m[c.Trigger] = c.Replacement
	}

	return m
}

func SettingsFromDisk() (Settings, error) {
	f, err := openCreateConfigFile(afero.NewOsFs(), settingsFileName)
	if err != nil {
		return Settings{}, err
	}

	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return Settings{}, err
	}

	if stat.Size() == 0 {
		return BuildDefaultSettings(), nil
	}

	b, err := io.ReadAll(f)
	if err != nil {
		return Settings{}, err
	}

	settings := BuildDefaultSettings()

	if err := yaml.Unmarshal(b, &settings); err != nil {
		return Settings{}, err
	}

	if err := settings.validate(); err != nil {
		return Settings{}, err
	}

	return settings, nil
}

// SetThemeSetting stores the theme id in the settings file at path.
//
// The document is edited as a yaml.Node instead of being re-marshalled from
// Settings: settings.yaml is hand-edited by users, and a round-trip through the
// struct would drop every comment and every key this version does not model yet.
func SetThemeSetting(fsys afero.Fs, path, id string) error {
	data, err := afero.ReadFile(fsys, path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}

	doc := &yaml.Node{}

	if len(bytes.TrimSpace(data)) > 0 {
		if err := yaml.Unmarshal(data, doc); err != nil {
			return fmt.Errorf("failed to parse %s: %w", path, err)
		}
	}

	root, err := rootMappingNode(doc)
	if err != nil {
		return fmt.Errorf("failed to update %s: %w", path, err)
	}

	setMappingValue(root, themeSettingKey, id)

	var buf bytes.Buffer

	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2) // match the two-space nesting used in the docs

	if err := encoder.Encode(doc); err != nil {
		return fmt.Errorf("failed to encode %s: %w", path, err)
	}

	if err := encoder.Close(); err != nil {
		return fmt.Errorf("failed to encode %s: %w", path, err)
	}

	out := buf.Bytes()

	mode := os.FileMode(0o600)
	if stat, statErr := fsys.Stat(path); statErr == nil {
		mode = stat.Mode().Perm()
	}

	if err := afero.WriteFile(fsys, path, out, mode); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}

	return nil
}

// rootMappingNode returns the top-level mapping of doc, creating it for an empty
// document.
func rootMappingNode(doc *yaml.Node) (*yaml.Node, error) {
	if doc.Kind == 0 {
		root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		doc.Kind = yaml.DocumentNode
		doc.Content = []*yaml.Node{root}

		return root, nil
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("unexpected document structure")
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("settings file is not a mapping")
	}

	return root, nil
}

// setMappingValue sets key to value in mapping, reusing the existing value node
// so its comments survive.
func setMappingValue(mapping *yaml.Node, key, value string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}

		node := mapping.Content[i+1]
		node.Kind = yaml.ScalarNode
		node.Tag = "!!str"
		node.Value = value

		return
	}

	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}
