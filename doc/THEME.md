# Themes

Chatuino ships with a set of themes and can load your own. The active theme is
selected in `settings.yaml`:

```yaml
theme: dracula
```

Themes are read from `~/.config/chatuino/themes/` (the config directory may
differ depending on your OS). Each file in that directory is one theme; the file
name without its extension is the theme id, so `themes/dracula.yaml` is selected
with `theme: dracula`.

## Switching themes

Press **ctrl+alt+h** to open the theme picker. Moving the cursor previews each
theme live, **enter** saves the selection to `settings.yaml` and **esc** returns
to the theme you started with. Themes you copy into the `themes` directory show
up in the picker without a restart.

The picker lists the shipped themes first, then your own. Each row shows where
the theme comes from (`built-in`, `user`, `legacy`) and marks the active one.
A theme file that cannot be loaded is listed with the reason and cannot be
selected, so a typo in a file you dropped in is visible instead of silently
missing.

To start with a theme without changing your saved selection, pass the id on the
command line (or set `CHATUINO_THEME`):

```sh
chatuino --theme tokyo-night
```

## Built-in themes

| Id                       | Name                     | Notes                                  |
| ------------------------ | ------------------------ | -------------------------------------- |
| `nord`                   | Nord                     | Default                                |
| `ansi16-high-contrast`   | ANSI 16 High Contrast    | Uses ANSI colors, follows the terminal |
| `catppuccin-mocha`       | Catppuccin Mocha         |                                        |
| `dracula`                | Dracula                  |                                        |
| `gruvbox-dark`           | Gruvbox Dark             |                                        |
| `one-dark`               | One Dark                 |                                        |
| `solarized-dark`         | Solarized Dark           |                                        |
| `tokyo-night`            | Tokyo Night              |                                        |
| `catppuccin-latte`       | Catppuccin Latte         | Light theme                            |
| `gruvbox-light`          | Gruvbox Light            | Light theme                            |
| `one-light`              | One Light                | Light theme                            |
| `solarized-light`        | Solarized Light          | Light theme                            |

### Light themes

Chatuino never paints the terminal background; it only sets foreground colors
and the few backgrounds it needs (tab headers, list selection). The light themes
therefore assume your terminal has a light background. On a dark terminal their
text will be hard to read, and there is no automatic detection — switch back
with the picker if that happens.

## Installing a theme

A theme is a single YAML file. Copy it into `~/.config/chatuino/themes/` and it
appears in the picker:

```sh
cp my-theme.yaml ~/.config/chatuino/themes/
```

Only files directly inside the `themes` directory ending in `.yaml` or `.yml`
are read. Subdirectories, hidden files, editor swap files and other extensions
are ignored. Ids are lowercase, so `My-Theme.yaml` and `my-theme.yaml` are the
same theme. If two files claim the same id the theme is flagged as unusable in
the picker with both file names, instead of one of them silently winning.

### Overriding a built-in theme

A file in the `themes` directory wins over a built-in theme with the same id.
To tweak the default instead of starting from scratch, drop a partial file at
`themes/nord.yaml`:

```yaml
name: My Nord
chat_streamer_color: "#ff79c6"
```

Delete the file to go back to the shipped version.

## Writing a theme

Every key is optional: a theme file is merged over the default theme, so you
only have to list the colors you want to change. Unknown keys are an error that
names the file and the key, so a typo cannot silently have no effect.

```yaml
# Optional display name for the picker; defaults to the file name.
name: My Theme

# Emote provider colors
seven_tv_emote_color: "#88c0d0"
twitch_tv_emote_color: "#b48ead"
better_ttv_emote_color: "#bf616a"
ffz_emote_color: "#a3be8c"

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
chatuino_splash_color: "#fd00eb"
splash_highlight_color: "#88c0d0"

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

Colors are hex strings (`"#rrggbb"`). The `ansi16-high-contrast` theme shows the
alternative: theme values are passed to the terminal as lipgloss colors, so
ANSI color numbers such as `"12"` also work and follow the palette of the
terminal instead of a fixed RGB value.

## Switching theme while running

Selecting a theme repaints the entire UI immediately. Unsent messages, scroll
positions, selections, filters and the workspace layout are preserved — only
colors change.

One exception: emote and badge colors from the emote providers are resolved when
chatuino starts. After a mid-session switch, chat text keeps the old theme for
those until you restart chatuino.

## Migrating from theme.yaml

Before themes lived in `themes/`, chatuino read a single
`~/.config/chatuino/theme.yaml`. That file is still honored: while
`settings.yaml` has no `theme:` key, it is the active theme and appears in the
picker as `custom (legacy)`.

As soon as you select a theme in the picker, `theme:` is written to
`settings.yaml` and `theme.yaml` is no longer used. To keep your current colors,
move the file instead of deleting it:

```sh
mkdir -p ~/.config/chatuino/themes
mv ~/.config/chatuino/theme.yaml ~/.config/chatuino/themes/custom.yaml
```

Then select `custom` in the picker. Chatuino never moves or rewrites theme files
itself.
