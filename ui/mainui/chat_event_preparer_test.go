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

// fakeTab records every message routed to it.
type fakeTab struct {
	id        string
	accountID string
	channel   string
	kind      tab.TabKind

	mu       sync.Mutex
	received []tea.Msg
}

func (f *fakeTab) Init() tea.Cmd                               { return nil }
func (f *fakeTab) InitWithUserData(twitchapi.UserData) tea.Cmd { return nil }
func (f *fakeTab) View() string                                { return "" }
func (f *fakeTab) ViewWithoutStatusBar() string                { return "" }
func (f *fakeTab) StatusBarView() string                       { return "" }
func (f *fakeTab) Focus()                                      {}
func (f *fakeTab) Blur()                                       {}
func (f *fakeTab) Close()                                      {}
func (f *fakeTab) AccountID() string                           { return f.accountID }
func (f *fakeTab) Channel() string                             { return f.channel }
func (f *fakeTab) IsComposing() bool                           { return false }
func (f *fakeTab) IsSearching() bool                           { return false }
func (f *fakeTab) IsDataLoaded() bool                          { return true }
func (f *fakeTab) ID() string                                  { return f.id }
func (f *fakeTab) Focused() bool                               { return false }
func (f *fakeTab) ChannelID() string                           { return "" }
func (f *fakeTab) HandleResize()                               {}
func (f *fakeTab) SetSize(int, int)                            {}
func (f *fakeTab) SetFullWidth(int)                            {}
func (f *fakeTab) Kind() tab.TabKind                           { return f.kind }
func (f *fakeTab) Title() string                               { return f.id }
func (f *fakeTab) Identity() string                            { return "" }

func (f *fakeTab) Update(msg tea.Msg) (tab.Tab, tea.Cmd) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.received = append(f.received, msg)

	return f, nil
}

func (f *fakeTab) messages() []tea.Msg {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]tea.Msg(nil), f.received...)
}

func newFakeTab(id string) *fakeTab {
	return &fakeTab{id: id, accountID: "acc", channel: "chan", kind: tab.BroadcastKind}
}

// execCmd runs a command and flattens batches so tests can feed the resulting
// messages back into Root.
func execCmd(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()

	if cmd == nil {
		return nil
	}

	msg := cmd()
	if msg == nil {
		return nil
	}

	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg

		for _, c := range batch {
			out = append(out, execCmd(t, c)...)
		}

		return out
	}

	if seq, ok := msg.(interface{ Commands() []tea.Cmd }); ok {
		var out []tea.Msg
		for _, c := range seq.Commands() {
			out = append(out, execCmd(t, c)...)
		}

		return out
	}

	return []tea.Msg{msg}
}

func privateMessage(content string) *twitchirc.PrivateMessage {
	return &twitchirc.PrivateMessage{
		RoomID:          "room-1",
		ChannelUserName: "chan",
		DisplayName:     "Alice",
		LoginName:       "alice",
		Message:         content,
	}
}

// TestRoot_PreparationPreservesArrivalOrder drives the real Root flow: events are
// queued on arrival, enriched one at a time, and routed in the same order.
func TestRoot_PreparationPreservesArrivalOrder(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	ft := newFakeTab("tab-1")
	root.tabs.Add(ft)

	cmd := root.enqueueChatEvent("acc", "", privateMessage("first"), false)
	require.NotNil(t, cmd, "the first event starts preparation")
	require.Nil(t, root.enqueueChatEvent("acc", "", privateMessage("second"), false), "queued, not started")
	require.Nil(t, root.enqueueChatEvent("acc", "", privateMessage("third"), false))

	for cmd != nil {
		var next tea.Cmd

		for _, msg := range execCmd(t, cmd) {
			prepared, ok := msg.(preparedChatEvent)
			if !ok {
				continue
			}

			_, next = root.Update(prepared)
		}

		cmd = next
	}

	messages := ft.messages()
	require.Len(t, messages, 3)

	for i, want := range []string{"first", "second", "third"} {
		event, ok := messages[i].(tab.ChatEventMessage)
		require.True(t, ok)
		require.Equal(t, want, event.Message.(*twitchirc.PrivateMessage).Message,
			"events must be routed in arrival order")
	}

	require.Equal(t, 0, root.events.len())
	require.Zero(t, root.events.inFlight)
}

// TestRoot_PrepareCommandRunsBeforeRouting keeps tab content behind the raw
// terminal command that places its images.
func TestRoot_PrepareCommandRunsBeforeRouting(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	ft := newFakeTab("tab-1")
	root.tabs.Add(ft)

	root.enqueueChatEvent("acc", "tab-1", privateMessage("image"), false)
	seq := root.events.inFlight
	require.NotZero(t, seq)

	event := tab.ChatEventMessage{
		TabID:          "tab-1",
		Message:        privateMessage("image"),
		PrepareCommand: "\x1b_G the image",
	}

	_, cmd := root.Update(preparedChatEvent{seq: seq, event: event})
	require.NotNil(t, cmd, "the raw command is emitted")
	require.Empty(t, ft.messages(), "content is not routed before the terminal command is written")
	require.Equal(t, seq, root.events.inFlight, "the queue stays in flight across the hand-off")

	// releasePreparedEvent clears the command before deferring the route.
	event.PrepareCommand = ""

	_, cmd = root.Update(routedPreparedEvent{seq: seq, event: event})
	require.Nil(t, cmd, "nothing left in the queue")

	messages := ft.messages()
	require.Len(t, messages, 1, "the event is routed exactly once")

	routed, ok := messages[0].(tab.ChatEventMessage)
	require.True(t, ok)
	require.Empty(t, routed.PrepareCommand, "the preparation command is not duplicated into the tab")
}

// TestRoot_DuplicatePreparedResultIsIgnored discards a repeated result instead of
// delivering the event twice.
func TestRoot_DuplicatePreparedResultIsIgnored(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	ft := newFakeTab("tab-1")
	root.tabs.Add(ft)

	root.enqueueChatEvent("acc", "", privateMessage("once"), false)
	seq := root.events.inFlight

	prepared := preparedChatEvent{seq: seq, event: tab.ChatEventMessage{Message: privateMessage("once")}}

	_, _ = root.Update(prepared)
	require.Len(t, ft.messages(), 1)

	_, _ = root.Update(prepared)
	require.Len(t, ft.messages(), 1, "the duplicate result is dropped")
}

// TestRoot_ClosedTabDropsLateEvent routes a late result for a tab that was closed
// in the meantime: no panic, and no other tab receives it.
func TestRoot_ClosedTabDropsLateEvent(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	closed := newFakeTab("tab-1")
	other := newFakeTab("tab-2")
	root.tabs.Add(closed)
	root.tabs.Add(other)

	_, ok := root.tabs.Remove("tab-1")
	require.True(t, ok)

	require.NotPanics(t, func() {
		_ = root.dispatchPreparedEvent(tab.ChatEventMessage{TabID: "tab-1", Message: privateMessage("late")})
	})

	require.Empty(t, closed.messages())
	require.Empty(t, other.messages(), "a tab-addressed event must not reach another tab")
}

// TestRoot_BroadcastRouting checks that non-addressed events fan out to every tab
// once, which is what lets channel tabs, the mention tab and the notification tab
// all see them.
func TestRoot_BroadcastRouting(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	one := newFakeTab("tab-1")
	two := newFakeTab("tab-2")
	root.tabs.Add(one)
	root.tabs.Add(two)

	_ = root.dispatchPreparedEvent(tab.ChatEventMessage{Message: privateMessage("hello")})

	require.Len(t, one.messages(), 1)
	require.Len(t, two.messages(), 1)
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
