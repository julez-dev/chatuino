package save

import (
	"io"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

const (
	themeFileName = "theme.yaml"
)

type Theme struct {
	SevenTVEmoteColor   string `yaml:"seven_tv_emote_color"`
	TwitchTVEmoteColor  string `yaml:"twitch_tv_emote_color"`
	BetterTTVEmoteColor string `yaml:"better_ttv_emote_color"`

	InputPromptColor string `yaml:"input_prompt_color"`

	ChatStreamerColor    string `yaml:"chat_streamer_color"`
	ChatVIPColor         string `yaml:"chat_vip_color"`
	ChatSubColor         string `yaml:"chat_sub_color"`
	ChatTurboColor       string `yaml:"chat_turbo_color"`
	ChatModeratorColor   string `yaml:"chat_moderator_color"`
	ChatIndicatorColor   string `yaml:"chat_indicator_color"`
	ChatSubAlertColor    string `yaml:"chat_sub_alert_color"`
	ChatNoticeAlertColor string `yaml:"chat_notice_alert_color"`
	ChatClearChatColor   string `yaml:"chat_clear_chat_color"`
	ChatErrorColor       string `yaml:"chat_error_color"`

	ListSelectedColor string `yaml:"list_selected_color"`
	ListLabelColor    string `yaml:"list_label_color"`

	StatusColor string `yaml:"status_color"`

	ChatuinoSplashColor  string `yaml:"chatuino_splash_color"`
	SplashHighlightColor string `yaml:"splash_highlight_color"`

	TabHeaderBackgroundColor       string `yaml:"tab_header_background_color"`
	TabHeaderActiveBackgroundColor string `yaml:"tab_header_active_background_color"`

	InspectBorderColor string `yaml:"inspect_border_color"`

	ListBackgroundColor string `yaml:"list_background_color"`
	ListFontColor       string `yaml:"list_font_color"`
}

func BuildDefaultTheme() Theme {
	return Theme{
		SevenTVEmoteColor:   "#8aadf4",
		TwitchTVEmoteColor:  "#c6a0f6",
		BetterTTVEmoteColor: "#ed8796",

		InputPromptColor: "#8aadf4",

		ChatStreamerColor:  "#fe640b",
		ChatVIPColor:       "#E005B9",
		ChatSubColor:       "#c6a0f6",
		ChatTurboColor:     "#c6a0f6",
		ChatModeratorColor: "#a6e3a1",
		ChatIndicatorColor: "#8aadf4",

		ChatSubAlertColor:    "#c6a0f6",
		ChatNoticeAlertColor: "#f5bde6",
		ChatClearChatColor:   "#a35df2",
		ChatErrorColor:       "#ed8796",

		ListSelectedColor: "#a6da95",
		ListLabelColor:    "#c6a0f6",

		StatusColor: "#8aadf4",

		ChatuinoSplashColor:  "#94e2d5",
		SplashHighlightColor: "#bac2de",

		TabHeaderBackgroundColor:       "#5b6078",
		TabHeaderActiveBackgroundColor: "#1e2030",

		InspectBorderColor: "#c6a0f6",

		ListBackgroundColor: "#303446",
		ListFontColor:       "#c6d0f5",
	}
}

func ThemeFromDisk() (Theme, error) {
	f, err := openCreateConfigFile(afero.NewOsFs(), themeFileName)
	if err != nil {
		return Theme{}, err
	}

	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return Theme{}, err
	}

	if stat.Size() == 0 {
		return BuildDefaultTheme(), nil
	}

	b, err := io.ReadAll(f)
	if err != nil {
		return Theme{}, err
	}

	theme := BuildDefaultTheme()

	if err := yaml.Unmarshal(b, &theme); err != nil {
		return Theme{}, err
	}

	return theme, nil
}
