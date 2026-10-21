package tab

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/stretchr/testify/require"
)

func leftClick(y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{Button: tea.MouseLeft, X: 1, Y: y}
}

func wheelUp(y int) tea.MouseWheelMsg {
	return tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: 1, Y: y}
}

// Test_Broadcast_MouseClickChatReachesTheMessageUnderTheCursor verifies that every
// row of the chat region maps to the message rendered there, after the resize that
// established the layout.
func Test_Broadcast_MouseClickChatReachesTheMessageUnderTheCursor(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
	addTestMessages(t, tab.chatWindow, 40)
	tab.HandleResize()

	require.True(t, tab.layout.chat.visible())

	for row := 0; row < tab.layout.chat.height; row++ {
		want := tab.chatWindow.viewportRenderStart() + row

		_ = tab.HandleMouseClick(leftClick(tab.layout.chat.top+row), 0, tab.layout.chat.top+row)

		require.Equal(t, want, selectedCursor(t, tab.chatWindow), "row %d", row)
		require.Equal(t, inChatWindow, tab.state)
	}
}

// Test_Broadcast_MouseClickComposer checks the composer hit box, including after
// the composer grows and the chat window shrinks.
func Test_Broadcast_MouseClickComposer(t *testing.T) {
	t.Parallel()

	account := save.Account{ID: "acc", DisplayName: "me"}

	t.Run("composer top row starts insert mode", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, account)

		y := tab.layout.composer.top
		_ = tab.HandleMouseClick(leftClick(y), 0, y)

		require.Equal(t, insertMode, tab.state)
	})

	t.Run("status bar does not start insert mode", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, account)

		y := tab.layout.status.top
		require.True(t, tab.layout.status.visible())
		_ = tab.HandleMouseClick(leftClick(y), 0, y)

		require.Equal(t, inChatWindow, tab.state, "the status bar is not part of the composer")
	})

	t.Run("hit box follows a wrapped composer", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 40, 24, account)
		addTestMessages(t, tab.chatWindow, 40)
		oldHeight := tab.layout.composer.height

		tab.input.SetValue(strings.Repeat("word ", 60))
		tab.HandleResize()

		require.Greater(t, tab.layout.composer.height, oldHeight, "the composer grew")
		require.Less(t, tab.layout.composer.top, tab.height-tab.layout.composer.height, "the taller composer starts higher")

		// The row just above the composer belongs to the chat window.
		above := tab.layout.composer.top - 1
		require.True(t, tab.layout.chat.contains(above))

		_ = tab.HandleMouseClick(leftClick(above), 0, above)
		require.Equal(t, inChatWindow, tab.state)

		_ = tab.HandleMouseClick(leftClick(tab.layout.composer.top), 0, tab.layout.composer.top)
		require.Equal(t, insertMode, tab.state)
	})

	t.Run("anonymous account has no composer", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "anon", IsAnonymous: true})
		addTestMessages(t, tab.chatWindow, 40)
		tab.HandleResize()

		// The bottom row is the status bar, not a composer.
		y := tab.height - 1
		require.True(t, tab.layout.status.contains(y))
		_ = tab.HandleMouseClick(leftClick(y), 0, y)

		require.Equal(t, inChatWindow, tab.state)
	})
}

// Test_Broadcast_MouseClickUserInspect covers both inspect regions: the inspect
// window scrolls its own history and the main chat still selects messages.
func Test_Broadcast_MouseClickUserInspect(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
	addTestMessages(t, tab.chatWindow, 40)
	tab.userInspect = newUserInspect(tab.id, tab.width, tab.height, "someone", tab.channelLogin, tab.account.ID,
		tab.settings, tab.theme, tab.keymap, tab.client.API, tab.messageLogger, tab.emoteReplacer, tab.badgeReplacer)

	tab.state = userInspectMode
	tab.HandleResize()
	addTestMessages(t, tab.userInspect.chatWindow, 20)
	tab.HandleResize()

	require.True(t, tab.layout.userInspect.visible())

	// Only the history below the info banner is clickable.
	chat := tab.inspectChatSpan()
	require.True(t, chat.visible())
	require.Greater(t, chat.top, tab.layout.userInspect.top, "the info banner sits above the history")

	row := chat.height - 1
	want := tab.userInspect.chatWindow.viewportRenderStart() + row
	_ = tab.HandleMouseClick(leftClick(chat.top+row), 0, chat.top+row)

	require.Equal(t, want, selectedCursor(t, tab.userInspect.chatWindow))
	require.Equal(t, userInspectMode, tab.state)

	// A click on the info banner does not select history.
	before := selectedCursor(t, tab.userInspect.chatWindow)
	_ = tab.HandleMouseClick(leftClick(tab.layout.userInspect.top), 0, tab.layout.userInspect.top)
	require.Equal(t, before, selectedCursor(t, tab.userInspect.chatWindow))

	// Clicking the main chat leaves inspect mode and selects there.
	_ = tab.HandleMouseClick(leftClick(tab.layout.chat.top), 0, tab.layout.chat.top)
	require.Equal(t, inChatWindow, tab.state)
}

// Test_Broadcast_MouseWheel verifies that the wheel scrolls the region under the
// cursor and ignores the composer.
func Test_Broadcast_MouseWheel(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
	addTestMessages(t, tab.chatWindow, 40)
	tab.HandleResize()

	start := tab.chatWindow.cursor
	_ = tab.HandleMouseWheel(wheelUp(tab.layout.chat.top+1), 0, tab.layout.chat.top+1)
	require.Less(t, tab.chatWindow.cursor, start, "the wheel over the chat scrolls it")

	scrolled := tab.chatWindow.cursor
	_ = tab.HandleMouseWheel(wheelUp(tab.layout.composer.top), 0, tab.layout.composer.top)
	require.Equal(t, scrolled, tab.chatWindow.cursor, "the composer does not scroll the chat")

	// The status bar is not a scroll target either.
	_ = tab.HandleMouseWheel(wheelUp(tab.layout.status.top), 0, tab.layout.status.top)
	require.Equal(t, scrolled, tab.chatWindow.cursor)
}

// Test_Broadcast_MouseWheelUserInspect scrolls the inspect history when the cursor
// is over it.
func Test_Broadcast_MouseWheelUserInspect(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
	addTestMessages(t, tab.chatWindow, 40)
	tab.userInspect = newUserInspect(tab.id, tab.width, tab.height, "someone", tab.channelLogin, tab.account.ID,
		tab.settings, tab.theme, tab.keymap, tab.client.API, tab.messageLogger, tab.emoteReplacer, tab.badgeReplacer)
	tab.state = userInspectMode
	tab.HandleResize()
	addTestMessages(t, tab.userInspect.chatWindow, 20)
	tab.HandleResize()

	chat := tab.inspectChatSpan()
	start := tab.userInspect.chatWindow.cursor
	_ = tab.HandleMouseWheel(wheelUp(chat.top), 0, chat.top)

	require.Less(t, tab.userInspect.chatWindow.cursor, start)
}

// Test_Broadcast_MouseInVerticalMode verifies that the layout recorded for
// vertical tab mode matches what Root renders: the status bar lives outside the
// tab, and the chat gets the extra rows.
func Test_Broadcast_MouseInVerticalMode(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 70, 24, save.Account{ID: "acc", DisplayName: "me"})
	tab.SetFullWidth(80)
	tab.HandleResize()
	addTestMessages(t, tab.chatWindow, 40)
	tab.HandleResize()

	require.False(t, tab.layout.status.visible())
	require.Equal(t, tab.height, lipgloss.Height(tab.ViewWithoutStatusBar()))

	// The row above the composer is a chat row, not the status bar.
	last := tab.layout.composer.top - 1
	require.True(t, tab.layout.chat.contains(last))

	want := tab.chatWindow.viewportRenderStart() + (last - tab.layout.chat.top)
	_ = tab.HandleMouseClick(leftClick(last), 0, last)
	require.Equal(t, want, selectedCursor(t, tab.chatWindow))

	_ = tab.HandleMouseClick(leftClick(tab.layout.composer.top), 0, tab.layout.composer.top)
	require.Equal(t, insertMode, tab.state)
}

// Test_Broadcast_MouseClickEmoteOverview ignores clicks in the overview.
func Test_Broadcast_MouseClickEmoteOverview(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
	tab.state = emoteOverviewMode
	tab.emoteOverview = NewEmoteOverview(tab.channelID, tab.emoteCache, tab.emoteReplacer, 1, tab.width, tab.height)
	tab.HandleResize()

	require.NotPanics(t, func() {
		_ = tab.HandleMouseClick(leftClick(1), 0, 1)
		_ = tab.HandleMouseWheel(wheelUp(1), 0, 1)
	})
}

// Test_Broadcast_MouseUnloadedTab tolerates events before the children exist.
func Test_Broadcast_MouseUnloadedTab(t *testing.T) {
	t.Parallel()

	tab := newTestTab("tab-1", 80, 24, save.Account{ID: "acc", DisplayName: "me"}, "chan")

	require.NotPanics(t, func() {
		_ = tab.HandleMouseClick(leftClick(1), 0, 1)
		_ = tab.HandleMouseWheel(wheelUp(1), 0, 1)
	})
}

// Test_Broadcast_MouseRowDiagnostics prints nothing but guards the invariant that
// regions never overlap: a row belongs to at most one interactive region.
func Test_Broadcast_MouseRegionsDoNotOverlap(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
	tab.userInspect = newUserInspect(tab.id, tab.width, tab.height, "someone", tab.channelLogin, tab.account.ID,
		tab.settings, tab.theme, tab.keymap, tab.client.API, tab.messageLogger, tab.emoteReplacer, tab.badgeReplacer)
	tab.state = userInspectMode
	tab.HandleResize()

	regions := []struct {
		name string
		span layoutSpan
	}{
		{"chat", tab.layout.chat},
		{"user inspect", tab.layout.userInspect},
		{"composer", tab.layout.composer},
	}

	for y := 0; y < tab.height; y++ {
		hits := make([]string, 0, len(regions))

		for _, region := range regions {
			if region.span.contains(y) {
				hits = append(hits, region.name)
			}
		}

		require.LessOrEqual(t, len(hits), 1, fmt.Sprintf("row %d belongs to %v", y, hits))
	}
}
