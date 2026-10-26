package tab

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/save"
	"github.com/stretchr/testify/require"
)

const newMessageBadgeText = "↓ 1 new message"

// scrolledUpChatWindow returns a window that is filled to the tail and then paged up
// twice, so the tail is out of view and arriving messages are missed rather than
// followed.
func scrolledUpChatWindow(t *testing.T, width, height int, settings save.Settings) *chatWindow {
	t.Helper()

	c := newChatWindow(width, height, settings, save.BuildDefaultTheme(), save.BuildDefaultKeyMap())

	for i := range 12 {
		addPrivateMessage(t, c, fmt.Sprintf("msg-%d", i), fmt.Sprintf("message %d", i))
	}

	require.True(t, c.atTail(), "the fixture starts at the tail")

	c.pageUp()
	c.pageUp()

	require.False(t, c.atTail(), "two pages up must leave the tail")

	return c
}

// viewRows splits the rendered view, stripped of ANSI sequences, into its rows.
func viewRows(t *testing.T, c *chatWindow) []string {
	t.Helper()

	rows := strings.Split(ansi.Strip(c.View()), "\n")
	require.Len(t, rows, c.height, "the view always renders exactly height rows")

	return rows
}

func TestChatWindow_NewMessageBadgeCountsMissedMessages(t *testing.T) {
	t.Parallel()

	c := scrolledUpChatWindow(t, 60, 6, save.BuildDefaultSettings())

	require.NotContains(t, strings.Join(viewRows(t, c), "\n"), "new message", "nothing was missed yet")

	addPrivateMessage(t, c, "missed-0", "first missed message")
	require.Contains(t, strings.Join(viewRows(t, c), "\n"), "↓ 1 new message")
	require.False(t, c.atTail(), "the viewport stays where the user put it")

	addPrivateMessage(t, c, "missed-1", "second missed message")
	addPrivateMessage(t, c, "missed-2", "third missed message")
	require.Contains(t, strings.Join(viewRows(t, c), "\n"), "↓ 3 new messages")

	c.newMessages = 120
	require.Contains(t, strings.Join(viewRows(t, c), "\n"), "↓ 99+ new messages")
}

func TestChatWindow_NewMessageBadgeHiddenAtTail(t *testing.T) {
	t.Parallel()

	c := newChatWindow(60, 6, save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap())

	for i := range 20 {
		addPrivateMessage(t, c, fmt.Sprintf("msg-%d", i), "hello")
	}

	require.True(t, c.atTail())
	require.Zero(t, c.newMessages, "following the tail never counts")
	require.NotContains(t, strings.Join(viewRows(t, c), "\n"), "new message")
}

// TestChatWindow_NewMessageBadgeCountsBelowAFullViewport pins the case where the
// buffer exactly fills the viewport: the tail is visible, but a message that arrives
// with an older message selected lands below the last row, so it must still count.
func TestChatWindow_NewMessageBadgeCountsBelowAFullViewport(t *testing.T) {
	t.Parallel()

	c := newChatWindow(60, 6, save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap())

	for i := range 6 {
		addPrivateMessage(t, c, fmt.Sprintf("msg-%d", i), fmt.Sprintf("message %d", i))
	}

	require.True(t, c.atTail(), "six messages exactly fill the viewport")

	c.selectEntry(c.activeEntries()[0])
	require.True(t, c.atTail(), "selecting an older message leaves the viewport at the tail")

	addPrivateMessage(t, c, "missed", "arrived below the fold")

	require.Equal(t, 1, c.newMessages, "the message is not in view, so it counts")
	require.False(t, c.atTail())
	require.Contains(t, strings.Join(viewRows(t, c), "\n"), newMessageBadgeText)
}

func TestChatWindow_NewMessageBadgeClearsAtTail(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		back func(c *chatWindow)
	}{
		{
			name: "jump to newest",
			back: func(c *chatWindow) { c.jumpToNewest() },
		},
		{
			name: "keyboard pages down",
			back: func(c *chatWindow) {
				for range 10 {
					c.pageDown()
				}
			},
		},
		{
			name: "selecting the newest message",
			back: func(c *chatWindow) {
				active := c.activeEntries()
				c.selectEntry(active[len(active)-1])
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := scrolledUpChatWindow(t, 60, 6, save.BuildDefaultSettings())
			addPrivateMessage(t, c, "missed", "missed message")
			require.Contains(t, strings.Join(viewRows(t, c), "\n"), newMessageBadgeText)

			tt.back(c)

			require.True(t, c.caughtUp())
			require.Zero(t, c.newMessages, "the counter resets once the user is caught up")
			require.NotContains(t, strings.Join(viewRows(t, c), "\n"), "new message")
		})
	}
}

func TestChatWindow_NewMessageBadgeDisabled(t *testing.T) {
	t.Parallel()

	settings := save.BuildDefaultSettings()
	settings.Chat.NewMessageIndicator = false

	c := scrolledUpChatWindow(t, 60, 6, settings)
	addPrivateMessage(t, c, "missed", "missed message")

	require.Zero(t, c.newMessages, "a disabled badge does not even count")
	require.NotContains(t, strings.Join(viewRows(t, c), "\n"), "new message")
}

func TestChatWindow_NewMessageBadgePlacement(t *testing.T) {
	t.Parallel()

	c := scrolledUpChatWindow(t, 60, 6, save.BuildDefaultSettings())
	addPrivateMessage(t, c, "missed", strings.Repeat("a very long message ", 8))

	view := c.View()
	rows := strings.Split(view, "\n")
	require.Len(t, rows, c.height)

	badgeRow := -1
	for i, row := range rows {
		if strings.Contains(ansi.Strip(row), newMessageBadgeText) {
			require.Equal(t, -1, badgeRow, "the badge is drawn once")
			badgeRow = i
		}
	}

	require.Equal(t, c.height-1, badgeRow, "the badge sits on the bottom row")
	require.True(t, strings.HasSuffix(ansi.Strip(rows[badgeRow]), newMessageBadgeText), "flush right, one column to spare")
	require.Equal(t, c.width-1, lipgloss.Width(rows[badgeRow]), "the badge row stops one column short of the pane edge")
}

func TestChatWindow_NewMessageBadgeTooNarrow(t *testing.T) {
	t.Parallel()

	// The badge needs 15 cells plus one column of margin.
	c := scrolledUpChatWindow(t, 16, 6, save.BuildDefaultSettings())
	addPrivateMessage(t, c, "missed", "missed message")

	require.NotContains(t, strings.Join(viewRows(t, c), "\n"), "new message")
	require.False(t, c.NewMessageBadgeHit(c.height-1, 1), "no badge, no hit area")
}

func TestChatWindow_NewMessageBadgeSkipsGraphicEmoteRows(t *testing.T) {
	t.Parallel()

	c := scrolledUpChatWindow(t, 60, 6, save.BuildDefaultSettings())
	addPrivateMessage(t, c, "missed", strings.Repeat("padding ", 6))

	// An image placeholder under the badge's columns owns those cells.
	image := strings.Repeat(" ", 44) + "\x1b[38;2;0;0;7m" + strings.Repeat("\U0010EEEE", 4) + "\x1b[39m"
	require.NotEmpty(t, kittyimg.PlaceholderSpans(image), "the fixture must look like an image to the window")

	bottom := c.viewportRenderStart() + c.height - 1
	c.lines[bottom] = image

	rows := viewRows(t, c)
	require.Contains(t, rows[c.height-2], newMessageBadgeText, "the badge steps over the image row")
	require.NotContains(t, rows[c.height-1], newMessageBadgeText)
}

func TestChatWindow_NewMessageBadgeHit(t *testing.T) {
	t.Parallel()

	c := scrolledUpChatWindow(t, 60, 6, save.BuildDefaultSettings())
	addPrivateMessage(t, c, "missed", "missed message")

	_, width := c.newMessageBadge()

	row, col, ok := c.newMessageBadgeSpan(c.viewportRenderStart(), c.height, width)
	require.True(t, ok)

	require.True(t, c.NewMessageBadgeHit(row, col), "the first badge cell is a hit")
	require.True(t, c.NewMessageBadgeHit(row, col+width-1), "the last badge cell is a hit")
	require.False(t, c.NewMessageBadgeHit(row, col-1), "the cell in front of the badge is a miss")
	require.False(t, c.NewMessageBadgeHit(row-1, col), "the row above the badge is a miss")
	require.False(t, c.NewMessageBadgeHit(row, col+width), "the column after the badge is a miss")

	c.jumpToNewest()
	require.False(t, c.NewMessageBadgeHit(row, col), "the hit area disappears with the badge")
	require.True(t, c.atTail())
}
