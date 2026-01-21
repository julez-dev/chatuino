# PRD: Join Modal UI/UX Redesign

**Date:** 2026-01-20

---

## Problem Statement

### What problem are we solving?

The current join screen in Chatuino is a full-screen overlay that feels outdated and has UX friction points:

1. **Visual design**: Uses basic ASCII borders, lacks modern polish (shadows, rounded corners, visual hierarchy)
2. **Screen real estate**: Takes over entire terminal when only needing small portion
3. **Confirm button**: Ugly, requires extra Tab navigation to reach, feels clunky
4. **Keybind confusion**: Enter key has dual purpose (autocomplete in input vs confirm on button), creating cognitive overhead
5. **No visual polish**: Doesn't leverage modern terminal capabilities or existing theme data

**User Impact**: Joining channels feels dated and requires more keystrokes than necessary. Every join action reinforces impression of unpolished UX.

**Business Impact**: First impression issue - joining channels is one of the first actions new users take. Poor UX here sets wrong tone.

### Why now?

The `bubbletea-overlay` library provides modal rendering capabilities that make this feasible without reimplementing compositing logic. This is the right time to modernize before user base grows.

### Who is affected?

- **Primary users**: All Chatuino users joining new channels/tabs (core workflow)
- **Secondary users**: New users evaluating Chatuino (first impression)

---

## Proposed Solution

### Overview

Replace the full-screen join overlay with a modern, centered modal dialog using `github.com/rmhubbert/bubbletea-overlay`. The modal will be percentage-based (resizes with terminal), feature rounded borders, shadow/blur effects, and streamlined keyboard navigation that eliminates the confirm button in favor of direct Enter key submission.

### User Experience

#### Visual Design

**Modal Presentation**:
- Centered modal overlay (percentage-based: ~60% width, auto height based on content)
- Rounded borders using lipgloss
- Shadow/blur effect behind modal (dimmed background)
- Existing theme colors (respects user's `Theme` configuration)
- Dynamic resize handling (terminal window changes)

**Layout Structure**:
```
[Dimmed background showing current chat]

    ╭─────────────────────────────────╮
    │  Join Channel                   │
    │                                 │
    │  > Tab type                     │
    │    • Channel                    │
    │    • Mention                    │
    │    • Live Notifications         │
    │                                 │
    │  > Identity                     │
    │    • account1                   │
    │    • account2                   │
    │                                 │
    │  > Channel                      │
    │    xqc|                         │
    │                                 │
    │  [Space: autocomplete]          │
    │  [Enter: confirm]               │
    ╰─────────────────────────────────╯
```

**Field Selection Indicator**:
- Selected field shows `>` prefix (existing pattern)
- Selected field label uses theme's `ListLabelColor`
- Non-selected fields appear dimmed

#### User Flow: Join Channel Tab

1. User presses `Ctrl+T` from main screen
2. Modal fades in centered over chat (background dimmed)
3. Focus starts on "Tab type" selector
4. User navigates with Tab/Shift+Tab between: Tab type → Identity → Channel
5. In channel input: Space accepts autocomplete suggestion, typing continues search
6. User presses Enter when inputs valid → channel joined, modal dismissed
7. If Enter pressed with invalid state → no action (could add error hint)

#### User Flow: Join Mention/Live Notification Tab

1. User presses `Ctrl+T` from main screen
2. Modal appears with only Tab type selector visible
3. User selects "Mention" or "Live Notifications" with arrow keys
4. User presses Enter → tab created, modal dismissed
5. Identity and Channel fields never shown (not needed for these types)

### Design Considerations

**Accessibility**:
- All interactions keyboard-driven (existing pattern maintained)
- Clear visual focus indicators
- Status hints at bottom ("Space: autocomplete | Enter: confirm")

**Terminal Compatibility**:
- Graceful degradation if terminal doesn't support advanced rendering
- Fallback to simpler borders if rounded/shadow unavailable
- Works in standard 80x24 minimum terminal size

**Theme Integration**:
- Uses existing `Theme.ListLabelColor` for accents
- Respects `Theme.BorderColor` if available (or add to theme)
- Background dim color configurable via theme

---

## End State

When this PRD is complete, the following will be true:

- [ ] Join screen renders as centered modal using `bubbletea-overlay`
- [ ] Modal is percentage-based (60% width, dynamic height) and handles terminal resizing
- [ ] Modal features rounded borders, shadow/blur effect on background
- [ ] Modal respects existing theme configuration colors
- [ ] Confirm button removed entirely from UI
- [ ] Enter key confirms join when inputs are valid (any field)
- [ ] Space key accepts autocomplete suggestion in channel input
- [ ] Tab/Shift+Tab navigation between fields works identically to current
- [ ] Arrow keys work in list selectors (tab type, identity) unchanged
- [ ] Tab type selector visual styling improved (modern list appearance)
- [ ] Invalid state prevents Enter confirmation (silent, no error shown v1)
- [ ] Existing autocomplete behavior unchanged (used elsewhere)
- [ ] ESC key dismisses modal and returns to main screen
- [ ] All existing join functionality preserved (API calls, channel normalization)
- [ ] Tests updated to cover new modal behavior
- [ ] Terminal resize events properly reflow modal positioning

---

## Success Metrics

### Qualitative
- User (you) approval: "looks more modern and fancy with better UX"
- Visual consistency with modern TUI applications (e.g., lazygit, k9s modals)
- Reduced cognitive load (fewer steps, clearer actions)

---

## Tasks

### Dependencies Setup [setup]
Add bubbletea-overlay library dependency.

**Verification:**
- Run `go get github.com/rmhubbert/bubbletea-overlay@v0.6.4`
- Verify `go.mod` contains overlay dependency
- Run `go mod tidy`
- Verify project builds: `go build`

### Modal Structure Conversion [ui]
Convert join screen from full-screen to modal structure.

**Verification:**
- `ui/mainui/join.go` struct updated to work as modal foreground
- View() method returns modal content (without full terminal dimensions)
- Width/height calculations use percentage of parent (60% width)
- Join modal View() renders within bounded dimensions
- No full-screen layout code remains in join.go

### Overlay Integration in Root [ui]
Integrate overlay wrapper in root model to composite join modal over active tab.

**Verification:**
- `ui/mainui/root.go` imports `github.com/rmhubbert/bubbletea-overlay`
- When inputScreen active, root.View() creates overlay.New() with:
  - Background: current active tab (r.tabs[r.tabCursor])
  - Foreground: join modal (r.joinInput)
  - Position: overlay.Center, overlay.Center
  - Offsets: 0, 0
- Overlay model's View() called instead of direct joinInput.View()
- Terminal resize (tea.WindowSizeMsg) updates both background and foreground models
- Manual test: Open join modal, verify chat visible behind it

### Modal Visual Styling [ui]
Apply rounded borders, dimmed background, and theme colors to modal.

**Verification:**
- Join modal border uses lipgloss.RoundedBorder()
- Modal background uses theme color (Theme.ListLabelColor or similar)
- Border color uses existing theme color
- Manual test: Modal has rounded corners (╭─╮ ╰─╯)
- Manual test: Background appears dimmed/muted behind modal
- Manual test: Colors match existing theme configuration

### Remove Confirm Button [ui]
Remove confirm button UI element and navigation state.

**Verification:**
- `confirmButton` const removed from currentJoinInput enum
- Join View() no longer renders confirm button
- Navigation no longer includes confirmButton in Tab cycle
- Manual test: Tab cycles only through: tabSelect → accountSelect → channelInput
- Manual test: No visible confirm button in modal

### Enter Key Confirmation [ui]
Implement Enter key to confirm join from any field when inputs are valid.

**Verification:**
- Enter key triggers confirmation when:
  - Channel input has value AND tab type is Channel, OR
  - Tab type is Mention/Live Notification (no channel needed)
- Enter key does nothing (silent) when inputs invalid
- Confirmation logic identical to old confirm button (channel normalization via API)
- Manual test: Enter in channel input with valid name → join succeeds
- Manual test: Enter in tab selector with Mention selected → join succeeds
- Manual test: Enter with empty channel on Channel tab → no action

### Space Key Autocomplete [ui]
Implement Space key to accept autocomplete suggestion in channel input.

**Verification:**
- Space key in channel input accepts current autocomplete suggestion
- Space key only active when channel input focused
- Space key does not interfere with tab/identity list navigation
- After accepting suggestion, cursor positioned after inserted text
- Manual test: Type "xq", press Space → "xqc" inserted (if suggestion exists)
- Manual test: Press Space in tab selector → no autocomplete action

### Keybind Documentation [ui]
Add status hints showing available keybinds at bottom of modal.

**Verification:**
- Modal View() includes hint bar at bottom
- Hint shows context-appropriate keybinds:
  - When channel input focused: "Space: autocomplete | Enter: confirm | Tab: next field"
  - When other field focused: "Enter: confirm | Tab: next field"
- Hint bar styled consistently with theme
- Manual test: Hints visible and update based on focused field

### Terminal Resize Handling [ui]
Ensure modal repositions and resizes correctly on terminal window changes.

**Verification:**
- Root Update() handles tea.WindowSizeMsg when inputScreen active
- Both joinInput and background tab receive window size updates
- Join modal recalculates 60% width based on new terminal width
- Overlay repositions to center after resize
- Manual test: Resize terminal while modal open → modal stays centered
- Manual test: Content remains readable at minimum 80x24 size

### Testing Coverage [testing]
Add tests for new modal behavior and keybinds.

**Verification:**
- Test: Enter key with valid channel input triggers joinChannelMessage
- Test: Enter key with empty channel on Channel tab does nothing
- Test: Enter key on Mention tab (no channel) triggers joinChannelMessage
- Test: Space key in channel input with suggestion accepts it
- Test: Tab navigation cycles through fields correctly (no confirm button)
- Test: Window resize updates modal dimensions
- Run `go test ./ui/mainui/... -v` → all tests pass

---

## Technical Context

### Existing Patterns

**Modal/Overlay Precedent**:
- Help screen (`ui/mainui/help.go`) uses full-screen overlay pattern
- Join screen (`ui/mainui/join.go`) uses full-screen overlay pattern
- Both managed via `activeScreen` enum in root model
- **New pattern**: Use `bubbletea-overlay` library for proper modal compositing

**Bubble Tea Model Lifecycle**:
- `ui/mainui/root.go:468-498` - Join screen creation on `Ctrl+T`
- `ui/mainui/root.go:616-636` - Screen rendering switch statement
- `ui/mainui/join.go:Init()` - Async followed channels fetch
- `ui/mainui/join.go:Update()` - Keybind handling, field navigation
- `ui/mainui/join.go:View()` - Rendering logic

**Theme Integration**:
- `save/theme.go` - Theme struct with colors
- Components use `deps.UserConfig.Theme.ListLabelColor`
- Lipgloss styles reference theme colors

### Key Files

**Join Modal Implementation**:
- `ui/mainui/join.go` - Current full-screen join screen (to be converted to modal)
- `ui/mainui/root.go` - Manages screen state, dispatches to join screen
- `ui/component/textinput.go` - Autocomplete input component (unchanged, reused elsewhere)

**Modal Integration**:
- NEW: Import `github.com/rmhubbert/bubbletea-overlay`
- `ui/mainui/root.go` - Wrap join model with overlay model
- Background model: current active tab's chat view
- Foreground model: new modal join view

**Styling**:
- `save/theme.go` - Theme configuration (may need new fields: `ModalBorderColor`, `ModalShadowColor`)
- Lipgloss styles in join.go View() method

**Dependencies**:
- `github.com/rmhubbert/bubbletea-overlay` - Modal compositing
- `github.com/charmbracelet/lipgloss` - Rounded borders, styling
- `github.com/charmbracelet/bubbles/list` - Tab type/identity selectors

### System Dependencies

**Go Modules**:
- Add: `github.com/rmhubbert/bubbletea-overlay` (latest: v0.6.4)

**Twitch API**:
- Existing: `FetchUserFollowedChannels` for autocomplete population
- Existing: `GetUsers` API for channel name normalization

**Data Model Changes**:
None - all existing data structures preserved.

### Overlay Library Usage

**Positioning Example** (from bubbletea-overlay docs):
```go
bgModel := r.tabs[r.tabCursor]  // Current tab chat view
fgModel := r.joinInput          // New modal join view
overlayModel := overlay.New(
    fgModel,
    bgModel,
    overlay.Center,  // X position
    overlay.Center,  // Y position
    0,               // X offset
    0,               // Y offset
)
```

**Update Pattern**:
- Overlay does NOT auto-update child models
- Root must dispatch updates to both background and foreground
- Example: `r.joinInput, cmd = r.joinInput.Update(msg)` then render via overlay

**View Pattern**:
- `overlayModel.View()` composites foreground onto background
- Margins/positioning on foreground lipgloss styles will conflict (use overlay positioning instead)

---

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Terminal doesn't support rounded borders | Medium | Low | Graceful fallback to ASCII borders (lipgloss handles this) |
| Blur/shadow effect performance issues | Low | Medium | Make shadow optional via theme config, default on |
| Overlay library bugs/limitations | Medium | High | Test with various terminal sizes early, have fallback to full-screen pattern |
| Space key breaks list navigation | Low | Medium | Space only active when channel input focused, lists use arrow keys |
| Enter validation logic complex | Low | Low | Simple boolean check: `channelInput != "" OR isMentionTab OR isLiveNotificationTab` |
| Terminal resize causes visual artifacts | Medium | Medium | Test resize thoroughly, ensure overlay recalculates on `tea.WindowSizeMsg` |

---

## Alternatives Considered

### Alternative 1: Keep Full-Screen, Just Improve Styling
- **Description**: Keep existing full-screen pattern, add rounded borders and better colors
- **Pros**: Less code change, no new dependency
- **Cons**: Still wastes screen space, doesn't address "modern" feel of modal overlays
- **Decision**: Rejected - modal pattern is core to modernization goal

### Alternative 2: Build Custom Overlay Compositing
- **Description**: Implement own string compositing logic (like Superfile)
- **Pros**: No external dependency, full control
- **Cons**: Reinventing wheel, bubbletea-overlay is well-tested and maintained
- **Decision**: Rejected - bubbletea-overlay exists and works

### Alternative 3: Remove Autocomplete Entirely
- **Description**: Simplify by removing autocomplete, just type channel name
- **Pros**: Simpler keybind model, no Space vs Tab confusion
- **Cons**: Autocomplete is valuable UX (followed channels list is long)
- **Decision**: Rejected - autocomplete is feature, not problem

### Alternative 4: Enter Confirms, Tab Autocompletes
- **Description**: Swap Space and Tab roles (Tab for autocomplete instead of navigation)
- **Pros**: More common keybind pattern
- **Cons**: Breaks muscle memory, Tab navigation is Chatuino convention, up/down already used by lists
- **Decision**: Rejected - Keep Tab navigation, Space for autocomplete is acceptable

---

## Non-Goals (v1)

Explicitly out of scope for this PRD:

- **Error messages on invalid Enter** - v1 silent fail, v2 can add red text hint
- **Animation (fade in/slide)** - Defer to v2, focus on static visual improvement first
- **Edit existing channel tabs** - Separate feature, this PRD only covers joining new
- **Autocomplete component refactor** - Used elsewhere, changing it risks breaking other inputs
- **Custom theme fields beyond existing** - Try to reuse existing theme colors first
- **Mobile/SSH client testing** - Assume standard terminal emulators, expand later
- **Accessibility beyond keyboard navigation** - Screen reader support deferred
- **Join history / recent channels** - Separate feature, not part of modal redesign

---

## Open Questions

| Question | Owner | Due Date | Status |
|----------|-------|----------|--------|
| Should modal have title bar "Join Channel"? | User | Before implementation | Open |
| Background dim: lipgloss alpha or overlay lib feature? | Developer | During implementation | Open |
| Add `ModalBorderColor` to theme or reuse `ListLabelColor`? | User/Developer | Before implementation | Open |
| Should invalid Enter show subtle red border flash? | User | Before implementation | Open |

---

## Appendix

### Glossary
- **Modal**: Overlay dialog that dims background, centers content, requires dismissal
- **Autocomplete**: Inline suggestion system using Trie for followed channels
- **Tab type**: Channel (specific channel chat), Mention (cross-channel mentions), Live Notification (go-live alerts)
- **Overlay compositing**: Rendering foreground string onto background string with positioning

### References
- bubbletea-overlay library: https://github.com/rmhubbert/bubbletea-overlay
- Current join screen: `ui/mainui/join.go`
- Lipgloss styling: https://github.com/charmbracelet/lipgloss
- Chatuino AGENTS.md knowledge base: Project root
