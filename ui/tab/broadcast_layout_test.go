package tab

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	tea "charm.land/bubbletea/v2"

	"github.com/julez-dev/chatuino/save"
	"github.com/stretchr/testify/require"
)

// Test_allocateBroadcastLayout pins the row allocation. The chat window and the
// user inspect window share the leftover space, and every visible region starts
// where the previous one ends.
func Test_allocateBroadcastLayout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   layoutInputs
		want broadcastLayout
	}{
		{
			name: "stream info slot only",
			in:   layoutInputs{height: 20, streamInfo: 1, composer: 4, status: 1},
			want: broadcastLayout{
				height:     20,
				streamInfo: layoutSpan{top: 0, height: 1},
				poll:       layoutSpan{top: 1, height: 0},
				chat:       layoutSpan{top: 1, height: 14},
				composer:   layoutSpan{top: 15, height: 4},
				status:     layoutSpan{top: 19, height: 1},
			},
		},
		{
			name: "stream info and poll",
			in:   layoutInputs{height: 20, streamInfo: 2, poll: 5, composer: 4, status: 1},
			want: broadcastLayout{
				height:     20,
				streamInfo: layoutSpan{top: 0, height: 2},
				poll:       layoutSpan{top: 2, height: 5},
				chat:       layoutSpan{top: 7, height: 8},
				composer:   layoutSpan{top: 15, height: 4},
				status:     layoutSpan{top: 19, height: 1},
			},
		},
		{
			name: "anonymous account has no composer",
			in:   layoutInputs{height: 20, streamInfo: 1, status: 1},
			want: broadcastLayout{
				height:     20,
				streamInfo: layoutSpan{top: 0, height: 1},
				poll:       layoutSpan{top: 1, height: 0},
				chat:       layoutSpan{top: 1, height: 18},
				composer:   layoutSpan{top: 19, height: 0},
				status:     layoutSpan{top: 19, height: 1},
			},
		},
		{
			name: "status bar rendered outside the tab",
			in:   layoutInputs{height: 20, streamInfo: 1, status: 0, composer: 4},
			want: broadcastLayout{
				height:     20,
				streamInfo: layoutSpan{top: 0, height: 1},
				poll:       layoutSpan{top: 1, height: 0},
				chat:       layoutSpan{top: 1, height: 15},
				composer:   layoutSpan{top: 16, height: 4},
				status:     layoutSpan{top: 20, height: 0},
			},
		},
		{
			name: "user inspect splits the space",
			in:   layoutInputs{height: 20, streamInfo: 1, composer: 4, status: 1, inspecting: true},
			want: broadcastLayout{
				height:      20,
				streamInfo:  layoutSpan{top: 0, height: 1},
				poll:        layoutSpan{top: 1, height: 0},
				chat:        layoutSpan{top: 1, height: 9},
				userInspect: layoutSpan{top: 10, height: 5},
				composer:    layoutSpan{top: 15, height: 4},
				status:      layoutSpan{top: 19, height: 1},
			},
		},
		{
			name: "emote overview replaces the content area",
			in:   layoutInputs{height: 20, streamInfo: 1, composer: 4, status: 1, emoteOverview: true},
			want: broadcastLayout{
				height:        20,
				emoteOverview: layoutSpan{top: 0, height: 19},
				status:        layoutSpan{top: 19, height: 1},
			},
		},
		{
			name: "short terminal clamps to zero instead of going negative",
			in:   layoutInputs{height: 3, streamInfo: 1, poll: 4, composer: 4, status: 1},
			want: broadcastLayout{
				height:     3,
				streamInfo: layoutSpan{top: 0, height: 1},
				poll:       layoutSpan{top: 1, height: 4},
				chat:       layoutSpan{top: 5, height: 0},
				composer:   layoutSpan{top: 5, height: 4},
				status:     layoutSpan{top: 9, height: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := allocateBroadcastLayout(tt.in)
			require.Equal(t, tt.want, got)

			for _, span := range []layoutSpan{got.streamInfo, got.poll, got.chat, got.userInspect, got.composer, got.status, got.emoteOverview} {
				require.GreaterOrEqual(t, span.top, 0)
				require.GreaterOrEqual(t, span.height, 0)
			}
		})
	}
}

// Test_allocateBroadcastLayout_visibleRegionsTileTheScreen checks that rendering
// the regions in order yields exactly the allocated rows, so hit-testing and
// rendering agree by construction.
func Test_allocateBroadcastLayout_visibleRegionsTileTheScreen(t *testing.T) {
	t.Parallel()

	inputs := []layoutInputs{
		{height: 24, streamInfo: 1, composer: 4, status: 1},
		{height: 24, streamInfo: 2, poll: 4, composer: 4, status: 1},
		{height: 24, streamInfo: 2, poll: 4, composer: 4, status: 1, inspecting: true},
		{height: 30, streamInfo: 1, composer: 6, status: 1, inspecting: true},
		{height: 18, streamInfo: 1, composer: 4, status: 0},
	}

	for i, in := range inputs {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()

			l := allocateBroadcastLayout(in)

			regions := []layoutSpan{l.streamInfo, l.poll, l.chat, l.userInspect, l.composer, l.status}
			if in.emoteOverview {
				regions = []layoutSpan{l.emoteOverview, l.status}
			}

			next := 0
			for _, span := range regions {
				if !span.visible() {
					continue
				}

				require.Equal(t, next, span.top, "regions must tile the screen without gaps or overlaps")
				next += span.height
			}

			require.Equal(t, in.height, next, "the allocation must use the whole tab height")
		})
	}
}

// newLoadedLayoutTestTab returns a broadcast tab with all children created, as if
// the channel data had been loaded.
func newLoadedLayoutTestTab(t *testing.T, width, height int, account save.Account) *Broadcast {
	t.Helper()

	tab := newTestTab("tab-1", width, height, account, "chan")
	tab.handleChannelDataLoaded(setChannelDataMessage{
		channelID:    "chan-id",
		channel:      "chan",
		channelLogin: "chan",
	})
	t.Cleanup(func() {
		tab.Close()
	})

	return tab
}

// addTestMessages fills the main chat window with one-row entries.
func addTestMessages(t *testing.T, c *chatWindow, count int) {
	t.Helper()

	c.lines = nil
	c.entries = nil

	for i := 0; i < count; i++ {
		c.lines = append(c.lines, fmt.Sprintf("line %d", i))
		c.entries = append(c.entries, &chatEntry{
			Position: position{CursorStart: i, CursorEnd: i},
		})
	}

	c.cursor = count - 1
	c.updatePort()
}

func selectedCursor(t *testing.T, c *chatWindow) int {
	t.Helper()

	_, entry := c.entryForCurrentCursor()
	require.NotNil(t, entry, "expected a selected entry")

	return entry.Position.CursorStart
}

// Test_Broadcast_ViewMatchesLayoutHeight covers the geometry contract: the view
// occupies exactly the rows the layout allocated, for both tab orientations and
// every mode that changes the arrangement.
func Test_Broadcast_ViewMatchesLayoutHeight(t *testing.T) {
	t.Parallel()

	account := save.Account{ID: "acc", DisplayName: "me"}

	t.Run("horizontal tab", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, account)

		require.Equal(t, tab.height, lipgloss.Height(tab.View()))
		require.Equal(t, tab.height-1, lipgloss.Height(tab.ViewWithoutStatusBar()))
	})

	t.Run("vertical tab renders status outside", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 70, 24, account)
		tab.SetFullWidth(80)
		tab.HandleResize()

		require.Equal(t, 70, tab.width)
		require.Equal(t, tab.height, lipgloss.Height(tab.ViewWithoutStatusBar()))
		require.Equal(t, 80, tab.statusInfo.width, "the status bar spans the terminal width")
		require.False(t, tab.layout.status.visible(), "the status bar is not part of the tab layout in vertical mode")
	})

	t.Run("poll and stream info", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, account)
		tab.poll.enabled = true
		tab.poll.title = "best map?"
		tab.poll.items = []pollItem{{id: "1", title: "dust2", votes: 1}}
		tab.streamInfo.loaded = true
		tab.streamInfo.title = "ranked grind"
		tab.streamInfo.game = "Counter-Strike"
		tab.streamInfo.viewer = 12
		tab.HandleResize()

		require.True(t, tab.layout.poll.visible())
		require.GreaterOrEqual(t, tab.layout.streamInfo.height, 1)
		require.Equal(t, tab.height, lipgloss.Height(tab.View()))
	})

	t.Run("wrapped composer", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 40, 24, account)
		before := tab.layout.composer.height

		tab.input.SetValue(strings.Repeat("word ", 60))
		tab.HandleResize()

		require.Greater(t, tab.layout.composer.height, before)
		require.Equal(t, tab.height, lipgloss.Height(tab.View()))
	})

	t.Run("armed reply", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, account)
		before := tab.layout.composer.height

		tab.input.ArmReply("msg-1", "otheruser", "nice play")
		tab.HandleResize()

		require.Equal(t, before+1, tab.layout.composer.height)
		require.Equal(t, tab.height, lipgloss.Height(tab.View()))
	})

	t.Run("user inspect", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, account)
		tab.userInspect = newUserInspect(tab.id, tab.width, tab.height, "someone", tab.channelLogin, tab.account.ID,
			tab.settings, tab.theme, tab.keymap, tab.client.API, tab.messageLogger, tab.emoteReplacer, tab.badgeReplacer)
		tab.state = userInspectMode
		tab.HandleResize()

		require.True(t, tab.layout.userInspect.visible())
		require.Equal(t, tab.height, lipgloss.Height(tab.View()))
		require.Equal(t, tab.layout.userInspect.height, lipgloss.Height(tab.userInspect.View()),
			"the inspect window must render exactly the rows the layout allocated")
	})

	t.Run("emote overview", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, account)
		tab.state = emoteOverviewMode
		tab.emoteOverview = NewEmoteOverview(tab.channelID, tab.emoteCache, tab.emoteReplacer, 1, tab.width, tab.height)
		tab.HandleResize()

		require.True(t, tab.layout.emoteOverview.visible())
		require.False(t, tab.layout.chat.visible())
		require.Equal(t, tab.height, lipgloss.Height(tab.View()))
	})

	t.Run("anonymous account", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "anon", IsAnonymous: true})

		require.False(t, tab.layout.composer.visible())
		require.Equal(t, tab.height, lipgloss.Height(tab.View()))
	})

	t.Run("short and narrow terminal", func(t *testing.T) {
		t.Parallel()

		for _, size := range [][2]int{{1, 1}, {10, 3}, {4, 40}, {20, 5}} {
			tab := newLoadedLayoutTestTab(t, size[0], size[1], account)
			tab.userInspect = newUserInspect(tab.id, tab.width, tab.height, "someone", tab.channelLogin, tab.account.ID,
				tab.settings, tab.theme, tab.keymap, tab.client.API, tab.messageLogger, tab.emoteReplacer, tab.badgeReplacer)
			tab.state = userInspectMode
			tab.HandleResize()

			require.NotPanics(t, func() {
				_ = tab.View()
				_ = tab.HandleMouseClick(tea.MouseClickMsg{Button: tea.MouseLeft}, 0, size[1]-1)
			}, "size %v", size)
		}
	})
}

// Test_Broadcast_LoadingAndErrorViews pins the non-loaded views, which do not use
// the layout.
func Test_Broadcast_LoadingAndErrorViews(t *testing.T) {
	t.Parallel()

	tab := newTestTab("tab-1", 40, 10, save.Account{ID: "acc", DisplayName: "me"}, "chan")

	loading := tab.View()
	require.Contains(t, loading, "Loading channel")
	require.Equal(t, tab.height, lipgloss.Height(loading))

	tab.err = fmt.Errorf("boom")
	require.Contains(t, tab.View(), "boom")
	require.Equal(t, tab.View(), tab.ViewWithoutStatusBar())
}

func TestBroadcast_EmptyInspectHistoryPreservesAllocatedRows(t *testing.T) {
	t.Parallel()

	for _, vertical := range []bool{false, true} {
		for _, height := range []int{21, 22} {
			t.Run(fmt.Sprintf("vertical=%t/height=%d", vertical, height), func(t *testing.T) {
				t.Parallel()

				tab := newLoadedLayoutTestTab(t, 80, height, save.Account{ID: "acc", DisplayName: "me"})
				if vertical {
					tab.SetFullWidth(90)
				}
				tab.userInspect = newUserInspect(tab.id, tab.width, tab.height, "someone", tab.channelLogin, tab.account.ID,
					tab.settings, tab.theme, tab.keymap, tab.client.API, tab.messageLogger, tab.emoteReplacer, tab.badgeReplacer)
				tab.state = userInspectMode
				tab.HandleResize()

				require.Equal(t, tab.layout.userInspect.height, lipgloss.Height(tab.userInspect.View()))
				view := tab.View()
				if vertical {
					view = tab.ViewWithoutStatusBar()
				}
				require.Equal(t, height, lipgloss.Height(view))

				// The displayed bottom row is not part of the composer, in either orientation.
				y := height - 1
				if vertical {
					y = tab.layout.composer.top - 1
				}
				_ = tab.HandleMouseClick(leftClick(y), 0, y)
				require.Equal(t, userInspectMode, tab.state)
				_ = tab.HandleMouseClick(leftClick(tab.layout.composer.top), 0, tab.layout.composer.top)
				require.Equal(t, userInspectInsertMode, tab.state)
			})
		}
	}
}

func TestUserInspect_InfoFitsShortAllocation(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 14, save.Account{ID: "acc", DisplayName: "me"})
	tab.userInspect = newUserInspect(tab.id, tab.width, tab.height, "someone", tab.channelLogin, tab.account.ID,
		tab.settings, tab.theme, tab.keymap, tab.client.API, tab.messageLogger, tab.emoteReplacer, tab.badgeReplacer)
	tab.state = userInspectMode
	tab.HandleResize()

	require.Equal(t, tab.layout.userInspect.height, lipgloss.Height(tab.userInspect.View()))
	require.Equal(t, tab.height, lipgloss.Height(tab.View()))
}

func TestBroadcast_OverviewShrinksToZeroRows(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
	tab.state = emoteOverviewMode
	tab.emoteOverview = NewEmoteOverview(tab.channelID, tab.emoteCache, tab.emoteReplacer, 1, tab.width, tab.height)
	tab.HandleResize()

	tab.SetSize(40, 1)
	tab.HandleResize()

	require.Zero(t, tab.layout.emoteOverview.height)
	require.Zero(t, tab.emoteOverview.vp.Height())
	require.Equal(t, 40, tab.emoteOverview.vp.Width())
	require.Equal(t, tab.StatusBarView(), tab.View())
	require.Empty(t, tab.ViewWithoutStatusBar())

	tab.SetSize(80, 24)
	tab.HandleResize()
	require.Equal(t, tab.layout.emoteOverview.height, tab.emoteOverview.vp.Height())
	require.Equal(t, tab.height, lipgloss.Height(tab.View()))
}
