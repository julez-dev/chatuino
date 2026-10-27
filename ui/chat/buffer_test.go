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

func newTestBuffer(t *testing.T, height, entryCount int) *Buffer {
	t.Helper()

	c := New(40, height, save.BuildDefaultSettings(), save.BuildDefaultTheme())

	for i := 0; i < entryCount; i++ {
		addPrivateMessage(t, c, fmt.Sprintf("msg-%d", i), fmt.Sprintf("line %d", i))
	}

	c.MoveToBottom()
	return c
}

func lineCount(c *Buffer) int {
	entries := c.Entries()
	if len(entries) == 0 {
		return 0
	}

	_, end, ok := c.LineRange(entries[len(entries)-1].ID)
	if !ok {
		return 0
	}

	return end + 1
}

func selectedLine(t *testing.T, c *Buffer) int {
	t.Helper()

	entry, ok := c.Selected()
	require.True(t, ok)
	start, _, ok := c.LineRange(entry.ID)
	require.True(t, ok)

	return start
}

func TestBuffer_SelectAtViewportRow(t *testing.T) {
	t.Parallel()

	c := newTestBuffer(t, 4, 10)

	require.Equal(t, 6, c.ViewportRenderStart(), "cursor at bottom should scroll viewport to the last page")

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

func TestBuffer_PageUpDown(t *testing.T) {
	t.Parallel()

	c := newTestBuffer(t, 4, 10)
	require.Equal(t, 9, selectedLine(t, c))

	c.PageUp()
	require.Equal(t, 6, selectedLine(t, c))

	c.PageUp()
	require.Equal(t, 3, selectedLine(t, c))
	require.Equal(t, 3, c.ViewportRenderStart(), "viewport should follow the selection upwards")

	c.PageDown()
	require.Equal(t, 6, selectedLine(t, c))

	c.PageDown()
	require.Equal(t, 9, selectedLine(t, c))

	// Already at the bottom: page down is a no-op.
	c.PageDown()
	require.Equal(t, 9, selectedLine(t, c))
}

func TestBuffer_ScrollByLines(t *testing.T) {
	t.Parallel()

	c := newTestBuffer(t, 4, 10)

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

func TestBuffer_ScrollByLinesAdvancesThroughTallEntry(t *testing.T) {
	t.Parallel()

	c := New(40, 4, save.BuildDefaultSettings(), save.BuildDefaultTheme())
	first := addPrivateMessage(t, c, "tall", strings.Repeat("word ", 20))
	for i := 0; i < 6; i++ {
		addPrivateMessage(t, c, fmt.Sprintf("short-%d", i), "short")
	}

	c.MoveToTop()
	startLine := c.ViewportRenderStart()

	for range 3 {
		c.ScrollByLines(3)
	}

	selected, ok := c.Selected()
	require.True(t, ok)
	require.NotEqual(t, first.ID, selected.ID, "wheel-down must not stay stuck in a tall selected message")
	require.Greater(t, c.ViewportRenderStart(), startLine, "wheel-down must eventually move the viewport")
}

func TestBuffer_EntryAtLineClamps(t *testing.T) {
	t.Parallel()

	c := newTestBuffer(t, 4, 3)

	_, first := c.EntryAtLine(-10)
	start, _, _ := c.LineRange(first.ID)
	require.Equal(t, 0, start)

	_, last := c.EntryAtLine(100)
	start, _, _ = c.LineRange(last.ID)
	require.Equal(t, 2, start)
}

func TestBuffer_ClipboardText(t *testing.T) {
	t.Parallel()

	user := "bob"
	banDuration := 600

	tests := []struct {
		name    string
		message ChatEventMessage
		want    string
	}{
		{
			name:    "private message strips the duplicate bypass rune",
			message: ChatEventMessage{Message: &twitchirc.PrivateMessage{Message: "hello " + string(DuplicateBypass) + "world"}},
			want:    "hello world",
		},
		{
			name:    "notice",
			message: ChatEventMessage{Message: &twitchirc.Notice{Message: "notice text"}},
			want:    "notice text",
		},
		{
			name:    "cleared message",
			message: ChatEventMessage{Message: &twitchirc.ClearMessage{Login: "bob"}},
			want:    "A message from bob was removed.",
		},
		{
			name:    "timeout",
			message: ChatEventMessage{Message: &twitchirc.ClearChat{UserName: &user, BanDuration: &banDuration}},
			want:    "bob was timed out for 10 minutes",
		},
		{
			name:    "announcement includes the author",
			message: ChatEventMessage{Message: &twitchirc.AnnouncementMessage{UserNotice: twitchirc.UserNotice{DisplayName: "alice"}, Message: "hello chat"}},
			want:    "alice: hello chat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := New(40, 10, save.BuildDefaultSettings(), save.BuildDefaultTheme())
			c.Append(tt.message)

			entry, ok := c.Newest()
			require.True(t, ok)
			require.Equal(t, tt.want, c.ClipboardText(entry.ID))
		})
	}

	c := New(40, 10, save.BuildDefaultSettings(), save.BuildDefaultTheme())
	require.Empty(t, c.ClipboardText(EntryID(999)), "an unknown entry has no text")
}

func newRenderedBuffer(t *testing.T, width, height int) *Buffer {
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

// TestBuffer_SelectionSurvivesReflow ensures the rendered-line index is
// rebuilt without losing the selected message when the width changes.
func TestBuffer_SelectionSurvivesReflow(t *testing.T) {
	t.Parallel()

	c := newRenderedBuffer(t, 80, 6)

	for i := 0; i < 5; i++ {
		addPrivateMessage(t, c, fmt.Sprintf("msg-%d", i), strings.Repeat("word ", 10))
	}

	target := c.Entries()[1]
	targetStart, _, ok := c.LineRange(target.ID)
	require.True(t, ok)
	beforeLines := lineCount(c)

	c.Select(target.ID)
	require.Equal(t, targetStart, selectedLine(t, c))

	c.Resize(30, 6)

	selected, ok := c.Selected()
	require.True(t, ok)
	require.Equal(t, target.ID, selected.ID, "the same message stays selected after reflow")
	require.Greater(t, lineCount(c), beforeLines, "narrower width must produce more rendered rows")

	entries := c.Entries()
	for i, entry := range entries {
		startLine, endLine, ok := c.LineRange(entry.ID)
		require.True(t, ok)
		require.LessOrEqual(t, startLine, endLine)

		if i == 0 {
			require.Equal(t, 0, startLine)
			continue
		}

		_, previousEnd, _ := c.LineRange(entries[i-1].ID)
		require.Equal(t, previousEnd+1, startLine, "positions stay contiguous after reflow")
	}
}

// TestBuffer_DeletionRebuildsLinePositions checks that marking a message
// deleted restyles it and recalculates every position.
func TestBuffer_DeletionRebuildsLinePositions(t *testing.T) {
	t.Parallel()

	c := newRenderedBuffer(t, 80, 10)

	first := addPrivateMessage(t, c, "msg-1", "first message")
	second := addPrivateMessage(t, c, "msg-2", "second message")

	c.applyDeletion(ChatEventMessage{Message: &twitchirc.ClearMessage{
		Login:       "bob",
		TargetMsgID: second.Event.Message.(*twitchirc.PrivateMessage).ID,
	}})

	entries := c.Entries()
	var deleted Entry
	for _, entry := range entries {
		if entry.ID == second.ID {
			deleted = entry
		}
	}

	require.True(t, deleted.Deleted)
	require.True(t, deleted.Event.DisplayModifier.Strikethrough)
	require.False(t, first.Deleted)

	rows, _ := c.Rows(c.height)
	require.Contains(t, strings.Join(rows, "\n"), "\x1b[9m")

	for i, entry := range entries {
		start, _, ok := c.LineRange(entry.ID)
		require.True(t, ok)
		if i == 0 {
			require.Equal(t, 0, start)
			continue
		}

		_, previousEnd, _ := c.LineRange(entries[i-1].ID)
		require.Equal(t, previousEnd+1, start)
	}
}

// TestBuffer_SearchFiltersRebuildLinePositions covers the search seam: only
// matching messages keep rendered rows, and their positions are recalculated.
func TestBuffer_SearchFiltersRebuildLinePositions(t *testing.T) {
	t.Parallel()

	c := newRenderedBuffer(t, 80, 10)

	addPrivateMessage(t, c, "msg-1", "alpha")
	addPrivateMessage(t, c, "msg-2", "beta target")
	addPrivateMessage(t, c, "msg-3", "gamma")

	c.SetFilter("beta")

	require.Equal(t, 1, c.FilterStatus().Matches)
	rows, _ := c.Rows(c.height)
	require.Contains(t, strings.Join(rows, "\n"), "beta target")
	require.NotContains(t, strings.Join(rows, "\n"), "gamma")

	c.ClearFilter()

	entries := c.Entries()
	require.Len(t, entries, 3)

	for i, entry := range entries {
		start, _, ok := c.LineRange(entry.ID)
		require.True(t, ok)
		if i == 0 {
			require.Equal(t, 0, start)
			continue
		}

		_, previousEnd, _ := c.LineRange(entries[i-1].ID)
		require.Equal(t, previousEnd+1, start)
	}
}

func TestBuffer_EmoteNameAt(t *testing.T) {
	t.Parallel()

	c := newTestBuffer(t, 4, 3)

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

// TestBuffer_SmoothScrollFollowIsBounded pins the frame budget of the follow
// animation. Every frame re-renders the whole layout and chat keeps arriving while
// the window scrolls, so an animation that needs dozens of frames to converge never
// settles at real message rates: it is re-armed by the next message and the 60Hz
// tick runs for as long as the channel is active.
func TestBuffer_SmoothScrollFollowIsBounded(t *testing.T) {
	t.Parallel()

	// frameBudget is asserted as a literal so the test keeps catching a raise of the
	// constant: one follow animation may not cost more than a fifth of a second of
	// rendering.
	const frameBudget = 12

	newAnimating := func(t *testing.T) *Buffer {
		t.Helper()

		c := newTestBuffer(t, 10, 40)
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

// TestBuffer_ReplyContextKeepsItsRow pins that the reference row belongs to the reply
// entry, so selection, wrapping and cleanup treat it as part of the message.
func TestBuffer_ReplyContextKeepsItsRow(t *testing.T) {
	t.Parallel()

	c, reply := replyTestWindow(t, save.BuildDefaultSettings())
	start, end, ok := c.LineRange(reply.ID)
	require.True(t, ok)
	require.Equal(t, start+1, end)

	rows := viewRows(t, c)
	reference := rows[start-c.ViewportRenderStart()]

	require.Contains(t, ansi.Strip(reference), "↩ Alice: that was a great play")
	require.Contains(t, ansi.Strip(rows[end-c.ViewportRenderStart()]), fmt.Sprintf("%s: no it wasn't", "carol"))
}

func TestBuffer_UpdateNotice(t *testing.T) {
	t.Parallel()

	c := newRenderedBuffer(t, 80, 10)
	c.Append(ChatEventMessage{Message: &twitchirc.Notice{
		MsgID:   "redemption-1",
		Message: "waiting for fulfilment",
	}})

	require.True(t, c.ReplaceNotice("redemption-1", "fulfilled"))
	require.Len(t, c.Entries(), 1, "updating a notice must not append an entry")
	rows, _ := c.Rows(c.height)
	require.Contains(t, plain(rows), "fulfilled")
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
	rows, _ := b.Rows(b.height)
	require.Contains(t, strings.Join(rows, "\n"), "beta target", "messages keep rendering after the filter is cleared")
}

func TestBuffer_ShortQueryCountsRunesNotBytes(t *testing.T) {
	t.Parallel()

	b := New(80, 10, save.BuildDefaultSettings(), save.BuildDefaultTheme())
	status := b.SetFilter("日本")
	require.False(t, status.Applied, "two runes must still count as a short query")
	require.Equal(t, "type at least 3 characters", status.Hint)
}
