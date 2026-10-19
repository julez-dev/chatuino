package mainui

import (
	"fmt"
	"testing"
	"time"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/nav"
	"github.com/stretchr/testify/require"
)

func newTestDeps() *DependencyContainer {
	return &DependencyContainer{
		UserConfig: UserConfiguration{
			Settings: save.BuildDefaultSettings(),
			Theme:    save.BuildDefaultTheme(),
		},
		Keymap: save.BuildDefaultKeyMap(),
	}
}

func newTestChatWindow(t *testing.T, height, entryCount int) *chatWindow {
	t.Helper()

	c := newChatWindow(40, height, newTestDeps())

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

// headerRows builds the registry projection the headers render. active is the
// index of the active row, or -1 for none.
func headerRows(active int, names ...string) []nav.Row {
	rows := make([]nav.Row, len(names))

	for i, name := range names {
		rows[i] = nav.Row{ID: name, Title: name, Identity: "me", Active: i == active}
	}

	return rows
}

func TestHorizontalTabHeader_TabAt(t *testing.T) {
	t.Parallel()

	h := newHorizontalTabHeader(80, newTestDeps())
	rows := headerRows(1, "alpha", "beta", "gamma")

	h.View(rows) // render once so the layout is exercised before hit-testing

	tests := []struct {
		name string
		x, y int
		want string
		ok   bool
	}{
		{"first tab", 4, 1, "alpha", true},
		{"second tab", 19, 0, "beta", true},
		{"third tab", 33, 2, "gamma", true},
		{"left border", 0, 1, "", false},
		{"outside rows", 4, 3, "", false},
		{"beyond tabs", 70, 1, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := h.TabAt(rows, tt.x, tt.y)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

// TabAt is a pure function of the rows it is given: the header keeps no cached
// layout, so a changed projection is picked up immediately.
func TestHorizontalTabHeader_TabAtReflectsRows(t *testing.T) {
	t.Parallel()

	h := newHorizontalTabHeader(80, newTestDeps())
	h.View(headerRows(0, "alpha"))

	rows := headerRows(1, "alpha", "beta")

	got, ok := h.TabAt(rows, 19, 1)
	require.True(t, ok)
	require.Equal(t, "beta", got)
}

func TestVerticalTabHeader_TabAt(t *testing.T) {
	t.Parallel()

	v := newVerticalTabHeader(30, 10, newTestDeps())
	rows := headerRows(1, "alpha", "beta", "gamma")
	v.View(rows)

	tests := []struct {
		name string
		x, y int
		want string
		ok   bool
	}{
		{"first tab", 2, 1, "alpha", true},
		{"second tab", 2, 2, "beta", true},
		{"third tab", 2, 3, "gamma", true},
		{"top border", 2, 0, "", false},
		{"bottom border", 2, 4, "", false},
		{"outside width", 99, 1, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := v.TabAt(rows, tt.x, tt.y)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestVerticalTabHeader_TabAtPaged(t *testing.T) {
	t.Parallel()

	v := newVerticalTabHeader(30, 6, newTestDeps())

	names := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		names = append(names, fmt.Sprintf("c%d", i))
	}

	rows := headerRows(7, names...)
	v.View(rows)

	// Selecting item 7 scrolls to the page starting at index 6.
	require.Equal(t, 6, v.list.Paginator.Page*v.list.Paginator.PerPage)

	for row := 1; row <= v.list.Paginator.PerPage; row++ {
		got, ok := v.TabAt(rows, 2, row)
		require.True(t, ok)
		require.Equal(t, names[5+row], got)
	}
}

func TestBroadcastTab_RegisterClick(t *testing.T) {
	t.Parallel()

	tab := &broadcastTab{}
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
