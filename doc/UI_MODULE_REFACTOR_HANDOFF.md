# Handoff: deepen Chatuino's UI modules

## Goal and current status

Improve UI maintainability by separating responsibilities and clarifying state ownership, rather than optimizing file sizes. Preserve user-visible behavior and the existing Bubble Tea architecture.

Repository: `/home/julez/code/chatuino`.
Analysis baseline: `afbdad2` (`move tests`). Re-read the current implementation before editing; the checkout may have advanced.

This handoff records recommendations, not an implemented design. No Go code was changed and no tests were run during this analysis. The working tree was clean before this document was added.

**Recommended first implementation slice:** phase 1, shared broadcast layout and rendering. Ship and verify each phase separately. The type names below are illustrative; choose the smallest interface that satisfies the actual callers.

## Start here

1. Read the implementation-status and deferred-decisions sections of [STATE_AND_ROUTING_HANDOFF.md](STATE_AND_ROUTING_HANDOFF.md). Its later findings and sketches describe an older baseline: use current source as the authority.
2. Inspect the files for the phase being implemented and their existing tests. Establish the baseline with `go test ./ui/...`; record pre-existing failures separately.
3. Add behavior coverage for the affected seam, then implement one cohesive extraction.
4. Verify the phase's completion criteria and report changed ownership, tests, and remaining work.

## Preserve the architecture already in place

- `ui/nav/registry.go` owns tab membership, active identity, and notifications.
- `ui/nav/stack.go` owns modal stacking and dispatch to the top modal. Root coordinates focus between the base tab and modals.
- `ui/nav/address.go` and registry dispatch provide tab-addressed messages.
- `ui/tab/broadcast_state.go` owns broadcast-mode transitions and their entry/exit actions. Preserve `transit` as the mode-change seam and retain its matrix tests.
- `ui/tab/chat_input.go` groups composer rendering, reply state, and input behavior.
- Dependencies flow from `ui/mainui` to `ui/tab`, not the reverse. Tabs must not acquire access to Root or the registry.
- Root owns terminal-wide `tea.View` flags, including alternate screen, window title, and mouse mode.

The existing `broadcast_*.go` split helps navigation, but those methods still operate on one shared struct. A further extraction earns its place when it hides rules or state that callers no longer need to understand.

## Phase 1 — share broadcast layout and rendering

### Evidence and entry points

- `ui/tab/broadcast_view.go`: `View`, `ViewWithoutStatusBar`, `StatusBarView`, `HandleResize`.
- `ui/tab/broadcast_mouse.go`: `chatLayout`, `userInspectLayout`, `inputLayout`, mouse handlers.
- `ui/mainui/root.go`: `screenContent`, `handleResize`.
- `ui/mainui/mouse.go`: translation from terminal coordinates to tab-local coordinates.

`View` and `ViewWithoutStatusBar` duplicate most of the rendering sequence. Resize and mouse handling independently reconstruct the same arrangement; `chatLayout` explicitly says it mirrors rendering.

### Target

Introduce a private layout calculation describing the regions allocated to stream info, poll, chat, user inspect, composer, and status. A rectangle-based result or simpler vertical regions are both reasonable.

Use this result for sizing children, rendering placement/separators, and hit-testing. Consolidate the common content rendering so the two tab views differ only in status-bar placement.

Measure children at the target width before allocating height: the composer wraps, and stream/poll content can change height. Preserve the current distinction between normal mode and vertical-tab mode, where Root renders the status bar at full terminal width.

Prefer an ordinary Go function/type. This calculation does not need a `tea.Model` or a general-purpose layout framework.

### Done when

- Shared broadcast content has one rendering path; layout and hit-testing derive from the same allocation.
- Tests cover horizontal and vertical tabs, absent/present stream info and poll, wrapped composer and reply indicator, inspect modes, emote overview, and narrow/short terminals.
- Mouse tests show that clicking or scrolling each displayed region reaches the intended child after resize and composer-height changes.
- Loading/error views, existing separators, and status-bar placement remain stable.

If characterization exposes an existing layout defect, distinguish its fix from the structural refactor and give it an explicit regression test. Do not silently redefine the expected display.

## Phase 2 — extract message rendering from the chat window

### Evidence and entry points

`ui/tab/chat_window.go` combines message retention/deletion, formatting, wrapping, search, selection, rendered-line indexing, clipboard text, and smooth scrolling.

Start with `messageToText`, `wordwrapMessage`, alert/text/badge formatting, color/style caches, and graphic-row expansion. Inspect `user_inspect.go` and `chat_input.go`: inspect changes the timestamp format, and composer construction currently shares `chatWindow.userColorCache`.

### Target

Create a private message-rendering module with a small interface, for example:

```go
func (r *messageRenderer) Render(event ChatEventMessage, width int) []string
```

Own formatting configuration and caches inside it. Keep it free of network/terminal I/O. Resolve shared color ownership explicitly, using a small palette module if warranted; keep composer and rendered-message colors consistent.

Keep entry retention, selection, search coordination, line-to-entry mapping, and scrolling in the chat window initially. Those responsibilities are tightly coupled; this phase is not an instruction to split each one into another module.

### Done when

- Formatting tests can exercise the renderer without constructing a chat window.
- Fixtures cover message/notice/error/subscription/announcement events, badges, graphic rows, Unicode/ANSI wrapping, deletion styling, duplicate-bypass stripping, and inspect timestamp formatting.
- Selection survives reflow on resize; search and deletion still rebuild correct line positions; scrolling behavior remains unchanged.
- Renderer configuration cannot drift from derived timestamp/padding widths.
- Any cache mutation stays private; rendering does not unexpectedly mutate shared event data.

## Phase 3 — extract startup loading from Root

### Evidence and entry points

`ui/mainui/root.go`: `Init`, `handlePersistedDataLoaded`, `TakeStateSnapshot`, and persistence commands. `Init` includes persisted-state loading, global asset refresh, user emotes, batched channel lookup, and channel suggestions.

### Target

Move startup fetching into a focused loader that takes explicit collaborators and returns a typed result. Root launches it and applies the result to create/activate tabs; the loader does not construct or mutate the UI tree.

Preserve detached sessions, batched lookup, suggestion deduplication, timeouts, and existing fatal versus best-effort error handling. Thread-safe shared caches may be collaborators; model-owned state is applied in `Update`.

### Done when

- Root's startup path expresses orchestration rather than implementing fetching details.
- Tests cover persisted and detached startup, missing accounts/channels, partial fetch failures, suggestions, and active-tab restoration.
- Existing unloaded-tab safety tests still pass: tabs may receive focus, blur, resize, and interface queries before their children exist.
- Dependency injection remains explicit and testable through fake adapters; a new omnibus dependency container is not substituted for the extraction.

## Phase 4 — extract ordered chat-event preparation

### Evidence and entry points

- `ui/mainui/root.go`: `buildChatEventMessage`, IRC event handling, and local-message request handling.
- `ui/tab/message.go`: chat-event payloads, modifiers, and preparation commands.
- `ui/tab/user_inspect.go`: historical-event preparation; inspect this for shared semantics, without assuming all paths are identical.

`buildChatEventMessage` mixes protocol projection, guest-channel lookup/cache refresh, emote/badge replacement, contributor badges, and link checking. The live IRC path invokes it directly inside `Update`; some branches perform network calls.

### Target

Give a focused preparation module ownership of enrichment and its collaborators. Root submits work, receives prepared results, executes required terminal preparation commands, and routes events.

Make the scheduling contract explicit before moving I/O: independently batching one command per message can reorder arrivals. Prefer a bounded, ordered queue/worker or an explicitly sequenced design. State the ordering scope, buffering/backpressure behavior, timeout/error policy, and shutdown behavior.

Capture immutable request data rather than reading mutable model fields from command closures. Model changes return through messages. Late results for closed tabs or superseded requests must not reach a different UI instance.

### Done when

- Network enrichment no longer runs synchronously inside Root's live-event `Update` branch.
- Controlled fake delays prove messages retain the declared arrival order and history/live interleaving policy.
- Tests cover enrichment failure/timeouts, duplicate deliveries where relevant, tab closure with work in flight, and orderly cancellation/shutdown without leaks.
- Broadcast events still reach matching channel/account tabs and aggregating mention/notification tabs. Tab-local history/notices remain local.
- Kitty preparation commands execute before their dependent content is displayed and are not duplicated by tab fan-out.
- Logging behavior remains correct; extraction does not introduce unbounded queues or one unrestricted goroutine per incoming event.

This phase changes scheduling as well as structure. Keep that behavioral change visible and test it explicitly.

## Follow-up — tighten child interfaces where extraction needs it

Replace parent knowledge of child representation with behavior-oriented operations:

- `Broadcast` reads `chatWindow.state` to detect search.
- Several paths reach through `userInspect.chatWindow`.
- Resize directly mutates child dimensions and dirty flags.
- Composer construction receives the chat window's mutable color cache.

Useful operations may include `Searching`, `SelectedMessage`, `Resize`, or a narrowly owned palette. Avoid getter/setter wrappers that expose the same representation under new names. Keep necessary orchestration in the parent.

Start with private types in existing packages. Introduce a separate package only when its interface is clear and package privacy provides useful enforcement. Preserve consumer-owned dependency interfaces in `ui/tab/ports.go`.

Completion means callers no longer need the extracted module's cursor representation, cache invalidation rules, or nested mode values to perform their work.

## Verification and delivery

For each phase, use behavior tests across the new interface, retaining the existing routing/state regression coverage. Golden rendering fixtures are useful for stable output; supplement them with selection, focus, geometry, and command-result assertions.

Run focused tests during development, then the broader checks relevant to delivery:

```sh
go test ./ui/...
go test -race ./ui/...
go vet ./...
go test -race ./...
```

Format changed Go files. Follow `.github/workflows/lint_test.yml` for full CI requirements, including staticcheck and any prerequisites for repository-wide checks. If a check is blocked, report the command and blocker rather than claiming a passing baseline.

Report:

1. Which phase is complete and which remain.
2. What state/rules moved behind each interface and what knowledge disappeared from callers.
3. Tests run, outcomes, and any manual terminal checks still needed for graphic emotes/mouse interaction.
4. Deliberate behavior changes or unresolved compatibility issues.

Keep this work scoped to module depth and locality. A router/store rewrite, new UI framework, full typestate conversion, mode-key-table rewrite, or unrelated account UI/completion refactor is outside this handoff.

## Primary-source precedents

These illustrate patterns, not architectures to copy wholesale. Links track upstream `main` and may change.

- [Soft Serve shell](https://github.com/charmbracelet/soft-serve/blob/main/pkg/ssh/ui.go) and [repository page](https://github.com/charmbracelet/soft-serve/blob/main/pkg/ui/pages/repo/repo.go): hierarchical shell/page/pane coordination.
- [Crush chat](https://github.com/charmbracelet/crush/blob/main/internal/ui/model/chat.go), [list](https://github.com/charmbracelet/crush/blob/main/internal/ui/list/list.go), and [dialogs](https://github.com/charmbracelet/crush/blob/main/internal/ui/dialog/dialog.go): specialized interaction/rendering modules without requiring every module to implement `tea.Model`.

Success is a change to message appearance, layout, or enrichment requiring less knowledge of the rest of the UI—not achieving a particular line-count threshold.
