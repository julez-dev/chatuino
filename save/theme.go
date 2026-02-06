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
	FFZEmoteColor       string `yaml:"ffz_emote_color"`

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
	ActiveLabelColor  string `yaml:"active_label_color"`

	StatusColor string `yaml:"status_color"`

	ChatuinoSplashColor  string `yaml:"chatuino_splash_color"`
	SplashHighlightColor string `yaml:"splash_highlight_color"`

	TabHeaderBackgroundColor       string `yaml:"tab_header_background_color"`
	TabHeaderActiveBackgroundColor string `yaml:"tab_header_active_background_color"`

	InspectBorderColor string `yaml:"inspect_border_color"`

	ListBackgroundColor string `yaml:"list_background_color"`
	ListFontColor       string `yaml:"list_font_color"`

	// UI chrome
	DimmedTextColor string `yaml:"dimmed_text_color"`
}

func BuildDefaultTheme() Theme {
	return Theme{
		// Emote provider colors - slightly more vibrant
		SevenTVEmoteColor:   "#88c0d0",
		TwitchTVEmoteColor:  "#b48ead",
		BetterTTVEmoteColor: "#bf616a",
		FFZEmoteColor:       "#a3be8c",

		// Input
		InputPromptColor: "#88c0d0",

		// Chat user colors - improved contrast
		ChatStreamerColor:  "#d08770",
		ChatVIPColor:       "#b48ead",
		ChatSubColor:       "#a3be8c",
		ChatTurboColor:     "#5e81ac",
		ChatModeratorColor: "#a3be8c",
		ChatIndicatorColor: "#88c0d0",

		// Alert colors - better visibility
		ChatSubAlertColor:    "#b48ead",
		ChatNoticeAlertColor: "#ebcb8b",
		ChatClearChatColor:   "#d08770",
		ChatErrorColor:       "#bf616a",

		// List/selection colors
		ListSelectedColor: "#88c0d0",
		ListLabelColor:    "#81a1c1",
		ActiveLabelColor:  "#ebcb8b",

		// Status
		StatusColor: "#88c0d0",

		// Splash screen
		ChatuinoSplashColor:  "#fd00eb",
		SplashHighlightColor: "#88c0d0",

		// Tab headers - deeper contrast
		TabHeaderBackgroundColor:       "#3b4252",
		TabHeaderActiveBackgroundColor: "#2e3440",

		// Borders
		InspectBorderColor: "#5e81ac",

		// List styling
		ListBackgroundColor: "#2e3440",
		ListFontColor:       "#d8dee9",

		// UI chrome
		DimmedTextColor: "#4c566a",
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
