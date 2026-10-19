# Handoff: UI state management & routing

**Status:** analysis complete, no production code changed.
**Purpose:** basis for task breakdown at a later date.
**Baseline:** commit `ebbb44c` ("feature: ergonomic keybinds and mouse support"), bubbletea `v2.1.0`, lipgloss `v2.0.6`.
**Audience:** whoever picks this up next (human or agent). Read §1–§3 to understand the problem and the design, then §4 for the work breakdown.

Line references are against the baseline commit and will rot. Every number in §1.7 can be
re-derived with the commands in §7 — re-run them before quoting them in a task description.

---

## TL;DR

Three different problems are currently tangled into "state management":

1. **Routing is inline in `Root.Update`** — screen transitions are hand-written `switch r.screenType` blocks that each re-implement *blur old → construct → focus new → resize*.
2. **`broadcastTab` has an implicit state machine** — 5 modes, 16 `t.state =` assignments across 3 files, 66 mode comparisons package-wide, no declared transition table, no invariants, no transition tests.
3. **Parents inspect children's modes through flattened booleans** — `Root` re-encodes `broadcastTab`'s state machine via `State()`, `IsSearching()`, `IsDataLoaded()`, so every new mode must be added to the parent too.

A fourth, smaller problem: **"which tab is active" is stored four times** (`tabs` index cursor, header entry `selected` flag, per-tab `focused` bool, and header `entries` slice).

**Recommendation:** do **not** adopt a third-party router or state-management library (see §2 — the ecosystem is 0–11★ experiments, one v2-compatible candidate has no license). Build a small in-repo `ui/nav` package (stack + registry + typed addresses, ~300 LOC) plus a per-tab transition table with entry/exit actions (~150 LOC), in the phased order of §4. The `ui/nav` package becomes a candidate for extraction only after `accountui` consumes it too.

---

## 1. Findings

### 1.1 Screen routing is inline in `Root.Update`

`ui/mainui/root.go:368` is a ~450-line `Update` that mixes global key matching with screen
transitions. The pattern repeats for each modal:

| Screen | Enter | Exit |
| --- | --- | --- |
| help | `root.go:531` | `root.go:540–547` (Escape) |
| join | `root.go:575` | `root.go:605` |
| quick join | `root.go:618` | `root.go:628` |

Each entry branch does the same five things by hand: blur the active tab, set `screenType`,
construct the component (`newJoin` / `newQuickJoin`), call `focus()`, return `Init()`. Each exit
branch does the mirror: re-focus the active tab, `blur()` the modal, reset `screenType`.

Consequences:

- **8 `r.screenType = ` assignments** and ~20 `screenType` reads in `root.go` and `mouse.go`
  (`mouse.go:85`, `mouse.go:105`, `mouse.go:133`, `root.go:500`, `root.go:719`, `root.go:755`).
- No screen has an enter/exit lifecycle hook, so "restore the previously focused component" is
  re-derived at each site. `Root.handleResize` (`root.go:987`) additionally has to know about
  every screen (`splash`, `joinInput`, `quickJoinInput`, `help`).
- `Root.isTyping()` (`mouse.go:87`) encodes "is the user typing" by checking a *screen* identity
  plus a *tab mode* plus a *nested chat window mode*.
- Modals are composited by `centerOverlay` (`ui/mainui/overlay.go`) which already exists and works;
  there is no compositing abstraction above it, only the `screenContent()` / `dimmedScreenBackground()`
  pair in `root.go:743–783`.

### 1.2 `broadcastTab` has an implicit state machine

Modes are declared at `ui/mainui/broadcast_tab.go:54–77`:

```go
inChatWindow, insertMode, userInspectMode, userInspectInsertMode, emoteOverviewMode
```

The transitions are not declared anywhere — they are the union of ad-hoc assignments:

| File | Assignments |
| --- | --- |
| `broadcast_tab_keys.go` | 13 |
| `broadcast_tab_send.go` | 2 |
| `broadcast_tab_commands.go` | 1 |
| **total for `broadcastTab.state`** | **16** |
| `chat_window.go` (nested `chatWindowState`) | 2 |

Concrete costs measured on the current tree:

- Adding **one** mode (`emoteOverviewMode`) touched **10 sites across 5 files**
  (`broadcast_tab.go:76`, `broadcast_tab_keys.go:135,387`, `broadcast_tab_mouse.go:76,150`,
  `broadcast_tab_update.go:384,399`, `broadcast_tab_view.go:26,100,222`).
- `userInspectInsertMode` now has **27** text touch points, `inChatWindow` **20**. Each new mode is
  a cross-product against every existing guard, so the cost is *superlinear in the number of modes*.
- 66 `state ==` / `state !=` comparisons outside tests (this total includes the nested
  `chatWindowState` sites); **30 of them in `broadcast_tab_keys.go` alone**, mostly in guard
  expressions like
  `(t.state == inChatWindow || t.state == userInspectMode) && !searching`.

**The same transition sequence is copy-pasted three times:**

- `handleCopyMessage` — `broadcast_tab_keys.go:212`
- `handleReplyMessage` — `broadcast_tab_keys.go:262`
- `handleTimeoutShortcut` — `broadcast_tab_keys.go:343`

All three do: *stop chat-window search if active → blur the active chat window → pick next mode
based on the current one → `input.Focus()` → mutate input value → `HandleResize()`.* Three
independent places where forgetting one step is a silent bug.

**Focus correctness is by construction, not by invariant.** Examples:

- `handleOpenEmoteOverview` (`broadcast_tab_keys.go:382`) sets `state = emoteOverviewMode` without
  blurring `chatWindow`. It happens to be harmless only because it is guarded by
  `if t.account.IsAnonymous { return }`, and anonymous tabs can never be in insert mode.
- `joinChannelMessage` (`root.go:387`) calls `nTab.Focus()` and sets `tabCursor` without blurring
  the previously active tab. It is correct only because the join flow always passes through the
  join modal, which blurs first. Any second entry point (a new CLI path, a slash command, a
  notification click) reintroduces the bug.
- If two tabs ever hold `focused == true`, a single `tea.KeyPressMsg` is **handled twice**:
  `Root.Update` fans every key out to all tabs (`root.go:719`), and each focused tab handles it
  (`broadcast_tab_update.go:92`).
- `broadcastTab.Focus()` (`broadcast_tab.go:370`) switches on the mode to decide which child to
  focus, and silently focuses nothing for `emoteOverviewMode`. Correctness depends on `Escape`
  later re-focusing — an invariant nobody wrote down.

### 1.3 Parents read child modes through flattened booleans

The `tab` interface (`root.go:50`) exposes **8** accessors, four of which leak the state machine:

```go
State() broadcastTabState
IsSearching() bool
IsDataLoaded() bool
Focused() bool
```

`Root` then re-encodes mode sets that belong to the tab:

- `root.go:637`, `root.go:646` (next/previous): `State() == insertMode || State() == userInspectInsertMode`
- `root.go:655` (close): the same two modes plus `IsSearching()`
- `mouse.go:94–95` (typing detection): the same two modes plus `IsSearching()`

So a new mode must be added in the child **and** in every parent condition. This is the actual
mechanism behind "components balloon": **N modes × M parent decisions × K guard sites**.

Same issue, one level down: `chatWindow.searchChatWindowState` is re-decoded by
`broadcastTab.IsSearching()` (`broadcast_tab.go:341`), by `bc.isChatSearching()`
(`broadcast_tab_keys.go:110`), and inline in `handleKeyPress` (`broadcast_tab_keys.go:24`,
`:135`, `:155`) — 9 references to `searchChatWindowState` in `broadcast_tab_keys.go` alone.

### 1.4 "Which tab is active" is stored four times

| Representation | Where |
| --- | --- |
| `r.tabs []tab` + `r.tabCursor int` | `root.go:119–120` — **49** `tabCursor` refs, **38** `r.tabs[` refs |
| `header.entries[].selected` | `horizontal_tab_header.go:21`, `vertical_tab_header.go` — **22** `entries` refs |
| `broadcastTab.focused` | one per tab |
| `header.entries` slice order | duplicates `r.tabs` order |

The "blur old → `header.SelectTab` → focus new" protocol is implemented **5 times**:
`nextTab` (`root.go:1027`), `prevTab` (`root.go:1046`), `switchToTab` (`mouse.go:58`),
`joinChannelMessage` (`root.go:387`), `handlePersistedDataLoaded` (`root.go:1162`).
`SelectTab` is called from 6 sites.

`closeTab` (`root.go:1415`) is particularly fragile: it does `r.tabCursor--` followed by
`r.nextTab()`, relying on `Blur()` being idempotent and on `nextTab` re-blurring a tab that was
never focused. It works; it is not obvious that it works.

### 1.5 Message routing is string-addressed with per-child filtering

22 custom message types (`type …Message struct` in `ui/mainui`). Five carry an address
(`targetID string` at `broadcast_tab.go:34,39,49`, `message.go:113`, `status.go:68`,
`user_inspect.go:20`; `target string` at `message.go:103`, `status.go:68`).

- **7** `if msg.targetID != t.id { return }` / `if msg.target != t.channelID` re-checks live inside
  child `Update` methods (`broadcast_tab_update.go:46,54,63`, among others).
- **8** fan-out loops in `Root` hand the same event to every tab, and each tab then re-decides
  (`root.go:440`, `root.go:470`, `root.go:483`, `root.go:497`, `root.go:927`, …).
- `broadcastTab.handleChatEvent` (`broadcast_tab_update.go:326`) additionally re-filters on
  `accountID` and `channel` — the third filtering layer for the same event.

Net effect: the "who is this for" decision exists in three places (producer field, root loop,
child guard) and none of them is authoritative. `chatEventMessage.tabID` is even *sometimes* empty
(`buildChatEventMessage(msg.accountID, "", …)`), which is why the child filter has to fall back to
matching `channel`.

### 1.6 There are no transition tests

All 40 test functions in `ui/mainui` test pure helpers, rendering, or mouse math
(`reply_test.go`, `mouse_test.go`, `chat_input_test.go`, …). Nothing asserts:

- which modes are reachable from which,
- that the focus invariant holds after a transition,
- that Escape/send/inspect sequences leave the tab in a sane state,
- that `Root`'s screen transitions restore the previously active tab.

The bugs in §1.2 are unassertable in the current shape — that is the argument for making the
transition table explicit before someone adds mode #6.

### 1.7 Measurements

| Metric | Count | How to re-derive |
| --- | --- | --- |
| Non-test LOC in `ui/` | 11,971 | §7.1 |
| `Root.Update` length | ~450 lines | §7.1 |
| Mode comparisons (`state ==`/`!=`) | 66 | §7.2 |
| Mode assignments (`t.state =`) | 16 | §7.3 |
| Screen assignments (`r.screenType =`) | 8 | §7.3 |
| `tabCursor` references | 49 | §7.4 |
| `r.tabs[` references | 38 | §7.4 |
| `header` `entries` references | 22 | §7.4 |
| `Focus()`/`Blur()` call sites | 59 | §7.5 |
| `HandleResize()` call sites | 25 | §7.5 |
| Manual `targetID`/`target` filters | 7 | §7.6 |
| Tab fan-out loops in `Root` | 8 | §7.6 |
| Custom message structs | 22 | §7.6 |
| State-transition tests | 0 | §7.7 |

---

## 2. Ecosystem survey

Surveyed while looking for an off-the-shelf answer. Star counts / last-push / license taken from
the GitHub API at the time of writing; re-check before acting on this table.

| Library | What it provides | Bubble Tea v2 | Maturity | Verdict |
| --- | --- | --- | --- | --- |
| [`donderom/bubblon/v2`](https://github.com/donderom/bubblon) | model stack, `Open()`/`Pop()` | ✅ v2.0.0 | 11★, MIT, has a design writeup | Closest match for `screenType`, but **fullscreen only** by its own admission ("suitable only for 'fullscreen' views or views that render on top of each other"). No overlay compositing (join/quick-join are overlays), no focus management, no tabs, no addressing. |
| [`pgavlin/bubbletea-nav`](https://github.com/pgavlin/bubbletea-nav) | `Stack` (push/pop/replace) + `FocusManager` + lifecycle hooks | ✅ v2.0.0 | 1★, **no LICENSE file** | API is the closest thing to what we need. **Unusable**: no license means all rights reserved. Also 1★ / AI-authored / no user base. |
| [`brohd11/bubblestack`](https://github.com/brohd11/bubblestack) | router + screens + header/status chrome + theme | not declared | 0★ | Whole app framework; would replace the app, not a component of it. |
| [`qmuntal/stateless`](https://github.com/qmuntal/stateless) | typed FSM, guards, entry/exit actions, hierarchical | n/a | 1.4k★, BSD-2, maintained | Viable for the tab FSM, but transitions return `error` and actions get args via `context`; producing a `tea.Cmd` from a transition needs glue. Glue ≈ the thing it replaces. |
| [`looplab/fsm`](https://github.com/looplab/fsm) | classic FSM, callbacks | n/a | 3.4k★, Apache-2 | `interface{}` events/args, go 1.16 module. Not worth the typing loss. |
| [`floodfx/gstate`](https://github.com/floodfx/gstate) | XState-style statecharts | n/a | 2★ | Overkill for 5 modes / ~8 events. |
| [`vknabel/go-bubblenav`](https://github.com/vknabel/go-bubblenav) | nav helpers (`NavPage`, `nav.Push`) | ❌ v1 | **archived 2023**, 3★ | Dead. |
| [`alexanderbh/bubbleapp`](https://github.com/alexanderbh/bubbleapp) | hooks + layout engine, React-like | ❌ v1 | 18★, last push 2025 | Different paradigm (hooks, not Elm); no v2 support. |

**Verdict: no dependency to adopt.** Known precedents support this: Charm's own large BT apps
(`crush`, `soft-serve`, `pop`) hand-roll their screen stacks rather than pulling a library.

**Why not a Redux-style store.** `Update` already is the reducer, `Msg` already is the action, and
the model already is the store. Go gives no immutability and no structural sharing, so a store layer
would add ceremony without adding a single guarantee. The missing guarantees here are about
**transitions** (which mode may follow which) and **identity** (which component owns this message),
not about where the data lives.

**What is genuinely missing from the ecosystem** — i.e. what a library would have to provide, and
what we therefore build in-repo:

1. an overlay-aware screen stack with enter/exit lifecycle hooks (bubblon is fullscreen-only),
2. a single-source-of-truth registry for tab sets with an active member,
3. typed message addressing + explicit broadcast (every BT app hand-rolls `targetID`).

---

## 3. Proposed design

Four pieces. A, B, C form `ui/nav`; D and E stay inside `mainui`.

### 3.1 `ui/nav.Stack` — overlay-aware screen stack (~150 LOC)

Replaces `screenType` and the six inline transition sites.

```go
package nav

type Screen interface {
    Init() tea.Cmd
    Update(tea.Msg) (Screen, tea.Cmd)
    View() string
}

// Overlay keeps the screen below rendered (dimmed) and owns all input.
type Overlay interface {
    Screen
    OverlaysBackground() bool
}

// Lifecycle is called exactly once per push/pop. This is where focus/blur goes.
type Lifecycle interface {
    Appeared() tea.Cmd
    Disappeared()
}

type Stack struct {
    screens []Screen // screens[0] is the root; last is top
}

func (s *Stack) Update(msg tea.Msg) (tea.Model, tea.Cmd) // routes to top only
func (s *Stack) View() tea.View                         // composites overlays

func Push(scr Screen) tea.Cmd // tea.Cmd → pure Elm effect
func Pop() tea.Cmd
func Replace(scr Screen) tea.Cmd
```

```go
func (s *Stack) View() tea.View {
    top := s.top()
    content := top.View()
    if o, ok := top.(Overlay); ok && o.OverlaysBackground() {
        content = centerOverlay(content, faint(s.below().View()))
    }
    return tea.NewView(content)
}
```

Notes for the implementer:

- **Do not let the stack own `tea.View` chrome.** `Root.View` (`root.go:735`) currently sets
  `AltScreen`, `WindowTitle`, and `MouseMode`. Keep those in the outermost app model and have the
  stack return a string (`View() string`) or transfer the flags verbatim. A router that swallows
  `MouseMode` silently disables chatuino's mouse support.
- `centerOverlay` (`ui/mainui/overlay.go`) already implements exactly the compositing needed; the
  package boundary should move it into `nav` (or `nav` takes a `Compose func(fg, bg string) string`).
- `join`, `quickJoin`, `help` implement `Screen` + `Overlay`. Their existing `Update` methods return
  self-typed values (`(*join, tea.Cmd)`), so each needs a thin adapter or a signature change to
  `(nav.Screen, tea.Cmd)`.
- `Appeared`/`Disappeared` replaces: `r.tabs[r.tabCursor].Blur()` on modal open, the
  re-focus on Escape/exit (`root.go:540–547`, `:605`, `:628`).

**Bonus:** `Root.isTyping()` (`mouse.go:87`) collapses to a capability query on the top screen:

```go
type InputIntent int
const (
    IntentNone InputIntent = iota // component handles keys itself
    IntentText                    // printable keys belong to a text field
    IntentModal                   // owns all keys
)
type Intentful interface{ Intent() InputIntent }
```

### 3.2 `ui/nav.Registry[T]` — one source of truth for tabs (~120 LOC)

Replaces `tabs` + `tabCursor` + `entries[].selected` + per-tab `focused`.

```go
type Item interface {
    ID() string
    Focus()
    Blur()
}

type Registry[T Item] struct {
    items  []T
    active string // the ONLY storage of "which tab is active"
}

func NewRegistry[T Item]() *Registry[T]
func (r *Registry[T]) Add(item T) T
func (r *Registry[T]) Remove(id string) (T, bool) // re-activates neighbour
func (r *Registry[T]) SetActive(id string)        // blur old, focus new — once
func (r *Registry[T]) Next() / Prev()
func (r *Registry[T]) Active() (T, bool)
func (r *Registry[T]) All() []T
func (r *Registry[T]) Update(msg tea.Msg) tea.Cmd // dispatch (see 3.3)
func (r *Registry[T]) View() []TabInfo            // what the header renders
```

`TabInfo` is the header's whole input:

```go
type TabInfo struct {
    ID             string
    Title          string
    Identity       string
    HasNotification bool
    Active         bool
}
```

The header stops being a `tea.Model` with a duplicate list and becomes a pure view function:

```go
func (h *horizontalTabHeader) View(tabs []TabInfo) string
func (h *horizontalTabHeader) TabAt(x, y int) (string, bool)
```

`horizontalTabHeader.Update` (`horizontal_tab_header.go:49`) only handles
`requestNotificationIconMessage` today; that moves to the registry/root, removing the last reason
for the duplicate list.

Go generics make `Registry[T Item]` work with the existing self-typed `Update` signatures
(`tab.Update(tea.Msg) (tab, tea.Cmd)`); no `interface{}` casts needed.

### 3.3 `ui/nav`: typed addressing (~40 LOC)

Replaces the `targetID` field + child-side filter pattern.

```go
type Addr string

type envelope struct {
    to  Addr
    msg any
}

func To(addr Addr, msg any) tea.Cmd { return func() tea.Msg { return envelope{to: addr, msg: msg} } }
func Broadcast(msg any) tea.Cmd     { return func() tea.Msg { return envelope{to: "*", msg: msg} } }
```

`Registry.Update` unwraps the envelope and delivers only to the addressed item (or all items for
`"*"`), so children never see a message meant for someone else:

```go
func (r *Registry[T]) Update(msg tea.Msg) tea.Cmd {
    env, ok := msg.(envelope)
    if !ok {
        return r.broadcast(msg) // unaddressed messages still fan out (compat)
    }
    if env.to == "*" {
        return r.broadcast(env.msg)
    }
    // dispatch to a single item
}
```

Wins: removes 7 filters, converts 8 implicit fan-outs into an explicit `Broadcast` at the call site,
and makes intent readable (`nav.To(tab.ID(), setChannelData{…})` vs. `if msg.targetID != t.id { return }`).

**Caveat:** `chatEventMessage` currently reaches every tab and is filtered by
`accountID` + `channel` inside the child (`broadcast_tab_update.go:326`). That filter must move to
the producer side, otherwise addressed delivery alone does not fix it. Note this explicitly in the
task.

### 3.4 Mode FSM with entry/exit actions (~150 LOC, inside `broadcastTab`)

The direct answer to "securing valid state transitions". Table-driven, no dependency.

```go
type mode uint8
const (
    chatMode mode = iota
    insertMode
    inspectMode
    inspectInsertMode
    emoteOverviewMode
)

type event uint8
const (
    evInsert event = iota
    evEscape
    evSend
    evQuickSend
    evInspect
    evCopy
    evReply
    evTimeout
    evEmoteOverview
    evBlur
)

type transition struct {
    to    mode
    guard func(*broadcastTab) bool
    exit  func(*broadcastTab)     // blur — the only place
    enter func(*broadcastTab) tea.Cmd // focus + resize — the only place
}

var transitions = map[mode]map[event]transition{
    chatMode: {
        evInsert: {to: insertMode, guard: canSend, exit: blurChat, enter: focusInput},
        evInspect: {to: inspectMode, guard: notSearching, enter: openInspect},
        evEmoteOverview: {to: emoteOverviewMode, guard: notAnonymous, exit: blurChat, enter: openEmoteOverview},
    },
    insertMode: {
        evSend:   {to: chatMode, exit: clearInput, enter: focusChat},
        evEscape: {to: chatMode, exit: blurInput, enter: focusChat},
    },
    inspectMode: {
        evInsert: {to: inspectInsertMode, exit: blurInspect, enter: focusInput},
        evEscape: {to: chatMode, exit: dropInspect, enter: focusChat},
    },
    // …
}

func (t *broadcastTab) transit(ev event) (bool, tea.Cmd) {
    tr, ok := transitions[t.mode][ev]
    if !ok || (tr.guard != nil && !tr.guard(t)) {
        return false, nil
    }
    if tr.exit != nil {
        tr.exit(t)
    }
    t.mode = tr.to
    if tr.enter != nil {
        return true, tr.enter(t)
    }
    return true, nil
}
```

This absorbs:

- the 3× duplicated *stop-search → blur → set mode → focus input → SetValue → resize* blocks
  (`handleCopyMessage`, `handleReplyMessage`, `handleTimeoutShortcut`),
- `handleEscapePressed` (`broadcast_tab_keys.go:134`) turning into `transit(evEscape)`,
- `Focus()`'s mode switch (`broadcast_tab.go:370`),
- the missing-blur bug class (`handleOpenEmoteOverview`).

**Companion test (new file `broadcast_tab_fsm_test.go`), non-negotiable:**

1. Iterate the full **modes × events** matrix; assert each pair is either declared or *explicitly*
   rejected (an `// unreachable, because …` list in the test, not a silent gap).
2. After every legal transition, assert the invariants in §5.
3. Assert `handleEscapePressed` from every mode lands in a mode where
   `chatWindow` is focused (or the mode is `emoteOverviewMode`).

### 3.5 Mode-scoped key tables (inside `broadcastTab`)

Biggest single LOC win; collapses most of `broadcast_tab_keys.go`'s 30 mode comparisons.

```go
type binding struct {
    keys key.Binding
    when func(*broadcastTab) bool
    run  func(*broadcastTab) (event, tea.Cmd)
}

var keyBindings = map[mode][]binding{
    chatMode: {
        {t.deps.Keymap.InsertMode, nil, doInsert},
        {t.deps.Keymap.InspectMode, notSearching, doInspect},
        {t.deps.Keymap.ReplyMessage, notSearching, doReply},
        {t.deps.Keymap.CopyMessage, notSearching, doCopy},
    },
    insertMode: {
        {t.deps.Keymap.Confirm, hasInput, doSend},
    },
    // …
}
```

`handleKeyPress` (`broadcast_tab_keys.go:17`) becomes: look up the mode's slice, return the first
match. The `(state == a || state == b) && !searching && …` cross-products disappear because the mode
*selects* the table.

Bonus: `help.go` can render its key list from the same table instead of hand-maintaining it.

Caveat: `t.deps.Keymap` is per-dependency-container, so the table must be built per tab (in
`newBroadcastTab`) rather than being a package-level `var`. Use a `func buildBindings(deps) map[mode][]binding`.

### 3.6 Deferred: typestate split

Only if §3.4–§3.5 do not bring the tab down enough. Split the struct in two:

- `tabSession` — long-lived data with no mode semantics: `deps`, `account`, `channel*`,
  `chatWindow`, `streamInfo`, `poll`, `statusInfo`, `input`, `lastMessages`, sizes.
- `tabMode` — a closed interface sum type holding only mode-valid fields
  (`chatMode{search}`, `insertMode{reply}`, `inspectMode{userInspect}`, …).

Go has no sum types, but an interface with an unexported method gives a closed set:

```go
type tabMode interface{ isTabMode() }
```

Then `t.userInspect` only exists on inspect modes, and the compiler prevents the §1.2 bug class
outright. Cost: touches all ~2,400 lines of `broadcast_tab*.go`. Do it last, or never.

### 3.7 Deliberately rejected

- **A generic store/Redux layer.** See §2.
- **Adopting `bubblon`** — fullscreen only, no overlay compositing, no address/focus story.
- **Adopting `bubbletea-nav`** — no license.
- **`stateless`/`looplab`** for 5 modes / ~10 events — more glue than the 150-line table, and it
  cannot return a `tea.Cmd` from a transition.
- **Publishing `ui/nav` immediately.** Build it in-tree, let `accountui` consume it, *then* consider
  extraction. `go-bubblenav` is the cautionary tale: designed before the second consumer, archived
  with 3★.

---

## 4. Phases → candidate tasks

Ordered by risk-adjusted payoff. Each phase is independently shippable and behaviour-preserving
except where noted.

### Phase 1 — `Registry` + header-as-view

**Goal:** single source of truth for the tab set and the active tab.
**Touches:** `ui/mainui/root.go` (`tabs`, `tabCursor`, `nextTab`, `prevTab`, `switchToTab`,
`closeTab`, `joinChannelMessage`, `handlePersistedDataLoaded`, `handleResize`, `createTab`),
`ui/mainui/mouse.go`, `ui/mainui/horizontal_tab_header.go`, `ui/mainui/vertical_tab_header.go`,
new `ui/nav/registry.go`.
**Removes:** `tabCursor` (49 refs), `entries` bookkeeping (22 refs), one `entries` slice copy,
5 implementations of the focus-swap protocol, `header.Update`.
**Done when:** `go test ./...` passes, existing mouse tests (`mouse_test.go` — `TabAt`,
`TabAtRebuildsAfterAdd`, `TabAtPaged`) still pass unchanged, no `tabCursor` remains, and a new test
asserts that exactly one tab is `focused` after `Add` / `Remove` / `SetActive` / `Next` / `Prev`.
**Risk:** low. Watch `closeTab`'s `cursor--; nextTab()` hack — the registry must own "activate the
neighbour of the removed tab" explicitly.
**Do not:** change visual layout/order semantics. `horizontalTabHeader.calculatePages` paging must
keep working from `[]TabInfo`.

### Phase 2 — `nav.Stack` for join / quick join / help

**Goal:** declarative screen routing with enter/exit lifecycle.
**Touches:** new `ui/nav/stack.go`; `ui/mainui/root.go` (`screenType`, `renderView`,
`screenContent`, `dimmedScreenBackground`, `handleResize`, `isTyping`, the key blocks at
`root.go:526–631`); `ui/mainui/overlay.go` (move into `nav` or inject as a composer);
`join.go`, `quick_join.go`, `help.go` (adapt to `nav.Screen`).
**Removes:** `activeScreen` (8 assignments, ~20 reads), the manual blur/focus dance in 3 enter and
3 exit branches, `screenType` branches in `mouse.go`.
**Done when:** modals still composite dimmed over the active tab (overlay tests
`Test_Root_modalKeepsScreenDimensions`, `Test_centerOverlay` pass), `MouseMode`/`AltScreen` are
still set, and Escape from each modal restores focus to the previously active tab (new test).
**Risk:** medium. Two traps: (a) `nav.Stack` must not own `tea.View` chrome; (b) `quickJoin`
receives `polledStreamInfoMessage` while open (`root.go:500`) — make sure addressed/stacked routing
still delivers it.

### Phase 3 — typed addressing (`To` / `Broadcast`)

**Goal:** one authoritative answer to "who is this message for".
**Touches:** new `ui/nav/address.go`; `ui/mainui/root.go` (8 fan-out loops),
`ui/mainui/broadcast_tab*.go` (7 filters), `ui/mainui/message.go` / `broadcast_tab.go` /
`status.go` / `user_inspect.go` (drop `targetID`/`target` fields).
**Done when:** no `msg.targetID != ` / `msg.target != ` remain in children; each former fan-out is
either `nav.Broadcast` or `nav.To`; `buildChatEventMessage`'s `tabID` is empty-or-set deterministically
(no more "empty means everyone"); existing behaviour for IRC events reaching *all tabs of an account*
is covered by a test.
**Risk:** medium — this is where a lost message shows up as a silently-dead feature.
Recommend doing it after Phase 1 so the registry can own dispatch, and adding one test per message
family (channel data / suggestions / stream info / emotes / notifications).

### Phase 4 — mode FSM table + matrix test

**Goal:** legal transitions are declared once; invariants are asserted.
**Touches:** `ui/mainui/broadcast_tab.go` (mode type + transition table + `transit`),
`broadcast_tab_keys.go` (`handleEscapePressed`, `handleStartInsertMode`, `leaveUserInspect`,
the three duplicated blocks), `broadcast_tab_send.go` (`resetAfterSend`), `broadcast_tab_commands.go`
(`handleOpenUserInspect`), `broadcast_tab.go` (`Focus`), new `broadcast_tab_fsm_test.go`.
**Done when:** no `t.state = ` assignment exists outside `transit`; the matrix test covers every
(mode, event) pair; the §5 invariants are asserted after every transition.
**Risk:** low-medium. Start by writing the matrix test against the *current* implementation to
capture the existing behaviour (including the quirks), then introduce the table and require the test
to stay green. That converts the refactor into a safe rewrite and documents today's semantics.
**Prerequisite:** decide whether `emoteOverviewMode` should stay a mode or become an overlay
(it renders full-tab and blurs the chat window — see §6).

### Phase 5 — mode-scoped key tables

**Goal:** kill the 30 guard comparisons in `broadcast_tab_keys.go`.
**Touches:** `ui/mainui/broadcast_tab_keys.go`, `ui/mainui/broadcast_tab.go` (build the table per
tab), `ui/mainui/help.go` (render from the table, optional).
**Done when:** `handleKeyPress` is a table lookup; a table-driven test asserts, for every mode, the
exact set of live bindings; `suggestion_keys_test.go` / `reply_test.go` stay green.
**Risk:** low. The one subtlety: `input.IsAcceptSuggestionKey(msg)` and
`t.input.Update(tea.KeyPressMsg{Code: tea.KeyEnter})` are input-specific, not keymap-driven — keep
those as explicit entries rather than pretending they are bindings.

### Phase 6 — (optional) typestate split

See §3.6. Only if Phases 4–5 leave `broadcastTab.Update` unreadable. Do not start this before
Phase 4 has landed and the invariants are locked down by tests.

---

## 5. Invariants to assert

These are the guarantees that "securing valid state transitions" should mean, concretely. They are
currently documented nowhere and tested nowhere.

**Tab-level:**
1. Exactly one child of a tab is focused at any time (`chatWindow` XOR `input` XOR `userInspect.chatWindow`).
2. `input.Focused()` ⟺ mode is `insertMode` or `inspectInsertMode`.
3. `userInspect != nil` ⟺ mode is `inspectMode` or `inspectInsertMode`.
4. `account.IsAnonymous` ⇒ never an insert mode.
5. Leaving a mode always clears mode-scoped transient state (`replyTarget`, search query, selection).
   Specifically: `clearReplyTarget()` must be called on every exit from an insert mode.
6. `chatWindow.state == searchChatWindowState` never survives a mode change.
7. A tab that is not `focused` never handles a `tea.KeyPressMsg` (see §1.2, double-handling).

**Root-level:**
8. Exactly one tab has `focused == true`, and it is `registry.Active()`.
9. `header`'s notion of the active tab equals `registry.Active()` (this invariant disappears in
   Phase 1 — that is the point).
10. A modal screen is the only component that receives input while it is open.
11. After popping a modal, focus returns to whatever had it before the push.

**Message-level:**
12. Every message is delivered to exactly one component, or to an explicitly declared broadcast set.
13. No child re-filters on account/channel (post Phase 3).

---

## 6. Open questions / decisions needed

1. **`emoteOverviewMode`: mode or overlay?** It hides the chat window and the input entirely
   (`broadcast_tab_view.go:26`), which makes it behaviourally an overlay. If it became one,
   `transitions` shrinks by one mode and the "missing blur" bug in `handleOpenEmoteOverview` becomes
   structurally impossible. Cost: it is per-tab, so it needs an overlay slot *inside* the tab, not
   in the root `nav.Stack`.
2. **Where do `tea.View` flags live** after Phases 1–2? Recommendation: outermost model only.
3. **Does `chatWindow` need its own transition table**, or is `searchActive bool` + `transit` enough?
   11 mode comparisons in `chat_window.go` suggest a smaller table, but it may not pay for itself.
4. **Should `tab` stay an interface with self-typed `Update`**, or move to `tea.Model`?
   `tea.Model` would let `nav.Registry[tea.Model]` drop generics, but loses type-safety on
   `tab`-specific methods (`Channel()`, `Kind()`, …). Recommendation: keep self-typed, use generics.
5. **`Intent()` vs. keeping `State()`**: if Phase 4 lands, does `Root` still need to know any mode?
   Inventory suggests only "may I take this key?" (CloseTab) and "is text being captured?" (typing
   detection). Both are one capability query.
6. **Is `accountui` a viable second consumer** for `ui/nav` (it has `list.go`, `create.go`, a
   `provider.go`)? If not, keep `ui/nav` unexported/internal and skip the extraction debate.

---

## 7. Appendix: measurement commands

Run from the repo root. Counts are for non-test files only.

```bash
# 7.1 size
find ui -name '*.go' ! -name '*_test.go' | xargs wc -l | tail -1
awk '/^func \(r \*Root\) Update/,/^}/' ui/mainui/root.go | wc -l

# 7.2 mode comparisons
grep -rn 'state == \|state != ' ui/mainui/*.go | grep -v _test | wc -l
grep -rn 'state == \|state != ' ui/mainui/*.go | grep -v _test | awk -F: '{print $1}' | sort | uniq -c | sort -rn

# 7.3 assignments
grep -rn 't\.state = '   ui/mainui/*.go | grep -v _test | wc -l
grep -rn 'r\.screenType = ' ui/mainui/*.go | grep -v _test | wc -l
grep -rn 'c\.state = '   ui/mainui/*.go | grep -v _test | wc -l

# 7.4 tab identity duplication
grep -rn 'tabCursor'     ui/mainui/*.go | grep -v _test | wc -l
grep -rn 'r\.tabs\['     ui/mainui/*.go | grep -v _test | wc -l
grep -rn 'entries'       ui/mainui/horizontal_tab_header.go ui/mainui/vertical_tab_header.go | wc -l

# 7.5 focus / layout churn
grep -rn '\.Focus()\|\.Blur()' ui/mainui/*.go | grep -v _test | wc -l
grep -rn 'HandleResize()'     ui/mainui/*.go | grep -v _test | wc -l

# 7.6 routing
grep -rn 'msg\.targetID != \|msg\.target != ' ui/mainui/*.go | grep -v _test | wc -l
grep -rn 'for i := range r\.tabs\|for _, tab := range r\.tabs\|for it, tab := range r\.tabs\|for i, tab := range r\.tabs' ui/mainui/*.go | grep -v _test | wc -l
grep -rn '^type .*Message struct' ui/mainui/*.go | grep -v _test | wc -l

# 7.7 transition tests
grep -rn 'func Test' ui/mainui/*_test.go | grep -ci 'state\|transition\|mode'
```

---

## 8. Reference: files in scope

| File | Non-test LOC | Role in this work |
| --- | --- | --- |
| `ui/mainui/root.go` | 1,430 | Phases 1–3: routing, tab set, fan-out |
| `ui/mainui/chat_window.go` | 1,454 | §6.3 (maybe), `searchChatWindowState` |
| `ui/mainui/broadcast_tab.go` | 411 | Phase 4: mode type, `Focus`/`Blur`/`State` |
| `ui/mainui/broadcast_tab_keys.go` | 407 | Phases 4–5: 30 mode guards, 13 assignments |
| `ui/mainui/broadcast_tab_update.go` | 411 | Phases 3–4: dispatch, `updateChildren`, child filter |
| `ui/mainui/broadcast_tab_send.go` | 375 | Phase 4: `resetAfterSend` |
| `ui/mainui/broadcast_tab_commands.go` | 337 | Phase 4: `handleOpenUserInspect` |
| `ui/mainui/broadcast_tab_view.go` | 227 | Phase 4: view branching on mode |
| `ui/mainui/broadcast_tab_mouse.go` | 213 | Phase 4: 6 mode guards |
| `ui/mainui/horizontal_tab_header.go` | 359 | Phase 1: `entries` → `[]TabInfo` |
| `ui/mainui/vertical_tab_header.go` | 309 | Phase 1: same |
| `ui/mainui/mouse.go` | 166 | Phases 1–2: `switchToTab`, `isTyping`, `screenType` |
| `ui/mainui/overlay.go` | 46 | Phase 2: move/inject into `nav` |
| `ui/mainui/join.go` | 521 | Phase 2: adapt to `nav.Screen` |
| `ui/mainui/quick_join.go` | 599 | Phase 2: adapt; `polledStreamInfoMessage` while open |
| `ui/mainui/help.go` | 170 | Phase 2 + 5 (render keys from table) |
| `ui/mainui/message.go` | 120 | Phase 3: message shapes / addresses |
