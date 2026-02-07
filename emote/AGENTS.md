# EMOTE SYSTEM

**OVERVIEW**: Concurrent emote fetching (Twitch/7TV/BTTV/FFZ), caching, replacement, Kitty graphics integration.

## EMOTE LIFECYCLE

1. **Fetch**: `Cache.RefreshGlobal()`/`RefreshLocal(channelID)` → parallel errgroup fetches from 4 providers
2. **Store**: Deduplicated in-memory `EmoteSet` per channel, global, user (subs), foreignEmotes (cross-channel)
3. **Replace**: `Replacer.Replace()` → split message words, lookup emote, generate Kitty display unit or colored text
4. **Display**: Graphics mode → Kitty protocol (kittyimg), fallback → lipgloss colored text (platform-specific)
5. **Foreign emotes**: IRC `emotes` tag → lazy load via `LoadSetForeignEmote()` (Twitch sub emotes from other channels)

## WHERE TO LOOK

| Task | Location | Notes |
|------|----------|-------|
| **Fetching** | `cache.go:82-230` | Singleflight dedup, errgroup parallel (Twitch/7TV/BTTV/FFZ), 404 tolerance |
| **Storage** | `cache.go:37-71` | RWMutex, global/channel/user/foreignEmotes maps |
| **Lookup** | `cache.go:361-413` | Priority: global → user → channel → foreign |
| **Replacement** | `replacer.go:56-122` | Word split, IRC tag parsing, graphics/colored fallback |
| **Display units** | `replacer.go:104-111` | `kittyimg.DisplayUnit` creation, lazy HTTP fetch |
| **Colored fallback** | `replacer.go:142-155` | lipgloss styles per platform (7TV/TTV/BTTV/FFZ) |
| **Foreign emotes** | `cache.go:446-470` | Fake entry from emoteID, Twitch CDN URL |
| **Platform enum** | `emote.go:5-26` | Twitch(1), SevenTV(2), BTTV(3), FFZ(4) |
| **EmoteSet** | `emote.go:40-50` | Slice with `GetByText()` lookup |

## CONVENTIONS

### Emote platforms
- **Twitch**: `static-cdn.jtvnw.net/emoticons/v2/{id}/default/light/1.0`
- **7TV**: `{host.url}/1x.avif` (animated) or `1x.webp` (static)
- **BTTV**: `cdn.betterttv.net/emote/{id}/1x`
- **FFZ**: `cdn.frankerfacez.com/emote/{id}/1` (always static PNG, integer IDs)

### Lookup priority
1. Global emotes (all platforms)
2. User-specific emotes (Twitch subs)
3. Channel emotes (Twitch/7TV/BTTV/FFZ)
4. Foreign emotes (cross-channel Twitch subs)

### Replacement algorithm
- Split message by spaces
- Per word: check IRC `emotes` tag → store lookup → graphics/colored fallback
- IRC `emotes` tag: `\x01ACTION ` prefix stripped, rune-indexed positions
- Kitty graphics: `PrepareCommand` + `ReplacementText` (display unit)
- Colored fallback: lipgloss style per platform (theme-based colors)

### Caching
- **Singleflight**: Deduplicates concurrent fetches (`channel+{id}`, `global`)
- **Fetch-once**: `globalFetched`, `channelsFetched[id]` guards
- **Thread-safe**: RWMutex for all reads/writes
- **404 tolerance**: 7TV/BTTV/FFZ 404 → skip, don't fail entire fetch

### Error handling
- **Twitch fetch fail**: Propagate error (required)
- **7TV/BTTV/FFZ fail**: Log + return `nil` (optional providers)
- **HTTP status != 200**: Return error from `fetchEmote()`
- **Graphics disabled**: Skip Kitty, use colored text

### Display units
- **Directory**: `emote`
- **ID**: `{platform}.{emoteID}` (lowercase)
- **Load func**: HTTP fetch on-demand (lazy)
- **Animated**: `IsAnimated` flag (7TV AVIF, BTTV `imageType`)

## ANTI-PATTERNS

### Emote fetching
- **NEVER** fetch channel emotes before global (waste parallel opportunity)
- **NEVER** fail entire fetch on 7TV/BTTV/FFZ 404 (graceful degradation)
- **NEVER** duplicate fetches (singleflight required)

### Replacement
- **NEVER** assume ASCII (use runes for IRC tag positions)
- **NEVER** skip `\x01ACTION ` prefix stripping (breaks indices)
- **NEVER** block on graphics conversion (fallback to colored)
