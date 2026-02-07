# TWITCH INTEGRATION

**Parent:** /home/julez/code/chatuino  
**Score:** 12 (parent of complex modules)

## OVERVIEW
Integration layer for Twitch + third-party services: IRC, API, EventSub, 7TV, BTTV, FFZ, IVR, recent messages.

## STRUCTURE
```
twitch/
├── twitchirc/         # IRC WebSocket: parser, conn, message types (IRCv3 tags)
├── twitchapi/         # Helix API: token refresh, singleflight, 429 handling
├── eventsub/          # EventSub WebSocket: lifecycle, duplicate filtering
├── seventv/           # 7TV emote API (global + channel)
├── bttv/              # BTTV emote API (global + channel)
├── ffz/               # FFZ emote API (global + channel)
├── ivr/               # IVR: subage, mod/VIP lists
└── recentmessage/     # Recent messages API (robotty.de)
```

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| **IRC connection** | `twitchirc/chat.go` | WebSocket, auto-reconnect (5s), rejoin channels |
| **IRC parsing** | `twitchirc/parser.go` | IRCv3 tags, discriminated unions, tag decoding (`\:` → `;`) |
| **IRC message types** | `twitchirc/message_types.go` | PRIVMSG, USERNOTICE, ROOMSTATE, CLEARMSG, etc. |
| **EventSub lifecycle** | `eventsub/conn.go` | Wait for sub request, dial, `session_welcome`, forced reconnect |
| **Duplicate filtering** | `eventsub/conn.go:39-41,199` | TTL cache (15min), messageID dedup |
| **Token refresh** | `twitchapi/api.go:793-843` | Singleflight, 401 + "Invalid OAuth token" → refresh |
| **Rate limiting** | `twitchapi/api.go:878-909` | 429 → wait until `Ratelimit-Reset` header |
| **EventSub subscriptions** | `twitchapi/api.go:536-605` | Create, fetch, delete subscriptions |
| **7TV emotes** | `seventv/api.go` | `/users/twitch/{id}`, `/emote-sets/global` |
| **BTTV emotes** | `bttv/api.go` | `/cached/users/twitch/{id}`, `/cached/emotes/global` |
| **FFZ emotes** | `ffz/api.go` | `/room/id/{twitch_id}`, `/set/global` (nested sets flattened) |
| **IVR API** | `ivr/ivr.go` | Subage (`/twitch/subage/{user}/{channel}`), mod/VIP lists |
| **Recent messages** | `recentmessage/api.go` | Fetch last 100 messages, parse via `twitchirc.ParseIRC` |

## CONVENTIONS

### IRC (twitchirc/)
- **IRCer interface**: `IRC() string` for bidirectional message handling
- **Tag decoding**: `\:` → `;`, `\s` → ` `, `\\` → `\`, `\r` → `\r`, `\n` → `\n` (parser.go:63-69)
- **Auto-rejoin**: Mutex-protected channel list, rejoin on reconnect (chat.go:192-196)
- **PONG handling**: Internal channel for reader→writer messages (chat.go:98,159-164)
- **Ignores**: PART, JOIN, `tmi.twitch.tv` server notices (chat.go:147-154)
- **Retry**: Infinite retries with 5s fixed delay, `RetryReachedError` wrapper (chat.go:249-253)

### Twitch API (twitchapi/)
- **Functional options**: `WithHTTPClient`, `WithClientSecret`, `WithUserAuthentication` (api.go:44-67)
- **Dual auth**: App token (clientSecret) OR user token (AccountProvider) (api.go:135-139, 662-666)
- **Singleflight**: Token refresh, badge fetches, chat colors (api.go:77-79, 130, 162, 808-826)
- **Pagination**: Manual loop with `after` cursor (api.go:227-253, 353-379)
- **401 handling**: "Invalid OAuth token" OR "OAuth token is missing" → singleflight refresh (api.go:805-838)
- **429 handling**: Wait until `Ratelimit-Reset` (Unix timestamp), retry same request (api.go:881-908)
- **EventSub cost limit**: Special error for `/eventsub/subscriptions` (api.go:882-884)

### EventSub (eventsub/)
- **10s subscription window**: Must send subscription within 10s or Twitch closes (conn.go:73,90)
- **Lifecycle**: Dial → `session_welcome` → `session_id` → create sub → `session_reconnect` → new URL (conn.go:168-193)
- **Forced reconnect**: `twitchForcedReconnect` typed error with new URL (conn.go:63-69, 185-193)
- **Duplicate filtering**: TTL cache (15min), messageID key (conn.go:39-41, 199-203)
- **Inbound channel**: Blocking receive before dial (conn.go:91), goroutine listens after `session_welcome` (conn.go:183-184)

### Third-party APIs (seventv/, bttv/, ffz/, ivr/, recentmessage/)
- **Shared pattern**: `doRequest[T]` generic helper, `APIError` type
- **No auth**: All use unauthenticated GET requests
- **Error handling**: Status code + JSON unmarshal fallback (seventv/api.go:68-77, bttv/api.go:68-79)
- **IVR simplicity**: No typed errors, just `fmt.Errorf` (ivr/ivr.go:69-71)
- **Recent messages**: easyjson for performance, hides moderation/moderated messages (recentmessage/api.go:42-44)

## ANTI-PATTERNS

### IRC
- **NEVER edit** `twitchirc_easyjson.go` - regenerate via `go generate`
- **NEVER assume** single message per WebSocket read - split by `\r\n` (chat.go:138)

### EventSub
- **NEVER dial** before receiving first inbound message (conn.go:91-94)
- **NEVER ignore** `session_reconnect` - new session URL required (conn.go:185-193)

### API
- **NEVER retry** without checking `Ratelimit-Reset` on 429 (api.go:881)
- **NEVER refresh** without singleflight - causes token invalidation (api.go:808-826)
