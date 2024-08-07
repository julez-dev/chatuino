package save

import (
	"io"

	"gopkg.in/yaml.v3"
)

const (
	settingsFileName = "settings.yaml"
)

type Settings struct {
	Moderation ModerationSettings `yaml:"moderation"`
}

type ModerationSettings struct {
	StoreChatLogs bool `yaml:"store_chat_logs"`
}

func BuildDefaultSettings() Settings {
	return Settings{
		Moderation: ModerationSettings{
			StoreChatLogs: true,
		},
	}
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

	return settings, nil
}
