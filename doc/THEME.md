# Themes

You can configure the colors used by Chatuino with the theme.yaml file.

Your theme file is read from `~/.config/chatuino/theme.yaml` (the config directory may differ depending on your OS). Create the file if it doesn't exist.

## Default Theme

The default theme is inspired by the Nord color scheme:

```yaml
# Emote provider colors
seven_tv_emote_color: "#88c0d0"
twitch_tv_emote_color: "#b48ead"
better_ttv_emote_color: "#bf616a"

# Input
input_prompt_color: "#88c0d0"

# Chat user colors
chat_streamer_color: "#d08770"
chat_vip_color: "#b48ead"
chat_sub_color: "#a3be8c"
chat_turbo_color: "#5e81ac"
chat_moderator_color: "#a3be8c"
chat_indicator_color: "#88c0d0"

# Alert colors
chat_sub_alert_color: "#b48ead"
chat_notice_alert_color: "#ebcb8b"
chat_clear_chat_color: "#d08770"
chat_error_color: "#bf616a"

# List/selection colors
list_selected_color: "#88c0d0"
list_label_color: "#81a1c1"
active_label_color: "#ebcb8b"

# Status
status_color: "#88c0d0"

# Splash screen
chatuino_splash_color: "#8fbcbb"
splash_highlight_color: "#d8dee9"

# Tab headers
tab_header_background_color: "#3b4252"
tab_header_active_background_color: "#2e3440"

# Borders
inspect_border_color: "#5e81ac"

# List styling
list_background_color: "#2e3440"
list_font_color: "#d8dee9"

# UI chrome
dimmed_text_color: "#4c566a"
```
