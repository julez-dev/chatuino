# Tiled Workspace Specification

**Status:** Implemented, including automatically managed workspaces. Automated validation is covered by the canonical Go tests; the real-terminal acceptance checks in section 10 remain outstanding before merge.

## 1. Purpose

Let users display and manage several existing Chatuino tabs in the main screen at once, using a keyboard-first, tiling-window-manager-style workspace.

Tiling is opt-in through explicit splits. Opening several tabs does not automatically tile them. Users who never split retain the current single-tab presentation and controls.

This specification records the confirmed design. It defines observable behavior and integration constraints, not a required package layout or set of Go APIs.

## 2. Vocabulary

| Term | Meaning |
| --- | --- |
| **Tab** | An existing open Channel, Mention, or Live Notification tab, with its own model and lifecycle. |
| **Workspace (screen)** | A tiling screen with one split tree and its own pane focus. Several workspaces may exist; the active one follows the selected tab. The main content area excludes the global tab list, shared status line, and application-level dialogs. |
| **Pane** | A leaf of the workspace's split tree, assigned to exactly one open tab. |
| **Assigned tab** | A tab represented by a pane in the layout, even when zoom or small-terminal fallback temporarily prevents that pane from being drawn. |
| **Unassigned tab** | An open tab not represented in the layout. It remains accessible through the tab list. |
| **Focused pane** | The pane whose tab is the keyboard destination when no application dialog or pane-control mode owns input. |
| **Visible pane** | A pane actually drawn on screen. Normally all assigned panes are visible; zoom and fallback draw only the focused pane. |
| **Split** | An internal tree node dividing a region left/right or top/bottom, with a preferred proportion between its children. |
| **Pane mode** | A visibly indicated keyboard-control mode for manipulating the workspace rather than interacting with chat. |

Distinct tabs for the same channel under different accounts remain distinct. The uniqueness constraint concerns tab identity, not channel name.

## 3. Scope and invariants

### Included in v1

- Nested left/right and top/bottom splits.
- All existing tab types as pane contents.
- Keyboard focus navigation, i3-style directional moves with edge re-docking, resizing, pane closure, and zoom.
- Click-to-focus and pane-local mouse interaction, including hover-scrolling.
- Automatically managed workspaces with exclusive tab ownership, tab-driven activation, and a global standalone tab set.
- Restoration of multiple saved layouts and their workspace focus with the normal saved session.
- Safe behavior when the terminal is too small or saved layout references become stale.

### Deferred

- Mouse-driven rearrangement and draggable resize borders.
- Displaying one tab in multiple panes.
- Dragging panes or dividers, moving whole subtrees to an arbitrary target, and automatic grid layouts.
- Workspace renaming or custom workspace ordering.
- New CLI layout/workspace syntax or changes to detached-session semantics.
- New persistence of drafts, chat history, scroll positions, or tab-local modes. Their existing lifetimes are unchanged.

### Required invariants

1. Open tabs and their order remain owned by the tab registry. Each tab is assigned to at most one workspace; a layout holds references, not duplicate tab models or a second list of open tabs. Standalone tabs belong to no workspace.
2. Each pane refers to one existing tab; each tab is assigned to at most one pane.
3. Workspaces are created and selected implicitly. The live workspace is the one owning the active tab; a standalone active tab leaves the current workspace in the background. Empty workspaces may be reused. With no open tabs, show the existing empty-workspace experience.
4. Exactly one pane is logically focused when panes exist. A global dialog may temporarily own keyboard input without losing that logical destination.
5. Tab activation, pane focus styling, the shared status line, notification acknowledgement, and input routing must agree on the focused tab. Do not introduce independent, conflicting focus authorities.
6. Hiding a tab by replacing or closing its pane does not close the tab or discard its state.
7. Layout operations do not reorder the tab registry's storage/creation order or create extra IRC/EventSub connections for existing tabs. The tab-header projection follows workspace/pane reading order, so a pane move also moves its tab entry.
8. Open tabs keep receiving their normal background events whether assigned, visible, zoomed away, or unassigned. Rendering additional panes must not duplicate event delivery.

## 4. Tab and pane lifecycle

### 4.1 Selecting and opening tabs

| Action | Required result |
| --- | --- |
| Select an assigned tab | Focus its existing pane and make the owning workspace live automatically. Do not duplicate it, including when its pane is temporarily hidden by zoom or fallback. |
| Select an unassigned tab from the tab list (click) | Activate the standalone view. Clicking never assigns a tab to a pane. |
| Use normal next/previous-tab controls outside pane mode | Navigate all tabs non-destructively. An assigned tab focuses its pane and automatically shows the owning workspace; a standalone tab is shown full-screen while every layout remains unchanged behind it. |
| Open and activate a tab through the normal creation flow (Ctrl+T outside pane mode) | Show the new standalone tab; do not implicitly create a split or evict a visible pane. |
| Open a tab while in pane mode (Ctrl+T) | Add it to the current workspace through the normal join flow as a new pane, then focus it. Pane mode remains active. |
| Enter pane mode on a standalone tab (F2) | Create or reuse an empty workspace, make the tab its first pane, and enter pane mode. This is the only implicit workspace creation path. |
| Enter pane mode on an assigned tab (F2) | Make its workspace live and enter pane mode without changing assignments. |
| Open the first tab | Display it standalone until pane mode is entered. |

Splitting is the explicit operation that increases pane count; Ctrl+T in pane mode uses that same split flow. Keyboard navigation is never an assignment operation: every tiled layout must survive Tab/Shift+Tab navigation unchanged. If a standalone tab is active, an assigned tab selected next makes its workspace live and focuses its pane.

### 4.2 Splitting

1. Request either a left/right split or a top/bottom split of the focused pane.
2. Refuse the request with an explanation if the prospective layout cannot meet its minimum sizes.
3. Present a picker of unassigned tabs, plus a path to open a new tab through the existing creation flow. Assigned tabs are not offered as duplicate views. Existing tab-type/account eligibility restrictions still apply.
4. After a valid selection, split the original pane 50/50. Keep its existing tab on the left/top; place the selected tab on the right/bottom.
5. Focus the new pane. Pane mode remains active until explicitly dismissed.

Do not create an empty pane while the picker is open. Cancellation or failure leaves the existing layout, focus, and manual zoom unchanged. Revalidate the target pane, selected tab, uniqueness, and available space at commit time; stale results must not modify a replacement pane or tab.

### 4.3 Closing and collapsing

| Action | Required result |
| --- | --- |
| Close a pane in a multi-pane layout | Remove the pane, retain its tab as unassigned, and expand the sibling region into the removed split's region. Do not run tab-close cleanup. |
| Close the sole pane | No-op; it must remain possible to view an open tab. |
| Close an assigned tab when multiple panes exist | Run normal tab closure, remove its pane, and expand the sibling region. Do not automatically fill the pane from unassigned tabs. |
| Close an unassigned tab | Run normal tab closure without changing the layout. |
| Close the tab in the sole pane | Activate another open tab using the existing tab-order fallback, or show the empty workspace if none remain. |

Collapsing a split preserves the surviving sibling's subtree and relative proportions. If the focused pane is removed, focus the first pane in screen-reading order within the expanding sibling region. Removing an unfocused pane must not steal focus from a surviving focused pane.

Closing a tab and closing a pane remain separate commands. Existing close-tab bindings must not be silently repurposed as close-pane bindings.

## 5. Keyboard controls and focus

### 5.1 Pane mode

Default entry is **F2**. Show an explicit pane-mode indicator and contextual command hints. **Escape** or **F2** exits to normal interaction with the currently focused tab, retaining that tab's own interaction mode.

Pane mode persists across pane commands and picker completion. While active, it must not forward typing, send-message keys, search edits, or unhandled keys into chat. Unknown printable keys are ignored. Ctrl+C retains its application quit behavior.

Application-level dialogs take input priority over pane controls. F2 must not manipulate a workspace behind a dialog. On dismissal, restore a valid logical pane destination and the prior pane-control state; a successful split picker instead focuses its newly created pane.

### 5.2 Default bindings

All pane bindings must be configurable through the existing `keymap.yaml` mechanism.

| Context | Default keys | Action |
| --- | --- | --- |
| Workspace, no application dialog | `f2` | Enter pane mode; a standalone tab starts its own workspace |
| Pane mode | `esc`, `f2` | Leave pane mode |
| Pane mode | `ctrl+t` | Add a new tab to the current workspace as a new pane |
| Pane mode | `left`, `h` | Focus left |
| Pane mode | `down`, `j` | Focus down |
| Pane mode | `up`, `k` | Focus up |
| Pane mode | `right`, `l` | Focus right |
| Pane mode | `H`, `J`, `K`, `L` | Move the focused pane left, down, up, or right |
| Pane mode | `v` | Split left/right; choose the new right-hand tab |
| Pane mode | `s` | Split top/bottom; choose the new bottom tab |
| Pane mode | `<`, `>` | Decrease/increase focused pane width |
| Pane mode | `-`, `+` | Decrease/increase focused pane height |
| Pane mode | `x` | Close pane, not tab |
| Pane mode | `z` | Toggle manual zoom |
| Pane mode | `tab`, `shift+tab` | Cycle panes forward/backward in screen-reading order |

Outside pane mode, existing input, suggestion, tab-navigation, and close-tab bindings retain their current scopes. Tab/Shift+Tab navigate tabs non-destructively as described in section 4.1. While a composer is active, Tab is handled by suggestion completion; while chat search is active, Tab stays in search rather than switching tabs. Do not reuse Ctrl+W as a pane prefix or Ctrl+B as a pane prefix: they already have application actions.

Use existing configuration overlay behavior: omitted new actions receive defaults, explicit overrides remain authoritative, and existing keymap files are not rewritten. Help must describe the new controls and distinguish pane closure from tab closure.

### 5.3 Navigation and moves

- Directional focus chooses the nearest pane in the requested direction. It does not wrap at workspace edges. Resolve geometric ties deterministically.
- Tab/Shift+Tab cycle through all panes in screen-reading order and wrap. Reading order is top-to-bottom by pane origin, then left-to-right for panes starting on the same row.
- Moving with a directional neighbour exchanges the focused tab with that neighbour. Keep the tree, pane dimensions, and registry storage order unchanged; the header projection and focus follow the moved tab's new pane position.
- Moving toward an outer edge with no neighbour detaches the pane and re-docks it as a new 50/50 root split on that edge. This is how a row can become a stack (or the reverse); the surviving subtree and its preferred proportions are preserved. Moving a pane that already occupies the requested root edge is a no-op so its ratio is not reset.
- Reject a re-dock if the resulting layout cannot meet the pane minima, leaving the tree unchanged. Only leaf panes move; whole subtrees and arbitrary drag targets are out of scope.
- Focus navigation at an edge remains a no-op; unlike a move, it never changes the tree.
- Navigation operates on the preserved layout even when zoom or fallback draws only one pane.

## 6. Rendering, input, and notifications

### 6.1 Workspace presentation

- Ordinary single-pane presentation retains today's appearance without permanent extra pane headers or borders. In pane mode, the shared bottom row uses the normal themed status treatment: `-- Pane --` plus compact, width-aware key hints for focus, moves, splits, resize, zoom, cycle, create, and exit. Errors, zoom, and small-terminal fallback remain visibly distinguished and clip safely on narrow terminals.
- Multi-pane layouts use thin separators and compact pane identity headers. Show the tab/channel and account identity where applicable, without inventing a single-account identity for aggregate tabs.
- Focus must be unmistakable and not depend on color alone; preserve usability with `NO_COLOR`.
- Keep one global tab list containing every open tab, supporting both horizontal and vertical settings, and one shared full-width status line supplied by the focused tab. Do not duplicate full status lines in every pane. Mark assigned tabs with workspace and pane (`W1.1`, `W2.3`) and standalone tabs with `·`; show a compact `N tiled` or `standalone` summary in the tab-list border. Pane headers use the same `Wn.pane` labels. Order workspace tabs by workspace order and pane reading order; append standalone tabs in registry order. This display order drives header hit-testing and ordinary Tab/Shift+Tab navigation while registry storage order remains unchanged. Ensure hit-testing includes badge width. There is no workspace filter or separate workspace selection: selecting a tab makes its owning workspace live. Standalone tabs remain global.
- Each channel tab retains its own composer and draft. Only the focused tab receives normal keyboard interaction; showing another tab must not recreate its model. A standalone tab uses the ordinary single-tab presentation, without pane chrome, while preserving every workspace layout behind it.
- Preserve each tab's history, scroll/selection/search state, reply/composer state, local filters, and tab-local modes across focus, replacement, move, and hide/show operations.
- Inspect, search, emote overview, and other tab-local content stay bounded by their pane. Application-level help, account, and join dialogs remain global overlays, blocking input behind them and restoring a valid focus destination afterward.
- Manual zoom and small-terminal fallback must be visibly distinguishable from an ordinary one-pane layout.

### 6.2 Geometry and mouse routing

A single layout result must drive pane drawing, child sizing, and hit-testing. Account for global chrome, pane chrome, and separators exactly once. Express bounds in terminal cells, not byte lengths, and preserve correct handling of ANSI styling, wide Unicode characters, and multi-row graphical emotes/badges.

- Left-click a pane to focus it, then apply that same click to its pane-local target. Existing message selection, double-click inspection, and composer interactions continue to work.
- Wheel events target the pane under the pointer without changing keyboard focus.
- Translate events to the tab content's local coordinates, excluding pane headers/separators and global chrome. Those cells must never masquerade as chat content.
- Existing tab-list mouse behavior applies the tab-selection rules in section 4. Global dialogs prevent clicks or scrolling from reaching panes behind them.
- Respect `mouse: false` and preserve terminal-native selection behavior.
- Hidden logical panes must not receive hit-tested mouse events through the currently rendered zoom/fallback pane.

### 6.3 Notifications

Keep existing notification triggers. A visible but unfocused tab may still have its normal notification indicator. Acknowledge it by focusing the tab, not by rendering it or hover-scrolling it. Normal event delivery and aggregation must continue for unassigned tabs.

## 7. Sizing and presentation modes

### 7.1 Layout sizing

- New splits start 50/50. Store preferred proportions rather than fixed terminal dimensions.
- Width/height resize commands adjust the nearest applicable split on that axis in **5-percentage-point steps**, in the direction that shrinks/grows the focused region. A divider can resize a whole subtree, not just an individual leaf.
- Clamp resizing to satisfy both child regions' minimum sizes. With no applicable divider or no legal movement, leave the layout unchanged.
- The initial multi-pane minimum is **24 columns × 8 rows per pane**, including that pane's chrome. Global chrome is outside this allowance; separators consume their actual additional space.
- Calculate subtree minima recursively. Side-by-side children need their combined minimum widths plus any divider, while stacked children need their combined minimum heights plus any divider.
- Respect integer-cell bounds without overlaps, gaps caused by inconsistent allocation, negative dimensions, or cumulative rounding drift.
- Terminal shrink/grow preserves preferred proportions. Temporary clamping or presentation fallback must not overwrite the preferred tree or ratios.
- Available space limits pane count; do not add an unrelated fixed maximum. The multi-pane minimum must not prevent a single tab from using a smaller terminal with safe clipping.

### 7.2 Manual zoom

Manual zoom draws the focused pane across the workspace while preserving the tree. Focus/tab navigation follows the rules above and keeps the newly focused pane zoomed. Unzoom returns to the preserved arrangement.

Successful layout edits—split, move, resize, or removal of an assigned pane—leave manual zoom and show the resulting tiled arrangement, subject to automatic fallback. A cancelled, rejected, or no-op operation must not destroy the layout or unexpectedly change its presentation. Do not persist manual zoom across application restarts.

### 7.3 Automatic small-terminal fallback

If the full layout cannot meet its minima after startup or terminal resize, temporarily draw only the focused pane and explain why. Do not close tabs, remove panes, or discard proportions to fit.

| Operation during fallback | Behavior |
| --- | --- |
| Navigate panes or select tabs | Allowed; update the focused single-pane presentation using normal assignment rules |
| Close panes or tabs | Allowed; re-evaluate whether the remaining layout fits |
| Split, move, or resize | Refused with an explanation; do not edit the unseen layout |
| Toggle manual zoom | Cannot defeat the automatic fit requirement |
| Terminal becomes large enough | End automatic fallback and show the preserved layout, unless an existing manual-zoom choice still applies |

Keep single-pane clipping safe even when the terminal is smaller than 24×8. An explanation must not itself overflow the terminal or steal chat input. Restore tiling immediately when closure or terminal growth makes it fit.

## 8. Persistence and compatibility

Extend normal saved-session state with optional layout metadata describing split orientation, preferred proportions, tab assignments, and logical focused tab/pane.

- Preserve existing tab records, ordering, filters, account associations, and numeric tab-kind values.
- Layout references must map reliably to restored tab records. Do not rely solely on runtime-generated tab IDs surviving a restart.
- Restore logical focus even if a global dialog had temporarily blurred the underlying tab when a snapshot was taken.
- If a standalone tab was active, persist it as the focused tab while keeping serialized layout focus on an assigned pane. Restore it standalone without promoting it into a pane; navigating to an assigned tab restores the tiled workspace.
- Do not persist pane mode, open dialogs, manual zoom, or automatic fallback. Recompute fit from the current terminal after restoration.
- Older state with no layout metadata restores the existing active-tab selection in a single pane; it must not automatically tile all restored tabs.
- Remove references to unavailable tabs and collapse redundant splits, retaining valid branches and preferred proportions where possible. Repair duplicate assignments or other structurally invalid references without violating the invariants.
- Restore saved focus when its pane survives; otherwise choose a valid surviving pane deterministically. If no usable layout remains, show a single normally restored tab, or the empty workspace if no tabs survive.
- Invalid layout metadata must not discard otherwise valid tab records or fail startup solely because the layout is unusable. Report repair/fallback through a warning. Existing handling of unrelated account, network, and whole-file errors remains unchanged.
- Detached `--tab` sessions continue not to load or save session state. They start with the normal single-pane presentation and may be tiled in memory without touching the persisted layout.

The concrete serialized field names and Go representation are implementation details; the compatibility and recovery behaviors above are required.

### 8.1 Multiple workspaces

- Workspaces are an implementation detail of tiling screens, not a user-managed collection. There is no workspace picker, naming UI, creation command, deletion command, or separate workspace selection.
- Every tab stays in the global tab list. Selecting a tab makes its owning workspace live automatically; selecting a standalone tab leaves the last workspace preserved in the background.
- A tab belongs to at most one workspace. Sharing a broadcast across workspaces requires a separate tab in the other workspace, with its own tab state; connections and caches may still be shared.
- Normal tab creation outside pane mode opens a standalone tab and does not assign it. Ctrl+T in pane mode adds a tab to the current workspace as a new pane. F2 on a standalone tab creates or reuses an empty workspace and makes that tab its first pane; F2 on an assigned tab enters its existing workspace.
- Empty workspaces may be reused. Closing the last pane of a workspace leaves an empty workspace available for the next implicit creation; it is not presented as a user-managed deletion operation. Renaming and custom ordering are not part of the model.
- Per-workspace layout proportions and focus are preserved and restored. Tabs in inactive workspaces continue to receive their normal background events and remain fully visible in the global tab list.
- Persist a versioned workspace set (IDs/names, layouts, per-workspace focus) plus the global focused tab. The active workspace is derived from that tab on restore, not chosen from a stored selection. Migrate legacy single-layout state into `W1`; repair duplicate ownership, missing tabs, invalid layouts, and invalid workspace references without dropping valid tabs.
- Detached sessions start with a fresh default workspace and never load or persist a workspace set.

## 9. Integration constraints

The current baseline is the merged UI ownership refactor, not its earlier split-file sketches. Relevant existing seams are:

| Existing code | Required responsibility |
| --- | --- |
| [`ui/workspace/layout.go`](../ui/workspace/layout.go) | Pure split-tree operations, sizing, navigation, and validated layout snapshots; no independent active-tab state. |
| [`ui/mainui/workspace.go`](../ui/mainui/workspace.go) | Root's workspace coordination, rendering, pane controls, transactional splits, and layout persistence. |
| [`ui/mainui/pane_picker.go`](../ui/mainui/pane_picker.go) | Ticketed tab selection without creating a pane before confirmation. |
| [`ui/nav/registry.go`](../ui/nav/registry.go) | Open-tab identity, order, activation, notifications. Do not duplicate its tab list or silently use tab-order focus fallback where pane-sibling focus is required. |
| [`ui/nav/stack.go`](../ui/nav/stack.go) | Global dialog lifecycle, rendering, and topmost-screen input ownership. |
| [`ui/mainui/root.go`](../ui/mainui/root.go) | Apply workspace changes, coordinate tab activation, integrate rendering/resizing, and snapshot state. |
| [`ui/mainui/mouse.go`](../ui/mainui/mouse.go) | Route through pane hit-testing and translate content-local coordinates. |
| [`ui/tab/tab.go`](../ui/tab/tab.go) and [`ui/tab/broadcast.go`](../ui/tab/broadcast.go) | Keep tab lifecycle, internal modes, draft/history ownership, and local rendering inside the existing tab model. |
| [`ui/mainui/startup.go`](../ui/mainui/startup.go) and [`save/app.go`](../save/app.go) | Restore tabs and then resolve optional layout metadata; preserve detached-session and recovery contracts. |
| [`save/key.go`](../save/key.go) | Configurable pane bindings and existing keymap compatibility. |

Keep the Bubble Tea architecture. Apply model changes through the update flow; do not add a separate window-manager event loop. Coordinate registry activation, layout changes, and focus as one coherent transition, without acknowledging unrelated tabs through intermediate activation. Keep the existing `Broadcast` implementation consolidated in `broadcast.go`. Isolate workspace tree operations, sizing, and navigation so they can be tested independently of network calls and terminal side effects.

This feature must preserve the existing enrichment queue, preparation ordering, cancellation, account/channel routing, and Kitty-before-delivery guarantees. It does not require replacing those mechanisms. Only actual tab closure triggers tab-close cleanup and existing late-result rejection; pane hiding must not impersonate tab closure.

## 10. Acceptance criteria

### Automated coverage

| ID | Scenario and required outcome |
| --- | --- |
| AC-01 | With no layout metadata, multiple open tabs still produce the current single-tab presentation and existing active-tab fallback. |
| AC-02 | Nested left/right and top/bottom splits display distinct existing tabs, including Mention and Live Notification tabs. New panes appear right/bottom and gain focus. |
| AC-03 | Split cancellation, failed creation, insufficient space, or stale picker results create no empty pane and leave the prior layout/focus/zoom intact. |
| AC-04 | Tab/Shift+Tab and tab clicks are non-destructive: assigned tabs focus their pane and automatically make the owning workspace live; standalone tabs show full-screen with every layout preserved. Clicking a tab never assigns it to a pane. Include zoomed/fallback-hidden assignments. |
| AC-05 | Close-pane retains its tab, draft, and history without calling tab-close cleanup; collapse preserves the sibling subtree. The sole pane cannot be hidden. |
| AC-06 | Close-tab removes its pane and runs normal cleanup. Hidden-tab closure does not alter layout. Sole-tab-pane closure follows normal tab-order fallback or shows the empty workspace. |
| AC-07 | Directional navigation stops at edges; cycling wraps in reading order; focused closure selects the expanding sibling's first pane; unfocused closure preserves focus. |
| AC-08 | Directional moves swap with an adjacent pane or re-dock at an outer edge as a 50/50 split, can change row/stack orientation, preserve the surviving subtree, tab-local state, and registry storage order, and reorder the header projection to match pane reading order. Reject no-fit moves and no-op at an occupied root edge. |
| AC-09 | Pane mode works while a tab was composing/searching, consumes unhandled keys without changing drafts or sending, remains active across commands, exits explicitly, and preserves Ctrl+C. |
| AC-10 | Outside pane mode, tab navigation, suggestion keys, close-tab bindings, and existing text-input behavior are unchanged. Omitted new keymap actions get defaults without rewriting overrides. |
| AC-11 | Global dialogs own input and restore valid focus/control state afterward, including when a previously targeted tab/pane disappears. Split-picker success focuses the new pane. |
| AC-12 | Only the focused tab receives normal keyboard input. Other panes continue to receive background events without duplicate insertion or extra connections. |
| AC-13 | Click-to-focus applies the same click locally; hover-scrolling affects the hovered pane without focus or notification acknowledgement. Cover pane chrome, separators, global headers, disabled mouse, and dialogs. |
| AC-14 | Focus clears normal notifications; visibility, zoom changes, and hover-scrolling do not acknowledge unfocused tabs. |
| AC-15 | Rendering, child sizes, and hit-tests agree for both tab-list orientations, nested splits, Unicode/ANSI content, and odd terminal dimensions. Nothing spills into adjacent panes or global chrome. |
| AC-16 | Resize steps, recursive minima, and split rejection enforce 24×8 pane minima without an unrelated pane-count cap. Requests with no applicable divider or no legal movement are no-ops. |
| AC-17 | Repeated shrink/grow cycles preserve the tree and preferred ratios, avoid rounding drift, and safely handle zero/tiny allocations. |
| AC-18 | Manual zoom follows focus and restores the arrangement. Successful layout edits leave zoom; cancellation/rejection/no-op preserves it. |
| AC-19 | Too-small-terminal fallback preserves all assignments, permits navigation/closure, blocks split/move/resize, and ends when the layout fits. Only the drawn pane receives mouse events. |
| AC-20 | Session round trips retain assignments, proportions, tab order, and logical focus, including snapshots while a global dialog is open. Temporary presentation/control modes are not saved. |
| AC-21 | Old session files, missing tabs/accounts, invalid layout metadata, duplicate references, and lost focused panes recover as specified without dropping otherwise valid tabs. |
| AC-22 | Detached sessions neither read nor write saved layout state, even after in-memory splits, moves, zoom, and closure. |
| AC-23 | Graphical emotes/badges, tab-local inspect/search/overview, and retained selections remain bounded and usable through resize, move, zoom, and hide/show. Existing preparation/shutdown ordering regressions remain passing. |
| AC-24 | Tab/Shift+Tab while a composer is active is handled by suggestion completion; while chat search is active it stays in search and does not change tabs or panes. |
| AC-25 | A standalone tab can be saved and restored without being promoted into a pane; selecting an assigned tab afterward shows the preserved tiled workspace. |
| AC-26 | The tab list shows every tab; assigned rows use `Wn.pane` badges and standalone rows use `·`. Workspace rows follow pane reading order, standalone rows follow them, and moving panes reorders the header without changing registry storage order. The tiled/standalone border summary tracks the active workspace; badge-aware hit-testing selects the correct tab. |
| AC-27 | Multiple workspaces are created implicitly by entering pane mode on standalone tabs. Tab switching activates the owning workspace automatically; every tab remains visible; Ctrl+T outside pane mode stays standalone and Ctrl+T in pane mode adds a pane to the current workspace. |
| AC-28 | Versioned multi-workspace state restores layouts, focus, the global focused tab, and standalone tabs; the active workspace is derived from the focused tab. Legacy state opens as W1. Empty workspaces are reused. |
| AC-29 | Pane mode renders a themed `-- Pane --` status row with compact, width-aware hints for configured focus/move/split/resize/zoom/cycle/create/close/exit keys. Hints drop progressively on narrow terminals; error, zoom, and small-terminal states remain visible and never overflow. |

Place Go tests in the canonical test file for their source: `foo.go` → `foo_test.go`. Extend those files rather than adding separate lifecycle, regression, geometry, or context test files for the same source.

### Real-terminal validation before merge

Automated geometry tests are necessary but not sufficient. Verify:

- Kitty graphical emotes/badges in adjacent and stacked panes, including configured multi-row image scales, resize, zoom, and fallback; no corruption or spill into neighbouring panes.
- Mouse selection, double-click inspect, composer clicks, hover-scrolling, separators, and global-dialog blocking in both tab-list orientations.
- Keyboard pane controls from normal, insert, search, and inspect states; no cross-chat typing or accidental sends.
- Clear account/channel destination and focused-pane indication with color enabled and `NO_COLOR`.
- Saved-layout and multi-workspace restart at a different terminal size, plus detached-session isolation.

Update user-facing feature/settings/help documentation when implementing the feature. This specification alone must not be presented as a claim that tiling is already available.
