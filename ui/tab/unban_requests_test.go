package tab

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/nav"
	"github.com/stretchr/testify/require"
)

func newTestUnbanQueue(t *testing.T, api ModerationAPI) *unbanRequestQueue {
	t.Helper()

	return newUnbanRequestQueue("queue-1", "channel-id", "account-id", api, 80, 20,
		save.BuildDefaultTheme(), save.BuildDefaultKeyMap())
}

func updateUnbanQueue(t *testing.T, q *unbanRequestQueue, msg tea.Msg) (*unbanRequestQueue, tea.Cmd) {
	t.Helper()

	updated, cmd := q.Update(msg)
	return updated, cmd
}

func TestUnbanRequestQueue_LoadsAllStatusesAndDefaultsToPending(t *testing.T) {
	t.Parallel()

	now := time.Now()
	api := newFakeModerationClient()
	api.unbanRequests = []twitchapi.UnbanRequest{
		{
			ID: "pending-1", UserID: "alice-id", UserLogin: "alice", UserName: "Alice",
			Text: "I was banned by mistake", Status: "pending", CreatedAt: now.Add(-2 * time.Minute),
		},
		{
			ID: "approved-1", UserLogin: "bob", UserName: "Bob",
			Text: "already resolved", Status: "approved", CreatedAt: now.Add(-time.Hour),
		},
	}

	q := newTestUnbanQueue(t, api)
	cmd := q.Init()
	require.NotNil(t, cmd)

	q, _ = updateUnbanQueue(t, q, cmd())

	require.False(t, q.loading)
	require.NoError(t, q.err)
	require.Len(t, q.requests, 2, "all statuses stay in the model")
	require.Len(t, q.visible, 1, "the default filter is pending")
	require.Equal(t, "pending-1", q.visible[0].ID)

	view := q.View()
	require.Contains(t, view, "Unban Requests")
	require.Contains(t, view, "Pending 1")
	require.Contains(t, view, "Approved 1")
	require.Contains(t, view, "Alice")
	require.Contains(t, view, "I was banned by mistake")

	q.setFilter("approved")
	require.Len(t, q.visible, 1)
	require.Equal(t, "approved-1", q.visible[0].ID)
	require.Contains(t, q.View(), "already resolved")
}

func TestUnbanRequestQueue_FilterCyclingAndResolvedRequestActions(t *testing.T) {
	t.Parallel()

	now := time.Now()
	api := newFakeModerationClient()
	api.unbanRequests = []twitchapi.UnbanRequest{
		{ID: "pending-1", UserLogin: "alice", UserName: "Alice", Text: "let me in", Status: "pending", CreatedAt: now},
		{ID: "approved-1", UserLogin: "bob", UserName: "Bob", Text: "thanks", Status: "approved", CreatedAt: now.Add(-time.Minute),
			ResolutionText: "apology accepted", ModeratorName: "Mod", ResolvedAt: now},
	}

	q := newTestUnbanQueue(t, api)
	q, _ = updateUnbanQueue(t, q, q.Init()())

	q, _ = updateUnbanQueue(t, q, tea.KeyPressMsg{Code: 'l', Text: "l"})
	require.Equal(t, "approved", q.filter)
	require.Equal(t, "approved-1", q.visible[0].ID)
	require.Contains(t, q.View(), "apology accepted")
	require.Contains(t, q.View(), "Resolved by Mod")

	q, _ = updateUnbanQueue(t, q, tea.KeyPressMsg{Code: 'a', Text: "a"})
	require.False(t, q.Resolving(), "resolved requests are not actionable")
	require.Contains(t, q.notice, "Only pending requests")

	q, _ = updateUnbanQueue(t, q, tea.KeyPressMsg{Code: 'h', Text: "h"})
	require.Equal(t, "pending", q.filter)
	require.Equal(t, "pending-1", q.visible[0].ID)
}

func TestUnbanRequestQueue_LoadErrorKeepsCurrentQueue(t *testing.T) {
	t.Parallel()

	api := newFakeModerationClient()
	api.unbanRequests = []twitchapi.UnbanRequest{{
		ID: "pending-1", UserLogin: "alice", UserName: "Alice", Text: "let me back in",
		Status: "pending", CreatedAt: time.Now(),
	}}

	q := newTestUnbanQueue(t, api)
	q, _ = updateUnbanQueue(t, q, q.Init()())

	api.unbanFetchErr = errors.New("twitch unavailable")
	q, _ = updateUnbanQueue(t, q, q.refreshCmd()())

	require.Len(t, q.requests, 1, "a failed refresh must not erase the queue")
	require.ErrorContains(t, q.err, "twitch unavailable")
	require.Contains(t, q.View(), "Alice")
}

func TestUnbanRequestQueue_ApproveRemovesRequestAndPassesResolutionText(t *testing.T) {
	t.Parallel()

	api := newFakeModerationClient()
	api.unbanRequests = []twitchapi.UnbanRequest{{
		ID: "pending-1", UserID: "alice-id", UserLogin: "alice", UserName: "Alice",
		Text: "sorry for the spam", Status: "pending", CreatedAt: time.Now(),
	}}

	q := newTestUnbanQueue(t, api)
	q, _ = updateUnbanQueue(t, q, q.Init()())

	q, _ = updateUnbanQueue(t, q, tea.KeyPressMsg{Code: 'a', Text: "a"})
	require.True(t, q.Resolving())
	q.input.SetValue("apology accepted")

	q, cmd := updateUnbanQueue(t, q, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	require.True(t, q.submitting)

	q, _ = updateUnbanQueue(t, q, cmd())

	require.Empty(t, q.visible, "the pending filter no longer shows the resolved request")
	require.Len(t, q.requests, 1)
	require.Equal(t, "approved", q.requests[0].Status)
	require.False(t, q.Resolving())
	require.Equal(t, "pending-1", api.resolvedID)
	require.Equal(t, "approved", api.resolvedStatus)
	require.Equal(t, "apology accepted", api.resolutionText)
	require.Contains(t, q.View(), "Pending 0")
	require.Contains(t, q.notice, "Alice was approved")

	q.setFilter("approved")
	require.Len(t, q.visible, 1)
	require.Contains(t, q.View(), "apology accepted")
}

func TestUnbanRequestQueue_DenyErrorKeepsRequestAndResolutionInput(t *testing.T) {
	t.Parallel()

	api := newFakeModerationClient()
	api.unbanRequests = []twitchapi.UnbanRequest{{
		ID: "pending-1", UserLogin: "alice", UserName: "Alice", Text: "let me back in",
		Status: "pending", CreatedAt: time.Now(),
	}}
	api.resolveErr = errors.New("forbidden")

	q := newTestUnbanQueue(t, api)
	q, _ = updateUnbanQueue(t, q, q.Init()())
	q, _ = updateUnbanQueue(t, q, tea.KeyPressMsg{Code: 'd', Text: "d"})
	q.input.SetValue("still no")

	q, cmd := updateUnbanQueue(t, q, tea.KeyPressMsg{Code: tea.KeyEnter})
	q, _ = updateUnbanQueue(t, q, cmd())

	require.Len(t, q.requests, 1, "a failed resolve must keep the request")
	require.True(t, q.Resolving(), "the moderator can retry without selecting the request again")
	require.Equal(t, "still no", q.input.Value())
	require.ErrorContains(t, q.err, "forbidden")
	require.Equal(t, "denied", api.resolvedStatus)
}

func TestUnbanRequestQueue_EscapeCancelsResolutionBeforeClosing(t *testing.T) {
	t.Parallel()

	api := newFakeModerationClient()
	api.unbanRequests = []twitchapi.UnbanRequest{{
		ID: "pending-1", UserLogin: "alice", UserName: "Alice", Status: "pending", CreatedAt: time.Now(),
	}}

	q := newTestUnbanQueue(t, api)
	q, _ = updateUnbanQueue(t, q, q.Init()())
	q, _ = updateUnbanQueue(t, q, tea.KeyPressMsg{Code: 'a', Text: "a"})
	require.True(t, q.Resolving())

	q, _ = updateUnbanQueue(t, q, tea.KeyPressMsg{Code: tea.KeyEscape})
	require.False(t, q.Resolving(), "first escape only cancels the resolution input")
	require.Len(t, q.requests, 1)
}

func TestBroadcast_UnbanRequestQueuePermissionAndLifecycle(t *testing.T) {
	t.Parallel()

	account := save.Account{ID: "acc", DisplayName: "me"}
	tab := newLoadedLayoutTestTab(t, 80, 24, account)

	handled, cmd := tab.handleKeyPress(tea.KeyPressMsg{Code: 'u', Mod: tea.ModAlt})
	require.True(t, handled)
	require.NotNil(t, cmd)
	msg := cmd()
	request, ok := msg.(RequestLocalMessageHandleMessage)
	require.True(t, ok)
	require.Contains(t, request.Message.(*twitchirc.Notice).Message, "only available to moderators")
	require.Equal(t, inChatWindow, tab.state)

	tab.isUserMod = true
	tab.client.Moderation = newFakeModerationClient()

	handled, cmd = tab.handleKeyPress(tea.KeyPressMsg{Code: 'u', Mod: tea.ModAlt})
	require.True(t, handled)
	require.Equal(t, unbanRequestMode, tab.state)
	require.NotNil(t, cmd)

	_, _ = tab.Update(cmd())

	updated, _ := tab.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	tab = updated.(*Broadcast)
	require.Equal(t, inChatWindow, tab.state)
}

func TestBroadcast_UnbanRequestQueueRejectsAnonymousAndCommandForNonModerators(t *testing.T) {
	t.Parallel()

	anonymous := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "anon", IsAnonymous: true})
	_, cmd := anonymous.handleKeyPress(tea.KeyPressMsg{Code: 'u', Mod: tea.ModAlt})
	require.NotNil(t, cmd)
	msg := cmd().(RequestLocalMessageHandleMessage)
	require.Contains(t, msg.Message.(*twitchirc.Notice).Message, "only available to moderators")

	nonMod := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
	cmd = nonMod.dispatchSlashCommand("unbanrequests", nil)
	require.NotNil(t, cmd)
	notice := noticeText(t, cmd())
	require.Contains(t, notice, "not available since you are not a moderator")
	require.Equal(t, inChatWindow, nonMod.state)
}

func TestBroadcast_UnbanRequestQueueTabNavigationDependsOnResolutionInput(t *testing.T) {
	t.Parallel()

	api := newFakeModerationClient()
	api.unbanRequests = []twitchapi.UnbanRequest{{
		ID: "pending-1", UserLogin: "alice", UserName: "Alice", Status: "pending", CreatedAt: time.Now(),
	}}

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "chan-id", DisplayName: "me"})
	tab.isUserMod = true
	tab.client.Moderation = api
	tab.state = unbanRequestMode
	tab.unbanRequests = newUnbanRequestQueue(tab.id, tab.channelID, tab.account.ID, api, tab.width, tab.height, tab.theme, tab.keymap)
	tab.unbanRequests.setRequests(api.unbanRequests)

	require.False(t, tab.IsComposing(), "browsing the queue must leave Tab available for tab navigation")

	tab.unbanRequests.startResolution("approved")
	require.True(t, tab.IsComposing(), "the resolution input captures text and Tab")
}

func TestBroadcast_UnbanRequestQueueEscapeCancelsResolutionBeforeClosing(t *testing.T) {
	t.Parallel()

	api := newFakeModerationClient()
	api.unbanRequests = []twitchapi.UnbanRequest{{
		ID: "pending-1", UserLogin: "alice", UserName: "Alice", Status: "pending", CreatedAt: time.Now(),
	}}

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "chan-id", DisplayName: "me"})
	tab.isUserMod = true
	tab.client.Moderation = api
	tab.state = unbanRequestMode
	tab.unbanRequests = newUnbanRequestQueue(tab.id, tab.channelID, tab.account.ID, api, tab.width, tab.height, tab.theme, tab.keymap)
	tab.unbanRequests.setRequests(api.unbanRequests)
	tab.unbanRequests.startResolution("approved")

	updated, _ := tab.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	tab = updated.(*Broadcast)
	require.Equal(t, unbanRequestMode, tab.state)
	require.False(t, tab.unbanRequests.Resolving())

	updated, _ = tab.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	tab = updated.(*Broadcast)
	require.Equal(t, inChatWindow, tab.state)
}

func TestBroadcast_UnbanRequestQueueAppliesLiveEvents(t *testing.T) {
	t.Parallel()

	api := newFakeModerationClient()
	api.unbanRequests = []twitchapi.UnbanRequest{{
		ID: "pending-1", UserLogin: "alice", UserName: "Alice", Text: "let me back in",
		Status: "pending", CreatedAt: time.Now(),
	}}

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "chan-id", DisplayName: "me"})
	tab.isUserMod = true
	tab.client.Moderation = api
	tab.state = unbanRequestMode
	tab.unbanRequests = newUnbanRequestQueue(tab.id, tab.channelID, tab.account.ID, api, tab.width, tab.height, tab.theme, tab.keymap)
	tab.unbanRequests.setRequests(api.unbanRequests)
	tab.HandleResize()

	create := eventsub.Message[eventsub.NotificationPayload]{
		Payload: eventsub.NotificationPayload{
			Subscription: eventsub.Subscription{
				Type:      "channel.unban_request.create",
				Condition: map[string]string{"broadcaster_user_id": "chan-id"},
			},
			Event: eventsub.Event{
				ID: "pending-2", BroadcasterUserID: "chan-id",
				UserID: "bob-id", UserLogin: "bob", UserName: "Bob",
				Text: "second chance please", CreatedAt: time.Now(),
			},
		},
	}
	require.Nil(t, tab.handleEventSubMessage(create))
	require.Len(t, tab.unbanRequests.requests, 2)
	require.Len(t, tab.unbanRequests.visible, 2)
	require.Contains(t, tab.unbanRequests.View(), "second chance please")

	resolve := eventsub.Message[eventsub.NotificationPayload]{
		Payload: eventsub.NotificationPayload{
			Subscription: eventsub.Subscription{
				Type:      "channel.unban_request.resolve",
				Condition: map[string]string{"broadcaster_user_id": "chan-id"},
			},
			Event: eventsub.Event{ID: "pending-2", Status: "approved", ModeratorID: "acc"},
		},
	}
	require.Nil(t, tab.handleEventSubMessage(resolve))
	require.Len(t, tab.unbanRequests.requests, 2, "resolved requests stay available in the history filters")
	require.Len(t, tab.unbanRequests.visible, 1)
	require.Equal(t, "pending-1", tab.unbanRequests.visible[0].ID)

	tab.unbanRequests.setFilter("approved")
	require.Len(t, tab.unbanRequests.visible, 1)
	require.Equal(t, "pending-2", tab.unbanRequests.visible[0].ID)
}

func TestUnbanRequestQueue_IgnoresMessagesForAnotherQueue(t *testing.T) {
	t.Parallel()

	api := newFakeModerationClient()
	api.unbanRequests = []twitchapi.UnbanRequest{{
		ID: "pending-1", UserLogin: "alice", UserName: "Alice", Status: "pending", CreatedAt: time.Now(),
	}}
	q := newTestUnbanQueue(t, api)
	q, _ = updateUnbanQueue(t, q, q.Init()())

	q, _ = updateUnbanQueue(t, q, unbanRequestsLoadedMessage{
		owner: "other-queue", requests: nil, err: errors.New("wrong tab"),
	})
	require.Len(t, q.requests, 1)
	require.NoError(t, q.err)

	q, _ = updateUnbanQueue(t, q, unbanRequestResolvedMessage{
		owner: "other-queue", requestID: "pending-1", status: "approved",
	})
	require.Len(t, q.visible, 1)
}

func TestUnbanRequestQueue_SelectionStaysWithRequestAcrossLiveUpdates(t *testing.T) {
	t.Parallel()

	now := time.Now()
	api := newFakeModerationClient()
	api.unbanRequests = []twitchapi.UnbanRequest{{
		ID: "pending-1", UserLogin: "alice", UserName: "Alice", Status: "pending", CreatedAt: now.Add(-time.Minute),
	}}

	q := newTestUnbanQueue(t, api)
	q, _ = updateUnbanQueue(t, q, q.Init()())
	require.Equal(t, "pending-1", q.selectedID)

	q.applyEvent(eventsub.Message[eventsub.NotificationPayload]{
		Payload: eventsub.NotificationPayload{
			Subscription: eventsub.Subscription{Type: "channel.unban_request.create"},
			Event: eventsub.Event{
				ID: "pending-2", UserLogin: "bob", UserName: "Bob", Text: "newer",
				CreatedAt: now,
			},
		},
	})

	require.Equal(t, "pending-1", q.selectedID, "the cursor follows the request, not the sorted index")
	require.Equal(t, 1, q.selected)
	selected, ok := q.selectedRequest()
	require.True(t, ok)
	require.Equal(t, "pending-1", selected.ID)
}

func TestUnbanRequestQueue_DeduplicatesStatusFanout(t *testing.T) {
	t.Parallel()

	api := newFakeModerationClient()
	q := newTestUnbanQueue(t, api)
	q.setRequests([]twitchapi.UnbanRequest{
		{ID: "request-1", UserLogin: "alice", UserName: "Alice", Status: "pending", CreatedAt: time.Now()},
		{ID: "request-1", UserLogin: "alice", UserName: "Alice", Status: "approved", CreatedAt: time.Now(),
			ResolutionText: "resolved while paging"},
	})

	require.Len(t, q.requests, 1)
	require.Equal(t, "approved", q.requests[0].Status)
	require.Equal(t, "resolved while paging", q.requests[0].ResolutionText)
}

func TestUnbanRequestQueue_ModifiedKeysDoNotTriggerActions(t *testing.T) {
	t.Parallel()

	api := newFakeModerationClient()
	api.unbanRequests = []twitchapi.UnbanRequest{{
		ID: "pending-1", UserLogin: "alice", UserName: "Alice", Status: "pending", CreatedAt: time.Now(),
	}}

	q := newTestUnbanQueue(t, api)
	q, _ = updateUnbanQueue(t, q, q.Init()())

	q, _ = updateUnbanQueue(t, q, tea.KeyPressMsg{Code: 'a', Mod: tea.ModAlt, Text: "a"})
	require.False(t, q.Resolving())

	q, _ = updateUnbanQueue(t, q, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl, Text: "r"})
	require.False(t, q.loading)
}

func TestBroadcast_UnbanRequestQueueFitsNarrowTabSizes(t *testing.T) {
	t.Parallel()

	for _, size := range [][2]int{{80, 24}, {40, 12}, {20, 6}, {1, 1}} {
		api := newFakeModerationClient()
		api.unbanRequests = []twitchapi.UnbanRequest{{
			ID: "pending-1", UserLogin: "alice", UserName: "Alice", Status: "pending",
			Text: strings.Repeat("please unban me ", 10), CreatedAt: time.Now(),
		}}

		tab := newLoadedLayoutTestTab(t, size[0], size[1], save.Account{ID: "chan-id", DisplayName: "me"})
		tab.isUserMod = true
		tab.client.Moderation = api
		tab.state = unbanRequestMode
		tab.unbanRequests = newUnbanRequestQueue(tab.id, tab.channelID, tab.account.ID, api, tab.width, tab.height, tab.theme, tab.keymap)
		tab.unbanRequests.setRequests(api.unbanRequests)
		tab.HandleResize()

		require.NotPanics(t, func() { _ = tab.View() }, "size %v", size)
		require.Equal(t, size[1], lipgloss.Height(tab.View()), "size %v", size)
	}
}

func TestUnbanRequestQueue_ViewFitsAllocatedRows(t *testing.T) {
	t.Parallel()

	api := newFakeModerationClient()
	api.unbanRequests = []twitchapi.UnbanRequest{{
		ID: "pending-1", UserLogin: "alice", UserName: "Alice",
		Text: strings.Repeat("please unban me ", 20), Status: "pending", CreatedAt: time.Now(),
	}}

	for _, size := range [][2]int{{80, 24}, {20, 6}, {1, 1}} {
		q := newUnbanRequestQueue("queue-1", "channel-id", "account-id", api, size[0], size[1],
			save.BuildDefaultTheme(), save.BuildDefaultKeyMap())
		q, _ = updateUnbanQueue(t, q, q.Init()())

		require.Equal(t, size[1], len(strings.Split(q.View(), "\n")), "size %v", size)
	}
}

func TestHandleCommand_UnbanRequestsOpensQueue(t *testing.T) {
	t.Parallel()

	cmd := handleCommand("tab-id", "unbanrequests", nil, "channel-id", "channel", "account-id", newFakeModerationClient())
	require.NotNil(t, cmd)

	envelope, ok := cmd().(nav.Envelope)
	require.True(t, ok)
	require.Equal(t, "tab-id", envelope.To)
	require.IsType(t, openUnbanRequestsMessage{}, envelope.Msg)
}
