package mainui

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/julez-dev/chatuino/chatstate"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/stretchr/testify/require"
)

type fakeAppStateManager struct {
	state save.AppState
	err   error
	calls int
}

func (f *fakeAppStateManager) LoadAppState() (save.AppState, error) {
	f.calls++

	return f.state, f.err
}

func (f *fakeAppStateManager) SaveAppState(save.AppState) error { return nil }

type fakeChannelHistory struct {
	entries []save.ChannelHistoryEntry
	err     error
}

func (f *fakeChannelHistory) LoadHistory() ([]save.ChannelHistoryEntry, error) {
	return f.entries, f.err
}

func (f *fakeChannelHistory) RecordChannel(string) error { return nil }

type fakeStartupUsers struct {
	mu      sync.Mutex
	logins  [][]string
	byLogin map[string]twitchapi.UserData
	err     error

	// chat settings served by the server proxy
	settings     map[string]twitchapi.ChatSettingData
	settingsErrs error
	settingsIDs  []string
}

func (f *fakeStartupUsers) GetUsers(_ context.Context, logins []string, _ []string) (twitchapi.UserResponse, error) {
	f.mu.Lock()
	f.logins = append(f.logins, append([]string(nil), logins...))
	f.mu.Unlock()

	if f.err != nil {
		return twitchapi.UserResponse{}, f.err
	}

	resp := twitchapi.UserResponse{}
	for _, login := range logins {
		if user, ok := f.byLogin[login]; ok {
			resp.Data = append(resp.Data, user)
		}
	}

	return resp, nil
}

func (f *fakeStartupUsers) GetChatSettings(_ context.Context, broadcasterID string, _ string) (twitchapi.GetChatSettingsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.settingsIDs = append(f.settingsIDs, broadcasterID)

	if f.settingsErrs != nil {
		return twitchapi.GetChatSettingsResponse{}, f.settingsErrs
	}

	settings, ok := f.settings[broadcasterID]
	if !ok {
		settings = twitchapi.ChatSettingData{BroadcasterID: broadcasterID}
	}

	return twitchapi.GetChatSettingsResponse{Data: []twitchapi.ChatSettingData{settings}}, nil
}

func (f *fakeStartupUsers) calls() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([][]string(nil), f.logins...)
}

func (f *fakeStartupUsers) settingsCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.settingsIDs...)
}

// fakeStartupChatClient is an account client that only records the chat-settings
// requests, which is all the prefetch needs from it.
type fakeStartupChatClient struct {
	mu  sync.Mutex
	ids []string
}

func (f *fakeStartupChatClient) GetUsers(context.Context, []string, []string) (twitchapi.UserResponse, error) {
	return twitchapi.UserResponse{}, nil
}

func (f *fakeStartupChatClient) GetStreamInfo(context.Context, []string) (twitchapi.GetStreamsResponse, error) {
	return twitchapi.GetStreamsResponse{}, nil
}

func (f *fakeStartupChatClient) GetChatSettings(_ context.Context, broadcasterID string, _ string) (twitchapi.GetChatSettingsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.ids = append(f.ids, broadcasterID)

	return twitchapi.GetChatSettingsResponse{Data: []twitchapi.ChatSettingData{{BroadcasterID: broadcasterID, EmoteMode: true}}}, nil
}

func (f *fakeStartupChatClient) settingsCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.ids...)
}

type fakeStartupEmotes struct {
	mu         sync.Mutex
	globalErr  error
	globalCall int
	userEmotes map[string][]emote.Emote
}

func (f *fakeStartupEmotes) RefreshGlobal(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.globalCall++

	return f.globalErr
}

func (f *fakeStartupEmotes) AddUserEmotes(userID string, emotes []emote.Emote) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.userEmotes == nil {
		f.userEmotes = map[string][]emote.Emote{}
	}

	f.userEmotes[userID] = emotes
}

type fakeStartupBadges struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeStartupBadges) RefreshGlobal(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++

	return f.err
}

type fakeUserEmoteClient struct {
	set      []twitchapi.UserEmoteImage
	template string
	err      error
	userIDs  []string
}

func (f *fakeUserEmoteClient) FetchAllUserEmotes(_ context.Context, userID string, _ string) ([]twitchapi.UserEmoteImage, string, error) {
	f.userIDs = append(f.userIDs, userID)

	return f.set, f.template, f.err
}

type fakeFollows struct {
	channels map[string][]twitchapi.FollowedChannel
	errors   map[string]error
}

func (f *fakeFollows) FetchUserFollowedChannels(_ context.Context, userID string, _ string) ([]twitchapi.FollowedChannel, error) {
	if err := f.errors[userID]; err != nil {
		return nil, err
	}

	return f.channels[userID], nil
}

func newStartupTestLoader() *startupLoader {
	return &startupLoader{
		accounts: []save.Account{
			{ID: "acc-1", DisplayName: "one"},
			{ID: "acc-2", DisplayName: "two"},
			{ID: "anon", DisplayName: "anon", IsAnonymous: true},
		},
		clients: map[string]tab.ClientCapabilities{
			"acc-1": {
				Emotes:  &fakeUserEmoteClient{set: []twitchapi.UserEmoteImage{{ID: "1", Name: "kappa", Format: []string{"static"}}}, template: "https://cdn/{{id}}/{{format}}.png"},
				Follows: &fakeFollows{channels: map[string][]twitchapi.FollowedChannel{"acc-1": {{BroadcasterLogin: "FollowedOne"}}}},
			},
		},
		serverAPI: &fakeStartupUsers{byLogin: map[string]twitchapi.UserData{
			"alpha": {ID: "alpha-id", Login: "alpha", DisplayName: "Alpha"},
			"beta":  {ID: "beta-id", Login: "beta", DisplayName: "Beta"},
		}},
		emoteCache:   &fakeStartupEmotes{},
		badgeCache:   &fakeStartupBadges{},
		chatSettings: chatstate.NewCache(),
	}
}

// TestStartupLoader_PersistedStartup covers the normal path: state from disk, one
// batched lookup for the restored broadcast tabs, suggestions from history and
// follows, and the best-effort cache refreshes.
func TestStartupLoader_PersistedStartup(t *testing.T) {
	t.Parallel()

	state := save.AppState{Tabs: []save.TabState{
		{Channel: "alpha", IdentityID: "acc-1", Kind: int(tab.BroadcastKind)},
		{Channel: "alpha", IdentityID: "acc-1", Kind: int(tab.BroadcastKind)},
		{Channel: "beta", IdentityID: "acc-1", Kind: int(tab.BroadcastKind)},
		{Channel: "ignored", Kind: int(tab.MentionKind)},
	}}

	appState := &fakeAppStateManager{state: state}
	loader := newStartupTestLoader()
	loader.appStateManager = appState
	loader.channelHistory = &fakeChannelHistory{entries: []save.ChannelHistoryEntry{
		{ChannelLogin: "alpha"},
		{ChannelLogin: "fromhistory"},
	}}

	msg := loader.load()

	require.NoError(t, msg.err)
	require.Equal(t, 1, appState.calls)
	require.Equal(t, state, msg.state)
	require.Equal(t, "alpha-id", msg.ttvUsers["alpha"].ID)
	require.Equal(t, "beta-id", msg.ttvUsers["beta"].ID)

	calls := loader.serverAPI.(*fakeStartupUsers).calls()
	require.Len(t, calls, 1, "restored channels are looked up in one call")
	require.ElementsMatch(t, []string{"alpha", "beta"}, calls[0])
	require.NotContains(t, calls[0], "ignored", "only broadcast tabs are looked up")

	require.ElementsMatch(t, []string{"alpha", "fromhistory", "followedone"}, msg.channelSuggestions,
		"history and follows are merged case-insensitively without duplicates")

	emotes := loader.emoteCache.(*fakeStartupEmotes)
	require.Equal(t, 1, emotes.globalCall)
	require.Len(t, emotes.userEmotes["acc-1"], 1)
	require.Equal(t, "kappa", emotes.userEmotes["acc-1"][0].Text)
	require.NotContains(t, emotes.userEmotes, "anon", "anonymous accounts have no user emotes")

	require.Equal(t, 1, loader.badgeCache.(*fakeStartupBadges).calls)
}

// TestStartupLoader_DetachedStartup uses the state passed on the command line and
// never touches the persisted state.
func TestStartupLoader_DetachedStartup(t *testing.T) {
	t.Parallel()

	state := save.AppState{Tabs: []save.TabState{
		{Channel: "alpha", IdentityID: "acc-1", Kind: int(tab.BroadcastKind), IsFocused: true},
	}}

	appState := &fakeAppStateManager{err: errors.New("must not be called")}
	loader := newStartupTestLoader()
	loader.appStateManager = appState
	loader.initialState = &state

	msg := loader.load()

	require.NoError(t, msg.err)
	require.Equal(t, 0, appState.calls, "detached sessions do not read persisted state")
	require.Equal(t, state, msg.state)
	require.Equal(t, "alpha-id", msg.ttvUsers["alpha"].ID)
}

// TestStartupLoader_StateLoadFailure is fatal: Root cannot restore anything.
func TestStartupLoader_StateLoadFailure(t *testing.T) {
	t.Parallel()

	users := &fakeStartupUsers{}
	loader := newStartupTestLoader()
	loader.appStateManager = &fakeAppStateManager{err: errors.New("disk on fire")}
	loader.serverAPI = users

	msg := loader.load()

	require.ErrorContains(t, msg.err, "failed to load save state")
	require.Empty(t, users.calls(), "nothing is fetched when the state cannot be loaded")
}

// TestStartupLoader_PartialFailures keeps the session startable: badge/emote and
// follow failures are logged, but a failing user lookup is fatal.
func TestStartupLoader_PartialFailures(t *testing.T) {
	t.Parallel()

	t.Run("best effort caches", func(t *testing.T) {
		t.Parallel()

		loader := newStartupTestLoader()
		loader.appStateManager = &fakeAppStateManager{state: save.AppState{Tabs: []save.TabState{
			{Channel: "alpha", IdentityID: "acc-1", Kind: int(tab.BroadcastKind)},
		}}}
		loader.emoteCache.(*fakeStartupEmotes).globalErr = errors.New("emotes down")
		loader.badgeCache.(*fakeStartupBadges).err = errors.New("badges down")
		loader.channelHistory = &fakeChannelHistory{err: errors.New("history down")}

		msg := loader.load()

		require.NoError(t, msg.err, "cache failures must not abort startup")
		require.Equal(t, "alpha-id", msg.ttvUsers["alpha"].ID)
	})

	t.Run("failing user emote fetch", func(t *testing.T) {
		t.Parallel()

		loader := newStartupTestLoader()
		loader.appStateManager = &fakeAppStateManager{}
		loader.clients["acc-1"] = tab.ClientCapabilities{
			Emotes: &fakeUserEmoteClient{err: errors.New("emotes down")},
		}

		msg := loader.load()

		require.NoError(t, msg.err)
	})

	t.Run("failing user lookup", func(t *testing.T) {
		t.Parallel()

		loader := newStartupTestLoader()
		loader.appStateManager = &fakeAppStateManager{state: save.AppState{Tabs: []save.TabState{
			{Channel: "alpha", IdentityID: "acc-1", Kind: int(tab.BroadcastKind)},
		}}}
		loader.serverAPI = &fakeStartupUsers{err: errors.New("twitch down")}

		msg := loader.load()

		require.ErrorContains(t, msg.err, "could not fetch users for tabs")
	})
}

// TestStartupLoader_FollowFailureDoesNotDropHistory covers a failing account's
// follows: the other accounts and the history still contribute suggestions.
func TestStartupLoader_FollowFailureDoesNotDropHistory(t *testing.T) {
	t.Parallel()

	loader := newStartupTestLoader()
	loader.appStateManager = &fakeAppStateManager{}
	loader.channelHistory = &fakeChannelHistory{entries: []save.ChannelHistoryEntry{{ChannelLogin: "alpha"}}}
	loader.clients = map[string]tab.ClientCapabilities{
		"acc-1": {Follows: &fakeFollows{errors: map[string]error{"acc-1": errors.New("follows down")}}},
		"acc-2": {Follows: &fakeFollows{channels: map[string][]twitchapi.FollowedChannel{"acc-2": {{BroadcasterLogin: "Beta"}}}}},
	}

	msg := loader.load()

	require.NoError(t, msg.err)
	require.ElementsMatch(t, []string{"alpha", "beta"}, msg.channelSuggestions)
}

// TestStartupLoader_NoServerAPISkipsLookup keeps detached and test setups without
// a server API working.
func TestStartupLoader_NoServerAPISkipsLookup(t *testing.T) {
	t.Parallel()

	loader := newStartupTestLoader()
	loader.appStateManager = &fakeAppStateManager{state: save.AppState{Tabs: []save.TabState{
		{Channel: "alpha", IdentityID: "acc-1", Kind: int(tab.BroadcastKind)},
	}}}
	loader.serverAPI = nil

	msg := loader.load()

	require.NoError(t, msg.err)
	require.Empty(t, msg.ttvUsers)
}

// TestStartupLoader_PrefetchesChatSettingsOncePerChannel covers the startup warm
// up of the status bars: Get Chat Settings takes one broadcaster per request, so
// the loader asks for every unique restored channel exactly once, and the tabs
// then render from the cache.
func TestStartupLoader_PrefetchesChatSettingsOncePerChannel(t *testing.T) {
	t.Parallel()

	state := save.AppState{Tabs: []save.TabState{
		{Channel: "alpha", IdentityID: "acc-1", Kind: int(tab.BroadcastKind)},
		{Channel: "alpha", IdentityID: "acc-2", Kind: int(tab.BroadcastKind), IsFocused: true},
		{Channel: "beta", IdentityID: "acc-1", Kind: int(tab.BroadcastKind)},
		{Channel: "unknown", IdentityID: "acc-1", Kind: int(tab.BroadcastKind)},
	}}

	loader := newStartupTestLoader()
	loader.appStateManager = &fakeAppStateManager{state: state}

	users := loader.serverAPI.(*fakeStartupUsers)
	users.settings = map[string]twitchapi.ChatSettingData{
		"alpha-id": {BroadcasterID: "alpha-id", SlowMode: true, SlowModeWaitTime: 10},
		"beta-id":  {BroadcasterID: "beta-id", FollowerMode: true, FollowerModeDuration: 30},
	}

	msg := loader.load()

	require.NoError(t, msg.err)
	require.ElementsMatch(t, []string{"alpha-id", "beta-id"}, users.settingsCalls(),
		"two tabs of one channel share a request, unknown channels are skipped")

	alpha, ok := loader.chatSettings.Get("alpha-id")
	require.True(t, ok)
	require.True(t, alpha.SlowMode)
	require.Equal(t, 10, alpha.SlowModeWaitTime)

	beta, ok := loader.chatSettings.Get("beta-id")
	require.True(t, ok)
	require.True(t, beta.FollowerMode)
}

// TestStartupLoader_ChatSettingsPrefetchFailures keeps a failing prefetch
// best-effort: the session still restores and the tab fetches on demand.
func TestStartupLoader_ChatSettingsPrefetchFailures(t *testing.T) {
	t.Parallel()

	loader := newStartupTestLoader()
	loader.appStateManager = &fakeAppStateManager{state: save.AppState{Tabs: []save.TabState{
		{Channel: "alpha", IdentityID: "acc-1", Kind: int(tab.BroadcastKind)},
	}}}
	loader.serverAPI.(*fakeStartupUsers).settingsErrs = errors.New("settings down")

	msg := loader.load()

	require.NoError(t, msg.err, "chat settings are not required to start")

	_, ok := loader.chatSettings.Get("alpha-id")
	require.False(t, ok)
}

// TestStartupLoader_ChatSettingsUsesAccountClient prefers the signed-in account's
// own client over the server proxy, matching the path a tab takes.
func TestStartupLoader_ChatSettingsUsesAccountClient(t *testing.T) {
	t.Parallel()

	client := &fakeStartupChatClient{}
	loader := newStartupTestLoader()
	loader.appStateManager = &fakeAppStateManager{state: save.AppState{Tabs: []save.TabState{
		{Channel: "alpha", IdentityID: "acc-1", Kind: int(tab.BroadcastKind)},
	}}}
	loader.clients = map[string]tab.ClientCapabilities{"acc-1": {API: client}}

	msg := loader.load()

	require.NoError(t, msg.err)
	require.Equal(t, []string{"alpha-id"}, client.settingsCalls())
	require.Empty(t, loader.serverAPI.(*fakeStartupUsers).settingsCalls(), "the proxy is not used when an account client exists")

	settings, ok := loader.chatSettings.Get("alpha-id")
	require.True(t, ok)
	require.True(t, settings.EmoteMode)
}
