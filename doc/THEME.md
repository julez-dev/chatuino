# Themes

You can configure colors used by Chatuino using the theme.yaml file.

Your theme file is read from ~/.config/chatuino/theme.yaml (config directory may be different depending on OS). You may want to create a new theme file if it doesn't exists already.

## Default Theme

Here is the default theme used by Chatuino (inspired by Nord color scheme):

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

# Status bar state colors (mode indicators)
status_state_view_color: "#88c0d0"      # Default view mode - calm blue
status_state_insert_color: "#a3be8c"    # Insert mode - green (active/go)
status_state_search_color: "#ebcb8b"    # Search mode - yellow (attention)
status_state_inspect_color: "#b48ead"   # Inspect mode - purple (examine)

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

## Color Guidelines

When creating custom themes, consider:

- **Contrast:** Ensure text colors have good contrast against backgrounds for readability
- **Cohesion:** Use a harmonious color palette (e.g., all warm tones or all cool tones)
- **Accessibility:** Avoid color combinations that are hard to distinguish for colorblind users
- **Terminal compatibility:** Test your theme in both light and dark terminal backgrounds

## UI Improvements

The following improvements were made:

- **Modernized color palette** - Nord-inspired colors with better contrast
- **Dimmed timestamps** - Secondary text (timestamps, hints) use `dimmed_text_color` for better visual hierarchy
- **ASCII indicators** - Tab notifications use `[!]` instead of emoji bell, message indicator is `>` instead of colored `@`
- **Semantic state colors** - Status bar mode indicator uses color-coded states:
  - **View mode** (blue): Default viewing state
  - **Insert mode** (green): Actively typing/inputting
  - **Search mode** (yellow): Searching through chat
  - **Inspect mode** (purple): Examining user details

### State Color Fields

The following fields control the status bar mode indicator colors:

- `status_state_view_color` - Color for View and Emote Overview modes (default: `#88c0d0`)
- `status_state_insert_color` - Color for Insert mode (default: `#a3be8c`)
- `status_state_search_color` - Color for Search mode (default: `#ebcb8b`)
- `status_state_inspect_color` - Color for Inspect mode (default: `#b48ead`)

These colors provide instant visual feedback about the current application state.
