package mainui

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/stretchr/testify/require"
)

func privateMessage(content string) *twitchirc.PrivateMessage {
	return &twitchirc.PrivateMessage{
		RoomID:          "room-1",
		ChannelUserName: "chan",
		DisplayName:     "Alice",
		LoginName:       "alice",
		Message:         content,
	}
}

type fakePreparationEmotes struct {
	mu       sync.Mutex
	channels []string
}

func (f *fakePreparationEmotes) RefreshLocal(_ context.Context, channelID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.channels = append(f.channels, channelID)

	return nil
}

type fakePreparationBadges struct {
	mu       sync.Mutex
	channels []string
}

func (f *fakePreparationBadges) RefreshChannel(_ context.Context, broadcasterID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.channels = append(f.channels, broadcasterID)

	return nil
}

type fakePreparationServer struct {
	mu        sync.Mutex
	users     map[string]twitchapi.UserData
	userCalls int
	linkErr   error
	linkDelay time.Duration
	linkResp  server.CheckLinkResponse
}

func (f *fakePreparationServer) GetUsers(_ context.Context, _ []string, ids []string) (twitchapi.UserResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.userCalls++

	resp := twitchapi.UserResponse{}
	for _, id := range ids {
		if user, ok := f.users[id]; ok {
			resp.Data = append(resp.Data, user)
		}
	}

	return resp, nil
}

func (f *fakePreparationServer) CheckLink(ctx context.Context, _ string) (server.CheckLinkResponse, error) {
	if f.linkDelay > 0 {
		select {
		case <-time.After(f.linkDelay):
		case <-ctx.Done():
			return server.CheckLinkResponse{}, ctx.Err()
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	return f.linkResp, f.linkErr
}

func (f *fakePreparationServer) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.userCalls
}

func newTestPreparer(t *testing.T) (*chatEventPreparer, *fakePreparationServer, *fakePreparationEmotes, *fakePreparationBadges) {
	t.Helper()
	server := &fakePreparationServer{
		users:    map[string]twitchapi.UserData{"guest-id": {ID: "guest-id", Login: "guest", DisplayName: "Guest"}},
		linkResp: server.CheckLinkResponse{RemoteStatusCode: 200, RemoteContentType: "text/html; charset=utf-8"},
	}

	emotes := &fakePreparationEmotes{}
	badges := &fakePreparationBadges{}

	ctx, cancel := context.WithCancel(t.Context())
	prep := &chatEventPreparer{
		ctx:        ctx,
		cancel:     cancel,
		settings:   save.BuildDefaultSettings(),
		emoteCache: emotes,
		badgeCache: badges,
		server:     server,
		userNames:  &sync.Map{},
		timeout:    time.Second,
	}
	t.Cleanup(prep.close)
	return prep, server, emotes, badges
}

func TestChatEventPreparer_LinkAnnotation(t *testing.T) {
	t.Parallel()

	t.Run("successful check annotates the token", func(t *testing.T) {
		t.Parallel()

		prep, _, _, _ := newTestPreparer(t)

		event := prep.prepare(prepareRequest{ircer: privateMessage("see https://example.com now")})

		require.Equal(t, "https://example.com [OK, text/html]", event.DisplayModifier.WordReplacements["https://example.com"])
	})

	t.Run("failed check keeps the message unannotated", func(t *testing.T) {
		t.Parallel()

		prep, server, _, _ := newTestPreparer(t)
		server.linkErr = errors.New("link check down")

		event := prep.prepare(prepareRequest{ircer: privateMessage("see https://example.com now")})

		require.NotContains(t, event.DisplayModifier.WordReplacements, "https://example.com")
		require.Equal(t, "see https://example.com now", event.Message.(*twitchirc.PrivateMessage).Message)
	})

	t.Run("a slow check is bounded by the preparation timeout", func(t *testing.T) {
		t.Parallel()

		prep, server, _, _ := newTestPreparer(t)
		prep.timeout = 20 * time.Millisecond
		server.linkDelay = 2 * time.Second

		start := time.Now()
		event := prep.prepare(prepareRequest{ircer: privateMessage("see https://example.com now")})
		elapsed := time.Since(start)

		require.Less(t, elapsed, time.Second, "the preparation timeout bounds enrichment")
		require.NotContains(t, event.DisplayModifier.WordReplacements, "https://example.com")
	})
}

func TestChatEventPreparer_SharedChatGuest(t *testing.T) {
	t.Parallel()

	prep, server, emotes, badges := newTestPreparer(t)
	message := &twitchirc.PrivateMessage{
		RoomID:          "host-id",
		SourceRoomID:    "guest-id",
		ChannelUserName: "host",
		DisplayName:     "Alice",
		LoginName:       "alice",
		Message:         "hi",
	}

	event := prep.prepare(prepareRequest{ircer: message})

	require.Equal(t, "Guest", event.ChannelGuestDisplayName)
	require.Equal(t, []string{"guest-id"}, emotes.channels, "guest emotes are refreshed once")
	require.Equal(t, []string{"guest-id"}, badges.channels, "guest badges are refreshed once")

	second := prep.prepare(prepareRequest{ircer: message})

	require.Equal(t, "Guest", second.ChannelGuestDisplayName)
	require.Equal(t, 1, server.calls(), "the guest display name is cached after the first lookup")
	require.Len(t, emotes.channels, 1)
}

func TestChatEventPreparer_Projection(t *testing.T) {
	t.Parallel()

	prep, _, _, _ := newTestPreparer(t)

	event := prep.prepare(prepareRequest{
		accountID: "acc",
		tabID:     "tab-1",
		isFake:    true,
		ircer: &twitchirc.ClearChat{
			RoomID:          "room-1",
			ChannelUserName: "chan",
		},
	})

	require.True(t, event.IsFakeEvent)
	require.Equal(t, "acc", event.AccountID)
	require.Equal(t, "tab-1", event.TabID)
	require.Equal(t, "chan", event.Channel)
	require.Equal(t, "room-1", event.ChannelID)
}

func TestEventQueue(t *testing.T) {
	t.Parallel()

	q := newEventQueue(2)

	first, ok := q.add("acc", "", privateMessage("first"), false)
	require.True(t, ok)
	second, ok := q.add("acc", "", privateMessage("second"), false)
	require.True(t, ok)
	_, ok = q.add("acc", "", privateMessage("third"), false)
	require.False(t, ok, "the queue is bounded")

	inFlight, ok := q.start()
	require.True(t, ok)
	require.Equal(t, first.seq, inFlight.seq)

	_, ok = q.start()
	require.False(t, ok, "only one preparation runs at a time")

	require.False(t, q.finish(second.seq), "a result for another request is rejected")
	require.False(t, q.finish(0))
	require.True(t, q.finish(first.seq))

	next, ok := q.start()
	require.True(t, ok)
	require.Equal(t, second.seq, next.seq, "the next request is the one that was queued second")
	require.True(t, q.finish(next.seq))
	require.Equal(t, 0, q.len())
}

type emotePreparationFunc func(context.Context, string, string, []twitchirc.Emote) (string, map[string]string, error)

func (f emotePreparationFunc) Replace(ctx context.Context, channel, content string, emotes []twitchirc.Emote) (string, map[string]string, error) {
	return f(ctx, channel, content, emotes)
}

type badgePreparationFuncs struct {
	replace func(context.Context, string, []twitchirc.Badge) (string, map[string]string, error)
	inject  func(context.Context, string, map[string]string) (string, error)
}

func (f badgePreparationFuncs) Replace(ctx context.Context, channel string, badges []twitchirc.Badge) (string, map[string]string, error) {
	if f.replace != nil {
		return f.replace(ctx, channel, badges)
	}
	return "", nil, nil
}

func (f badgePreparationFuncs) InjectContributorBadge(ctx context.Context, login string, badges map[string]string) (string, error) {
	if f.inject != nil {
		return f.inject(ctx, login, badges)
	}
	return "", nil
}

func runPreparation(cmd tea.Cmd) <-chan tea.Msg {
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	return result
}

func awaitPreparation(t *testing.T, result <-chan tea.Msg) tea.Msg {
	t.Helper()
	select {
	case msg := <-result:
		return msg
	case <-time.After(time.Second):
		t.Fatal("preparation did not finish")
		return nil
	}
}

func TestChatEventPreparer_DeadlineCoversEveryReplacement(t *testing.T) {
	t.Parallel()

	for _, stage := range []string{"emote", "badge", "contributor"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()

			prep, _, _, _ := newTestPreparer(t)
			prep.timeout = 20 * time.Millisecond
			seen := make(chan error, 1)
			wait := func(ctx context.Context) error {
				<-ctx.Done()
				seen <- ctx.Err()
				return ctx.Err()
			}
			switch stage {
			case "emote":
				prep.emoteReplacer = emotePreparationFunc(func(ctx context.Context, _, _ string, _ []twitchirc.Emote) (string, map[string]string, error) {
					return "", nil, wait(ctx)
				})
			case "badge":
				prep.badgeReplacer = badgePreparationFuncs{replace: func(ctx context.Context, _ string, _ []twitchirc.Badge) (string, map[string]string, error) {
					return "", nil, wait(ctx)
				}}
			case "contributor":
				prep.badgeReplacer = badgePreparationFuncs{inject: func(ctx context.Context, _ string, _ map[string]string) (string, error) {
					return "", wait(ctx)
				}}
			}

			message := privateMessage("still displayed")
			message.Badges = []twitchirc.Badge{{Name: "subscriber", Version: "1"}}
			result := runPreparation(func() tea.Msg {
				return prep.prepare(prepareRequest{ircer: message})
			})
			event := awaitPreparation(t, result).(tab.ChatEventMessage)
			require.Same(t, message, event.Message, "enrichment failure must not discard the original message")
			require.NotNil(t, event.DisplayModifier.BadgeReplacement, "failed badge replacement must retain a writable contributor map")
			require.Empty(t, event.PrepareCommand)
			select {
			case err := <-seen:
				require.ErrorIs(t, err, context.DeadlineExceeded)
			default:
				t.Fatal("replacement did not observe the preparation deadline")
			}
		})
	}
}
