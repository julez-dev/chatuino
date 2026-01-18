# UI KNOWLEDGE BASE

Bubble Tea TUI architecture for Chatuino - main app orchestration, account management, reusable components.

## STRUCTURE
```
ui/
├── mainui/              # Root orchestrator, tabs, chat rendering, stream polling
├── accountui/           # Account creation/list/management screens
└── component/           # Reusable textinput with autocomplete/history
```

## BUBBLE TEA PATTERNS

### Model Hierarchy
- **Root** (`mainui/root.go`): Top-level orchestrator managing screens, tabs, IRC/EventSub channels
- **Tabs** (`mainui/*_tab.go`): Implement `tab` interface - broadcast, mention, live notification types
- **Components**: Self-contained Bubble Tea models with Init/Update/View

### Message Flow (Unidirectional)
1. **External events** → channels (`in`, `out`, `eventSubIn`)
2. **Root.waitChatEvents()** → receives IRC → builds `chatEventMessage`
3. **chatEventMessage** → routed to tabs via `Update()`
4. **Tabs** → update chat window → render via `View()`

### State Machines
- **Root screens**: `mainScreen`, `inputScreen`, `helpScreen`
- **broadcastTab states**: `inChatWindow`, `insertMode`, `userInspectMode`, `userInspectInsertMode`, `emoteOverviewMode`
- **chatWindow states**: `viewChatWindowState`, `searchChatWindowState`

### Channel Architecture
- **IRC**: `in chan multiplex.InboundMessage` → `out <-chan multiplex.OutboundMessage`
- **EventSub**: `eventSubIn chan multiplex.EventSubInboundMessage`
- **Logger**: `messageLoggerChan chan<- *twitchirc.PrivateMessage`
- WaitGroups (`closerWG`, `eventSubInInFlight`) ensure graceful shutdown

### Command Patterns
- **tea.Batch()**: Parallel independent commands
- **tea.Sequence()**: Sequential dependent commands (e.g., batch message rendering)
- **tea.Tick()**: Periodic tasks (app state save: 15s, stream poll: 90s, image cleanup: 1min)
- **Blocking commands**: Wrap goroutines, return tea.Msg via channels

## CONVENTIONS

### Dependency Injection
- **DependencyContainer** (`mainui/dependencies.go`): All external deps injected via constructors
- No global state - thread-safe via channels, mutexes (`sync.Map` for user display names)

### Interface Segregation
- **tab interface**: 15 methods - `Init()`, `Update()`, `View()`, `Focus()`, `Blur()`, state checks
- **header interface**: 8 methods - tab management, resize, rendering
- **Component interfaces**: `APIClient`, `EmoteCache`, `AccountProvider`, etc.

### Message Conventions
- **Suffixed structs**: `*Message` types in `mainui/message.go`
- **Internal messages**: `chatEventMessage`, `requestLocalMessageHandleMessage`
- **External events**: `persistedDataLoadedMessage`, `polledStreamInfoMessage`
- **isFakeEvent flag**: Distinguishes local vs. IRC events (prevents double `waitChatEvents()`)

### Component Design
- **SuggestionTextInput** (`component/textinput.go`): Trie-based autocomplete, history, emote replacement
- **No @ prefix for commands** (line 367) - users have @, commands don't
- **Validation**: Reject newlines, limit char count

### Persistence Integration
- **State snapshots**: `TakeStateSnapshot()` → `save.AppState` (tabs, focus, local flags)
- **Restore**: `handlePersistedDataLoaded()` rebuilds tabs from persisted state
- **Auto-save**: 15s ticker saves current state

### Resource Management
- **Image cleanup**: Kitty graphics protocol - delete unused images after 10min TTL
- **Emote cache removal**: Remove channel emotes when last tab for channel closes
- **IRC PART**: Send only when last tab for account+channel combo closes

## ANTI-PATTERNS

### Message Handling
- **NEVER** start new `waitChatEvents()` for `isFakeEvent=true` - causes infinite loops
- **NEVER** batch via newlines in bash - use tea.Batch/Sequence
- **NEVER** mutate shared state without sync primitives (use `sync.Map` or mutexes)

### Tab Management
- **broadcastTab only**: Check `Kind() == broadcastTabKind` before casting `t.(*broadcastTab)`
- **Cursor bounds**: Always validate `len(r.tabs) > r.tabCursor` before access
- **Focus discipline**: Blur old tab before focusing new, update header selection

### Rendering
- **Viewport limits**: Only render visible lines - chat.go tracks `lineStart`/`lineEnd`
- **Entry cleanup**: Prune at `cleanupThreshold` (1200) to prevent memory bloat
- **Color caching**: Reuse lipgloss render funcs (`userColorCache`) - don't recreate per message

## TESTING PATTERNS

- **Subtests with t.Parallel()** for component unit tests
- **Message mock flows**: Simulate tea.Msg sequences
- **State assertions**: Verify cursor, focus, screen transitions
