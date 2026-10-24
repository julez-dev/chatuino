package tab

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/search"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/rs/zerolog/log"
)

const (
	cleanupAfterMessage float64 = 800.0
	cleanupThreshold            = int(cleanupAfterMessage * 1.5)
	// prefixPadding               = 41
	prefixPadding = 0

	// Smooth scroll animation parameters.
	smoothScrollFPS        = 60
	smoothScrollInterval   = time.Second / smoothScrollFPS
	smoothScrollLerpFactor = 0.08 // per-frame interpolation factor
	smoothScrollSnap       = 0.2  // snap when within this many lines of target
)

// smoothScrollTick drives the viewport scroll animation.
// The owner pointer scopes ticks to the originating chatWindow so other
// windows (tabs, user inspect) don't process ticks meant for a different window.
type smoothScrollTick struct {
	owner *chatWindow
}

func (c *chatWindow) smoothScrollTickCmd() tea.Cmd {
	return tea.Tick(smoothScrollInterval, func(time.Time) tea.Msg {
		return smoothScrollTick{owner: c}
	})
}

type chatEntry struct {
	Position   position
	Selected   bool
	IsDeleted  bool
	Event      ChatEventMessage
	IsFiltered bool // message is filtered out by search
}

type position struct {
	CursorStart int
	CursorEnd   int
}

// searchMatcher tests whether a PrivateMessage satisfies a search criterion.
// Defined here at the point of use; implemented by the search package.
type searchMatcher interface {
	Match(msg *twitchirc.PrivateMessage) bool
}

type chatWindowState int

const (
	viewChatWindowState chatWindowState = iota
	searchChatWindowState
)

type chatWindow struct {
	settings save.Settings
	keymap   save.KeyMap

	width, height int

	focused bool
	state   chatWindowState

	cursor             int
	lineStart, lineEnd int

	// Smooth scroll: smoothLineStart interpolates toward lineStart.
	smoothLineStart float64
	animating       bool

	// Entries keep track which actual original message is behind a single row.
	// A single message can span multiple lines so this is needed to resolve a message based on a line
	entries []*chatEntry

	// Every single row, multiple rows may be part of a single message
	lines []string

	// formatter owns message formatting and the caches derived from it
	renderer *messageRenderer
	// palette is shared with the composer and the user inspect window, so a user
	// keeps the same color everywhere
	palette *userPalette

	searchInput textinput.Model

	// cached filtered entries for search mode; invalidated when entries/filters change
	filteredEntries      []*chatEntry
	filteredEntriesDirty bool

	// parsed search matcher; nil when query is empty or invalid
	currentMatcher searchMatcher
	searchError    string
	matchCount     int

	// reusable buffer for visible lines in View() to avoid allocation per frame
	visibleBuf []string

	// indicator marks the selected message in the viewport
	indicator string
}

func newChatWindow(width, height int, settings save.Settings, theme save.Theme, keymap save.KeyMap) *chatWindow {
	input := textinput.New()
	input.CharLimit = 128
	input.Prompt = "  /"
	input.Placeholder = "search — content: user: /regex/ badge: is:mod|sub|vip|first"
	styles := input.Styles()
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.InputPromptColor))
	styles.Cursor.BlinkSpeed = time.Millisecond * 750
	input.SetStyles(styles)
	input.SetWidth(width)

	indicator := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ChatIndicatorColor)).Background(lipgloss.Color(theme.ChatIndicatorColor)).Render(">")

	palette := newUserPalette()

	return &chatWindow{
		settings:    settings,
		keymap:      keymap,
		width:       width,
		height:      height,
		searchInput: input,
		renderer:    newMessageRenderer(width, lipgloss.Width(indicator), settings, theme, palette),
		palette:     palette,
		indicator:   indicator,
	}
}

// UsePalette makes this window share another window's user colors. Rendering and
// the composer then resolve the same user to the same color.
func (c *chatWindow) UsePalette(palette *userPalette) {
	c.palette = palette
	c.renderer.palette = palette
}

// SetTimeFormat changes the timestamp format of rendered messages.
func (c *chatWindow) SetTimeFormat(format string) {
	c.renderer.SetTimeFormat(format)
}

func (c *chatWindow) Init() tea.Cmd {
	return nil
}

func (c *chatWindow) Update(msg tea.Msg) (*chatWindow, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case smoothScrollTick:
		if msg.owner != c || !c.animating {
			return c, nil
		}

		target := float64(c.lineStart)
		diff := target - c.smoothLineStart

		if math.Abs(diff) < smoothScrollSnap {
			c.smoothLineStart = target
			c.animating = false
			return c, nil
		}

		c.smoothLineStart += diff * smoothScrollLerpFactor
		return c, c.smoothScrollTickCmd()
	case ChatEventMessage:
		return c, c.handleMessage(msg)
	case tea.KeyPressMsg:
		if c.focused {
			switch {
			// start search (only when not already searching — otherwise '/' is forwarded to the input)
			case key.Matches(msg, c.keymap.SearchMode) && c.state != searchChatWindowState:
				return c, c.handleStartSearchMode()
			// stop search
			case key.Matches(msg, c.keymap.Escape) && c.state == searchChatWindowState:
				c.handleStopSearchMode()
				return c, nil
			case key.Matches(msg, c.keymap.Confirm) && c.state == searchChatWindowState:
				c.handleStopSearchModeKeepSelected()
				return c, nil
			// update search, allow up and down arrow keys for navigation in result
			case c.state == searchChatWindowState && msg.String() != "up" && msg.String() != "down":
				c.searchInput, cmd = c.searchInput.Update(msg)
				c.applySearch()
				cmds = append(cmds, cmd)
				return c, tea.Batch(cmds...)
			case key.Matches(msg, c.keymap.Down):
				c.messageDown(1)
				c.snapScroll()
				return c, nil
			case key.Matches(msg, c.keymap.Up):
				c.messageUp(1)
				c.snapScroll()
				return c, nil
			case key.Matches(msg, c.keymap.PageUp):
				c.pageUp()
				c.snapScroll()
				return c, nil
			case key.Matches(msg, c.keymap.PageDown):
				c.pageDown()
				c.snapScroll()
				return c, nil
			case key.Matches(msg, c.keymap.GoToBottom):
				c.moveToBottom()
				c.snapScroll()
				return c, nil
			case key.Matches(msg, c.keymap.GoToTop):
				c.moveToTop()
				c.snapScroll()
				return c, nil
			case key.Matches(msg, c.keymap.CopyToClipboard):
				_, entry := c.entryForCurrentCursor()
				if entry == nil {
					return c, nil
				}

				if text := c.clipboardText(entry); text != "" {
					return c, tea.SetClipboard(text)
				}

				return c, nil
			case key.Matches(msg, c.keymap.DumpChat):
				c.debugDumpChat()
			}
		}
	}

	if c.state == searchChatWindowState && c.focused {
		c.searchInput, cmd = c.searchInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	c.updatePort()

	return c, tea.Batch(cmds...)
}

func (c *chatWindow) handleStartSearchMode() tea.Cmd {
	c.state = searchChatWindowState
	c.searchInput.Focus()
	c.recalculateLines()
	c.snapScroll()
	return c.searchInput.Focus()
}

func (c *chatWindow) handleStopSearchModeKeepSelected() {
	_, e := c.entryForCurrentCursor()

	if e == nil {
		c.handleStopSearchMode()
		return
	}

	c.handleStopSearchMode()
	c.goToEntry(e)
}

func (c *chatWindow) handleStopSearchMode() {
	c.state = viewChatWindowState
	c.currentMatcher = nil
	c.searchError = ""
	c.matchCount = 0

	var last *chatEntry
	for e := range slices.Values(c.entries) {
		e.IsFiltered = false
		last = e
		e.Selected = false
	}

	if last != nil {
		last.Selected = true
	}
	c.searchInput.SetValue("")
	c.invalidateFilteredEntries()
	c.recalculateLines()
	c.moveToBottom()
	c.snapScroll()
}

func (c *chatWindow) View() string {
	height := c.height

	if c.state == searchChatWindowState {
		height--
	}

	if height < 1 {
		return ""
	}

	// Use smooth scroll position for rendering when animating.
	renderStart := c.lineStart
	renderEnd := c.lineEnd
	if c.animating {
		renderStart = int(math.Round(c.smoothLineStart))
		if renderStart < 0 {
			renderStart = 0
		}
		renderEnd = renderStart + height
		if renderEnd > len(c.lines) {
			renderEnd = len(c.lines)
			renderStart = renderEnd - height
			if renderStart < 0 {
				renderStart = 0
			}
		}
	}

	src := c.lines[renderStart:renderEnd]

	// Reuse a buffer to copy visible lines and apply indicator at render time.
	// This avoids mutating c.lines and eliminates per-frame allocation.
	if cap(c.visibleBuf) < height {
		c.visibleBuf = make([]string, height)
	}
	visible := c.visibleBuf[:len(src)]
	copy(visible, src)
	c.applyIndicatorToVisible(visible, renderStart)

	// Pad remaining lines with empty strings for unfilled viewport
	lines := c.visibleBuf[:height]
	for i := len(src); i < height; i++ {
		lines[i] = ""
	}

	if c.state == searchChatWindowState {
		searchLine := c.searchInput.View()

		if c.searchError != "" {
			searchLine += "  [!] " + c.searchError
		} else if c.matchCount > 0 {
			searchLine += fmt.Sprintf("  [%d matches]", c.matchCount)
		}

		return searchLine + "\n" + strings.Join(lines, "\n")
	}

	return strings.Join(lines, "\n")
}

// applyIndicatorToVisible applies the selection indicator to a copy of visible
// lines. Uses the selected entry (via binary search) or falls back to the
// bottom-most visible entry when the selected entry is outside the rendered range.
func (c *chatWindow) applyIndicatorToVisible(visible []string, renderStart int) {
	renderEnd := renderStart + len(visible)

	// Try to find the selected entry via binary search (O(log n)).
	_, target := c.entryForCurrentCursor()

	// If the selected entry is outside the visible range, fall back to the
	// bottom-most visible entry.
	if target == nil || target.Position.CursorEnd < renderStart || target.Position.CursorStart >= renderEnd {
		target = nil
		active := c.activeEntries()
		for i := len(active) - 1; i >= 0; i-- {
			e := active[i]
			if e.Position.CursorEnd < renderStart {
				break
			}
			if e.Position.CursorStart < renderEnd {
				target = e
				break
			}
		}
	}

	if target == nil {
		return
	}

	lo := max(target.Position.CursorStart, renderStart) - renderStart
	hi := min(target.Position.CursorEnd+1, renderEnd) - renderStart

	for i := lo; i < hi; i++ {
		visible[i] = c.indicator + " " + strings.TrimPrefix(visible[i], "  ")
	}
}

// Resize updates dimensions. Only recalculates lines when width changes since
// word-wrap depends on width. Height-only changes just reposition the viewport.
func (c *chatWindow) Resize(width, height int) {
	widthChanged := width != c.width
	c.width = width
	c.height = height
	c.renderer.SetWidth(width)

	if widthChanged {
		c.recalculateLines()
	} else {
		c.updatePort()
		c.snapScroll()
	}
}

func (c *chatWindow) Focus() {
	c.focused = true
}

func (c *chatWindow) Blur() {
	c.focused = false
}

// Focused reports whether this window currently owns the selection.
func (c *chatWindow) Focused() bool {
	return c.focused
}

// startAnimating begins the smooth scroll animation loop if not already running.
func (c *chatWindow) startAnimating() tea.Cmd {
	if c.animating {
		return nil
	}
	c.animating = true
	return c.smoothScrollTickCmd()
}

// snapScroll instantly sets smoothLineStart to lineStart, stopping any animation.
func (c *chatWindow) snapScroll() {
	c.smoothLineStart = float64(c.lineStart)
	c.animating = false
}

func (c *chatWindow) debugDumpChat() {
	// chat
	type state struct {
		Lines              []string
		Cursor             int
		LineStart, LineEnd int
		View               string
		Entries            []*chatEntry
		UserCache          []string
	}

	dump := state{
		//Lines:     c.lines,
		Cursor:    c.cursor,
		LineEnd:   c.lineEnd,
		LineStart: c.lineStart,
		//View:      c.View(),
		Entries: c.entries,
	}

	dump.UserCache = c.palette.Names()

	f, err := os.Create("chat_dump.json")
	if err != nil {
		panic(err)
	}

	defer func() {
		_ = f.Close()
	}()

	bytes, err := json.Marshal(dump)
	if err != nil {
		panic(err)
	}

	_, _ = f.Write([]byte(StripANSI(string(bytes))))
}

func (c *chatWindow) entryForCurrentCursor() (int, *chatEntry) {
	active := c.activeEntries()
	if len(active) == 0 {
		return -1, nil
	}

	// Binary search: entries are sorted by position. Find the first entry
	// whose CursorEnd >= cursor, then verify cursor is within its range.
	lo, hi := 0, len(active)-1
	for lo <= hi {
		mid := lo + (hi-lo)/2
		if active[mid].Position.CursorEnd < c.cursor {
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}

	if lo < len(active) && active[lo].Position.CursorStart <= c.cursor {
		return lo, active[lo]
	}

	return -1, nil
}

// entryAtLine returns the entry containing the given absolute rendered line.
// When the line falls outside every entry, the closest entry is returned.
func (c *chatWindow) entryAtLine(line int) (int, *chatEntry) {
	active := c.activeEntries()
	if len(active) == 0 {
		return -1, nil
	}

	if line <= active[0].Position.CursorStart {
		return 0, active[0]
	}

	last := active[len(active)-1]
	if line >= last.Position.CursorEnd {
		return len(active) - 1, last
	}

	lo, hi := 0, len(active)-1
	for lo <= hi {
		mid := lo + (hi-lo)/2
		if active[mid].Position.CursorEnd < line {
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}

	if lo >= len(active) {
		lo = len(active) - 1
	}

	return lo, active[lo]
}

// selectEntry makes entry the selected message and moves the cursor to its
// first line, keeping the viewport in sync.
func (c *chatWindow) selectEntry(entry *chatEntry) {
	if entry == nil {
		return
	}

	for _, e := range c.entries {
		e.Selected = false
	}

	entry.Selected = true
	c.cursor = entry.Position.CursorStart
	c.updatePort()
}

// moveSelectionByLines moves the selected message by n rendered lines, clamped
// to the first/last message.
func (c *chatWindow) moveSelectionByLines(n int) {
	active := c.activeEntries()
	if len(active) == 0 || n == 0 {
		return
	}

	i, current := c.entryForCurrentCursor()
	_, entry := c.entryAtLine(c.cursor + n)
	if entry == current {
		// A tall selected message can contain the whole wheel delta. Selecting
		// the same entry would reset the cursor to its start and make repeated
		// wheel-down events a permanent no-op. Advance to the neighbouring
		// message instead; at either end the clamp below remains a no-op.
		if n > 0 && i >= 0 && i+1 < len(active) {
			entry = active[i+1]
		} else if n < 0 && i > 0 {
			entry = active[i-1]
		}
	}
	c.selectEntry(entry)
}

func (c *chatWindow) pageUp() {
	c.moveSelectionByLines(-max(1, c.height-1))
}

func (c *chatWindow) pageDown() {
	c.moveSelectionByLines(max(1, c.height-1))
}

// scrollByLines scrolls the chat by n lines. Positive values move towards newer
// messages, negative values towards older ones.
func (c *chatWindow) scrollByLines(n int) {
	c.moveSelectionByLines(n)
}

// viewportRenderStart returns the absolute line index of the first visible row.
func (c *chatWindow) viewportRenderStart() int {
	if !c.animating {
		return c.lineStart
	}

	start := int(math.Round(c.smoothLineStart))
	if start < 0 {
		start = 0
	}

	if start+c.height > len(c.lines) {
		start = max(0, len(c.lines)-c.height)
	}

	return start
}

// selectLine selects the entry containing the given absolute rendered line.
func (c *chatWindow) selectLine(line int) {
	_, entry := c.entryAtLine(line)
	c.selectEntry(entry)
	c.snapScroll()
}

// selectAtViewportRow selects the message rendered at the given row (zero-based
// relative to the chat view).
func (c *chatWindow) selectAtViewportRow(row int) {
	if c.state == searchChatWindowState {
		row-- // first row is the search input
		if row < 0 || row >= c.height-1 {
			return
		}
	} else if row < 0 || row >= c.height {
		return
	}

	c.selectLine(c.viewportRenderStart() + row)
}

func (c *chatWindow) goToEntry(entry *chatEntry) {
	active := c.activeEntries()
	if len(active) < 1 {
		return
	}

	for i := range c.entries {
		c.entries[i].Selected = false
	}

	for _, e := range active {
		if e == entry {
			e.Selected = true
			c.cursor = e.Position.CursorEnd
			c.updatePort()
			c.snapScroll()
			return
		}
	}
}

func (c *chatWindow) messageDown(n int) {
	active := c.activeEntries()
	if len(active) < 1 {
		return
	}

	i, e := c.entryForCurrentCursor()

	if i == -1 {
		return
	}

	e.Selected = false

	i = clamp(i+n, 0, len(active)-1)

	active[i].Selected = true
	c.cursor = active[i].Position.CursorEnd

	c.updatePort()
}

func (c *chatWindow) messageUp(n int) {
	active := c.activeEntries()
	if len(active) < 1 {
		return
	}

	i, e := c.entryForCurrentCursor()

	if i == -1 {
		return
	}

	e.Selected = false

	i = clamp(i-n, 0, len(active)-1)

	active[i].Selected = true
	c.cursor = active[i].Position.CursorStart

	c.updatePort()
}

func (c *chatWindow) moveToBottom() {
	i, currentEntry := c.entryForCurrentCursor()

	if currentEntry == nil {
		return
	}

	active := c.activeEntries()
	c.messageDown(len(active) - i)
}

func (c *chatWindow) moveToTop() {
	i, currentEntry := c.entryForCurrentCursor()

	if currentEntry == nil {
		return
	}

	c.messageUp(i)
}

func (c *chatWindow) getNewestEntry() *chatEntry {
	active := c.activeEntries()
	if len(active) > 0 {
		return active[len(active)-1]
	}

	return nil
}

func (c *chatWindow) cleanup() {
	// todo: make this smarter, so we can delete more often
	// c.logger.Info().Msgf("(%d/%d)", len(c.entries), cleanupThreshold)
	if len(c.entries) < cleanupThreshold {
		return
	}

	if c.state == searchChatWindowState {
		return
	}

	if e := c.getNewestEntry(); e == nil || !e.Selected {
		log.Logger.Info().Msg("skip cleanup because not on newest message")
		return
	}

	log.Logger.Info().Int("cleanup-after", int(cleanupAfterMessage)).Int("len", len(c.entries)).Msg("cleanup")
	c.entries = c.entries[int(cleanupAfterMessage):]
	c.invalidateFilteredEntries()
	c.recalculateLines()

	// users that should not be removed from the color cache
	usersLeft := make(map[string]struct{}, len(c.entries))
	for _, e := range c.entries {
		if privMsg, ok := e.Event.Message.(*twitchirc.PrivateMessage); ok {
			usersLeft[strings.ToLower(privMsg.LoginName)] = struct{}{}
		}
	}

	// remove no longer needed user color cache entries
	c.palette.Prune(usersLeft)

	// for i, e := range c.entries {
	// 	if e.IsIgnored {
	// 		continue
	// 	}

	// 	if c.lineStart >= e.Position.CursorStart && c.lineStart <= e.Position.CursorEnd {
	// 		c.logger.Info().Int("index", i).Int("threshold", cleanupThreshold).Msg("cleanup")

	// 		if i > cleanupThreshold {
	// 			c.entries = c.entries[i:]
	// 			c.recalculateLines()
	// 		}

	// 		return
	// 	}
	// }
}

func (c *chatWindow) handleMessage(msg ChatEventMessage) tea.Cmd {
	switch msg.Message.(type) {
	case error, *twitchirc.PrivateMessage, *twitchirc.Notice, *twitchirc.ClearChat, *twitchirc.SubMessage, *twitchirc.SubGiftMessage, *twitchirc.AnnouncementMessage, *twitchirc.ClearMessage: // supported Message types
	default: // exit only on other types
		return nil
	}

	c.cleanup()
	c.handleTimeoutMessage(msg)
	c.handleMessageDeletion(msg)

	lines := c.renderer.Render(msg)

	// create new message - append to entries list
	var (
		positionStart    = -1
		wasLatestMessage = true
	)

	if newestEntry := c.getNewestEntry(); newestEntry != nil {
		positionStart = newestEntry.Position.CursorEnd
		wasLatestMessage = newestEntry.Selected
		newestEntry.Selected = false
	}

	entry := &chatEntry{
		Position: position{
			CursorStart: positionStart + 1,
			CursorEnd:   positionStart + len(lines),
		},
		Selected: wasLatestMessage,
		Event:    msg,
	}

	// we are currently searching and the new entry does not match the search, then ignore new entry
	if c.state == searchChatWindowState && !c.entryMatchesSearch(entry) {
		entry.IsFiltered = true
		entry.Position = position{} // lines not appended; position invalid until recalculateLines
		c.entries = append(c.entries, entry)
	} else {
		c.entries = append(c.entries, entry)
		c.lines = append(c.lines, lines...)
	}

	c.invalidateFilteredEntries()

	c.updatePort()

	if wasLatestMessage {
		c.moveToBottom()
		if c.settings.Chat.SmoothScroll {
			return c.startAnimating()
		}
	}

	return nil
}

func (c *chatWindow) handleTimeoutMessage(msg ChatEventMessage) {
	if timeoutMsg, ok := msg.Message.(*twitchirc.ClearChat); ok && timeoutMsg.UserName != nil {
		var hasDeleted bool
		for _, e := range c.entries {
			privMsg, ok := e.Event.Message.(*twitchirc.PrivateMessage)

			if !ok {
				continue
			}

			if strings.EqualFold(privMsg.LoginName, *timeoutMsg.UserName) && !e.IsDeleted {
				hasDeleted = true
				e.IsDeleted = true
				e.Event.DisplayModifier.Strikethrough = true
			}
		}

		if hasDeleted {
			c.recalculateLines()
		}
	}
}

func (c *chatWindow) handleMessageDeletion(msg ChatEventMessage) {
	if clearMsg, ok := msg.Message.(*twitchirc.ClearMessage); ok {
		var hasDeleted bool
		for _, e := range c.entries {
			privMsg, ok := e.Event.Message.(*twitchirc.PrivateMessage)

			if !ok {
				continue
			}

			if strings.EqualFold(privMsg.ID, clearMsg.TargetMsgID) && !e.IsDeleted {
				hasDeleted = true
				e.IsDeleted = true
				e.Event.DisplayModifier.Strikethrough = true
			}
		}

		if hasDeleted {
			c.recalculateLines()
		}
	}
}

// clipboardText returns a plain-text representation of the entry suitable for
// the system clipboard. Chat messages use their raw text so emote tokens and
// links survive; alert events get a readable summary.
func (c *chatWindow) clipboardText(entry *chatEntry) string {
	if entry == nil {
		return ""
	}

	switch msg := entry.Event.Message.(type) {
	case *twitchirc.PrivateMessage:
		return strings.ReplaceAll(msg.Message, string(duplicateBypass), "")
	case *twitchirc.AnnouncementMessage:
		return fmt.Sprintf("%s: %s", msg.DisplayName, msg.Message)
	case *twitchirc.SubMessage:
		text := fmt.Sprintf("%s just subscribed with a %s subscription. (%d Months, %d Month Streak)",
			msg.DisplayName, msg.SubPlan.String(), msg.CumulativeMonths, msg.StreakMonths)

		if msg.Message != "" {
			text += ": " + msg.Message
		}

		return text
	case *twitchirc.SubGiftMessage:
		return fmt.Sprintf("%s gifted a %s sub to %s. (%d Months)",
			msg.DisplayName, msg.SubPlan.String(), msg.ReceiptDisplayName, msg.Months)
	case *twitchirc.Notice:
		return msg.Message
	case *twitchirc.ClearMessage:
		return "A message from " + msg.Login + " was removed."
	case *twitchirc.ClearChat:
		if msg.UserName == nil {
			return "Clear chat prevented by Chatuino. Chat restored."
		}

		text := *msg.UserName

		if msg.BanDuration != nil && *msg.BanDuration > 0 {
			text += " was timed out for " + humanizeDuration(time.Duration(*msg.BanDuration)*time.Second)
		} else {
			text += " was permanently banned."
		}

		return text
	case error:
		return msg.Error()
	}

	return ""
}

func (c *chatWindow) updatePort() {
	height := c.height
	if c.state == searchChatWindowState {
		height--
	}

	if height <= 0 {
		c.lineStart = 0
		c.lineEnd = 0
		return
	}

	// Clamp cursor to valid range
	if len(c.lines) > 0 {
		c.cursor = clamp(c.cursor, 0, len(c.lines)-1)
	} else {
		c.cursor = 0
		c.lineStart = 0
		c.lineEnd = 0
		return
	}

	// If viewport is larger than content, show everything
	if height >= len(c.lines) {
		c.lineStart = 0
		c.lineEnd = len(c.lines)
		return
	}

	// Adjust viewport if cursor is outside
	if c.cursor < c.lineStart {
		// Cursor is above the viewport, so move viewport up to show cursor at the top
		c.lineStart = c.cursor
	} else if c.cursor >= c.lineStart+height {
		// Cursor is below the viewport, so move viewport down to show cursor at the bottom
		c.lineStart = c.cursor - height + 1
	}

	// The viewport is defined by lineStart and height
	c.lineEnd = c.lineStart + height

	// Final check to ensure lineEnd does not exceed the number of lines.
	if c.lineEnd > len(c.lines) {
		c.lineEnd = len(c.lines)
		c.lineStart = c.lineEnd - height
		if c.lineStart < 0 {
			c.lineStart = 0
		}
	}
}

func (c *chatWindow) recalculateLines() {
	c.searchInput.SetWidth(c.width)

	entries := c.activeEntries()

	if len(entries) < 1 && len(c.lines) < 1 {
		return
	}

	// get the currently selected entry, to reset the cursor to the new position once calculated
	_, selected := c.entryForCurrentCursor()

	c.lines = make([]string, 0, len(c.lines))

	var prevEntry *chatEntry

	for e := range slices.Values(entries) {
		lastCursorEnd := -1

		if prevEntry != nil {
			lastCursorEnd = prevEntry.Position.CursorEnd
		}

		lines := c.renderer.Render(e.Event)
		c.lines = append(c.lines, lines...)

		e.Position.CursorStart = lastCursorEnd + 1
		e.Position.CursorEnd = lastCursorEnd + len(lines)
		prevEntry = e
	}

	if selected != nil {
		c.cursor = selected.Position.CursorEnd
	}

	c.updatePort()
	c.snapScroll()
}

func (c *chatWindow) activeEntries() []*chatEntry {
	if c.searchInput.Value() == "" {
		return c.entries
	}

	if !c.filteredEntriesDirty && c.filteredEntries != nil {
		return c.filteredEntries
	}

	c.filteredEntries = make([]*chatEntry, 0, c.height)
	for _, e := range c.entries {
		if !e.IsFiltered {
			c.filteredEntries = append(c.filteredEntries, e)
		}
	}
	c.filteredEntriesDirty = false

	return c.filteredEntries
}

func (c *chatWindow) invalidateFilteredEntries() {
	c.filteredEntriesDirty = true
	c.filteredEntries = nil
}

// clearSearchFilter un-hides all entries and resets the viewport.
// Used when the query is too short or invalid — the user should see all messages.
func (c *chatWindow) clearSearchFilter() {
	var last *chatEntry
	for e := range slices.Values(c.entries) {
		e.IsFiltered = false
		e.Selected = false
		last = e
	}

	if last != nil {
		last.Selected = true
	}

	c.invalidateFilteredEntries()
	c.recalculateLines()
	c.moveToBottom()
}

func (c *chatWindow) applySearch() {
	query := c.searchInput.Value()

	// parse query into matcher; show all entries on empty/short/invalid input
	if len(query) <= 2 {
		c.currentMatcher = nil
		c.searchError = ""
		c.matchCount = 0

		c.clearSearchFilter()

		return
	}

	matcher, err := search.Parse(query)
	if err != nil {
		c.currentMatcher = nil
		c.searchError = err.Error()
		c.matchCount = 0

		c.clearSearchFilter()

		return
	}

	c.currentMatcher = matcher
	c.searchError = ""

	var (
		last  *chatEntry
		count int
	)

	for e := range slices.Values(c.entries) {
		e.Selected = false
		if c.entryMatchesSearch(e) {
			e.IsFiltered = false
			last = e
			count++

			continue
		}

		e.IsFiltered = true
	}

	c.matchCount = count

	if last != nil {
		last.Selected = true
	}

	c.invalidateFilteredEntries()
	c.recalculateLines()
	c.moveToBottom()
}

func (c *chatWindow) entryMatchesSearch(e *chatEntry) bool {
	if c.currentMatcher == nil {
		return false
	}

	cast, ok := e.Event.Message.(*twitchirc.PrivateMessage)
	if !ok {
		return false
	}

	return c.currentMatcher.Match(cast)
}
