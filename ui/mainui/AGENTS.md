# UI/MAINUI

**Complexity:** 24 (high) - Root orchestrator with state machines, viewport management, tab system

## OVERVIEW
Bubble Tea root model: tab management, IRC/EventSub routing, viewport rendering, state machines for broadcast/mention/live tabs.

## COMPONENTS

### Root (`root.go:103`)
- **Entry point**: `NewUI()` - initializes IRC/EventSub channels, header (horizontal/vertical), splash, help, joinInput
- **Lifecycle**: `Init()` loads persisted state, refreshes badges/emotes (15s ctx), fetches bulk user data, starts 4 goroutines (IRC wait, EventSub listen, stream poll 90s, image cleanup 1min)
- **Tab orchestration**: `tabs []tab`, `tabCursor int`, creates/closes tabs, routes messages to focused tab
- **Screens**: `mainScreen` (tabs), `inputScreen` (join dialog), `helpScreen`
- **Persistence**: `TakeStateSnapshot()` every 15s via `tickSaveAppState()`
- **IRC/EventSub routing**: `in chan multiplex.InboundMessage`, `eventSubIn chan multiplex.EventSubInboundMessage`
- **Cleanup**: `Close()` waits for `closerWG`, `eventSubInInFlight`, closes channels

### Tab Interface (`root.go:49`)
- **Methods**: `Init()`, `InitWithUserData()`, `Update()`, `View()`, `Focus()/Blur()`, `HandleResize()`, `SetSize()`
- **Metadata**: `AccountID()`, `Channel()`, `ChannelID()`, `ID()`, `Kind()`, `State()`, `IsDataLoaded()`, `Focused()`
- **Types**: `broadcastTabKind`, `mentionTabKind`, `liveNotificationTabKind` (enum `tabKind`)

### Broadcast Tab (`broadcast_tab.go:112`)
- **State machine**: `inChatWindow`, `insertMode`, `userInspectMode`, `userInspectInsertMode`, `emoteOverviewMode`
- **Init sequence**: `Init()` → fetch user → `InitWithUserData()` → fetch recent msgs (robotty.de), mod/VIP status → `setChannelDataMessage` → refresh emotes/badges → send `JoinMessage` → EventSub subscriptions (polls, raids, ads if own channel)
- **Components**: `chatWindow` (viewport), `messageInput` (SuggestionTextInput), `streamInfo`, `poll`, `statusInfo`, `userInspect`, `emoteOverview`, `spinner`
- **Message filtering**: `shouldIgnoreMessage()` - blocks per `BlockSettings`, `isLocalSub` (non-sub filter), `isUniqueOnlyChat` (fuzzy Levenshtein<3 dedup via TTL cache 10s)
- **Commands**: `/inspect`, `/pyramid`, `/localsubscribers[off]`, `/uniqueonly[off]`, `/createclip`, `/emotes`, mod cmds if `isUserMod`
- **Template replacement**: `replaceInputTemplate()` - Go templates with `CurrentTime`, `BroadcastName`, `SelectedDisplayName`, `MessageID`, etc.
- **Cleanup**: `close()` stops TTL cache, frees emoteOverview

### Chat Window (`chat.go:50`)
- **Viewport**: `lines []string`, `entries []*chatEntry`, `cursor int`, `lineStart/lineEnd` (visible range)
- **Entry→line mapping**: `chatEntry.Position{CursorStart, CursorEnd}` maps IRC message to line range (multi-line support)
- **States**: `viewChatWindowState`, `searchChatWindowState`
- **Cleanup**: At 1200 entries (`cleanupThreshold`), prune to 800 (`cleanupAfterMessage`), only when newest selected + not searching
- **Search**: `applySearch()` filters entries by fuzzy match on `DisplayName`/`Message`, `IsFiltered` flag hides from viewport
- **Rendering**: `messageToText()` → wordwrap with `indicatorWidth` + prefix padding → `recalculateLines()` rebuilds `lines` + recalcs `Position`
- **Color cache**: `userColorCache map[string]func(...string) string` - lipgloss render funcs per user, cleaned on pruning
- **Modifiers**: `messageContentModifier` - `wordReplacements` (emotes/badges/links), `strikethrough` (timeout/delete), `italic` (notices)
- **Timeout/delete**: `handleTimeoutMessage()`, `handleMessageDeletion()` set `IsDeleted`, `strikethrough`, trigger `recalculateLines()`

### Headers (`horizontal_tab_header.go`, `vertical_tab_header.go`)
- **Interface**: `AddTab()`, `RemoveTab()`, `SelectTab()`, `Resize()`, `MinWidth()`
- **Horizontal**: lipgloss tabs joined, 1 line height
- **Vertical**: stacked tabs, left sidebar, `MinWidth()` = longest tab name

### Other Tabs
- **Mention tab** (`mention_tab.go`): aggregates `PrivateMessage` with user display name across all accounts/channels
- **Live notification tab** (`live_notification_tab.go`): EventSub `stream.online` events for followed channels

## STATE MACHINES

### Broadcast Tab States
```
inChatWindow ──[InsertMode key]──> insertMode
            ──[InspectMode key]──> userInspectMode
insertMode ──[Escape]──> inChatWindow
userInspectMode ──[InsertMode key]──> userInspectInsertMode
               ──[Escape]──> inChatWindow
userInspectInsertMode ──[Escape]──> userInspectMode
emoteOverviewMode ──[Escape]──> inChatWindow
```

### Chat Window States
```
viewChatWindowState ──[SearchMode key]──> searchChatWindowState
searchChatWindowState ──[Escape]──> viewChatWindowState (clear filter)
                     ──[Confirm]──> viewChatWindowState (keep selection)
```

### Root Screens
```
mainScreen ──[Create key]──> inputScreen (join dialog)
          ──[Help key]──> helpScreen
inputScreen/helpScreen ──[Escape]──> mainScreen
```

## MESSAGE FLOW

1. **IRC → Root**: `out <-chan multiplex.OutboundMessage` from `ChatPool.ListenAndServe()` → `waitChatEvents()` loops → `buildChatEventMessage()` → `chatEventMessage` broadcasted to all tabs matching `accountID`/`channel`
2. **EventSub → Root**: `eventSubIn chan multiplex.EventSubInboundMessage` → `EventSubPool.ListenAndServe()` → tabs handle via `EventSubMessage`
3. **Local messages**: `requestLocalMessageHandleMessage` → `buildChatEventMessage()` with `isFakeEvent=true` (no new wait cmd)
4. **Tab → IRC**: `forwardChatMessage` wraps `multiplex.InboundMessage` → `in` chan → `ChatPool`
5. **User input**: `handleMessageSent()` → API `SendChatMessage` or command handler → fake `Notice` if error
6. **Routing**: `chatEventMessage.tabID` targets specific tab, else broadcast; tabs filter by `accountID`/`channel`

## ANTI-PATTERNS

### Message Filtering (`broadcast_tab.go:1083`)
**NEVER ignore**: user's own msgs, broadcaster, subs (if sub filter off), mods, VIPs, paid msgs, staff, bits, mentions
**Order matters**: Check exemptions before `isLocalSub`/`isUniqueOnlyChat` filters

### Viewport Management
**NEVER call** `recalculateLines()` without checking `state == searchChatWindowState` - causes search filter reset
**ALWAYS call** `updatePort()` after `recalculateLines()` to sync `lineStart/lineEnd` with `cursor`
**Entry cleanup**: Only prune when `getNewestEntry().Selected == true` (user at bottom) to avoid jarring jumps

### State Transitions
**NEVER skip** `Blur()`/`Focus()` on components when changing `broadcastTabState` - causes double focus (e.g., `messageInput` + `chatWindow`)
**Commands NEVER use** `@` prefix (vs. users in templates: `doc/SETTINGS.md:367`)

### Initialization
**IRC JOIN timing**: `InitWithUserData()` sends `JoinMessage` AFTER recent msgs via `tea.Sequence(ircCmds...)` to preserve order
**EventSub subscriptions**: MUST be sent within 10s of session creation or Twitch closes connection (`twitch/eventsub/conn.go:73`)

### Performance
**Color cache**: NEVER create `lipgloss.Style` per message - cache `Render` func in `userColorCache`
**Wordwrap**: Use `wrap.String(wordwrap.String())` combo - soft wrap first, force break if needed
**Tab counter**: `multiplex.IncrementTabCounter`/`DecrementTabCounter` manages IRC reconnect logic - always pair on tab open/close
