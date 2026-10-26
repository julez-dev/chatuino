package tab

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/stretchr/testify/require"
)

func newTestChatWindow(t *testing.T, height, entryCount int) *chatWindow {
	t.Helper()

	c := newChatWindow(40, height, save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap())

	for i := 0; i < entryCount; i++ {
		c.lines = append(c.lines, fmt.Sprintf("line %d", i))
		c.entries = append(c.entries, &chatEntry{
			Position: position{CursorStart: i, CursorEnd: i},
		})
	}

	c.cursor = entryCount - 1
	c.updatePort()

	return c
}

func selectedLine(t *testing.T, c *chatWindow) int {
	t.Helper()

	_, entry := c.entryForCurrentCursor()
	require.NotNil(t, entry)

	return entry.Position.CursorStart
}

func TestChatWindow_SelectAtViewportRow(t *testing.T) {
	t.Parallel()

	c := newTestChatWindow(t, 4, 10)

	require.Equal(t, 6, c.lineStart, "cursor at bottom should scroll viewport to the last page")

	c.selectAtViewportRow(0)
	require.Equal(t, 6, selectedLine(t, c))

	c.selectAtViewportRow(3)
	require.Equal(t, 9, selectedLine(t, c))

	// Out-of-range rows are ignored.
	c.selectAtViewportRow(-1)
	require.Equal(t, 9, selectedLine(t, c))

	c.selectAtViewportRow(4)
	require.Equal(t, 9, selectedLine(t, c))
}

func TestChatWindow_PageUpDown(t *testing.T) {
	t.Parallel()

	c := newTestChatWindow(t, 4, 10)
	require.Equal(t, 9, selectedLine(t, c))

	c.pageUp()
	require.Equal(t, 6, selectedLine(t, c))

	c.pageUp()
	require.Equal(t, 3, selectedLine(t, c))
	require.Equal(t, 3, c.lineStart, "viewport should follow the selection upwards")

	c.pageDown()
	require.Equal(t, 6, selectedLine(t, c))

	c.pageDown()
	require.Equal(t, 9, selectedLine(t, c))

	// Already at the bottom: page down is a no-op.
	c.pageDown()
	require.Equal(t, 9, selectedLine(t, c))
}

func TestChatWindow_ScrollByLines(t *testing.T) {
	t.Parallel()

	c := newTestChatWindow(t, 4, 10)

	c.scrollByLines(-2)
	require.Equal(t, 7, selectedLine(t, c))

	c.scrollByLines(2)
	require.Equal(t, 9, selectedLine(t, c))

	// Scrolling past either end clamps to the first/last message.
	c.scrollByLines(100)
	require.Equal(t, 9, selectedLine(t, c))

	c.scrollByLines(-100)
	require.Equal(t, 0, selectedLine(t, c))
}

func TestChatWindow_ScrollByLinesAdvancesThroughTallEntry(t *testing.T) {
	t.Parallel()

	c := newChatWindow(40, 4, save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap())
	c.lines = []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}
	c.entries = []*chatEntry{
		{Position: position{CursorStart: 0, CursorEnd: 5}, Selected: true},
		{Position: position{CursorStart: 6, CursorEnd: 6}},
		{Position: position{CursorStart: 7, CursorEnd: 7}},
		{Position: position{CursorStart: 8, CursorEnd: 8}},
		{Position: position{CursorStart: 9, CursorEnd: 9}},
		{Position: position{CursorStart: 10, CursorEnd: 10}},
		{Position: position{CursorStart: 11, CursorEnd: 11}},
	}
	c.cursor = 0
	c.updatePort()
	startLine := c.lineStart

	for range 3 {
		c.scrollByLines(3)
	}

	_, selected := c.entryForCurrentCursor()
	require.NotSame(t, c.entries[0], selected, "wheel-down must not stay stuck in a tall selected message")
	require.Greater(t, c.lineStart, startLine, "wheel-down must eventually move the viewport")
}

func TestChatWindow_EntryAtLineClamps(t *testing.T) {
	t.Parallel()

	c := newTestChatWindow(t, 4, 3)

	_, first := c.entryAtLine(-10)
	require.Equal(t, 0, first.Position.CursorStart)

	_, last := c.entryAtLine(100)
	require.Equal(t, 2, last.Position.CursorStart)
}

func TestChatWindow_ClipboardText(t *testing.T) {
	t.Parallel()

	c := newChatWindow(40, 10, save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap())

	user := "bob"
	banDuration := 600

	tests := []struct {
		name  string
		entry *chatEntry
		want  string
	}{
		{
			name:  "private message strips the duplicate bypass rune",
			entry: &chatEntry{Event: ChatEventMessage{Message: &twitchirc.PrivateMessage{Message: "hello " + string(duplicateBypass) + "world"}}},
			want:  "hello world",
		},
		{
			name:  "notice",
			entry: &chatEntry{Event: ChatEventMessage{Message: &twitchirc.Notice{Message: "notice text"}}},
			want:  "notice text",
		},
		{
			name:  "cleared message",
			entry: &chatEntry{Event: ChatEventMessage{Message: &twitchirc.ClearMessage{Login: "bob"}}},
			want:  "A message from bob was removed.",
		},
		{
			name:  "timeout",
			entry: &chatEntry{Event: ChatEventMessage{Message: &twitchirc.ClearChat{UserName: &user, BanDuration: &banDuration}}},
			want:  "bob was timed out for 10 minutes",
		},
		{
			name:  "announcement includes the author",
			entry: &chatEntry{Event: ChatEventMessage{Message: &twitchirc.AnnouncementMessage{UserNotice: twitchirc.UserNotice{DisplayName: "alice"}, Message: "hello chat"}}},
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

func newRenderedChatWindow(t *testing.T, width, height int) *chatWindow {
	t.Helper()

	return newChatWindow(width, height, save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap())
}

func addPrivateMessage(t *testing.T, c *chatWindow, id, content string) *chatEntry {
	t.Helper()

	c.handleMessage(ChatEventMessage{Message: &twitchirc.PrivateMessage{
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

	c.selectLine(target.Position.CursorStart)
	require.True(t, target.Selected)
	require.Equal(t, target.Position.CursorStart, c.cursor)

	c.Resize(30, 6)

	require.Same(t, target, c.selectedEntry(), "the same message stays selected after reflow")
	require.Greater(t, len(c.lines), beforeLines, "narrower width must produce more rendered rows")
	require.GreaterOrEqual(t, c.cursor, target.Position.CursorStart)
	require.LessOrEqual(t, c.cursor, target.Position.CursorEnd)

	for i, e := range c.activeEntries() {
		wantStart := 0
		if i > 0 {
			wantStart = c.activeEntries()[i-1].Position.CursorEnd + 1
		}

		require.Equal(t, wantStart, e.Position.CursorStart, "positions stay contiguous after reflow")
	}
}

// selectedEntry returns the currently selected entry, failing the test when there
// is none.
func (c *chatWindow) selectedEntry() *chatEntry {
	_, entry := c.entryForCurrentCursor()
	if entry != nil && entry.Selected {
		return entry
	}

	for _, e := range c.entries {
		if e.Selected {
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

	c.handleMessageDeletion(ChatEventMessage{Message: &twitchirc.ClearMessage{
		Login:       "bob",
		TargetMsgID: second.Event.Message.(*twitchirc.PrivateMessage).ID,
	}})

	require.True(t, second.IsDeleted)
	require.True(t, second.Event.DisplayModifier.Strikethrough)
	require.False(t, first.IsDeleted)

	// Re-rendered lines carry the strike-through sequence.
	require.Contains(t, strings.Join(c.lines, "\n"), "\x1b[9m")

	for i, e := range c.activeEntries() {
		wantStart := 0
		if i > 0 {
			wantStart = c.activeEntries()[i-1].Position.CursorEnd + 1
		}

		require.Equal(t, wantStart, e.Position.CursorStart)
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

	c.state = searchChatWindowState
	c.searchInput.SetValue("beta")
	c.applySearch()

	active := c.activeEntries()
	require.Len(t, active, 1)
	require.Equal(t, 0, active[0].Position.CursorStart)
	require.Equal(t, 1, c.matchCount)

	// The visible lines only contain the matching message.
	require.Len(t, c.lines, 1)
	require.Contains(t, plain(c.lines), "beta target")

	c.handleStopSearchMode()

	active = c.activeEntries()
	require.Len(t, active, 3)

	for i, e := range active {
		wantStart := 0
		if i > 0 {
			wantStart = active[i-1].Position.CursorEnd + 1
		}

		require.Equal(t, wantStart, e.Position.CursorStart)
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

	start := c.viewportRenderStart()
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

	newAnimating := func(t *testing.T) *chatWindow {
		t.Helper()

		c := newTestChatWindow(t, 10, 40)
		c.settings.Chat.SmoothScroll = true
		require.NotNil(t, c.startAnimating())

		return c
	}

	// follow moves the window the way a message arriving at the bottom does.
	follow := func(c *chatWindow) {
		c.lineStart++
		c.updatePort()
		_ = c.startAnimating()
	}

	t.Run("a single follow stops within the frame budget", func(t *testing.T) {
		t.Parallel()

		c := newAnimating(t)

		frames := 0
		for {
			var cmd tea.Cmd
			c, cmd = c.Update(smoothScrollTick{owner: c})
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
				var cmd tea.Cmd
				c, cmd = c.Update(smoothScrollTick{owner: c})
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

// TestChatWindow_ReplyContextKeepsItsRow pins that the reference row belongs to the reply
// entry, so selection, wrapping and cleanup treat it as part of the message.
func TestChatWindow_ReplyContextKeepsItsRow(t *testing.T) {
	t.Parallel()

	c, reply := replyTestWindow(t, save.BuildDefaultSettings())

	require.Equal(t, reply.Position.CursorStart+1, reply.Position.CursorEnd)

	rows := viewRows(t, c)
	reference := rows[reply.Position.CursorStart-c.viewportRenderStart()]

	require.Contains(t, ansi.Strip(reference), "↩ Alice: that was a great play")
	require.Contains(t, ansi.Strip(rows[reply.Position.CursorEnd-c.viewportRenderStart()]), fmt.Sprintf("%s: no it wasn't", "carol"))
}
