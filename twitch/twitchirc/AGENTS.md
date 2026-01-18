# TWITCHIRC

## OVERVIEW
IRC WebSocket client for Twitch chat - parsing IRCv3 tags, auto-reconnect (5s), PONG handling, discriminated unions.

## WHERE TO LOOK

| Task | File | Notes |
|------|------|-------|
| **WebSocket connection** | `chat.go:68` | Dial, auth (PASS/NICK/CAP), 5s retry, 10s ping |
| **Auto-rejoin** | `chat.go:192-196` | Mutex-protected channel list, re-JOIN on reconnect |
| **IRC parsing** | `parser.go:71` | Tags (@), prefix (:), command, params, trailing |
| **Tag decoding** | `parser.go:567-597` | `\:` → `;`, `\s` → ` `, `\\` → `\`, `\r`, `\n` |
| **Message dispatch** | `parser.go:139-461` | Switch on Command → typed structs |
| **PRIVMSG** | `parser.go:140-200` | Badges, emotes, bits, hype chat, replies, thread |
| **USERNOTICE** | `parser.go:215-351` | Sub/resub/gift/raid/announcement - discriminated by `MsgID` |
| **ROOMSTATE** | `parser.go:385-421` | Optional pointers (`*bool`, `*int`) - delta updates |
| **CLEARCHAT/CLEARMSG** | `parser.go:422-457` | Timeouts, bans, message deletions |
| **Emote parsing** | `parser.go:475-517` | Format: `79382:20-24,40-44/...` → `[]Emote` |
| **Badge parsing** | `parser.go:519-538` | Format: `subscriber/18,no_audio/1` → `[]Badge` |
| **IRCer interface** | `message_types.go:95-101` | Bidirectional: parse incoming, generate outgoing |

## MESSAGE TYPES

### Core Messages
- **PrivateMessage**: Chat message (easyjson for perf) - badges, bits, emotes, hype chat, replies, source-* (shared chat)
- **UserNotice**: Base for sub/gift/raid/announcement - discriminated by `MsgID` field
- **SubMessage**, **SubGiftMessage**, **RaidMessage**, **AnnouncementMessage**: Typed variants with msg-param-* fields
- **RoomState**: Delta updates only (optional pointers) - emote-only, slow, followers-only, r9k
- **ClearChat**: Timeout/ban - `BanDuration` nil = permaban
- **ClearMessage**: Single message deletion

### Control
- **PingMessage**/**PongMessage**: Keep-alive
- **JoinMessage**/**PartMessage**: Channel join/leave
- **UserState**: Self state after JOIN
- **Whisper**: DM (deprecated by Twitch)
- **Notice**: System messages (msg-id)

## ANTI-PATTERNS

### Parser
- **NEVER parse** PART, JOIN, `tmi.twitch.tv` notices - ignored by design (chat.go:147-151)
- **NEVER assume** tag values exist - use `string(c.tags["key"])` → empty string, or `c.tags["key"] == "1"` → bool
- **NEVER return error** for `ErrUnhandledCommand` - gracefully skip unknown commands (parser.go:460)
- **NEVER split** without checking length - always validate `len(parts)` after split

### Connection
- **NEVER block** on channel sends in reader - use `select` with `innerCtx.Done()` (chat.go:160-164)
- **NEVER forget** PONG - reader must send to `innerMessages` (chat.go:159-164)
- **NEVER rejoin** explicitly - auto-rejoin on reconnect via stored channel list (chat.go:192-196)
- **NEVER prefix** oauth token twice - check `HasPrefix("oauth:")` (chat.go:181-183)

### State
- **NEVER assume** RoomState is complete - only changed fields set (optional pointers) (message_types.go:352-360)
- **NEVER compare** `FollowersOnly == 0` - pointer nil means unchanged, `*ptr == -1` = disabled, `>= 0` = duration
- **NEVER store** prefix name/user/host - ephemeral, use DisplayName/LoginName from tags

### Testing
- testdata/messages.txt contains real Twitch IRC samples for fuzzing/parsing validation
