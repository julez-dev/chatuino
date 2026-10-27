package chat

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/search"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
)

const (
	cleanupAfterMessage float64 = 800.0
	cleanupThreshold            = int(cleanupAfterMessage * 1.5)

	// Smooth scroll animation parameters.
	smoothScrollFPS        = 60
	smoothScrollInterval   = time.Second / smoothScrollFPS
	smoothScrollLerpFactor = 0.08
	smoothScrollSnap       = 0.2
	smoothScrollMaxFrames  = 8
)

// EntryID identifies one transcript entry. It stays valid for the lifetime of
// the buffer; positions and line numbers are deliberately not part of the
// interface because they change on every reflow.
type EntryID int

// Entry is one chat event in the transcript. Position, selection and filter
// state stay private: callers identify an entry by EntryID and read its Event.
type Entry struct {
	ID      EntryID
	Event   ChatEventMessage
	Deleted bool

	position position
	selected bool
	filtered bool
}

// Selected reports whether this entry currently carries the selection.
func (e Entry) Selected() bool {
	return e.selected
}

type position struct {
	cursorStart int
	cursorEnd   int
}

// searchMatcher tests whether a private message satisfies a search criterion.
type searchMatcher interface {
	Match(msg *twitchirc.PrivateMessage) bool
}

// FilterStatus reports what happened to the search filter.
type FilterStatus struct {
	Applied bool
	Matches int
	Err     string
	Hint    string
}

// Buffer is one chat transcript: entries, rendered rows, selection, viewport,
// search filter and new-message badge. It has no key handling, search widget or
// tab mode; the tab shell owns those and calls this module.
type Buffer struct {
	settings save.Settings

	width, height int

	cursor             int
	lineStart, lineEnd int

	smoothLineStart float64
	animating       bool
	animatingFrames int

	entries []*Entry
	lines   []string

	renderer *messageRenderer
	palette  *Palette

	filterActive  bool
	searchActive  bool
	filterQuery   string
	filteredItems []*Entry
	filterDirty   bool

	currentMatcher searchMatcher
	filterStatus   FilterStatus

	visibleBuf []string
	indicator  string

	newMessages     int
	newMessageStyle lipgloss.Style

	nextID EntryID
}

// New creates an empty transcript.
func New(width, height int, settings save.Settings, theme save.Theme) *Buffer {
	indicator := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ChatIndicatorColor)).
		Background(lipgloss.Color(theme.ChatIndicatorColor)).
		Render(">")

	newMessageStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.ListBackgroundColor)).
		Background(lipgloss.Color(theme.ChatIndicatorColor))

	palette := NewPalette()

	return &Buffer{
		settings:        settings,
		width:           width,
		height:          height,
		renderer:        newMessageRenderer(width, lipgloss.Width(indicator), settings, theme, palette),
		palette:         palette,
		indicator:       indicator,
		newMessageStyle: newMessageStyle,
	}
}

// Palette returns the shared user-color palette.
func (b *Buffer) Palette() *Palette {
	return b.palette
}

// UsePalette shares another transcript's user colors.
func (b *Buffer) UsePalette(palette *Palette) {
	b.palette = palette
	b.renderer.palette = palette
}

// SetTimeFormat changes the timestamp format of rendered messages.
func (b *Buffer) SetTimeFormat(format string) {
	b.renderer.SetTimeFormat(format)
}

// Resize updates the transcript dimensions.
func (b *Buffer) Resize(width, height int) {
	widthChanged := width != b.width
	b.width = width
	b.height = height
	b.renderer.SetWidth(width)

	if widthChanged {
		b.recalculateLines()
	} else {
		b.updatePort()
		b.snapScroll()
	}
}

// Update handles the messages the transcript owns. handled is false for every
// other message, so the tab shell can route keys and the search widget.
func (b *Buffer) Update(msg tea.Msg) (cmd tea.Cmd, handled bool) {
	switch msg := msg.(type) {
	case smoothScrollTick:
		if msg.owner != b || !b.animating {
			return nil, true
		}

		b.animatingFrames++

		target := float64(b.lineStart)
		diff := target - b.smoothLineStart

		if math.Abs(diff) < smoothScrollSnap || b.animatingFrames >= smoothScrollMaxFrames {
			b.snapScroll()
			return nil, true
		}

		b.smoothLineStart += diff * smoothScrollLerpFactor
		return b.smoothScrollTickCmd(), true
	case ChatEventMessage:
		return b.Append(msg), true
	}

	return nil, false
}

// Append adds one event and returns a smooth-scroll command when the viewport
// follows the newest message.
func (b *Buffer) Append(msg ChatEventMessage) tea.Cmd {
	switch msg.Message.(type) {
	case error, *twitchirc.PrivateMessage, *twitchirc.Notice, *twitchirc.ClearChat, *twitchirc.SubMessage, *twitchirc.SubGiftMessage, *twitchirc.AnnouncementMessage, *twitchirc.ClearMessage:
	default:
		return nil
	}

	b.cleanup()
	b.applyTimeout(msg)
	b.applyDeletion(msg)

	lines := b.renderer.Render(msg)

	var (
		positionStart    = -1
		wasLatestMessage = true
	)

	if newest := b.newestEntry(); newest != nil {
		positionStart = newest.position.cursorEnd
		wasLatestMessage = newest.selected
		newest.selected = false
	}

	entry := &Entry{
		ID:    b.nextID,
		Event: msg,
		position: position{
			cursorStart: positionStart + 1,
			cursorEnd:   positionStart + len(lines),
		},
		selected: wasLatestMessage,
	}
	b.nextID++

	// While filtering, a new message that does not match stays in the buffer but
	// contributes no rows. Its position is rebuilt by the next reflow.
	if b.filterActive && !b.entryMatchesSearch(entry) {
		entry.filtered = true
		b.entries = append(b.entries, entry)
	} else {
		b.entries = append(b.entries, entry)
		b.lines = append(b.lines, lines...)
	}

	b.invalidateFiltered()
	b.updatePort()
	b.countNewMessage(entry)

	if wasLatestMessage {
		b.moveToBottom()
		if b.settings.Chat.SmoothScroll {
			return b.startAnimating()
		}
	}

	return nil
}

// ReplaceNotice rewrites an existing notice in place and reflows the rows after
// its text changed. It returns false when the notice left the buffer.
func (b *Buffer) ReplaceNotice(id, message string) bool {
	for _, entry := range b.entries {
		notice, ok := entry.Event.Message.(*twitchirc.Notice)
		if !ok || notice.MsgID != twitchirc.MsgID(id) {
			continue
		}

		notice.Message = message
		b.recalculateLines()

		return true
	}

	return false
}

// Entry returns one entry by ID.
func (b *Buffer) Entry(id EntryID) (Entry, bool) {
	entry := b.entryByID(id)
	if entry == nil {
		return Entry{}, false
	}

	return *entry, true
}

// Entries returns a copy of the transcript entries in order.
func (b *Buffer) Entries() []Entry {
	entries := make([]Entry, 0, len(b.entries))
	for _, entry := range b.entries {
		entries = append(entries, *entry)
	}

	return entries
}

// Current returns the index and pointer of the entry under the cursor.
func (b *Buffer) Current() (int, *Entry) {
	return b.entryForCurrentCursor()
}

// EntryAtLine returns the index and pointer of the entry containing an absolute
// rendered line.
func (b *Buffer) EntryAtLine(line int) (int, *Entry) {
	return b.entryAtLine(line)
}

// LineRange returns the absolute rendered rows occupied by an entry.
func (b *Buffer) LineRange(id EntryID) (int, int, bool) {
	entry := b.entryByID(id)
	if entry == nil {
		return 0, 0, false
	}

	return entry.position.cursorStart, entry.position.cursorEnd, true
}

// SnapScroll stops the smooth-scroll animation at the current viewport.
func (b *Buffer) SnapScroll() {
	b.snapScroll()
}

// Selected returns the selected entry.
func (b *Buffer) Selected() (Entry, bool) {
	_, entry := b.entryForCurrentCursor()
	if entry == nil {
		return Entry{}, false
	}

	return *entry, true
}

// Newest returns the newest entry.
func (b *Buffer) Newest() (Entry, bool) {
	entry := b.newestEntry()
	if entry == nil {
		return Entry{}, false
	}

	return *entry, true
}

// Rows returns the visible rows and the absolute line the first row represents.
// The selection indicator and the new-message badge are already applied.
func (b *Buffer) Rows(height int) ([]string, int) {
	if height < 1 {
		return nil, 0
	}

	renderStart := b.lineStart
	renderEnd := b.lineEnd
	if b.animating {
		renderStart = int(math.Round(b.smoothLineStart))
		if renderStart < 0 {
			renderStart = 0
		}

		renderEnd = renderStart + height
		if renderEnd > len(b.lines) {
			renderEnd = len(b.lines)
			renderStart = renderEnd - height
			if renderStart < 0 {
				renderStart = 0
			}
		}
	}

	if renderStart > len(b.lines) {
		renderStart = len(b.lines)
	}
	if renderEnd > len(b.lines) {
		renderEnd = len(b.lines)
	}
	if renderEnd < renderStart {
		renderEnd = renderStart
	}

	src := b.lines[renderStart:renderEnd]

	if cap(b.visibleBuf) < height {
		b.visibleBuf = make([]string, height)
	}

	visible := b.visibleBuf[:len(src)]
	copy(visible, src)
	b.applyIndicatorToVisible(visible, renderStart)

	lines := b.visibleBuf[:height]
	for i := len(src); i < height; i++ {
		lines[i] = ""
	}

	if !b.filterActive && !b.searchActive {
		b.drawNewMessageBadge(lines, renderStart, height)
	}

	return lines, renderStart
}

// ViewportRenderStart returns the absolute line of the first visible row.
func (b *Buffer) ViewportRenderStart() int {
	if !b.animating {
		return b.lineStart
	}

	start := int(math.Round(b.smoothLineStart))
	if start < 0 {
		start = 0
	}

	if start+b.height > len(b.lines) {
		start = max(0, len(b.lines)-b.height)
	}

	return start
}

func (b *Buffer) Hit(row int) (EntryID, bool) {
	_, entry := b.entryAtLine(b.ViewportRenderStart() + row)
	if entry == nil {
		return 0, false
	}

	return entry.ID, true
}

// Select selects an entry by ID.
func (b *Buffer) Select(id EntryID) bool {
	entry := b.entryByID(id)
	if entry == nil {
		return false
	}

	b.selectEntry(entry)
	return true
}

// SelectLine selects the entry containing the absolute rendered line.
func (b *Buffer) SelectLine(line int) {
	b.selectLine(line)
}

// SelectAtViewportRow selects the entry at a viewport row.
func (b *Buffer) SelectAtViewportRow(row int) {
	b.selectAtViewportRow(row)
}

func (b *Buffer) Move(n int) {
	b.moveSelectionByLines(n)
}

// PageUp moves the selection up one page.
func (b *Buffer) PageUp() {
	b.moveSelectionByLines(-max(1, b.height-1))
}

// PageDown moves the selection down one page.
func (b *Buffer) PageDown() {
	b.moveSelectionByLines(max(1, b.height-1))
}

// ScrollByLines moves the selection by n rendered lines.
func (b *Buffer) ScrollByLines(n int) {
	b.moveSelectionByLines(n)
}

// MoveToTop selects the first entry.
func (b *Buffer) MoveToTop() {
	b.moveToTop()
}

// MoveToBottom selects the newest entry.
func (b *Buffer) MoveToBottom() {
	b.moveToBottom()
}

// MessageDown moves the selection down n messages.
func (b *Buffer) MessageDown(n int) {
	b.messageDown(n)
}

// MessageUp moves the selection up n messages.
func (b *Buffer) MessageUp(n int) {
	b.messageUp(n)
}

// ClipboardText returns the plain-text form of one entry.
func (b *Buffer) ClipboardText(id EntryID) string {
	entry := b.entryByID(id)
	return b.clipboardText(entry)
}

// EmoteNameAt returns the graphic emote name at a viewport cell.
func (b *Buffer) EmoteNameAt(row, col int) (string, bool) {
	return b.emoteNameAt(row, col)
}

// HandleChatClick consumes a click on the new-message badge or a reply reference.
func (b *Buffer) HandleChatClick(row, col int) bool {
	if b.handleNewMessageBadgeClick(row, col) {
		return true
	}

	return b.handleReplyReferenceClick(b.ViewportRenderStart()+row, col)
}

// CaughtUp reports whether the newest line is visible or selected.
func (b *Buffer) CaughtUp() bool {
	return b.caughtUp()
}

// NewMessages returns the number of messages that arrived while scrolled up.
func (b *Buffer) NewMessages() int {
	return b.newMessages
}

// SetFilter applies a search query. Queries shorter than three characters show
// the hint and leave the transcript unfiltered.
func (b *Buffer) SetFilter(query string) FilterStatus {
	b.filterQuery = query

	if utf8.RuneCountInString(query) <= 2 {
		b.currentMatcher = nil
		b.filterStatus = FilterStatus{Hint: "type at least 3 characters"}
		b.clearFilter()

		return b.filterStatus
	}

	matcher, err := search.Parse(query)
	if err != nil {
		b.currentMatcher = nil
		b.filterStatus = FilterStatus{Err: err.Error()}
		b.clearFilter()

		return b.filterStatus
	}

	b.currentMatcher = matcher
	b.filterActive = true
	b.filterStatus = FilterStatus{Applied: true}

	var (
		last  *Entry
		count int
	)

	for _, entry := range b.entries {
		entry.selected = false
		if b.entryMatchesSearch(entry) {
			entry.filtered = false
			last = entry
			count++

			continue
		}

		entry.filtered = true
	}

	b.filterStatus.Matches = count

	if last != nil {
		last.selected = true
	}

	b.invalidateFiltered()
	b.recalculateLines()
	b.moveToBottom()

	return b.filterStatus
}

// ClearFilter removes the search filter and shows the whole transcript again.
func (b *Buffer) ClearFilter() {
	b.filterQuery = ""
	b.currentMatcher = nil
	b.filterActive = false
	b.filterStatus = FilterStatus{}
	b.clearFilter()
}

// ResetFilter removes the filter without moving the selection. The search
// widget calls it when it opens, so entering search does not jump to the tail.
func (b *Buffer) ResetFilter() {
	b.filterQuery = ""
	b.currentMatcher = nil
	b.filterActive = false
	b.filterStatus = FilterStatus{}

	for _, entry := range b.entries {
		entry.filtered = false
	}

	b.invalidateFiltered()
	b.recalculateLines()
}

// FilterStatus returns the last filter result.
func (b *Buffer) FilterStatus() FilterStatus {
	return b.filterStatus
}

// IsFiltering reports whether a filter is currently applied.
func (b *Buffer) IsFiltering() bool {
	return b.filterActive
}

func (b *Buffer) SetSearchActive(active bool) {
	b.searchActive = active
	b.updatePort()
}

func (b *Buffer) startAnimating() tea.Cmd {
	if b.animating {
		return nil
	}

	b.animating = true
	b.animatingFrames = 0

	return b.smoothScrollTickCmd()
}

type smoothScrollTick struct {
	owner *Buffer
}

func (b *Buffer) smoothScrollTickCmd() tea.Cmd {
	return tea.Tick(smoothScrollInterval, func(time.Time) tea.Msg {
		return smoothScrollTick{owner: b}
	})
}

func (b *Buffer) snapScroll() {
	b.smoothLineStart = float64(b.lineStart)
	b.animating = false
	b.animatingFrames = 0
}

func (b *Buffer) entryByID(id EntryID) *Entry {
	for _, entry := range b.entries {
		if entry.ID == id {
			return entry
		}
	}

	return nil
}

func (b *Buffer) entryForCurrentCursor() (int, *Entry) {
	active := b.activeEntries()
	if len(active) == 0 {
		return -1, nil
	}

	lo, hi := 0, len(active)-1
	for lo <= hi {
		mid := lo + (hi-lo)/2
		if active[mid].position.cursorEnd < b.cursor {
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}

	if lo < len(active) && active[lo].position.cursorStart <= b.cursor {
		return lo, active[lo]
	}

	return -1, nil
}

func (b *Buffer) entryAtLine(line int) (int, *Entry) {
	active := b.activeEntries()
	if len(active) == 0 {
		return -1, nil
	}

	if line <= active[0].position.cursorStart {
		return 0, active[0]
	}

	last := active[len(active)-1]
	if line >= last.position.cursorEnd {
		return len(active) - 1, last
	}

	lo, hi := 0, len(active)-1
	for lo <= hi {
		mid := lo + (hi-lo)/2
		if active[mid].position.cursorEnd < line {
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

func (b *Buffer) selectEntry(entry *Entry) {
	if entry == nil {
		return
	}

	for _, candidate := range b.entries {
		candidate.selected = false
	}

	entry.selected = true
	b.cursor = entry.position.cursorStart
	b.updatePort()
}

func (b *Buffer) moveSelectionByLines(n int) {
	active := b.activeEntries()
	if len(active) == 0 || n == 0 {
		return
	}

	i, current := b.entryForCurrentCursor()
	_, entry := b.entryAtLine(b.cursor + n)
	if entry == current {
		// A tall selected message can contain the whole wheel delta. Selecting
		// the same entry would reset the cursor to its start and make repeated
		// wheel events a permanent no-op. Advance to the neighbouring message
		// instead; at either end the clamp below remains a no-op.
		if n > 0 && i >= 0 && i+1 < len(active) {
			entry = active[i+1]
		} else if n < 0 && i > 0 {
			entry = active[i-1]
		}
	}

	b.selectEntry(entry)
	b.snapScroll()
}

func (b *Buffer) selectLine(line int) {
	_, entry := b.entryAtLine(line)
	b.selectEntry(entry)
	b.snapScroll()
}

func (b *Buffer) selectAtViewportRow(row int) {
	if row < 0 || row >= b.height {
		return
	}

	b.selectLine(b.ViewportRenderStart() + row)
}

func (b *Buffer) goToEntry(entry *Entry) {
	active := b.activeEntries()
	if len(active) < 1 {
		return
	}

	for _, candidate := range b.entries {
		candidate.selected = false
	}

	for _, candidate := range active {
		if candidate == entry {
			candidate.selected = true
			// Jumping to a parent must show the entry from its first row: a
			// parent that is itself a reply carries its quoted context there.
			b.cursor = candidate.position.cursorStart
			b.updatePort()
			b.snapScroll()
			return
		}
	}
}

func (b *Buffer) messageDown(n int) {
	b.moveMessage(n, true)
}

func (b *Buffer) messageUp(n int) {
	b.moveMessage(-n, false)
}

func (b *Buffer) moveMessage(delta int, atEnd bool) {
	active := b.activeEntries()
	if len(active) < 1 {
		return
	}

	i, entry := b.entryForCurrentCursor()
	if i == -1 {
		return
	}

	entry.selected = false
	i = min(max(i+delta, 0), len(active)-1)
	active[i].selected = true
	if atEnd {
		b.cursor = active[i].position.cursorEnd
	} else {
		b.cursor = active[i].position.cursorStart
	}
	b.updatePort()
}

func (b *Buffer) moveToBottom() {
	i, current := b.entryForCurrentCursor()
	if current == nil {
		return
	}

	active := b.activeEntries()
	b.messageDown(len(active) - i)
}

func (b *Buffer) moveToTop() {
	i, current := b.entryForCurrentCursor()
	if current == nil {
		return
	}

	b.messageUp(i)
}

func (b *Buffer) newestEntry() *Entry {
	active := b.activeEntries()
	if len(active) == 0 {
		return nil
	}

	return active[len(active)-1]
}

func (b *Buffer) cleanup() {
	if b.filterActive || !b.caughtUp() {
		return
	}

	if len(b.entries) < cleanupThreshold {
		return
	}

	b.entries = b.entries[len(b.entries)-int(cleanupAfterMessage):]
	b.recalculateLines()

	keep := make(map[string]struct{}, len(b.entries))
	for _, entry := range b.entries {
		msg, ok := entry.Event.Message.(*twitchirc.PrivateMessage)
		if !ok {
			continue
		}

		keep[strings.ToLower(msg.LoginName)] = struct{}{}
	}
	b.palette.Prune(keep)
}

func (b *Buffer) applyTimeout(msg ChatEventMessage) {
	timeoutMsg, ok := msg.Message.(*twitchirc.ClearChat)
	if !ok || timeoutMsg.UserName == nil {
		return
	}

	b.markDeleted(func(private *twitchirc.PrivateMessage) bool {
		return strings.EqualFold(private.LoginName, *timeoutMsg.UserName)
	})
}

func (b *Buffer) applyDeletion(msg ChatEventMessage) {
	clearMsg, ok := msg.Message.(*twitchirc.ClearMessage)
	if !ok {
		return
	}

	b.markDeleted(func(private *twitchirc.PrivateMessage) bool {
		return strings.EqualFold(private.ID, clearMsg.TargetMsgID)
	})
}

func (b *Buffer) markDeleted(match func(*twitchirc.PrivateMessage) bool) {
	var hasDeleted bool
	for _, entry := range b.entries {
		private, ok := entry.Event.Message.(*twitchirc.PrivateMessage)
		if !ok || entry.Deleted || !match(private) {
			continue
		}

		hasDeleted = true
		entry.Deleted = true
		entry.Event.DisplayModifier.Strikethrough = true
	}

	if hasDeleted {
		b.recalculateLines()
	}
}

func (b *Buffer) clipboardText(entry *Entry) string {
	if entry == nil {
		return ""
	}

	switch msg := entry.Event.Message.(type) {
	case *twitchirc.PrivateMessage:
		return strings.ReplaceAll(msg.Message, string(DuplicateBypass), "")
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

func (b *Buffer) UpdatePort() {
	b.updatePort()
}

func (b *Buffer) updatePort() {
	b.calculateViewport()

	if b.caughtUp() {
		b.newMessages = 0
	}
}

func (b *Buffer) atTail() bool {
	return b.lineEnd >= len(b.lines)
}

func (b *Buffer) caughtUp() bool {
	if b.atTail() {
		return true
	}

	newest := b.newestEntry()

	return newest != nil && newest.selected
}

func (b *Buffer) calculateViewport() {
	height := b.height
	if b.searchActive {
		height--
	}
	if height <= 0 {
		b.lineStart = 0
		b.lineEnd = 0
		return
	}

	if len(b.lines) > 0 {
		b.cursor = min(max(b.cursor, 0), len(b.lines)-1)
	} else {
		b.cursor = 0
		b.lineStart = 0
		b.lineEnd = 0
		return
	}

	if height >= len(b.lines) {
		b.lineStart = 0
		b.lineEnd = len(b.lines)
		return
	}

	if b.cursor < b.lineStart {
		b.lineStart = b.cursor
	} else if b.cursor >= b.lineStart+height {
		b.lineStart = b.cursor - height + 1
	}

	b.lineEnd = b.lineStart + height

	if b.lineEnd > len(b.lines) {
		b.lineEnd = len(b.lines)
		b.lineStart = b.lineEnd - height
		if b.lineStart < 0 {
			b.lineStart = 0
		}
	}
}

func (b *Buffer) recalculateLines() {
	entries := b.activeEntries()

	if len(entries) < 1 && len(b.lines) < 1 {
		return
	}

	_, selected := b.entryForCurrentCursor()

	b.lines = make([]string, 0, len(b.lines))

	var prevEntry *Entry

	for _, entry := range entries {
		lastCursorEnd := -1

		if prevEntry != nil {
			lastCursorEnd = prevEntry.position.cursorEnd
		}

		lines := b.renderer.Render(entry.Event)
		b.lines = append(b.lines, lines...)

		entry.position.cursorStart = lastCursorEnd + 1
		entry.position.cursorEnd = lastCursorEnd + len(lines)
		prevEntry = entry
	}

	if selected != nil {
		b.cursor = selected.position.cursorEnd
	}

	b.updatePort()
	b.snapScroll()
}

func (b *Buffer) activeEntries() []*Entry {
	if !b.filterActive {
		return b.entries
	}

	if !b.filterDirty && b.filteredItems != nil {
		return b.filteredItems
	}

	b.filteredItems = make([]*Entry, 0, b.height)
	for _, entry := range b.entries {
		if !entry.filtered {
			b.filteredItems = append(b.filteredItems, entry)
		}
	}
	b.filterDirty = false

	return b.filteredItems
}

func (b *Buffer) invalidateFiltered() {
	b.filterDirty = true
	b.filteredItems = nil
}

func (b *Buffer) clearFilter() {
	b.filterActive = false

	var last *Entry
	for _, entry := range b.entries {
		entry.filtered = false
		entry.selected = false
		last = entry
	}

	if last != nil {
		last.selected = true
	}

	b.invalidateFiltered()
	b.recalculateLines()
	b.moveToBottom()
}

func (b *Buffer) entryMatchesSearch(entry *Entry) bool {
	if b.currentMatcher == nil {
		return false
	}

	cast, ok := entry.Event.Message.(*twitchirc.PrivateMessage)
	if !ok {
		return false
	}

	return b.currentMatcher.Match(cast)
}

func (b *Buffer) applyIndicatorToVisible(visible []string, renderStart int) {
	renderEnd := renderStart + len(visible)

	_, target := b.entryForCurrentCursor()

	if target == nil || target.position.cursorEnd < renderStart || target.position.cursorStart >= renderEnd {
		target = nil
		active := b.activeEntries()
		for i := len(active) - 1; i >= 0; i-- {
			entry := active[i]
			if entry.position.cursorEnd < renderStart {
				break
			}
			if entry.position.cursorStart < renderEnd {
				target = entry
				break
			}
		}
	}

	if target == nil {
		return
	}

	lo := max(target.position.cursorStart, renderStart) - renderStart
	hi := min(target.position.cursorEnd+1, renderEnd) - renderStart

	for i := lo; i < hi; i++ {
		visible[i] = b.indicator + " " + strings.TrimPrefix(visible[i], "  ")
	}
}

func (b *Buffer) handleReplyReferenceClick(line, col int) bool {
	if col < b.renderer.timePaddingWidth {
		return false
	}

	_, entry := b.entryAtLine(line)
	if entry == nil || line != entry.position.cursorStart {
		return false
	}

	msg, ok := entry.Event.Message.(*twitchirc.PrivateMessage)
	if !ok {
		return false
	}

	ctx, ok := b.renderer.replyContextFor(msg)
	if !ok {
		return false
	}

	for _, candidate := range b.activeEntries() {
		if parent, ok := candidate.Event.Message.(*twitchirc.PrivateMessage); ok && parent.ID == ctx.ParentID {
			b.goToEntry(candidate)
			return true
		}
	}

	return false
}

func (b *Buffer) emoteNameAt(row, col int) (string, bool) {
	if b.searchActive {
		row-- // the first row is the search widget
	}

	if row < 0 || row >= b.height {
		return "", false
	}

	line := b.ViewportRenderStart() + row
	if line < 0 || line >= len(b.lines) {
		return "", false
	}

	var id int32
	found := false
	for _, span := range kittyimg.PlaceholderSpans(b.lines[line]) {
		if col >= span.Col && col < span.Col+span.Width {
			id, found = span.ID, true
			break
		}
	}

	if !found {
		return "", false
	}

	_, entry := b.entryAtLine(line)
	if entry == nil || line < entry.position.cursorStart || line > entry.position.cursorEnd {
		return "", false
	}

	for word, replacement := range entry.Event.DisplayModifier.WordReplacements {
		if replacementID, ok := kittyimg.PlaceholderID(replacement); ok && replacementID == id {
			return word, true
		}
	}

	return "", false
}

func (b *Buffer) DebugDump() {
	type state struct {
		Lines     []string
		Cursor    int
		LineStart int
		LineEnd   int
		Entries   []*Entry
		UserCache []string
	}

	dump := state{
		Cursor:    b.cursor,
		LineEnd:   b.lineEnd,
		LineStart: b.lineStart,
		Entries:   b.entries,
	}
	dump.UserCache = b.palette.Names()

	f, err := os.Create("chat_dump.json")
	if err != nil {
		panic(err)
	}
	defer func() { _ = f.Close() }()

	bytes, err := json.Marshal(dump)
	if err != nil {
		panic(err)
	}

	_, _ = f.Write([]byte(StripANSI(string(bytes))))
}

func (b *Buffer) Badge() (string, int) {
	return b.newMessageBadge()
}

func (b *Buffer) BadgeSpan(renderStart, height, width int) (row, col int, ok bool) {
	return b.newMessageBadgeSpan(renderStart, height, width)
}

func (b *Buffer) BadgeHit(row, col int) bool {
	return b.newMessageBadgeHit(row, col)
}

func (b *Buffer) TimePaddingWidth() int {
	return b.renderer.timePaddingWidth
}

func (b *Buffer) CursorLine() int {
	_, entry := b.entryForCurrentCursor()
	if entry == nil {
		return 0
	}

	return entry.position.cursorStart
}

func (b *Buffer) NewestPtr() *Entry {
	return b.newestEntry()
}

func (b *Buffer) RawRows() []string {
	return b.lines
}
