package tab

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
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

func TestUnbanRequestQueue_LoadsOnlyPendingRequests(t *testing.T) {
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
	require.Len(t, q.requests, 1)
	require.Equal(t, "pending-1", q.requests[0].ID)

	view := q.View()
	require.Contains(t, view, "Unban requests (1 pending)")
	require.Contains(t, view, "Alice")
	require.Contains(t, view, "pending")
	require.Contains(t, view, "I was banned by mistake")
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

	require.Empty(t, q.requests)
	require.False(t, q.Resolving())
	require.Equal(t, "pending-1", api.resolvedID)
	require.Equal(t, "approved", api.resolvedStatus)
	require.Equal(t, "apology accepted", api.resolutionText)
	require.Contains(t, q.View(), "No pending unban requests")
	require.Contains(t, q.notice, "Alice was approved")
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
	cmd = nonMod.dispatchSlashCommand("unban_requests", nil)
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
	require.Len(t, tab.unbanRequests.requests, 1)
	require.Equal(t, "pending-1", tab.unbanRequests.requests[0].ID)
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

	cmd := handleCommand("tab-id", "unban_requests", nil, "channel-id", "channel", "account-id", newFakeModerationClient())
	require.NotNil(t, cmd)
	require.NotPanics(t, func() { _ = cmd() })
}
