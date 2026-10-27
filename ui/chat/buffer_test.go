package chat

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/stretchr/testify/require"
)

func newTestChatWindow(t *testing.T, height, entryCount int) *Buffer {
	t.Helper()

	c := New(40, height, save.BuildDefaultSettings(), save.BuildDefaultTheme())

	for i := 0; i < entryCount; i++ {
		c.lines = append(c.lines, fmt.Sprintf("line %d", i))
		c.entries = append(c.entries, &Entry{
			position: position{cursorStart: i, cursorEnd: i},
		})
	}

	c.cursor = entryCount - 1
	c.updatePort()

	return c
}

func selectedLine(t *testing.T, c *Buffer) int {
	t.Helper()

	_, entry := c.entryForCurrentCursor()
	require.NotNil(t, entry)

	return entry.position.cursorStart
}

func TestChatWindow_SelectAtViewportRow(t *testing.T) {
	t.Parallel()

	c := newTestChatWindow(t, 4, 10)

	require.Equal(t, 6, c.lineStart, "cursor at bottom should scroll viewport to the last page")

	c.SelectAtViewportRow(0)
	require.Equal(t, 6, selectedLine(t, c))

	c.SelectAtViewportRow(3)
	require.Equal(t, 9, selectedLine(t, c))

	// Out-of-range rows are ignored.
	c.SelectAtViewportRow(-1)
	require.Equal(t, 9, selectedLine(t, c))

	c.SelectAtViewportRow(4)
	require.Equal(t, 9, selectedLine(t, c))
}

func TestChatWindow_PageUpDown(t *testing.T) {
	t.Parallel()

	c := newTestChatWindow(t, 4, 10)
	require.Equal(t, 9, selectedLine(t, c))

	c.PageUp()
	require.Equal(t, 6, selectedLine(t, c))

	c.PageUp()
	require.Equal(t, 3, selectedLine(t, c))
	require.Equal(t, 3, c.lineStart, "viewport should follow the selection upwards")

	c.PageDown()
	require.Equal(t, 6, selectedLine(t, c))

	c.PageDown()
	require.Equal(t, 9, selectedLine(t, c))

	// Already at the bottom: page down is a no-op.
	c.PageDown()
	require.Equal(t, 9, selectedLine(t, c))
}

func TestChatWindow_ScrollByLines(t *testing.T) {
	t.Parallel()

	c := newTestChatWindow(t, 4, 10)

	c.ScrollByLines(-2)
	require.Equal(t, 7, selectedLine(t, c))

	c.ScrollByLines(2)
	require.Equal(t, 9, selectedLine(t, c))

	// Scrolling past either end clamps to the first/last message.
	c.ScrollByLines(100)
	require.Equal(t, 9, selectedLine(t, c))

	c.ScrollByLines(-100)
	require.Equal(t, 0, selectedLine(t, c))
}

func TestChatWindow_ScrollByLinesAdvancesThroughTallEntry(t *testing.T) {
	t.Parallel()

	c := New(40, 4, save.BuildDefaultSettings(), save.BuildDefaultTheme())
	c.lines = []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}
	c.entries = []*Entry{
		{position: position{cursorStart: 0, cursorEnd: 5}, selected: true},
		{position: position{cursorStart: 6, cursorEnd: 6}},
		{position: position{cursorStart: 7, cursorEnd: 7}},
		{position: position{cursorStart: 8, cursorEnd: 8}},
		{position: position{cursorStart: 9, cursorEnd: 9}},
		{position: position{cursorStart: 10, cursorEnd: 10}},
		{position: position{cursorStart: 11, cursorEnd: 11}},
	}
	c.cursor = 0
	c.updatePort()
	startLine := c.lineStart

	for range 3 {
		c.ScrollByLines(3)
	}

	_, selected := c.entryForCurrentCursor()
	require.NotSame(t, c.entries[0], selected, "wheel-down must not stay stuck in a tall selected message")
	require.Greater(t, c.lineStart, startLine, "wheel-down must eventually move the viewport")
}

func TestChatWindow_EntryAtLineClamps(t *testing.T) {
	t.Parallel()

	c := newTestChatWindow(t, 4, 3)

	_, first := c.entryAtLine(-10)
	require.Equal(t, 0, first.position.cursorStart)

	_, last := c.entryAtLine(100)
	require.Equal(t, 2, last.position.cursorStart)
}

func TestChatWindow_ClipboardText(t *testing.T) {
	t.Parallel()

	c := New(40, 10, save.BuildDefaultSettings(), save.BuildDefaultTheme())

	user := "bob"
	banDuration := 600

	tests := []struct {
		name  string
		entry *Entry
		want  string
	}{
		{
			name:  "private message strips the duplicate bypass rune",
			entry: &Entry{Event: ChatEventMessage{Message: &twitchirc.PrivateMessage{Message: "hello " + string(duplicateBypass) + "world"}}},
			want:  "hello world",
		},
		{
			name:  "notice",
			entry: &Entry{Event: ChatEventMessage{Message: &twitchirc.Notice{Message: "notice text"}}},
			want:  "notice text",
		},
		{
			name:  "cleared message",
			entry: &Entry{Event: ChatEventMessage{Message: &twitchirc.ClearMessage{Login: "bob"}}},
			want:  "A message from bob was removed.",
		},
		{
			name:  "timeout",
			entry: &Entry{Event: ChatEventMessage{Message: &twitchirc.ClearChat{UserName: &user, BanDuration: &banDuration}}},
			want:  "bob was timed out for 10 minutes",
		},
		{
			name:  "announcement includes the author",
			entry: &Entry{Event: ChatEventMessage{Message: &twitchirc.AnnouncementMessage{UserNotice: twitchirc.UserNotice{DisplayName: "alice"}, Message: "hello chat"}}},
			want:  "alice: hello chat",
		},
		{
			name:  "nil entry",
			entry: nil,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, c.clipboardText(tt.entry))
		})
	}
}

func newRenderedChatWindow(t *testing.T, width, height int) *Buffer {
	t.Helper()

	return New(width, height, save.BuildDefaultSettings(), save.BuildDefaultTheme())
}

func addPrivateMessage(t *testing.T, c *Buffer, id, content string) *Entry {
	t.Helper()

	c.Append(ChatEventMessage{Message: &twitchirc.PrivateMessage{
		ID:          id,
		DisplayName: "Alice",
		LoginName:   "alice",
		Message:     content,
		TMISentTS:   time.Date(2026, 3, 4, 5, 6, 7, 0, time.Local),
	}})

	entries := c.activeEntries()
	require.NotEmpty(t, entries)

	return entries[len(entries)-1]
}

// TestChatWindow_SelectionSurvivesReflow ensures the rendered-line index is
// rebuilt without losing the selected message when the width changes.
func TestChatWindow_SelectionSurvivesReflow(t *testing.T) {
	t.Parallel()

	c := newRenderedChatWindow(t, 80, 6)

	for i := 0; i < 5; i++ {
		addPrivateMessage(t, c, fmt.Sprintf("msg-%d", i), strings.Repeat("word ", 10))
	}

	entries := c.activeEntries()
	target := entries[1]
	beforeLines := len(c.lines)

	c.SelectLine(target.position.cursorStart)
	require.True(t, target.selected)
	require.Equal(t, target.position.cursorStart, c.cursor)

	c.Resize(30, 6)

	require.Same(t, target, c.selectedEntry(), "the same message stays selected after reflow")
	require.Greater(t, len(c.lines), beforeLines, "narrower width must produce more rendered rows")
	require.GreaterOrEqual(t, c.cursor, target.position.cursorStart)
	require.LessOrEqual(t, c.cursor, target.position.cursorEnd)

	for i, e := range c.activeEntries() {
		wantStart := 0
		if i > 0 {
			wantStart = c.activeEntries()[i-1].position.cursorEnd + 1
		}

		require.Equal(t, wantStart, e.position.cursorStart, "positions stay contiguous after reflow")
	}
}

// selectedEntry returns the currently selected entry, failing the test when there
// is none.
func (c *Buffer) selectedEntry() *Entry {
	_, entry := c.entryForCurrentCursor()
	if entry != nil && entry.selected {
		return entry
	}

	for _, e := range c.entries {
		if e.selected {
			return e
		}
	}

	return nil
}

// TestChatWindow_DeletionRebuildsLinePositions checks that marking a message
// deleted restyles it and recalculates every position.
func TestChatWindow_DeletionRebuildsLinePositions(t *testing.T) {
	t.Parallel()

	c := newRenderedChatWindow(t, 80, 10)

	first := addPrivateMessage(t, c, "msg-1", "first message")
	second := addPrivateMessage(t, c, "msg-2", "second message")

	c.applyDeletion(ChatEventMessage{Message: &twitchirc.ClearMessage{
		Login:       "bob",
		TargetMsgID: second.Event.Message.(*twitchirc.PrivateMessage).ID,
	}})

	require.True(t, second.Deleted)
	require.True(t, second.Event.DisplayModifier.Strikethrough)
	require.False(t, first.Deleted)

	// Re-rendered lines carry the strike-through sequence.
	require.Contains(t, strings.Join(c.lines, "\n"), "\x1b[9m")

	for i, e := range c.activeEntries() {
		wantStart := 0
		if i > 0 {
			wantStart = c.activeEntries()[i-1].position.cursorEnd + 1
		}

		require.Equal(t, wantStart, e.position.cursorStart)
	}
}

// TestChatWindow_SearchFiltersRebuildLinePositions covers the search seam: only
// matching messages keep rendered rows, and their positions are recalculated.
func TestChatWindow_SearchFiltersRebuildLinePositions(t *testing.T) {
	t.Parallel()

	c := newRenderedChatWindow(t, 80, 10)

	addPrivateMessage(t, c, "msg-1", "alpha")
	addPrivateMessage(t, c, "msg-2", "beta target")
	addPrivateMessage(t, c, "msg-3", "gamma")

	c.SetFilter("beta")

	active := c.activeEntries()
	require.Len(t, active, 1)
	require.Equal(t, 0, active[0].position.cursorStart)
	require.Equal(t, 1, c.FilterStatus().Matches)

	// The visible lines only contain the matching message.
	require.Len(t, c.lines, 1)
	require.Contains(t, plain(c.lines), "beta target")

	c.ClearFilter()

	active = c.activeEntries()
	require.Len(t, active, 3)

	for i, e := range active {
		wantStart := 0
		if i > 0 {
			wantStart = active[i-1].position.cursorEnd + 1
		}

		require.Equal(t, wantStart, e.position.cursorStart)
	}
}

func TestChatWindow_EmoteNameAt(t *testing.T) {
	t.Parallel()

	c := newTestChatWindow(t, 4, 3)

	emoteLine := "  hi \x1b[38;2;0;0;7m\U0010EEEE\U0010EEEE\x1b[39m there"
	c.lines[2] = emoteLine
	c.entries[2].Event.DisplayModifier.WordReplacements = WordReplacement{
		"Kappa":    "\x1b[38;2;0;0;7m\U0010EEEE\U0010EEEE\x1b[39m",
		"@someone": "colored mention",
	}
	c.updatePort()

	start := c.ViewportRenderStart()
	row := 2 - start

	name, ok := c.emoteNameAt(row, 5)
	require.True(t, ok)
	require.Equal(t, "Kappa", name)

	name, ok = c.emoteNameAt(row, 6)
	require.True(t, ok)
	require.Equal(t, "Kappa", name)

	_, ok = c.emoteNameAt(row, 4)
	require.False(t, ok, "cell before the emote")

	_, ok = c.emoteNameAt(row, 7)
	require.False(t, ok, "cell after the emote")

	_, ok = c.emoteNameAt(row-1, 5)
	require.False(t, ok, "other row")

	_, ok = c.emoteNameAt(-1, 5)
	require.False(t, ok)
}

// TestChatWindow_SmoothScrollFollowIsBounded pins the frame budget of the follow
// animation. Every frame re-renders the whole layout and chat keeps arriving while
// the window scrolls, so an animation that needs dozens of frames to converge never
// settles at real message rates: it is re-armed by the next message and the 60Hz
// tick runs for as long as the channel is active.
func TestChatWindow_SmoothScrollFollowIsBounded(t *testing.T) {
	t.Parallel()

	// frameBudget is asserted as a literal so the test keeps catching a raise of the
	// constant: one follow animation may not cost more than a fifth of a second of
	// rendering.
	const frameBudget = 12

	newAnimating := func(t *testing.T) *Buffer {
		t.Helper()

		c := newTestChatWindow(t, 10, 40)
		c.settings.Chat.SmoothScroll = true
		require.NotNil(t, c.startAnimating())

		return c
	}

	// follow moves the window the way a message arriving at the bottom does.
	follow := func(c *Buffer) {
		c.lineStart++
		c.updatePort()
		_ = c.startAnimating()
	}

	t.Run("a single follow stops within the frame budget", func(t *testing.T) {
		t.Parallel()

		c := newAnimating(t)

		frames := 0
		for {
			cmd, _ := c.Update(smoothScrollTick{owner: c})
			frames++

			require.Less(t, frames, frameBudget*4, "the follow animation must terminate")

			if cmd == nil {
				break
			}
		}

		require.LessOrEqual(t, frames, frameBudget)
		require.Equal(t, float64(c.lineStart), c.smoothLineStart, "the animation snaps onto the target")
	})

	t.Run("a moving chat cannot keep the loop alive", func(t *testing.T) {
		t.Parallel()

		c := newAnimating(t)

		const (
			messages         = 30
			framesPerMessage = 20 // 330ms at 60fps, the rate of a mid-sized channel
		)

		frames := 0
		for range messages {
			follow(c)

			for range framesPerMessage {
				cmd, _ := c.Update(smoothScrollTick{owner: c})
				frames++

				if cmd == nil {
					break
				}
			}
		}

		require.LessOrEqual(t, frames, messages*frameBudget,
			"each message may only animate for the frame budget")
	})
}

func viewRows(t *testing.T, c *Buffer) []string {
	t.Helper()

	rows, _ := c.Rows(c.height)
	require.Len(t, rows, c.height, "the transcript always renders exactly height rows")

	return rows
}

// TestChatWindow_ReplyContextKeepsItsRow pins that the reference row belongs to the reply
// entry, so selection, wrapping and cleanup treat it as part of the message.
func TestChatWindow_ReplyContextKeepsItsRow(t *testing.T) {
	t.Parallel()

	c, reply := replyTestWindow(t, save.BuildDefaultSettings())

	require.Equal(t, reply.position.cursorStart+1, reply.position.cursorEnd)

	rows := viewRows(t, c)
	reference := rows[reply.position.cursorStart-c.ViewportRenderStart()]

	require.Contains(t, ansi.Strip(reference), "↩ Alice: that was a great play")
	require.Contains(t, ansi.Strip(rows[reply.position.cursorEnd-c.ViewportRenderStart()]), fmt.Sprintf("%s: no it wasn't", "carol"))
}

func TestBuffer_UpdateNotice(t *testing.T) {
	t.Parallel()

	c := newRenderedChatWindow(t, 80, 10)
	c.Append(ChatEventMessage{Message: &twitchirc.Notice{
		MsgID:   "redemption-1",
		Message: "waiting for fulfilment",
	}})

	require.True(t, c.ReplaceNotice("redemption-1", "fulfilled"))
	require.Len(t, c.entries, 1, "updating a notice must not append an entry")
	require.Contains(t, plain(c.lines), "fulfilled")
	require.False(t, c.ReplaceNotice("missing", "ignored"))
}

func TestBuffer_ShortQueryAfterActiveFilterKeepsAppending(t *testing.T) {
	t.Parallel()

	b := New(80, 10, save.BuildDefaultSettings(), save.BuildDefaultTheme())
	addPrivateMessage(t, b, "msg-1", "alpha target")

	require.True(t, b.SetFilter("alpha").Applied)
	require.True(t, b.IsFiltering())

	status := b.SetFilter("a")
	require.False(t, status.Applied)
	require.Equal(t, "type at least 3 characters", status.Hint)
	require.False(t, b.IsFiltering(), "a short query must stop the active filter")

	addPrivateMessage(t, b, "msg-2", "beta target")
	require.Contains(t, strings.Join(b.RawRows(), "\n"), "beta target", "messages keep rendering after the filter is cleared")
}

func TestBuffer_ShortQueryCountsRunesNotBytes(t *testing.T) {
	t.Parallel()

	b := New(80, 10, save.BuildDefaultSettings(), save.BuildDefaultTheme())
	status := b.SetFilter("日本")
	require.False(t, status.Applied, "two runes must still count as a short query")
	require.Equal(t, "type at least 3 characters", status.Hint)
}
