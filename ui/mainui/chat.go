package mainui

import (
	"encoding/json"
	"fmt"
	"maps"
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
	"github.com/julez-dev/reflow/wordwrap"
	"github.com/julez-dev/reflow/wrap"
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
	Event      chatEventMessage
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
	deps          *DependencyContainer
	width, height int

	timeFormatFunc func(time.Time) string

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

	// optimize color rendering by caching render functions
	// so we don't need to recreate a new lipgloss.Style for every message
	userColorCache map[string]func(...string) string
	searchInput    textinput.Model

	// cached filtered entries for search mode; invalidated when entries/filters change
	filteredEntries      []*chatEntry
	filteredEntriesDirty bool

	// parsed search matcher; nil when query is empty or invalid
	currentMatcher searchMatcher
	searchError    string
	matchCount     int

	// reusable buffer for sorting replacement keys in applyWordReplacements
	replacementKeysBuf []string

	// reusable buffer for visible lines in View() to avoid allocation per frame
	visibleBuf []string

	// cached padding width for wrapped continuation lines when DisablePaddingWrappedLines is set;
	// derived from time format output length, computed once at construction.
	timePaddingWidth int
	// time format output length without the "  " prefix + " " separator; used for error prefix alignment.
	timeFormatWidth int

	// styles
	indicator      string
	indicatorWidth int

	subAlertStyle       lipgloss.Style
	noticeAlertStyle    lipgloss.Style
	clearChatAlertStyle lipgloss.Style
	errorAlertStyle     lipgloss.Style
	dimmedStyle         lipgloss.Style

	// pre-created styles for message modifiers (strikethrough, italic, both)
	strikethroughStyle       lipgloss.Style
	italicStyle              lipgloss.Style
	strikethroughItalicStyle lipgloss.Style
}

func newChatWindow(width, height int, deps *DependencyContainer) *chatWindow {
	input := textinput.New()
	input.CharLimit = 128
	input.Prompt = "  /"
	input.Placeholder = "search — content: user: /regex/ badge: is:mod|sub|vip|first"
	styles := input.Styles()
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.InputPromptColor))
	styles.Cursor.BlinkSpeed = time.Millisecond * 750
	input.SetStyles(styles)
	input.SetWidth(width)

	indicator := lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.ChatIndicatorColor)).Background(lipgloss.Color(deps.UserConfig.Theme.ChatIndicatorColor)).Render(">")

	timeFormat := deps.UserConfig.Settings.Chat.TimeFormat
	timeFormatFn := func(t time.Time) string {
		return t.Local().Format(timeFormat)
	}

	// Pre-compute padding width for continuation lines.
	// Using zero-time gives the maximum-length output for variable-width formats (e.g. "3:04 PM" → "12:00 AM"),
	// so padding may be 1 char wider than the actual time for some hours — acceptable cosmetic tradeoff.
	timeFormatWidth := len(timeFormatFn(time.Time{}))
	timePadWidth := timeFormatWidth + 3 // +3 matches the "  " prefix + " " separator in messageToText

	c := chatWindow{
		deps:             deps,
		width:            width,
		height:           height,
		userColorCache:   map[string]func(...string) string{},
		timeFormatFunc:   timeFormatFn,
		timePaddingWidth: timePadWidth,
		timeFormatWidth:  timeFormatWidth,
		searchInput:      input,

		indicator:           indicator,
		indicatorWidth:      lipgloss.Width(indicator),
		subAlertStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.ChatSubAlertColor)).Bold(true),
		noticeAlertStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.ChatNoticeAlertColor)).Bold(true),
		clearChatAlertStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.ChatClearChatColor)).Bold(true),
		errorAlertStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.ChatErrorColor)).Bold(true),
		dimmedStyle:         lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.DimmedTextColor)),

		strikethroughStyle:       lipgloss.NewStyle().Strikethrough(true).StrikethroughSpaces(false),
		italicStyle:              lipgloss.NewStyle().Italic(true),
		strikethroughItalicStyle: lipgloss.NewStyle().Strikethrough(true).StrikethroughSpaces(false).Italic(true),
	}

	return &c
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
	case chatEventMessage:
		return c, c.handleMessage(msg)
	case tea.KeyPressMsg:
		if c.focused {
			switch {
			// start search (only when not already searching — otherwise '/' is forwarded to the input)
			case key.Matches(msg, c.deps.Keymap.SearchMode) && c.state != searchChatWindowState:
				return c, c.handleStartSearchMode()
			// stop search
			case key.Matches(msg, c.deps.Keymap.Escape) && c.state == searchChatWindowState:
				c.handleStopSearchMode()
				return c, nil
			case key.Matches(msg, c.deps.Keymap.Confirm) && c.state == searchChatWindowState:
				c.handleStopSearchModeKeepSelected()
				return c, nil
			// update search, allow up and down arrow keys for navigation in result
			case c.state == searchChatWindowState && msg.String() != "up" && msg.String() != "down":
				c.searchInput, cmd = c.searchInput.Update(msg)
				c.applySearch()
				cmds = append(cmds, cmd)
				return c, tea.Batch(cmds...)
			case key.Matches(msg, c.deps.Keymap.Down):
				c.messageDown(1)
				c.snapScroll()
				return c, nil
			case key.Matches(msg, c.deps.Keymap.Up):
				c.messageUp(1)
				c.snapScroll()
				return c, nil
			case key.Matches(msg, c.deps.Keymap.GoToBottom):
				c.moveToBottom()
				c.snapScroll()
				return c, nil
			case key.Matches(msg, c.deps.Keymap.GoToTop):
				c.moveToTop()
				c.snapScroll()
				return c, nil
			case key.Matches(msg, c.deps.Keymap.DumpChat):
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

	dump.UserCache = make([]string, 0, len(c.userColorCache))

	for k := range c.userColorCache {
		dump.UserCache = append(dump.UserCache, k)
	}

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

	_, _ = f.Write([]byte(stripAnsi(string(bytes))))
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
		if privMsg, ok := e.Event.message.(*twitchirc.PrivateMessage); ok {
			usersLeft[strings.ToLower(privMsg.LoginName)] = struct{}{}
		}
	}

	// remove no longer needed user color cache entries
	for user := range c.userColorCache {
		// user no longer exists in chat
		if _, ok := usersLeft[user]; !ok {
			log.Logger.Info().Str("user", user).Msg("delete user from cache")
			delete(c.userColorCache, user)
		}
	}

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

func (c *chatWindow) handleMessage(msg chatEventMessage) tea.Cmd {
	switch msg.message.(type) {
	case error, *twitchirc.PrivateMessage, *twitchirc.Notice, *twitchirc.ClearChat, *twitchirc.SubMessage, *twitchirc.SubGiftMessage, *twitchirc.AnnouncementMessage, *twitchirc.ClearMessage: // supported Message types
	default: // exit only on other types
		return nil
	}

	c.cleanup()
	c.handleTimeoutMessage(msg)
	c.handleMessageDeletion(msg)

	lines := c.messageToText(msg)

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
		if c.deps.UserConfig.Settings.Chat.SmoothScroll {
			return c.startAnimating()
		}
	}

	return nil
}

func (c *chatWindow) handleTimeoutMessage(msg chatEventMessage) {
	if timeoutMsg, ok := msg.message.(*twitchirc.ClearChat); ok && timeoutMsg.UserName != nil {
		var hasDeleted bool
		for _, e := range c.entries {
			privMsg, ok := e.Event.message.(*twitchirc.PrivateMessage)

			if !ok {
				continue
			}

			if strings.EqualFold(privMsg.LoginName, *timeoutMsg.UserName) && !e.IsDeleted {
				hasDeleted = true
				e.IsDeleted = true
				e.Event.displayModifier.strikethrough = true
			}
		}

		if hasDeleted {
			c.recalculateLines()
		}
	}
}

func (c *chatWindow) handleMessageDeletion(msg chatEventMessage) {
	if clearMsg, ok := msg.message.(*twitchirc.ClearMessage); ok {
		var hasDeleted bool
		for _, e := range c.entries {
			privMsg, ok := e.Event.message.(*twitchirc.PrivateMessage)

			if !ok {
				continue
			}

			if strings.EqualFold(privMsg.ID, clearMsg.TargetMsgID) && !e.IsDeleted {
				hasDeleted = true
				e.IsDeleted = true
				e.Event.displayModifier.strikethrough = true
			}
		}

		if hasDeleted {
			c.recalculateLines()
		}
	}
}

// buildAlertPrefix creates a standardized prefix with timestamp and styled alert label.
// Example output: "  15:04:05 [Notice]: "
func (c *chatWindow) buildAlertPrefix(timestamp time.Time, label string, style lipgloss.Style) string {
	return "  " + c.dimmedStyle.Render(c.timeFormatFunc(timestamp)) + " [" + style.Render(label) + "]: "
}

// formatMessageText applies word replacements and color processing to message content.
func (c *chatWindow) formatMessageText(content string, modifier messageContentModifier) string {
	if modifier.strikethrough && modifier.italic {
		return c.strikethroughItalicStyle.Render(content)
	}
	if modifier.strikethrough {
		return c.strikethroughStyle.Render(content)
	}
	if modifier.italic {
		return c.italicStyle.Render(content)
	}

	content = c.applyWordReplacements(content, modifier.wordReplacements)

	if modifier.messageSuffix != "" {
		content += modifier.messageSuffix
	}
	return content
}

// applyWordReplacements applies word replacements from the display modifier to the given content.
// It replaces each key in the wordReplacements map with its corresponding value.
// Keys are sorted longest-first to prevent partial matches.
func (c *chatWindow) applyWordReplacements(content string, replacements wordReplacement) string {
	if len(replacements) == 0 {
		return content
	}

	// Reuse a buffer to avoid allocating a new slice per call during recalculateLines.
	keys := c.replacementKeysBuf[:0]
	for k := range replacements {
		keys = append(keys, k)
	}

	slices.SortFunc(keys, func(a, b string) int {
		if len(a) != len(b) {
			return len(b) - len(a)
		}
		return strings.Compare(a, b)
	})

	// Keep buffer for next call (may have grown)
	c.replacementKeysBuf = keys

	for _, original := range keys {
		content = strings.ReplaceAll(content, original, replacements[original])
	}
	return content
}

func (c *chatWindow) setUserColorModifier(content string, modifier *messageContentModifier) {
	words := strings.Split(content, " ")

	if modifier.wordReplacements == nil {
		modifier.wordReplacements = make(wordReplacement)
	}

	for _, word := range words {
		cleaned := strings.ToLower(stripDisplayNameEdges(word))
		renderFn, ok := c.userColorCache[cleaned]

		if !ok {
			// fallback try if empty
			if f, ok := c.userColorCache[word]; ok {
				renderFn = f
			} else {
				continue
			}
		}

		stripped := stripDisplayNameEdges(word)
		modifier.wordReplacements[stripped] = renderFn(stripped)
		modifier.wordReplacements[word] = renderFn(word)
	}
}

func (c *chatWindow) messageToText(event chatEventMessage) []string {
	switch msg := event.message.(type) {
	case error:
		prefix := "  " + strings.Repeat(" ", c.timeFormatWidth) + " [" + c.errorAlertStyle.Render("Error") + "]: "
		text := strings.ReplaceAll(msg.Error(), "\n", "")
		return c.wordwrapMessage(prefix, c.formatMessageText(text, event.displayModifier))
	case *twitchirc.PrivateMessage:
		userRenderFunc := c.getSetUserColorFunc(msg.LoginName, msg.Color)

		// Build prefix components: time, [guest channel], [badges], username
		parts := []string{"  " + c.dimmedStyle.Render(c.timeFormatFunc(msg.TMISentTS))}

		if event.channelGuestDisplayName != "" {
			parts = append(parts, "|"+event.channelGuestDisplayName+"|")
		}

		if len(event.displayModifier.badgeReplacement) > 0 && !c.deps.UserConfig.Settings.Chat.DisableBadges {
			badges := formatBadgeReplacement(c.deps.UserConfig.Settings, event.displayModifier.badgeReplacement)
			if c.deps.UserConfig.Settings.Chat.GraphicBadges {
				// Hair space (U+200A) - narrower gap since badges have pixel padding
				parts = append(parts, badges+" "+userRenderFunc(msg.DisplayName)+": ")
			} else {
				parts = append(parts, badges)
				parts = append(parts, userRenderFunc(msg.DisplayName)+": ")
			}
		} else {
			parts = append(parts, userRenderFunc(msg.DisplayName)+": ")
		}
		prefix := strings.Join(parts, " ")

		c.setUserColorModifier(msg.Message, &event.displayModifier)
		return c.wordwrapMessage(prefix, c.formatMessageText(msg.Message, event.displayModifier))
	case *twitchirc.Notice:
		title := "Notice"
		if event.isFakeEvent {
			title = "Fake-Notice"
		}

		prefix := c.buildAlertPrefix(msg.FakeTimestamp, title, c.noticeAlertStyle)

		event.displayModifier.italic = true
		c.setUserColorModifier(msg.Message, &event.displayModifier)

		return c.wordwrapMessage(prefix, c.formatMessageText(msg.Message, event.displayModifier))
	case *twitchirc.ClearMessage:
		prefix := c.buildAlertPrefix(msg.TMISentTS, "Clear Message", c.clearChatAlertStyle)
		prefix += "A message from "
		text := msg.Login + " was removed."

		c.setUserColorModifier(text, &event.displayModifier)

		return c.wordwrapMessage(prefix, c.formatMessageText(text, event.displayModifier))
	case *twitchirc.ClearChat:
		prefix := c.buildAlertPrefix(msg.TMISentTS, "Clear Chat", c.clearChatAlertStyle)

		if msg.TargetUserID == nil {
			return c.wordwrapMessage(prefix, c.formatMessageText("Clear chat prevented by Chatuino. Chat restored.", event.displayModifier))
		}

		text := *msg.UserName

		if msg.BanDuration != nil && *msg.BanDuration > 0 {
			text += " was timed out for " + humanizeDuration(time.Duration(*msg.BanDuration)*time.Second)
		} else {
			text += " was permanently banned."
		}

		c.setUserColorModifier(text, &event.displayModifier)

		return c.wordwrapMessage(prefix, c.formatMessageText(text, event.displayModifier))
	case *twitchirc.SubMessage:
		prefix := c.buildAlertPrefix(msg.TMISentTS, "Sub Alert", c.subAlertStyle)

		subResubText := "subscribed"
		if msg.MsgID == "resub" {
			subResubText = "resubscribed"
		}

		_ = c.getSetUserColorFunc(msg.Login, msg.Color)
		text := fmt.Sprintf("%s just %s with a %s subscription. (%d Months, %d Month Streak)",
			msg.DisplayName,
			subResubText,
			msg.SubPlan.String(),
			msg.CumulativeMonths,
			msg.StreakMonths,
		)

		// Append user message if present
		if msg.Message != "" {
			text += ": " + msg.Message
		}

		c.setUserColorModifier(text, &event.displayModifier)

		return c.wordwrapMessage(prefix, c.formatMessageText(text, event.displayModifier))
	case *twitchirc.SubGiftMessage:
		prefix := c.buildAlertPrefix(msg.TMISentTS, "Sub Gift Alert", c.subAlertStyle)

		_ = c.getSetUserColorFunc(msg.Login, msg.Color)

		text := fmt.Sprintf("%s gifted a %s sub to %s. (%d Months)",
			msg.DisplayName,
			msg.SubPlan.String(),
			msg.ReceiptDisplayName,
			msg.Months,
		)

		c.setUserColorModifier(text, &event.displayModifier)

		return c.wordwrapMessage(prefix, c.formatMessageText(text, event.displayModifier))
	case *twitchirc.AnnouncementMessage:
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(msg.ParamColor.RGBHex())).Bold(true)
		prefix := "  " + c.timeFormatFunc(msg.TMISentTS) + " [" + style.Render("Announcement") + "] "

		_ = c.getSetUserColorFunc(msg.Login, msg.Color)
		text := fmt.Sprintf("%s: %s",
			msg.DisplayName,
			c.applyWordReplacements(msg.Message, event.displayModifier.wordReplacements),
		)

		c.setUserColorModifier(text, &event.displayModifier)

		return c.wordwrapMessage(prefix, c.formatMessageText(text, event.displayModifier))
	}

	return []string{}
}

func (c *chatWindow) getSetUserColorFunc(name string, colorHex string) func(strs ...string) string {
	_, ok := c.userColorCache[name]

	if !ok {
		if colorHex == "" {
			colorHex = randomHexColor()
		}

		style := lipgloss.NewStyle().Foreground(lipgloss.Color(colorHex))
		c.userColorCache[name] = style.Render
	}

	return c.userColorCache[name]
}

func (c *chatWindow) wordwrapMessage(prefix, content string) []string {
	// Strip duplicate-bypass rune (U+E0000) used to bypass Twitch spam detection.
	// Guard with ContainsRune to avoid allocating when the rune isn't present (vast majority of messages).
	if strings.ContainsRune(content, duplicateBypass) {
		content = strings.Map(func(r rune) rune {
			if r == duplicateBypass {
				return -1
			}
			return r
		}, content)
	}

	prefixWidth := lipgloss.Width(prefix)

	// Assure that the prefix is at least prefixPadding wide
	if prefixWidth < prefixPadding {
		prefix = prefix + strings.Repeat(" ", prefixPadding-prefixWidth)
		prefixWidth = lipgloss.Width(prefix)
	}

	contentWidthLimit := c.width - c.indicatorWidth - prefixWidth

	// softwrap text to contentWidthLimit, if soft wrapping fails (for example in links) force break
	wrappedText := wrap.String(wordwrap.String(content, contentWidthLimit), contentWidthLimit)
	splits := strings.Split(wrappedText, "\n")

	lines := make([]string, 0, len(splits))
	lines = append(lines, prefix+splits[0]) // first line is prefix + content at index 0

	// if there are more lines, add prefixPadding spaces to the beginning of the line
	for _, line := range splits[1:] {
		if c.deps.UserConfig.Settings.Chat.DisablePaddingWrappedLines {
			lines = append(lines, strings.Repeat(" ", c.timePaddingWidth)+line)
		} else {
			lines = append(lines, strings.Repeat(" ", prefixWidth)+line)
		}
	}

	return lines
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

		lines := c.messageToText(e.Event)
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

	cast, ok := e.Event.message.(*twitchirc.PrivateMessage)
	if !ok {
		return false
	}

	return c.currentMatcher.Match(cast)
}

func formatBadgeReplacement(settings save.Settings, replacements map[string]string) string {
	// Sort keys for deterministic badge order
	keys := slices.Sorted(maps.Keys(replacements))
	values := make([]string, 0, len(keys))
	for _, k := range keys {
		values = append(values, replacements[k])
	}

	if !settings.Chat.GraphicBadges {
		return fmt.Sprintf("[%s]", strings.Join(values, ","))
	}

	return strings.Join(values, "")
}
