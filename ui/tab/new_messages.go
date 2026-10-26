package tab

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/julez-dev/chatuino/kittyimg"
)

// newMessageBadgeScanRows bounds how far above the bottom edge the badge looks for a
// row to occupy: the tallest image plus the row it is bottom-aligned with. Scanning
// further would place the badge in the middle of the chat instead of at its edge.
const newMessageBadgeScanRows = kittyimg.MaxRows + 1

// newMessageBadgeText is the label of the missed-message badge, for example
// "↓ 3 new messages". The counter is capped so the badge keeps its width.
func (c *chatWindow) newMessageBadgeText() string {
	switch {
	case c.newMessages < 1:
		return ""
	case c.newMessages == 1:
		return "↓ 1 new message"
	case c.newMessages > 99:
		return "↓ 99+ new messages"
	}

	return fmt.Sprintf("↓ %d new messages", c.newMessages)
}

// newMessageBadge renders the badge and reports the cells it needs. Both come from
// here so that measuring and drawing cannot disagree about its width.
func (c *chatWindow) newMessageBadge() (string, int) {
	badge := c.newMessageStyle.Render(c.newMessageBadgeText())

	return badge, lipgloss.Width(badge)
}

// newMessageIndicatorEnabled reports whether this window counts and shows new
// messages. The setting is read in one place so counting and drawing cannot drift
// apart.
func (c *chatWindow) newMessageIndicatorEnabled() bool {
	return c.settings.Chat.NewMessageIndicator
}

// newMessageBadgeVisible reports whether this window shows the badge at all.
func (c *chatWindow) newMessageBadgeVisible() bool {
	return c.newMessages > 0 && c.newMessageIndicatorEnabled()
}

// countNewMessage records a message that ended up outside the viewport. A message is
// only in view when the viewport reaches its last rendered line, so a buffer that
// exactly fills the viewport still reports messages that landed below the fold.
func (c *chatWindow) countNewMessage(entry *chatEntry) {
	if entry.Position.CursorEnd < c.lineEnd || !c.newMessageIndicatorEnabled() {
		return
	}

	c.newMessages++
}

// newMessageBadgeSpan locates a badge of the given width in the view, where
// renderStart is the first rendered line and height the number of rows. The badge
// sits on the bottom-most row that carries text, so it stays attached to the chat
// instead of floating in the padding of a partly filled buffer, and it steps over
// rows that hold a graphic emote: a kitty placeholder owns those cells and the image
// would cover the badge.
func (c *chatWindow) newMessageBadgeSpan(renderStart, height, width int) (row, col int, ok bool) {
	if height < 1 {
		return 0, 0, false
	}

	// One column of breathing room between the badge and the pane edge.
	col = c.width - width - 1
	if col < 1 {
		return 0, 0, false
	}

	for i := height - 1; i >= 0 && i > height-1-newMessageBadgeScanRows; i-- {
		line := renderStart + i
		if line < 0 || line >= len(c.lines) {
			continue
		}

		if lipgloss.Width(c.lines[line]) == 0 {
			continue
		}

		if hasPlaceholderIn(c.lines[line], col, col+width) {
			continue
		}

		return i, col, true
	}

	return 0, 0, false
}

// drawNewMessageBadge overlays the badge on the right edge of the row found by
// newMessageBadgeSpan. That row is truncated to make room, so a long message loses
// its tail for as long as the badge is up.
func (c *chatWindow) drawNewMessageBadge(view []string, renderStart, height int) {
	if !c.newMessageBadgeVisible() {
		return
	}

	badge, width := c.newMessageBadge()

	row, col, ok := c.newMessageBadgeSpan(renderStart, height, width)
	if !ok || row < 0 || row >= len(view) {
		return
	}

	prefix := ansi.Truncate(view[row], col, "")
	if pad := col - lipgloss.Width(prefix); pad > 0 {
		prefix += strings.Repeat(" ", pad)
	}

	// The truncated message may leave a style open, so reset it first: an italic or
	// underlined message would otherwise format the badge as well.
	view[row] = prefix + ansi.ResetStyle + badge
}

// handleNewMessageBadgeClick catches up with the chat when a click on the chat view,
// relative to its top-left cell, lands on the badge, and reports whether the click was
// consumed. It lives here so every tab wires its chat clicks the same way.
func (c *chatWindow) handleNewMessageBadgeClick(row, col int) bool {
	if !c.newMessageBadgeHit(row, col) {
		return false
	}

	c.jumpToNewest()

	return true
}

// newMessageBadgeHit reports whether the given cell is covered by the badge. The badge
// then means "show me the new messages" rather than "select the message underneath it".
func (c *chatWindow) newMessageBadgeHit(row, col int) bool {
	if c.state == searchChatWindowState || !c.newMessageBadgeVisible() {
		return false
	}

	_, badgeWidth := c.newMessageBadge()

	badgeRow, badgeCol, ok := c.newMessageBadgeSpan(c.viewportRenderStart(), c.height, badgeWidth)
	if !ok {
		return false
	}

	return row == badgeRow && col >= badgeCol && col < badgeCol+badgeWidth
}

// jumpToNewest selects the newest message and snaps the viewport to it, so the
// badge disappears in the same frame.
func (c *chatWindow) jumpToNewest() {
	c.moveToBottom()
	c.snapScroll()
}

// hasPlaceholderIn reports whether any image placeholder of line covers a cell in
// [start, end). The columns are terminal cells, matching kittyimg.PlaceholderSpan.
func hasPlaceholderIn(line string, start, end int) bool {
	for _, span := range kittyimg.PlaceholderSpans(line) {
		if span.Col < end && span.Col+span.Width > start {
			return true
		}
	}

	return false
}
