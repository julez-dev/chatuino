package save

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

const (
	settingsFileName = "settings.yaml"
)

type Settings struct {
	Moderation ModerationSettings `yaml:"moderation"`
	Chat       ChatSettings       `yaml:"chat"`
}

type ModerationSettings struct {
	StoreChatLogs      bool     `yaml:"store_chat_logs"`
	LogsChannelInclude []string `yaml:"logs_channel_include"`
	LogsChannelExclude []string `yaml:"logs_channel_exclude"`
}

type ChatSettings struct {
	GraphicEmotes bool `yaml:"graphic_emotes"`
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

	return nil
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
