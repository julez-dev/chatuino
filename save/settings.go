package save

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/julez-dev/chatuino/command"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

const (
	settingsFileName = "settings.yaml"
	maxGraphicScale  = 4 // must not exceed kittyimg.MaxRows
)

type Settings struct {
	VerticalTabList bool               `yaml:"vertical_tab_list"`
	Mouse           bool               `yaml:"mouse"` // capture mouse clicks/wheel; default: true
	Moderation      ModerationSettings `yaml:"moderation"`
	Chat            ChatSettings       `yaml:"chat"`
	CustomCommands  []CustomCommand    `yaml:"custom_commands"`
	BlockSettings   BlockSettings      `yaml:"block_settings"`
	Security        SecuritySettings   `yaml:"security"`
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
