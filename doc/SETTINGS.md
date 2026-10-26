# Settings

Chatuino can run without a settings file, but you may want to configure its default behavior.

Your settings file is read from `~/.config/chatuino/settings.yaml` (the config directory may differ depending on your OS). Create the file if it doesn't exist.

```yaml
vertical_tab_list: false # Display tabs vertically instead of horizontally
mouse: true # Capture mouse clicks and wheel events; set false to restore native text selection; Default: true
moderation:
  store_chat_logs: true # Store chat logs in a SQLite database; Default: false

  # NOTE: logs_channel_include and logs_channel_exclude are mutually exclusive.
  logs_channel_include: ["lirik", "sodapoppin"] # Only log specified channels
  # logs_channel_exclude: ["lec"] # Log all channels except those specified

security:
  check_links: true # Check and display HTTP redirects next to URLs. Uses Chatuino server to hide IP when resolving; Default: true

# Globally block specific users and words
block_settings:
  users:
    - julezdev
  words:
    - Kappa
chat:
  # NOTE: Read the README for more information about emote rendering before enabling this feature
  graphic_emotes: true # Display emotes as images instead of text; Default: false
  graphic_badges: true # Display badges as images instead of text; Default: false
  graphic_scale: 2 # Height of graphic emotes/badges in terminal rows (1-4). Lines containing images take up this many rows; Default: 1
  disable_badges: false # Hide badges entirely; Default: false
  smooth_scroll: true # Animate chat scrolling when new messages arrive; Default: false
  new_message_indicator: true # Show a badge counting messages that arrive while you are not caught up with the newest message; Default: true
  reply_context: true # Quote the message a reply answers above the reply, dropping Twitch's own @name from the reply text; Default: true
  disable_emote_tooltip: false # Don't show the emote name when hovering a graphic emote with the mouse (needs mouse: true); Default: false
  time_format: "15:04:05" # Go time format for message timestamps; Default: "15:04:05"
custom_commands:
  # Custom commands are available as command suggestions
  - trigger: "/ocean"
    replacement: "OCEAN MAN 🌊 😍 Take me by the hand ✋ lead me to the land that you understand 🙌 🌊 OCEAN MAN 🌊 😍 The voyage 🚲 to the corner of the 🌎 globe is a real trip 👌 🌊 OCEAN MAN 🌊 😍 The crust of a tan man 👳 imbibed by the sand 👍 Soaking up the 💦 thirst of the land 💯"
```

## Time Format

The `time_format` setting uses Go's reference time format. Go uses a specific reference time (`Mon Jan 2 15:04:05 MST 2006`) to define formats. You construct your desired format by showing how this reference time should be displayed.

Common format examples:

| Format String | Example Output | Description |
| ------------- | -------------- | ----------- |
| `15:04:05` | `14:30:45` | 24-hour with seconds (default) |
| `15:04` | `14:30` | 24-hour without seconds |
| `3:04 PM` | `2:30 PM` | 12-hour with AM/PM |
| `3:04:05 PM` | `2:30:45 PM` | 12-hour with seconds and AM/PM |
| `Jan 2 15:04` | `Dec 25 14:30` | Month, day, and time |
| `2006-01-02 15:04` | `2024-12-25 14:30` | ISO date and time |

You can use [this Go Playground link](https://go.dev/play/p/mgHrQLytjZM) to prototype and test custom time formats.

Reference components:
- Year: `2006`
- Month: `01` or `Jan`
- Day: `02`
- Hour (24h): `15`
- Hour (12h): `3`
- Minute: `04`
- Second: `05`
- AM/PM: `PM`

## NO_COLOR

Chatuino respects the `NO_COLOR` environment variable and will not render colors if enabled.

```sh
NO_COLOR=1 chatuino
```

## Key Bindings

Key bindings are configurable in the `~/.config/chatuino/keymap.yaml` file (the config directory may differ depending on your OS).

Press `?` inside Chatuino to view an overview of available key bindings.

### Defaults

Bindings are additive: every action accepts all keys listed for it.

| Action | Default keys |
| ------ | ------------ |
| Up / Down | `up`/`k`, `down`/`j` |
| Escape / Confirm | `esc`, `enter` |
| Help | `?`, `f1` |
| Quit | `ctrl+c`, `q` (normal mode only) |
| New tab | `ctrl+t` |
| Close tab | `ctrl+q`, `ctrl+w`, `alt+w` |
| Next tab | `tab`, `]`, `ctrl+n`, `alt+l` |
| Previous tab | `shift+tab`, `[`, `alt+h` |
| Quick join | `ctrl+j` |
| Insert mode | `i`, `I`, `enter` |
| User inspect | `ctrl+l`, `u` |
| Chat popup | `ctrl+p`, `alt+p` |
| Channel popup | `ctrl+b`, `alt+b` |
| Go to top | `t`, `g`, `home` |
| Go to bottom | `b`, `G`, `end` |
| Page up / down | `pgup`/`ctrl+u`, `pgdown`/`ctrl+d` |
| Search | `/`, `ctrl+f` |
| Accept suggestion | `tab`, `ctrl+space`, `→` (join modal also `space`) |
| Next / previous suggestion | `down`/`ctrl+n`, `up`/`ctrl+p` |
| Copy message to input | `alt+c`, `y` |
| Copy message to clipboard | `Y`, `alt+y` |
| Reply | `alt+r`, `r` |
| Quick timeout | `alt+t` |
| Send and keep typing | `alt+enter`, `ctrl+enter` |
| Dump screen / chat | `ctrl+alt+d`, `ctrl+alt+c` |

Plain-letter bindings (`q`, `g`, `G`, `r`, `u`, `y`, `Y`) only apply outside text input, chat search, and insert mode.

Clipboard copies use the terminal's OSC 52 sequence, so they also work over SSH. Some terminals or multiplexers (for example tmux) must allow OSC 52 before the clipboard is actually updated.

> **Upgrading:** Chatuino never rewrites an existing `keymap.yaml`. Existing actions keep exactly the keys listed in your file; newly introduced actions that are absent use their defaults. To pick up new aliases for an existing action, edit that action or delete/rename `keymap.yaml` to regenerate the defaults. Explicitly empty lists disable an action.

### Pane Controls

Press **F2** to enter pane mode, then use the controls below. A standalone tab becomes the first pane of its own workspace; pressing F2 on a tab already in a workspace enters that workspace's pane mode. Ctrl+T inside pane mode adds a tab to the current workspace; outside pane mode it creates a standalone tab. The status line shows `-- Pane --` with themed, width-aware hints for focus, moves, splits, creating a pane, resize, zoom, cycling, and exit; printable commands are consumed by pane mode rather than typed into chat. **Escape** or **F2** exits to the focused tab's previous interaction mode. Normal chat/suggestion/tab-navigation controls retain their existing behavior outside pane mode.

A move with a directional neighbour swaps the two pane contents without changing geometry. At an outer edge it detaches the pane and re-docks it as a new 50/50 split, so **H/J/K/L** can change a row layout into a stacked one, or the reverse. A move is refused if the result cannot satisfy the pane minima; the layout is left unchanged.

Outside pane mode, **Tab/Shift+Tab** cycle the displayed tab list in header order without changing pane assignments: assigned tabs focus their pane and automatically show the workspace that owns them; unassigned tabs appear full-screen while the layout stays preserved. In pane mode, Tab/Shift+Tab cycle panes instead. While composing, Tab is handled as suggestion completion; while searching, it stays in search rather than switching tabs.

| YAML action | Default keys | Behavior |
| --- | --- | --- |
| `pane_mode` | `f2` | Enter/leave pane mode |
| `pane_focus_left` | `left`, `h` | Focus left |
| `pane_focus_down` | `down`, `j` | Focus down |
| `pane_focus_up` | `up`, `k` | Focus up |
| `pane_focus_right` | `right`, `l` | Focus right |
| `pane_swap_left` | `H` | Move left; swaps with a neighbour or re-docks to the left edge |
| `pane_swap_down` | `J` | Move down; swaps with a neighbour or re-docks to the bottom edge |
| `pane_swap_up` | `K` | Move up; swaps with a neighbour or re-docks to the top edge |
| `pane_swap_right` | `L` | Move right; swaps with a neighbour or re-docks to the right edge |
| `pane_split_columns` | `v` | Choose a tab for a new right-hand pane |
| `pane_split_rows` | `s` | Choose a tab for a new bottom pane |
| `pane_narrow` / `pane_widen` | `<` / `>` | Shrink/grow width |
| `pane_shorten` / `pane_taller` | `-` / `+` | Shrink/grow height |
| `pane_close` | `x` | Hide the pane; keep its tab open |
| `pane_zoom` | `z` | Toggle focused-pane zoom |
| `pane_next` / `pane_previous` | `tab` / `shift+tab` | Cycle panes in screen-reading order |

All bindings use the existing `keymap.yaml` format, for example:

```yaml
pane_mode: [f2]
pane_split_columns: [v]
pane_split_rows: [s]
pane_close: [x]
pane_focus_left: [left, h]
```

The existing `escape` action also leaves pane mode. Pane mode remains active after layout commands until explicitly dismissed. **Ctrl+C** still quits; **x** is not a replacement for the existing close-tab action. For lifecycle, minimum sizes, and restoration behavior, see [Tiled Workspace](FEATURES.md#tiled-workspace).

## Mouse

Mouse support is enabled by default. It can be disabled with `mouse: false` in `settings.yaml`.

| Interaction | Action |
| ----------- | ------ |
| Left-click a tab | Switch to that tab |
| Left-click a chat message | Select it (for copy, reply, timeout) |
| Double-click a chat message | Inspect the message author |
| Left-click the message input | Start typing |
| Wheel over chat | Scroll the chat |
| Wheel over the tab list (vertical mode) | Switch to the previous/next tab |
| Wheel over the help screen | Scroll the help |

Mouse capture takes over click and drag in the terminal. Hold `shift` while dragging to use the terminal's native text selection.

## Custom Commands

The settings allow you to configure custom commands which will be suggested to you during text input.

```yaml
custom_commands:
  - trigger: "/ocean"
    replacement: "OCEAN MAN 🌊 😍 Take me by the hand ✋ lead me to the land that you understand 🙌 🌊 OCEAN MAN 🌊 😍 The voyage 🚲 to the corner of the 🌎 globe is a real trip 👌 🌊 OCEAN MAN 🌊 😍 The crust of a tan man 👳 imbibed by the sand 👍 Soaking up the 💦 thirst of the land 💯"
```

You can also create templated dynamic commands:

```yaml
custom_commands:
  - trigger: "/with-template"
    replacement: "CurrentTime: {{ .CurrentTime }}; CurrentDateTime: {{ .CurrentDateTime }}; BroadcastID: {{ .BroadcastID }}; BroadcastName: {{ .BroadcastName }}; SelectedDisplayName: {{ .SelectedDisplayName }}; SelectedUserID: {{ .SelectedUserID }}; Message: {{ .SelectedMessageContent }} "
    # You can even create custom mod commands/shortcuts
  - trigger: "/custom-spam-timeout"
    replacement: "/timeout {{ .SelectedDisplayName }} 10 Please stop spamming."
```

All features of [Go's templating engine](https://pkg.go.dev/text/template) are available.

Available template data:

| Name | Situation | Description |
| ---- | --------- | ----------- |
| CurrentTime | Any | The current local time |
| CurrentDateTime | Any | The current local date time |
| BroadcastID | Any | The broadcaster ID |
| BroadcastName | Any | The broadcaster name |
| SelectedDisplayName | Chat Message Selected | The senders display name |
| SelectedUserID | Chat Message Selected | The senders user ID |
| SelectedMessageContent | Chat Message Selected | The senders message |
| SubMessageCumulativeMonths | Sub Alert Message Selected | Chatters subbed months (cumulative) |
| SubMessageStreakMonths | Sub Alert Message Selected | Chatters subbed streak |
| SubMessageSubPlan | Sub Alert Message Selected | Chatters sub tier |
| SubGiftReceiptDisplayName | Sub Gift Message Selected | Gift receipt display name |
| SubGiftRecipientID | Sub Gift Message Selected | Gift receipt user id |
| SubGiftMonths | Sub Gift Message Selected | Gift months |
| SubGiftSubPlan | Sub Gift Message Selected | Gift sub tier |
| SubGiftGiftMonths | Sub Gift Message Selected | Gift months |
| RawMessage | Any | The complete internal message struct |
| MessageType | Any | Type of message (PrivateMessage, SubMessage, SubGiftMessage) |

The data is inserted when you accept the suggestion (default: Tab / Ctrl+Space / →), allowing you to review the text before sending.

You can use templating at any time when composing a message. The data is inserted when you accept the suggestion.

> **Note**: Templates are never rendered when you press Enter. Always review your input before sending.

## Emote Support

### Text Emotes

When `graphic_emotes` is disabled, Chatuino displays emotes as colored text. The color depends on the emote provider:

| Color | Provider |
| ----- | -------- |
| <span style="color:#0aa6ec">Blue</span> | SevenTV |
| <span style="color:#a35df2">Purple</span> | Twitch |
| <span style="color:#d50014">Red</span> | BetterTTV |
| <span style="color:#a3be8c">Green</span> | FFZ |

### Graphic Emotes

Chatuino can display images and animated images as Twitch emotes using the Kitty Graphics Protocol. This protocol is implemented by Kitty and some other terminals. However, it uses the [Unicode placeholder method](https://sw.kovidgoyal.net/kitty/graphics-protocol/#unicode-placeholders), which is currently only fully implemented by Kitty. It also works with Ghostty, but animated emotes display as static images.

Currently, this feature is **only** available in Kitty and Ghostty terminals on Unix platforms. This may change in the future.

#### Image Size

By default, graphic emotes and badges are one terminal row high. Set `graphic_scale` (1-4) to render them multiple rows high. Lines containing images then take up `graphic_scale` rows, with text and images aligned to the bottom row. Other lines are unaffected. Has no effect when both `graphic_emotes` and `graphic_badges` are disabled. With a scale above 1, the inline emote preview in the message input is disabled, since the input can only display a single row.

#### Format Support and Caching

Chatuino is statically compiled without dynamic library dependencies, allowing it to run on any system without additional requirements. Emote format support prioritizes native Go decoding for performance and stability.

Chatuino prefers formats that can be decoded natively in Go:
- **PNG** (static images) - Native Go support
- **GIF** (animated images) - Native Go support
- **WebP** (static images) - Native Go support
- **AVIF** and **animated WebP** - Decoded via WASM (libavif/libwebp) when necessary

The WASM-based decoders may consume more memory but are only used as a fallback. Chatuino caches all decoded images, so each emote is decoded only once per session.

Emotes are cached in the `~/.local/share/chatuino/emote` directory using the Kitty image transmission format, compressed with RFC 1950 ZLIB deflate compression.

Query the current cache size:

```sh
chatuino cache
```

Delete cached data:

```sh
chatuino cache clear --emotes --database --badges
```
