# CHATUINO KNOWLEDGE BASE

**Generated:** 2026-01-15  
**Commit:** a6b16ac  
**Branch:** main

## OVERVIEW
Terminal UI Twitch IRC client (Go) with EventSub, multiple emote/badge providers, SQLite logging, Bubble Tea UI.

## STRUCTURE
```
chatuino/
├── main.go              # CLI entry (urfave/cli/v3: account, server, cache cmds)
├── twitch/              # See twitch/AGENTS.md - IRC/API/EventSub/emote providers
├── ui/                  # See ui/AGENTS.md - Bubble Tea architecture
├── save/                # See save/AGENTS.md - Persistence (JSON/YAML/SQLite/keyring)
├── emote/               # See emote/AGENTS.md - Emote fetching, caching, replacement
├── badge/               # Badge fetching, caching (Twitch API), lipgloss rendering
├── server/              # HTTP server for accounts, emotes, badges (optional)
├── multiplex/           # IRC/EventSub connection pooling, message routing
├── kittyimg/            # Kitty terminal graphics protocol (emote display)
├── httputil/            # HTTP utilities (RoundTripperFunc, debug logging)
├── mocks/               # Generated mockery mocks (TwitchEmoteFetcher, EmoteStore, etc.)
└── doc/                 # Screenshots, settings docs
```

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| **IRC connection** | `twitch/twitchirc/conn.go` | WebSocket, auto-reconnect (5s), PONG handling |
| **IRC parsing** | `twitch/twitchirc/parser.go` | IRCv3 tags, Twitch dialect, discriminated unions |
| **EventSub** | `twitch/eventsub/conn.go` | WebSocket, session lifecycle, forced reconnects |
| **API integration** | `twitch/twitchapi/api.go` | Token refresh, rate limits (429), singleflight |
| **Main UI** | `ui/mainui/root.go` | Bubble Tea orchestrator, tab management |
| **Chat rendering** | `ui/mainui/chat.go` | Viewport, search, entry→line mapping, pruning |
| **Tab types** | `ui/mainui/*_tab.go` | broadcast/mention/live notification tabs |
| **Emote system** | `emote/replacer.go` | Concurrent fetching, caching, display unit creation |
| **Persistence** | `save/app.go`, `save/settings.go` | JSON state, YAML configs, keyring tokens |
| **Message logging** | `save/messagelog/logger.go` | SQLite WAL, batch insert (20 items/5s) |
| **Connection pools** | `multiplex/pool.go` | IRC/EventSub pools with routing |
| **CLI commands** | `command/*.go`, `main.go` | account, server, cache subcommands |

## CONVENTIONS

### Deviations from Standard Go
- **Root-level `.go` files**: `account.go`, `cache.go`, `server.go`, `version.go`, `termsize_*.go` at root (standard: packages)
- **Nested UI**: `ui/mainui/`, `ui/accountui/` (standard: flatter)
- **PascalCase mocks**: `mocks/TwitchEmoteFetcher.go` (generated, not manually controlled)

### Project Standards
- **DI via containers**: `ui/mainui/dependencies.go` - inject all external deps
- **Optional pointer fields**: `*bool`, `*int` for "present vs value" (e.g., `RoomState.EmoteOnly`)
- **IRCer interface**: `IRC() string` for bidirectional IRC message handling
- **easyjson**: Performance-critical types (API responses, IRC messages)
- **Context propagation**: All network ops accept `context.Context`
- **Discriminated unions**: `UserNotice.MsgID` → typed messages (`*SubMessage`, `*RaidMessage`)

### Release & Build
- goreleaser: Linux/Windows/Darwin (amd64/arm64), CGO disabled, static builds
- Docker: `ghcr.io/julez-dev/chatuino`
- AUR: `chatuino-bin`
- Changelog groups: `feature:`, `fix:`, excludes `doc:`, `test:`, `chore:`

## ANTI-PATTERNS (THIS PROJECT)

### Generated Code
- **NEVER edit** `*_easyjson.go`, `mocks/*.go` - regenerate via `go generate`, `mockery`

### Business Logic
- **NEVER ignore** messages from: user, broadcaster, subs, mods, VIPs, paid, staff, bits, mentions (ui/mainui/broadcast_tab.go:1083)
- Commands **NEVER** use `@` prefix, unlike users (ui/component/textinput.go:367)
- Templates **NEVER** rendered on Enter key (doc/SETTINGS.md:98)

### Twitch Integration
- EventSub subscriptions **MUST** be sent within 10s or Twitch closes connection (twitch/eventsub/conn.go:73,90)

### Code Quality (from existing AGENTS.md)
- **NEVER**: `any`, non-null assertion `!`, type assertions `as Type`
- **NEVER**: global state, DI frameworks
- **NEVER**: `pkg` package
- **AVOID**: `utils`, `common`, `helpers`, `misc` names

## COMMANDS

```bash
# Dev
go run . account list                # List accounts
go run . server --port 8080          # HTTP server mode
go run .                             # Run TUI (default)

# Test
go test --race -v ./...              # Tests with race detector
go fmt ./...                         # Format
go vet ./...                         # Vet
staticcheck ./...                    # Lint

# Build
goreleaser build --snapshot --clean  # Local build (all platforms)
goreleaser release --clean           # Release (requires tag)

# Generate
mockery                              # Regenerate mocks (.mockery.yaml)
go generate ./...                    # easyjson generation
```

## CI/CD NOTES

### Security Issue
- `release.yml` writes SSH keys to disk (`~/.ssh/id_rsa`) - should use ssh-agent

### Improvements Needed
- `lint_test.yml`: Custom bash function (`assert-nothing-changed`) should use GitHub Action
- Daily CodeQL (wasteful) - should trigger on push/PR only
- No dependency caching (Go modules) - slows CI
- No test coverage reporting
- Race detector slows tests (no timeout)

## TESTING

- **Subtests with parallel** (`t.Parallel()` at top-level typically)
- **Table-driven** tests common
- **Mockery** for interfaces (TwitchEmoteFetcher, EmoteStore, etc.)
- **sqlmock** for database tests
- **Fuzzing**: `Fuzz_ParseIRC` (parser)
- **Testdata**: `emote/testdata/pepeLaugh.webp`, `twitchirc/testdata/messages.txt`
- **Require not assert** (fail immediately, no `assert`)

## NOTES

### IRC Quirks
- Auto-rejoin on reconnect (mutex-protected channel list)
- Ignores PART, JOIN, `tmi.twitch.tv` server notices
- Tag decoding: `\:` → `;`, `\s` → ` `, `\\` → `\`, `\r` → `\r`, `\n` → `\n`

### EventSub Lifecycle
1. Wait for subscription request
2. Dial WS → receive `session_welcome` → get `session_id`
3. Create subscription with `session_id`
4. Handle `session_reconnect` → new URL, forced reconnect
5. Duplicate filtering via TTL cache (15min)

### API Patterns
- 401 + "Invalid OAuth token" → singleflight token refresh
- 429 → wait until `Ratelimit-Reset` header time
- Pagination: manual loop with `after` cursor

### UI State Machine
- **broadcastTab states**: inChatWindow, insertMode, userInspectMode, userInspectInsertMode, emoteOverviewMode
- **chatWindow states**: viewChatWindowState, searchChatWindowState
- **Root screens**: mainScreen, inputScreen, helpScreen

### Performance
- **Viewport limiting**: Only render visible lines
- **Entry cleanup**: Prune at 1200 messages
- **Color cache**: Reuse lipgloss render functions
- **Image cleanup**: TTL-based emote/badge deletion
- **Singleflight**: Deduplicates token refreshes, badge fetches

### Persistence
- **Config dir**: `$XDG_CONFIG_DIR/chatuino/`
- **Keyring**: System keyring (fallback: plaintext `accounts.json`)
- **SQLite**: WAL mode, batch insert (20 items or 5s)
- **Defaults merge**: YAML configs merge with user overrides (not replace)

## ARCHITECTURE PRINCIPLES

1. **Interface segregation**: `IRCer`, `AccountProvider`, `TokenRefresher`, `tab`
2. **No global state**: All deps injected via constructors/DI containers
3. **Typed errors**: `APIError`, `RetryReachedError`, `twitchForcedReconnect`
4. **Graceful degradation**: Skip unhandled IRC commands, unparsable messages
5. **Resource cleanup**: Explicit channel closing, cache pruning, context cancellation
6. **Unidirectional data flow**: External events → channels → messages → state updates → view
