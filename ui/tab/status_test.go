package tab

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/julez-dev/chatuino/chatstate"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/stretchr/testify/require"
)

func Test_humanizeDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{name: "zero", duration: 0, want: "0 seconds"},
		{name: "sub-second", duration: 500 * time.Millisecond, want: "0 seconds"},
		{name: "1 second", duration: time.Second, want: "1 second"},
		{name: "30 seconds", duration: 30 * time.Second, want: "30 seconds"},
		{name: "1 minute", duration: time.Minute, want: "1 minute"},
		{name: "1 minute 30 seconds", duration: 90 * time.Second, want: "1 minute 30 seconds"},
		{name: "5 minutes", duration: 5 * time.Minute, want: "5 minutes"},
		{name: "1 hour", duration: time.Hour, want: "1 hour"},
		{name: "1 hour 30 minutes", duration: 90 * time.Minute, want: "1 hour 30 minutes"},
		{name: "2 hours", duration: 2 * time.Hour, want: "2 hours"},
		{name: "1 day", duration: 24 * time.Hour, want: "1 day"},
		{name: "1 day 1 hour", duration: 25 * time.Hour, want: "1 day 1 hour"},
		{name: "1 day 2 hours", duration: 26 * time.Hour, want: "1 day 2 hours"},
		{name: "2 days", duration: 48 * time.Hour, want: "2 days"},
		{name: "7 days", duration: 7 * 24 * time.Hour, want: "7 days"},
		{name: "7 days 12 hours", duration: 7*24*time.Hour + 12*time.Hour, want: "7 days 12 hours"},
		// Edge cases: minutes/seconds hidden at larger scales
		{name: "1 hour hides seconds", duration: time.Hour + 30*time.Second, want: "1 hour"},
		{name: "1 day hides minutes", duration: 24*time.Hour + 30*time.Minute, want: "1 day"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := humanizeDuration(tt.duration); got != tt.want {
				t.Errorf("humanizeDuration() = %q, want %q", got, tt.want)
			}
		})
	}
}

// roomTag builds the pointer IRC tags use to mark "sent" fields.
func roomTag[T any](v T) *T { return &v }

// settingsAPI is an APIClient that records the chat-settings requests it served.
type settingsAPI struct {
	mu       sync.Mutex
	requests []string
	settings twitchapi.ChatSettingData
	err      error
}

func (a *settingsAPI) GetUsers(context.Context, []string, []string) (twitchapi.UserResponse, error) {
	return twitchapi.UserResponse{}, nil
}

func (a *settingsAPI) GetStreamInfo(context.Context, []string) (twitchapi.GetStreamsResponse, error) {
	return twitchapi.GetStreamsResponse{}, nil
}

func (a *settingsAPI) GetChatSettings(_ context.Context, broadcasterID string, _ string) (twitchapi.GetChatSettingsResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.requests = append(a.requests, broadcasterID)

	if a.err != nil {
		return twitchapi.GetChatSettingsResponse{}, a.err
	}

	return twitchapi.GetChatSettingsResponse{Data: []twitchapi.ChatSettingData{a.settings}}, nil
}

func (a *settingsAPI) requestCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()

	return len(a.requests)
}

// newStatusTestTab builds a loaded broadcast tab whose status bar uses the given
// API and settings cache.
func newStatusTestTab(api APIClient, cache *chatstate.Cache) *Broadcast {
	account := save.Account{ID: "acc", DisplayName: "me"}
	tab := NewBroadcast("tab-1", 80, 24, account, "chan",
		save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap(),
		ClientCapabilities{API: api}, nil, nil, nil, nil, nil, nil, nil, cache)

	tab.handleChannelDataLoaded(setChannelDataMessage{
		channelID:    "chan-id",
		channel:      "chan",
		channelLogin: "chan",
	})

	return tab
}

// TestStreamStatus_RendersCachedSettingsWithoutRequest covers the restored-tab
// case: the startup prefetch filled the cache, so the status bar neither shows a
// spinner nor makes a request.
func TestStreamStatus_RendersCachedSettingsWithoutRequest(t *testing.T) {
	t.Parallel()

	cache := chatstate.NewCache()
	cache.Put("chan-id", twitchapi.ChatSettingData{BroadcasterID: "chan-id", SlowMode: true, SlowModeWaitTime: 10})

	api := &settingsAPI{}
	tab := newStatusTestTab(api, cache)

	require.Nil(t, tab.statusInfo.Init(), "a cached channel schedules no fetch")
	require.True(t, tab.statusInfo.isDataFetched)
	require.Equal(t, 0, api.requestCount())
	require.Contains(t, tab.statusInfo.View(), "Slow Mode")
}

// TestStreamStatus_RoomStateAvoidsRequest covers the live-join case: ROOMSTATE
// carries every setting the bar renders, so the scheduled fallback must not fire.
func TestStreamStatus_RoomStateAvoidsRequest(t *testing.T) {
	t.Parallel()

	cache := chatstate.NewCache()
	api := &settingsAPI{settings: twitchapi.ChatSettingData{BroadcasterID: "chan-id", SubscriberMode: true}}

	tab := newStatusTestTab(api, cache)
	status := tab.statusInfo

	require.NotNil(t, status.Init(), "an unknown channel starts its spinner and fallback")
	require.False(t, status.isDataFetched)

	var fallback settingsFetchFallbackMessage
	require.IsType(t, fallback, fallbackAfter(status))

	status.ApplyRoomState(&twitchirc.RoomState{RoomID: "chan-id", Slow: roomTag(30), EmoteOnly: roomTag(true)})

	require.True(t, status.isDataFetched)
	require.True(t, status.settings.SlowMode)
	require.Equal(t, 30, status.settings.SlowModeWaitTime)
	require.True(t, status.settings.EmoteMode)
	require.Contains(t, status.View(), "Slow Mode")

	cached, ok := cache.Get("chan-id")
	require.True(t, ok, "the room state is shared with the other tabs of the channel")
	require.Equal(t, status.settings, cached)

	// The fallback armed before the message arrived is stale.
	_, cmd := status.Update(fallback)
	require.Nil(t, cmd)
	require.Equal(t, 0, api.requestCount(), "ROOMSTATE removes the need for /chat/settings")

	// A fallback aimed at another tab is ignored as well.
	status.isDataFetched = false
	_, cmd = status.Update(settingsFetchFallbackMessage{target: "other-tab", seq: status.fallbackSeq})
	require.Nil(t, cmd)
	require.Equal(t, 0, api.requestCount())
}

// TestStreamStatus_FallbackRequestsOncePerChannel covers a channel whose room
// state never arrived: the tab fetches, and any other tab of the channel then
// renders from the cache instead of repeating the request.
func TestStreamStatus_FallbackRequestsOncePerChannel(t *testing.T) {
	t.Parallel()

	cache := chatstate.NewCache()
	api := &settingsAPI{settings: twitchapi.ChatSettingData{BroadcasterID: "chan-id", SubscriberMode: true}}

	tab := newStatusTestTab(api, cache)
	status := tab.statusInfo

	require.NotNil(t, status.Init())

	_, cmd := status.Update(fallbackAfter(status))
	require.NotNil(t, cmd, "the scheduled fallback requests the settings")

	msg := cmd()
	require.IsType(t, setSteamStatusDataMessage{}, msg)

	_, cmd = status.Update(msg)
	require.Nil(t, cmd)
	require.True(t, status.isDataFetched)
	require.True(t, status.settings.SubscriberMode)
	require.Equal(t, 1, api.requestCount())

	second := newStatusTestTab(api, cache)
	require.Nil(t, second.statusInfo.Init(), "a channel another tab already fetched needs no request")
	require.True(t, second.statusInfo.isDataFetched)
	require.Equal(t, 1, api.requestCount())
}

// TestStreamStatus_FallbackErrorIsShown keeps a failed request visible in the bar
// and retryable, because the cache does not keep the failure.
func TestStreamStatus_FallbackErrorIsShown(t *testing.T) {
	t.Parallel()

	cache := chatstate.NewCache()
	api := &settingsAPI{err: errors.New("twitch down")}

	tab := newStatusTestTab(api, cache)
	status := tab.statusInfo

	require.NotNil(t, status.Init())

	_, cmd := status.Update(fallbackAfter(status))
	require.NotNil(t, cmd)
	_, cmd = status.Update(cmd())
	require.Nil(t, cmd)

	require.True(t, status.isDataFetched)
	require.ErrorContains(t, status.err, "twitch down")

	_, ok := cache.Get("chan-id")
	require.False(t, ok, "a failed request is not cached")
}

// TestStreamStatus_RoomStateFromChatEvent covers the wiring in the tab: a
// ROOMSTATE arriving as a chat event updates the bar and returns no fetch command.
func TestStreamStatus_RoomStateFromChatEvent(t *testing.T) {
	t.Parallel()

	tab := newFSMTestTab(t, false)
	require.False(t, tab.statusInfo.isDataFetched)

	_, cmd := tab.Update(ChatEventMessage{
		AccountID: "acc",
		Channel:   "chan",
		ChannelID: "chan-id",
		Message:   &twitchirc.RoomState{RoomID: "chan-id", SubsOnly: roomTag(true)},
	})

	require.Nil(t, cmd, "applying ROOMSTATE issues no request")
	require.True(t, tab.statusInfo.isDataFetched)
	require.True(t, tab.statusInfo.settings.SubscriberMode)
	require.Contains(t, tab.statusInfo.View(), "Sub Only")
}

// fallbackAfter returns the fallback message the status bar is currently waiting
// for, so tests drive the timer without sleeping.
func fallbackAfter(status *streamStatus) settingsFetchFallbackMessage {
	return settingsFetchFallbackMessage{target: status.tab.id, seq: status.fallbackSeq}
}
