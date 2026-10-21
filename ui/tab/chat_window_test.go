package tab

import (
	"fmt"
	"testing"
	"time"

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

func TestBroadcastTab_RegisterClick(t *testing.T) {
	t.Parallel()

	tab := &Broadcast{}
	entry := &chatEntry{Position: position{CursorStart: 0, CursorEnd: 2}}
	other := &chatEntry{Position: position{CursorStart: 3, CursorEnd: 4}}

	require.False(t, tab.registerClick(entry, 0))
	require.True(t, tab.registerClick(entry, 2), "second click on the same entry within the window is a double-click")

	require.False(t, tab.registerClick(other, 3), "a different entry is not a double-click")
	require.True(t, tab.registerClick(other, 3), "the tracker follows the new entry")

	require.False(t, tab.registerClick(entry, 10), "a click outside the entry resets the tracker")
	require.False(t, tab.registerClick(entry, 0), "the reset click cannot pair with the earlier one")
	require.False(t, tab.registerClick(nil, 0), "nil entries are ignored")

	tab.lastClickEntry = entry
	tab.lastClickAt = time.Now().Add(-doubleClickWindow - time.Millisecond)
	require.False(t, tab.registerClick(entry, 0), "clicks outside the time window are not a double-click")
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
