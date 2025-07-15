# Settings

Chatuino can be run without creating any settings file but you may want to configure some of Chatuinos default behavior.

Your settings file is read from ~/.config/chatuino/settings.yaml (config directory may be different depending on OS). You may want to create a new settings file if it doesn't exists already.

```yaml
vertical_tab_list: false # if Chatuino should display tabs vertically instead of horizontally
moderation:
  store_chat_logs: true # if Chatuino should store chat logs in a sqlite database; Default: false

  # NOTE: logs_channel_include and logs_channel_exclude are mutually exclusive.
  logs_channel_include: ["lirik", "sodapoppin"] # Chatuino will only log channels that are in this list, if set
  logs_channel_exclude: ["lec"] # Chatuino will not log channels that are in this list, but all others, if set

chat:
  # NOTE: read the README for more information about how emote rendering works before enabling this feature
  graphic_emotes: true # EXPERIMENTAL: if Chatuino should display emotes as images instead of text; Default: false

custom_commands:
  # Custom commands are available as command suggestions
  - trigger: "/ocean"
    replacement: "OCEAN MAN 🌊 😍 Take me by the hand ✋ lead me to the land that you understand 🙌 🌊 OCEAN MAN 🌊 😍 The voyage 🚲 to the corner of the 🌎 globe is a real trip 👌 🌊 OCEAN MAN 🌊 😍 The crust of a tan man 👳 imbibed by the sand 👍 Soaking up the 💦 thirst of the land 💯"
```

## NO_COLOR

Chatuino respects the `NO_COLOR` environment variable and will not render colors if enabled.

```sh
NO_COLOR=1 chatuino
```

## key-binds

Key-binds are configurable in the ~/.config/chatuino/keymap.yaml file (config directory may be different depending on OS)

An overview of the keybinds can be opened with `?` inside Chatuino.

## Custom Commands

The settings allow you to configure custom commands which will be suggested to you during text input.

```yaml
custom_commands:
  - trigger: "/ocean"
    replacement: "OCEAN MAN 🌊 😍 Take me by the hand ✋ lead me to the land that you understand 🙌 🌊 OCEAN MAN 🌊 😍 The voyage 🚲 to the corner of the 🌎 globe is a real trip 👌 🌊 OCEAN MAN 🌊 😍 The crust of a tan man 👳 imbibed by the sand 👍 Soaking up the 💦 thirst of the land 💯"
```

Additionally you can also template dynamic commands like this

```yaml
custom_commands:
  - trigger: "/with-template"
    replacement: "CurrentTime: {{ .CurrentTime }}; CurrentDateTime: {{ .CurrentDateTime }}; BroadcastID: {{ .BroadcastID }}; BroadcastName: {{ .BroadcastName }}; SelectedDisplayName: {{ .SelectedDisplayName }}; SelectedUserID: {{ .SelectedUserID }}; Message: {{ .SelectedMessageContent }} "
    # You can even create custom mod commands/shortcuts
  - trigger: "/custom-spam-timeout"
    replacement: "/timeout {{ .SelectedDisplayName }} 10 Please stop spamming."
```

All features of [Go's templating engine](https://pkg.go.dev/text/template) are available to you.

Here is an overview which data is available inside templates

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

The data is inserted on the accept suggestion keybind (default tab) press so the text can be viewed before sending the message.

You can use the templating feature at any time when composing a message, the data will be inserted once the accept suggestion keybind is pressed.

> **Note**: Templates are never rendered on enter pressed. Be sure to review your input before pressing send.

## A word about Chatuinos emote support

### Text emotes

When the graphic_emotes setting is disabled, Chatuino will display emotes as colored text. The color of the emote depends on the emote provider.

| Color | Provider |
| ----- | -------- |
| <span style="color:#0aa6ec">Blue</span> | SevenTV |
| <span style="color:#a35df2">Purple</span> | Twitch |
| <span style="color:#d50014">Red</span> | BetterTTV |

### Graphic emotes

Chatuino can display rendered images and animated images as twitch emotes using the Kitty Graphics Protocol. This protocol is implemented by the Kitty and some other terminals. **However** it uses the [Unicode placeholder method](https://sw.kovidgoyal.net/kitty/graphics-protocol/#unicode-placeholders) which as of right now is only implemented by Kitty.

Right now this feature is **only** available in Kitty terminals on Unix platforms. This may change in the future.

#### Drawbacks and workarounds

Chatuino is statically compiled without any dynamic library dependencies. This has some advantage but also a drawback when it comes to emote support. Not all formats used by emote providers are supported by native Go image modules. For example while .webp images can be decoded, animated .webp images can't be decoded.

Since animated .webps can't be decoded Chatuino tries to fallback to .avifs versions if possible.

There is a module to decode static and animated .avif files but it's not a native implementation and uses wazero to run a WASM build of libavif.

During development I noticed a high memory consumption (sometimes 10x over non graphic usage) by the avif decoder, which I could not resolve. Chatuino caches all decoded images so once an emote is decoded for the first it never needs to be decoded again.

The emotes are cached at the same location Chatuino will put the message log sqlite database at: In the `$HOME/chatuino` directory. The format used to store the image data is the same format Kitty requires to be used to transmit images, which means that it's not very space efficient.

But a lot of emotes are used on twitch so this process takes a long time. You can speed the caching up by pre loading all emotes for all chats you will most likely join in the future. For this you can use a sub command build inside Chatuino.

```sh
chatuino rebuild-cache --channels=channel1,channel2,...
```

This will pre decode all emotes for all the provided channels and all global emotes for SevenTV, BetterTTV and Twitch, drastically reducing memory consumption at the cost of disk space.

Some animated emotes seem to not be animated even if nothing is wrong with the protocol commands or cached files, this is still an open bug.
