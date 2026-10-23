package mainui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/nav"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/stretchr/testify/require"
)

// newTestRoot wires a Root with the default test configuration and a size.
func newTestRoot(t *testing.T, accounts []save.Account, clients map[string]tab.ClientCapabilities, initialState *save.AppState) *Root {
	t.Helper()

	root := NewUI(
		t.Context(),
		nil,                         // message logger channel
		save.BuildDefaultSettings(), // settings
		save.BuildDefaultTheme(),    // theme
		save.BuildDefaultKeyMap(),   // keymap
		accounts,                    // accounts
		"",                          // version
		nil,                         // server API
		clients,                     // per-account clients
		nil,                         // account provider
		nil,                         // emote cache
		nil,                         // badge cache
		nil,                         // emote replacer
		nil,                         // badge replacer
		nil,                         // image display manager
		nil,                         // recent messages
		nil,                         // message logger
		nil,                         // connection pool
		nil,                         // app state manager
		nil,                         // channel history
		initialState,                // initial app state
	)
	t.Cleanup(root.Close)
	root.width, root.height = 80, 24

	return root
}

// newTestTab builds a chat tab wired with the default test configuration.
func newTestTab(id string, width, height int, account save.Account, channel string) *tab.Broadcast {
	return tab.NewBroadcast(id, width, height, account, channel,
		save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap(),
		tab.ClientCapabilities{}, nil, nil, nil, nil, nil, nil, nil)
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

	h := newHorizontalTabHeader(80, save.BuildDefaultTheme())
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

	h := newHorizontalTabHeader(80, save.BuildDefaultTheme())
	h.View(headerRows(0, "alpha"))

	rows := headerRows(1, "alpha", "beta")

	got, ok := h.TabAt(rows, 19, 1)
	require.True(t, ok)
	require.Equal(t, "beta", got)
}

func TestVerticalTabHeader_TabAt(t *testing.T) {
	t.Parallel()

	v := newVerticalTabHeader(30, 10, save.BuildDefaultTheme())
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

	v := newVerticalTabHeader(30, 6, save.BuildDefaultTheme())

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

// The shell reaches a tab's mouse handlers through a capability assertion, not a
// concrete type. If the methods drift (renamed, re-signatured) the assertion fails
// silently and clicks stop reaching tabs, so pin the capability here.
func Test_Root_activeTabSatisfiesMouseAware(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	root.hasLoadedSession = true

	tb := newTestTab("tab-1", 80, 24, save.Account{ID: "acc", DisplayName: "me"}, "chan")
	root.tabs.Add(tb)
	root.tabs.SetActive(tb.ID())

	aware, ok := root.activeTab().(mouseAware)
	require.True(t, ok, "a tab must expose the mouse capability the shell routes through")

	// An unloaded tab has no children yet; the handlers must tolerate that.
	require.NotPanics(t, func() {
		_ = aware.HandleMouseClick(tea.MouseClickMsg{Button: tea.MouseLeft}, 1, 1)
		_ = aware.HandleMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown}, 0, 1)
	})
}
