package mainui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/stretchr/testify/require"
)

func newRestoreTestRoot(t *testing.T) *Root {
	t.Helper()

	return newTestRoot(t, []save.Account{{ID: "acc", DisplayName: "me"}}, map[string]ClientCapabilities{}, nil)
}

// Restoring a session activates the outgoing tab before the incoming one. Restored
// tabs have no composer yet: their children are only created once the channel data
// has been fetched, which happens after the restore message is handled.
func Test_Root_handlePersistedDataLoaded_RestoresUnloadedTabs(t *testing.T) {
	root := newRestoreTestRoot(t)

	msg := persistedDataLoadedMessage{
		state: save.AppState{Tabs: []save.TabState{
			{Channel: "alpha", IdentityID: "acc", Kind: int(BroadcastTabKind)},
			{Channel: "beta", IdentityID: "acc", Kind: int(BroadcastTabKind), IsFocused: true},
		}},
		ttvUsers: map[string]twitchapi.UserData{},
	}

	require.NotPanics(t, func() {
		root.handlePersistedDataLoaded(msg)
	}, "blurring a restored tab must not touch the composer it does not have yet")

	t.Cleanup(func() {
		for _, tab := range root.tabs.All() {
			if bt, ok := tab.(*broadcastTab); ok {
				bt.close()
			}
		}
	})

	require.Equal(t, 2, root.tabs.Len())

	active, ok := root.tabs.Active()
	require.True(t, ok)
	require.Equal(t, "beta", active.Title(), "the persisted focused tab becomes active")

	// Only the active tab is focused; the other one was blurred during the restore.
	focused := 0

	for _, tab := range root.tabs.All() {
		if tab.Focused() {
			focused++
		}
	}

	require.Equal(t, 1, focused)
}

// Without a focused tab in the persisted state, the first restored tab is active.
func Test_Root_handlePersistedDataLoaded_FallsBackToFirstTab(t *testing.T) {
	root := newRestoreTestRoot(t)

	msg := persistedDataLoadedMessage{
		state: save.AppState{Tabs: []save.TabState{
			{Channel: "alpha", IdentityID: "acc", Kind: int(BroadcastTabKind)},
			{Channel: "beta", IdentityID: "acc", Kind: int(BroadcastTabKind)},
		}},
		ttvUsers: map[string]twitchapi.UserData{},
	}

	require.NotPanics(t, func() {
		root.handlePersistedDataLoaded(msg)
	})

	t.Cleanup(func() {
		for _, tab := range root.tabs.All() {
			if bt, ok := tab.(*broadcastTab); ok {
				bt.close()
			}
		}
	})

	active, ok := root.tabs.Active()
	require.True(t, ok)
	require.Equal(t, "alpha", active.Title())
}

// A tab whose channel data never loaded must tolerate Blur: it has no composer and
// no chat window to hand focus back from.
func Test_BroadcastTab_BlurWithoutComposer(t *testing.T) {
	tab := newTestTab("tab-1", 80, 24, save.Account{ID: "acc", DisplayName: "me"}, "chan")
	tab.Focus()

	require.NotPanics(t, tab.Blur)
	require.False(t, tab.Focused())
}

// isTyping runs on every key press that might be a shortcut, including while a
// restored tab is still loading its channel data.
func Test_Root_isTyping_ToleratesUnloadedTabs(t *testing.T) {
	root := newRestoreTestRoot(t)

	root.handlePersistedDataLoaded(persistedDataLoadedMessage{
		state: save.AppState{Tabs: []save.TabState{
			{Channel: "alpha", IdentityID: "acc", Kind: int(BroadcastTabKind), IsFocused: true},
		}},
		ttvUsers: map[string]twitchapi.UserData{},
	})

	t.Cleanup(func() {
		for _, tab := range root.tabs.All() {
			if bt, ok := tab.(*broadcastTab); ok {
				bt.close()
			}
		}
	})

	require.False(t, root.activeTab().IsDataLoaded(), "precondition: the tab has no children yet")
	require.NotPanics(t, func() {
		root.isTyping()
	})
	require.False(t, root.isTyping())
}

// A tab becomes active before its channel data is fetched, so every method Root
// may call through the tab interface has to tolerate missing children.
func Test_BroadcastTab_UnloadedTabToleratesInterfaceCalls(t *testing.T) {
	tab := newTestTab("tab-1", 80, 24, save.Account{ID: "acc", DisplayName: "me"}, "chan")

	require.NotPanics(t, func() {
		_ = tab.View()
		_ = tab.ViewWithoutStatusBar()
		_ = tab.StatusBarView()
		_ = tab.IsSearching()
		_ = tab.IsDataLoaded()
		_ = tab.Focused()
		_ = tab.State()
		_ = tab.Title()
		_ = tab.Identity()
		tab.SetSize(80, 24)
		tab.SetFullWidth(80)
		tab.HandleResize()
		tab.Focus()
		tab.Blur()
		_, _ = tab.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
		_, _ = tab.Update(tea.MouseClickMsg{X: 2, Y: 2, Button: tea.MouseLeft})
		_, _ = tab.Update(tea.MouseWheelMsg{X: 2, Y: 2, Button: tea.MouseWheelDown})
		tab.close()
	})
}
