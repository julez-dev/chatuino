package save

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/julez-dev/chatuino/ui/component"
	"gopkg.in/yaml.v3"
)

const (
	settingsFileName = "settings.yaml"
)

type Settings struct {
	VerticalTabList bool               `yaml:"vertical_tab_list"`
	Moderation      ModerationSettings `yaml:"moderation"`
	Chat            ChatSettings       `yaml:"chat"`
	CustomCommands  []CustomCommand    `yaml:"custom_commands"`
}

type ModerationSettings struct {
	StoreChatLogs      bool     `yaml:"store_chat_logs"`
	LogsChannelInclude []string `yaml:"logs_channel_include"`
	LogsChannelExclude []string `yaml:"logs_channel_exclude"`
}

type ChatSettings struct {
	GraphicEmotes bool `yaml:"graphic_emotes"`
}

type CustomCommand struct {
	Trigger     string `yaml:"trigger"`
	Replacement string `yaml:"replacement"`
}

func BuildDefaultSettings() Settings {
	return Settings{
		Moderation: ModerationSettings{
			StoreChatLogs: true,
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
		predefinedCommands := append(component.CommandSuggestions[:], component.ModeratorSuggestions[:]...)

		if slices.Contains(predefinedCommands, c.Trigger) {
			return fmt.Errorf("custom command trigger %q is already a default command", c.Trigger)
		}
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
	f, err := openCreateConfigFile(settingsFileName)
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
