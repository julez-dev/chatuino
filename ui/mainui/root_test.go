package mainui

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/badge"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/chat"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/julez-dev/chatuino/wspool"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func newRestoreTestRoot(t *testing.T) *Root {
	t.Helper()

	return newTestRoot(t, []save.Account{{ID: "acc", DisplayName: "me"}}, map[string]tab.ClientCapabilities{}, nil)
}

// Restoring a session activates the outgoing tab before the incoming one. Restored
// tabs have no composer yet: their children are only created once the channel data
// has been fetched, which happens after the restore message is handled.
func Test_Root_handlePersistedDataLoaded_RestoresUnloadedTabs(t *testing.T) {
	root := newRestoreTestRoot(t)

	msg := persistedDataLoadedMessage{
		state: save.AppState{Tabs: []save.TabState{
			{Channel: "alpha", IdentityID: "acc", Kind: int(tab.BroadcastKind)},
			{Channel: "beta", IdentityID: "acc", Kind: int(tab.BroadcastKind), IsFocused: true},
		}},
		ttvUsers: map[string]twitchapi.UserData{},
	}

	require.NotPanics(t, func() {
		root.handlePersistedDataLoaded(msg)
	}, "blurring a restored tab must not touch the composer it does not have yet")

	t.Cleanup(func() {
		for _, t := range root.tabs.All() {
			t.Close()
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
			{Channel: "alpha", IdentityID: "acc", Kind: int(tab.BroadcastKind)},
			{Channel: "beta", IdentityID: "acc", Kind: int(tab.BroadcastKind)},
		}},
		ttvUsers: map[string]twitchapi.UserData{},
	}

	require.NotPanics(t, func() {
		root.handlePersistedDataLoaded(msg)
	})

	t.Cleanup(func() {
		for _, t := range root.tabs.All() {
			t.Close()
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
			{Channel: "alpha", IdentityID: "acc", Kind: int(tab.BroadcastKind), IsFocused: true},
		}},
		ttvUsers: map[string]twitchapi.UserData{},
	})

	t.Cleanup(func() {
		for _, t := range root.tabs.All() {
			t.Close()
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
		tab.Close()
	})
}

// Restored tabs whose account or channel is no longer available are skipped
// instead of creating a tab that cannot load.
func Test_Root_handlePersistedDataLoaded_SkipsUnavailableTabs(t *testing.T) {
	root := newRestoreTestRoot(t)

	root.handlePersistedDataLoaded(persistedDataLoadedMessage{
		state: save.AppState{Tabs: []save.TabState{
			{Channel: "alpha", IdentityID: "unknown-account", Kind: int(tab.BroadcastKind)},
			{Channel: "", IdentityID: "acc", Kind: int(tab.BroadcastKind)},
			{Channel: "beta", IdentityID: "acc", Kind: int(tab.BroadcastKind)},
			{Kind: int(tab.MentionKind)},
		}},
		ttvUsers: map[string]twitchapi.UserData{},
	})

	t.Cleanup(func() {
		for _, t := range root.tabs.All() {
			t.Close()
		}
	})

	require.Equal(t, 2, root.tabs.Len(), "only the restorable broadcast tab and the mention tab are created")

	titles := make([]string, 0, 2)
	for _, tb := range root.tabs.All() {
		titles = append(titles, tb.Title())
	}

	require.Contains(t, titles, "beta")
}

type blockingBadgeFetcher struct {
	started chan<- context.Context
	release <-chan struct{}
}

func (f blockingBadgeFetcher) GetGlobalChatBadges(context.Context) ([]twitchapi.BadgeSet, error) {
	return nil, nil
}

func (f blockingBadgeFetcher) GetChannelChatBadges(ctx context.Context, _ string) ([]twitchapi.BadgeSet, error) {
	f.started <- ctx
	select {
	case <-f.release:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type observedBadgeRefresh struct {
	cache   *badge.Cache
	entered chan struct{}
}

func (r observedBadgeRefresh) RefreshChannel(ctx context.Context, channelID string) error {
	close(r.entered)
	return r.cache.RefreshChannel(ctx, channelID)
}

// A tab's independent refresh must not keep Root.Close waiting after preparation
// joins the same cache flight. Cancelling the UI only cancels its own waiter.
func TestRoot_CloseCancelsSharedBadgeCacheWait(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	release := make(chan struct{})
	releaseRefresh := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseRefresh)
	started := make(chan context.Context, 1)
	cache := badge.NewCache(blockingBadgeFetcher{started: started, release: release})
	refreshCtx, cancelRefresh := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancelRefresh()
	refreshDone := make(chan error, 1)
	go func() { refreshDone <- cache.RefreshChannel(refreshCtx, "guest") }()

	var sharedCtx context.Context
	select {
	case sharedCtx = <-started:
	case <-time.After(time.Second):
		t.Fatal("independent badge refresh did not start")
	}

	entered := make(chan struct{})
	root.preparer.emoteCache = nil
	root.preparer.badgeCache = observedBadgeRefresh{cache: cache, entered: entered}
	message := privateMessage("hello")
	message.SourceRoomID = "guest"
	result := runPreparation(root.enqueueChatEvent("acc", "tab", message, false))
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("preparation did not reach the shared badge cache")
	}

	closed := make(chan struct{})
	go func() { root.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Error("Root.Close remained blocked on another caller's cache refresh after UI cancellation")
	}
	select {
	case <-sharedCtx.Done():
		t.Error("UI cancellation cancelled another caller's refresh")
	default:
	}

	releaseRefresh()
	select {
	case err := <-refreshDone:
		require.NoError(t, err, "the independent refresh must still complete successfully")
	case <-time.After(time.Second):
		t.Fatal("independent badge refresh did not finish")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Root.Close did not finish after the refresh was released")
	}
	_, next := root.Update(awaitPreparation(t, result))
	require.Nil(t, next, "shutdown must not route late content or start more work")
}

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
		event, ok := messages[i].(chat.ChatEventMessage)
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

	event := chat.ChatEventMessage{
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

	routed, ok := messages[0].(chat.ChatEventMessage)
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

	prepared := preparedChatEvent{seq: seq, event: chat.ChatEventMessage{Message: privateMessage("once")}}

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
		_ = root.dispatchPreparedEvent(chat.ChatEventMessage{TabID: "tab-1", Message: privateMessage("late")})
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

	_ = root.dispatchPreparedEvent(chat.ChatEventMessage{Message: privateMessage("hello")})

	require.Len(t, one.messages(), 1)
	require.Len(t, two.messages(), 1)
}

func TestRoot_ShutdownCancelsInFlightPreparation(t *testing.T) {
	t.Parallel()

	for _, trigger := range []string{"close", "quit", "parent cancellation"} {
		t.Run(trigger, func(t *testing.T) {
			t.Parallel()

			root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			root.preparer.close()
			root.preparer = newChatEventPreparer(ctx, root)
			started := make(chan struct{})
			finished := make(chan struct{})
			root.preparer.emoteReplacer = emotePreparationFunc(func(ctx context.Context, _, _ string, _ []twitchirc.Emote) (string, map[string]string, error) {
				close(started)
				defer close(finished)
				<-ctx.Done()
				return "", nil, ctx.Err()
			})
			ft := newFakeTab("tab-1")
			root.tabs.Add(ft)
			result := runPreparation(root.enqueueChatEvent("acc", "tab-1", privateMessage("blocked"), true))
			require.Nil(t, root.enqueueChatEvent("acc", "tab-1", privateMessage("pending"), true))
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("enrichment did not start")
			}

			switch trigger {
			case "close":
				root.Close()
			case "quit":
				_, cmd := root.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
				require.IsType(t, tea.QuitMsg{}, cmd())
			case "parent cancellation":
				cancel()
				select {
				case <-finished:
				case <-time.After(time.Second):
					t.Fatal("application cancellation did not reach enrichment")
				}
				root.Close()
			}
			select {
			case <-finished:
			default:
				t.Fatal("Close returned before in-flight work unwound")
			}
			_, cmd := root.Update(awaitPreparation(t, result))
			require.Nil(t, cmd, "shutdown must neither route late content nor start pending work")
			require.Empty(t, ft.messages())
			require.Nil(t, root.enqueueChatEvent("acc", "tab-1", privateMessage("after shutdown"), true))
			root.Close() // idempotent, including the deferred application cleanup
		})
	}
}

func TestRoot_CloseBeforeScheduledPreparationStarts(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	var calls atomic.Int32
	root.preparer.emoteReplacer = emotePreparationFunc(func(context.Context, string, string, []twitchirc.Emote) (string, map[string]string, error) {
		calls.Add(1)
		return "", nil, nil
	})
	cmd := root.enqueueChatEvent("acc", "", privateMessage("scheduled"), false)
	root.Close()
	_, next := root.Update(cmd())
	require.Nil(t, next)
	require.Zero(t, calls.Load())
}

func chatEvents(ft *fakeTab) []chat.ChatEventMessage {
	var events []chat.ChatEventMessage
	for _, msg := range ft.messages() {
		if event, ok := msg.(chat.ChatEventMessage); ok {
			events = append(events, event)
		}
	}
	return events
}

func eventContents(ft *fakeTab) []string {
	var contents []string
	for _, event := range chatEvents(ft) {
		contents = append(contents, event.Message.(*twitchirc.PrivateMessage).Message)
	}
	return contents
}

func blockFirstPreparation(t *testing.T, root *Root) (<-chan struct{}, chan<- struct{}) {
	t.Helper()
	started := make(chan struct{})
	release := make(chan struct{})
	root.preparer.emoteReplacer = emotePreparationFunc(func(ctx context.Context, _, content string, _ []twitchirc.Emote) (string, map[string]string, error) {
		if content == "first" {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return "", nil, ctx.Err()
			}
		}
		return "", nil, nil
	})
	return started, release
}

func drainPreparation(t *testing.T, root *Root, msg tea.Msg) {
	t.Helper()
	_, cmd := root.Update(msg)
	for cmd != nil {
		messages := execCmd(t, cmd)
		require.Len(t, messages, 1)
		_, cmd = root.Update(messages[0])
	}
}

func TestRoot_DelayedLiveEventKeepsHistoryBatchContiguous(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	logger := make(chan *twitchirc.PrivateMessage, 2)
	root.messageLoggerChan = logger
	one, two := newFakeTab("tab-1"), newFakeTab("tab-2")
	root.tabs.Add(one)
	root.tabs.Add(two)
	started, release := blockFirstPreparation(t, root)
	_, cmd := root.Update(wspool.IRCEvent{AccountID: "acc", Message: privateMessage("first")})
	result := runPreparation(cmd)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first preparation did not start")
	}

	_, cmd = root.Update(tab.RequestLocalMessageHandleBatchMessage{
		AccountID: "acc", TabID: "tab-1",
		Messages: []twitchirc.IRCer{privateMessage("history-1"), privateMessage("history-2")},
	})
	require.Nil(t, cmd)
	_, cmd = root.Update(wspool.IRCEvent{AccountID: "acc", Message: privateMessage("last")})
	require.Nil(t, cmd)
	require.Empty(t, chatEvents(one), "later work cannot overtake the delayed event")
	require.Len(t, logger, 2, "live logging must not wait for enrichment or include tab-local history")

	close(release)
	drainPreparation(t, root, awaitPreparation(t, result))
	require.Equal(t, []string{"first", "history-1", "history-2", "last"}, eventContents(one))
	require.Equal(t, []string{"first", "last"}, eventContents(two), "history remains tab-local")
	require.True(t, chatEvents(one)[1].IsFakeEvent)
	require.False(t, chatEvents(one)[3].IsFakeEvent)
	require.Zero(t, root.events.inFlight)
	require.Zero(t, root.events.len())
}

func TestRoot_TabClosesWhilePreparationIsInFlight(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	closed, other := newFakeTab("closed"), newFakeTab("other")
	root.tabs.Add(closed)
	root.tabs.Add(other)
	started, release := blockFirstPreparation(t, root)
	result := runPreparation(root.enqueueChatEvent("acc", "closed", privateMessage("first"), true))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("preparation did not start")
	}
	root.tabs.Remove("closed")
	replacement := newFakeTab("replacement") // same account/channel, different UI instance
	root.tabs.Add(replacement)
	root.enqueueChatEvent("acc", "", privateMessage("live"), false)

	close(release)
	drainPreparation(t, root, awaitPreparation(t, result))
	require.Empty(t, chatEvents(closed))
	require.Equal(t, []string{"live"}, eventContents(replacement))
	require.Equal(t, []string{"live"}, eventContents(other))
}

// Use Bubble Tea's actual command scheduler rather than simulating Sequence: raw
// preparation must reach the terminal before either fan-out recipient gets data.
type preparationProgramModel struct {
	root        *Root
	initial     tea.Cmd
	recipients  []*fakeTab
	rawCommands []string
	rawAtRoute  []int
}

func (m *preparationProgramModel) Init() tea.Cmd  { return m.initial }
func (m *preparationProgramModel) View() tea.View { return tea.NewView("") }
func (m *preparationProgramModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// The headless program has no terminal size; keep shell geometry fixed so
	// this test exercises scheduling rather than zero-sized header rendering.
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		msg = tea.WindowSizeMsg{Width: 80, Height: 24}
	}
	if raw, ok := msg.(tea.RawMsg); ok {
		m.rawCommands = append(m.rawCommands, raw.Msg.(string))
	}
	before := len(chatEvents(m.recipients[0]))
	_, cmd := m.root.Update(msg)
	if len(chatEvents(m.recipients[0])) > before {
		for _, recipient := range m.recipients {
			m.rawAtRoute = append(m.rawAtRoute, len(m.rawCommands))
			if len(chatEvents(recipient)) != 1 {
				m.rawAtRoute = append(m.rawAtRoute, -1)
			}
		}
		return m, tea.Quit
	}
	return m, cmd
}

func TestRoot_KittyPreparationPrecedesFanOutWithRealScheduler(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	root.preparer.emoteReplacer = emotePreparationFunc(func(context.Context, string, string, []twitchirc.Emote) (string, map[string]string, error) {
		return "kitty-image", map[string]string{"image": "placeholder"}, nil
	})
	one, two := newFakeTab("one"), newFakeTab("two")
	root.tabs.Add(one)
	root.tabs.Add(two)
	model := &preparationProgramModel{
		root: root, recipients: []*fakeTab{one, two},
		initial: root.enqueueChatEvent("acc", "", privateMessage("image"), false),
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	program := tea.NewProgram(model, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer())
	_, err := program.Run()
	require.NoError(t, err)
	require.Equal(t, []string{"kitty-image"}, model.rawCommands)
	require.Equal(t, []int{1, 1}, model.rawAtRoute)
	for _, recipient := range model.recipients {
		require.Len(t, chatEvents(recipient), 1)
		require.Empty(t, chatEvents(recipient)[0].PrepareCommand)
	}
}

func TestRoot_DuplicateGraphicsPreparationHasNoSideEffects(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	ft := newFakeTab("tab-1")
	root.tabs.Add(ft)
	root.enqueueChatEvent("acc", "tab-1", privateMessage("image"), false)
	seq := root.events.inFlight
	prepared := preparedChatEvent{seq: seq, event: chat.ChatEventMessage{
		TabID: "tab-1", Message: privateMessage("image"), PrepareCommand: "kitty-image",
	}}

	_, first := root.Update(prepared)
	require.NotNil(t, first)
	require.Empty(t, ft.messages())

	_, duplicate := root.Update(prepared)
	require.Nil(t, duplicate, "a duplicate must not resend the terminal command during the handoff")

	event := prepared.event
	event.PrepareCommand = ""
	root.Update(routedPreparedEvent{seq: seq, event: event})
	require.Len(t, ft.messages(), 1)

	_, duplicate = root.Update(prepared)
	require.Nil(t, duplicate, "a completed request must not resend the terminal command")
	_, duplicate = root.Update(routedPreparedEvent{seq: seq, event: event})
	require.Nil(t, duplicate)
	require.Len(t, ft.messages(), 1)

	next := root.enqueueChatEvent("acc", "tab-1", privateMessage("next"), false)
	require.NotNil(t, next)
	nextSeq := root.events.inFlight
	_, duplicate = root.Update(prepared)
	require.Nil(t, duplicate, "a stale result must not affect a newer request")
	require.Equal(t, nextSeq, root.events.inFlight)
	require.Len(t, ft.messages(), 1)
}

// A resize can change the cell pixel size (font or monitor switch), so the
// display manager re-reads it instead of keeping the startup value forever.
func TestRoot_ResizeRefreshesCellSize(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, nil, nil)
	refreshes := 0
	root.imageDisplayManager = kittyimg.NewDisplayManager(afero.NewMemMapFs(), kittyimg.CellSize{Width: 10, Height: 10}, kittyimg.DisplayOptions{
		Rows: 1,
		CellSize: func() (kittyimg.CellSize, error) {
			refreshes++
			return kittyimg.CellSize{Width: 12, Height: 24}, nil
		},
	})

	root.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	require.Equal(t, 1, refreshes)
}
