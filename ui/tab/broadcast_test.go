package tab

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/chatstate"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/chat"
	"github.com/julez-dev/chatuino/wspool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// Mode transitions and shared tab fixtures.

// fsmModes is every mode the tab can be in.
var fsmModes = []broadcastTabState{
	inChatWindow,
	insertMode,
	userInspectMode,
	userInspectInsertMode,
	emoteOverviewMode,
	unbanRequestMode,
}

// fsmEvents is every input that may move the tab.
var fsmEvents = []tabEvent{
	eventCompose,
	eventSend,
	eventCancel,
	eventInspect,
	eventEmoteOverview,
	eventUnbanRequests,
	eventLeaveInspect,
}

type expectation struct {
	allowed bool
	to      broadcastTabState
}

// fsmExpectations is the specification of the mode machine, written out in full
// so that adding a mode or an event without deciding what happens to every pair
// fails the matrix test instead of silently falling through.
var fsmExpectations = map[broadcastTabState]map[tabEvent]expectation{
	inChatWindow: {
		eventCompose:       {allowed: true, to: insertMode},
		eventSend:          {allowed: false, to: inChatWindow},
		eventCancel:        {allowed: false, to: inChatWindow},
		eventInspect:       {allowed: true, to: userInspectMode},
		eventEmoteOverview: {allowed: true, to: emoteOverviewMode},
		eventUnbanRequests: {allowed: true, to: unbanRequestMode},
		eventLeaveInspect:  {allowed: false, to: inChatWindow},
	},
	insertMode: {
		eventCompose:       {allowed: false, to: insertMode},
		eventSend:          {allowed: true, to: inChatWindow},
		eventCancel:        {allowed: true, to: inChatWindow},
		eventInspect:       {allowed: true, to: userInspectMode},
		eventEmoteOverview: {allowed: true, to: emoteOverviewMode},
		eventUnbanRequests: {allowed: false, to: insertMode},
		eventLeaveInspect:  {allowed: false, to: insertMode},
	},
	userInspectMode: {
		eventCompose:       {allowed: true, to: userInspectInsertMode},
		eventSend:          {allowed: false, to: userInspectMode},
		eventCancel:        {allowed: true, to: inChatWindow},
		eventInspect:       {allowed: true, to: userInspectMode},
		eventEmoteOverview: {allowed: true, to: emoteOverviewMode},
		eventUnbanRequests: {allowed: false, to: userInspectMode},
		eventLeaveInspect:  {allowed: true, to: inChatWindow},
	},
	userInspectInsertMode: {
		eventCompose:       {allowed: false, to: userInspectInsertMode},
		eventSend:          {allowed: true, to: userInspectMode},
		eventCancel:        {allowed: true, to: userInspectMode},
		eventInspect:       {allowed: true, to: userInspectMode},
		eventEmoteOverview: {allowed: true, to: emoteOverviewMode},
		eventUnbanRequests: {allowed: false, to: userInspectInsertMode},
		eventLeaveInspect:  {allowed: true, to: inChatWindow},
	},
	emoteOverviewMode: {
		eventCompose:       {allowed: false, to: emoteOverviewMode},
		eventSend:          {allowed: false, to: emoteOverviewMode},
		eventCancel:        {allowed: true, to: inChatWindow},
		eventInspect:       {allowed: false, to: emoteOverviewMode},
		eventEmoteOverview: {allowed: false, to: emoteOverviewMode},
		eventUnbanRequests: {allowed: false, to: emoteOverviewMode},
		eventLeaveInspect:  {allowed: false, to: emoteOverviewMode},
	},
	unbanRequestMode: {
		eventCompose:       {allowed: false, to: unbanRequestMode},
		eventSend:          {allowed: false, to: unbanRequestMode},
		eventCancel:        {allowed: true, to: inChatWindow},
		eventInspect:       {allowed: false, to: unbanRequestMode},
		eventEmoteOverview: {allowed: false, to: unbanRequestMode},
		eventUnbanRequests: {allowed: false, to: unbanRequestMode},
		eventLeaveInspect:  {allowed: false, to: unbanRequestMode},
	},
}

// newTestTab builds a chat tab wired with the default test configuration.
func newTestTab(id string, width, height int, account save.Account, channel string) *Broadcast {
	return NewBroadcast(id, width, height, account, channel,
		save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap(),
		ClientCapabilities{}, nil, nil, nil, nil, nil, nil, nil, chatstate.NewCache())
}

func newFSMTestTab(t *testing.T, anonymous bool) *Broadcast {
	t.Helper()

	account := save.Account{ID: "acc", DisplayName: "me"}
	if anonymous {
		account = save.Account{ID: "anon", IsAnonymous: true}
	}

	tab := newTestTab("tab-1", 80, 24, account, "chan")
	tab.handleChannelDataLoaded(setChannelDataMessage{
		channelID:    "chan-id",
		channel:      "chan",
		channelLogin: "chan",
	})

	// Stand in for Root activating the tab.
	tab.Focus()

	return tab
}

// enterMode drives a fresh tab into the requested mode through legal transitions.
// It deliberately does not poke at state directly, so the setup exercises the same
// paths as the application.
func enterMode(t *testing.T, tab *Broadcast, mode broadcastTabState) {
	t.Helper()

	inspect := func() {
		tab.userInspect = newUserInspect(tab.id, tab.width, tab.height, "someone", tab.channelLogin, tab.account.ID,
			tab.settings, tab.theme, tab.keymap, tab.client.API, tab.messageLogger, tab.emoteReplacer, tab.badgeReplacer)
		_, _ = tab.transit(eventInspect)
	}

	switch mode {
	case inChatWindow:
		// already there
	case insertMode:
		_, _ = tab.transit(eventCompose)
	case userInspectMode:
		inspect()
	case userInspectInsertMode:
		inspect()
		_, _ = tab.transit(eventCompose)
	case emoteOverviewMode:
		_, _ = tab.transit(eventEmoteOverview)
	case unbanRequestMode:
		tab.isUserMod = true
		tab.client.Moderation = newFakeModerationClient()
		tab.unbanRequests = newUnbanRequestQueue(tab.id, tab.channelID, tab.account.ID, tab.client.Moderation, tab.width, tab.height, tab.theme, tab.keymap)
		_, _ = tab.transit(eventUnbanRequests)
	default:
		t.Fatalf("no setup path for mode %v", mode)
	}

	require.Equal(t, mode, tab.state, "test setup must reach the requested mode")
}

// assertTabInvariants checks the guarantees the mode machine is supposed to give.
func assertTabInvariants(t *testing.T, tab *Broadcast) {
	t.Helper()

	inputFocused := tab.input.Focused()
	chatFocused := tab.chatWindow.Focused()
	inspectFocused := tab.userInspect != nil && tab.userInspect.chatWindow.Focused()

	require.Equal(t, tab.state.isInspecting(), tab.userInspect != nil,
		"the inspect window exists exactly while inspecting")

	switch {
	case tab.state == emoteOverviewMode:
		require.False(t, inputFocused, "the composer must not keep the keyboard behind the emote overview")
		require.False(t, chatFocused, "the chat window must not keep the keyboard behind the emote overview")
	case tab.state == unbanRequestMode:
		require.False(t, inputFocused, "the composer must not keep the keyboard behind the unban queue")
		require.False(t, chatFocused, "the chat window must not keep the keyboard behind the unban queue")
	case tab.state.isComposing():
		require.True(t, inputFocused, "composing means the composer owns the keyboard")
		require.False(t, chatFocused)
	case tab.state.isInspecting():
		require.True(t, inspectFocused, "inspecting means the inspect window owns the keyboard")
		require.False(t, chatFocused)
	default:
		require.True(t, chatFocused, "viewing means the chat window owns the keyboard")
	}

	if tab.account.IsAnonymous {
		require.False(t, inputFocused, "anonymous accounts can never compose")
	}

	focused := 0
	for _, ok := range []bool{inputFocused, chatFocused, inspectFocused} {
		if ok {
			focused++
		}
	}

	require.LessOrEqual(t, focused, 1, "two widgets owned the keyboard at the same time")
}

func TestTabTransitions_Matrix(t *testing.T) {
	t.Parallel()

	for _, mode := range fsmModes {
		for _, event := range fsmEvents {
			t.Run(mode.String()+" / "+eventName(event), func(t *testing.T) {
				t.Parallel()

				tab := newFSMTestTab(t, false)
				enterMode(t, tab, mode)

				// Entering inspect mode needs a window to show; production builds it
				// from the selected message before calling transit.
				if event == eventInspect && fsmExpectations[mode][event].allowed && tab.userInspect == nil {
					tab.userInspect = newUserInspect(tab.id, tab.width, tab.height, "someone", tab.channelLogin, tab.account.ID,
						tab.settings, tab.theme, tab.keymap, tab.client.API, tab.messageLogger, tab.emoteReplacer, tab.badgeReplacer)
				}

				// The queue needs both a mod status and a moderation client, just like
				// production, so the opening transition is still exercised.
				if event == eventUnbanRequests && fsmExpectations[mode][event].allowed {
					tab.isUserMod = true
					tab.client.Moderation = newFakeModerationClient()
					tab.unbanRequests = newUnbanRequestQueue(tab.id, tab.channelID, tab.account.ID, tab.client.Moderation, tab.width, tab.height, tab.theme, tab.keymap)
				}

				applied, _ := tab.transit(event)
				want := fsmExpectations[mode][event]

				require.Equal(t, want.allowed, applied, "declared legality of the transition")
				require.Equal(t, want.to, tab.state, "resulting mode")

				assertTabInvariants(t, tab)
			})
		}
	}
}

func TestTabTransitions_AnonymousCannotCompose(t *testing.T) {
	t.Parallel()

	for _, event := range []tabEvent{eventCompose, eventEmoteOverview} {
		t.Run(eventName(event), func(t *testing.T) {
			t.Parallel()

			tab := newFSMTestTab(t, true)

			applied, _ := tab.transit(event)
			require.False(t, applied)
			require.Equal(t, inChatWindow, tab.state)
			assertTabInvariants(t, tab)
		})
	}
}

func TestTabTransitions_CancelKeepsTypedText(t *testing.T) {
	t.Parallel()

	tab := newFSMTestTab(t, false)
	enterMode(t, tab, insertMode)
	tab.input.SetValue("draft message")
	tab.input.ArmReply("msg-1", "someone", "hello")

	_, _ = tab.transit(eventCancel)

	require.Equal(t, inChatWindow, tab.state)
	require.Equal(t, "draft message", tab.input.Value(), "Escape drops focus, not the text")
	require.False(t, tab.input.ReplyArmed(), "Escape must disarm the reply")
}

func TestTabTransitions_SendClearsInput(t *testing.T) {
	t.Parallel()

	tab := newFSMTestTab(t, false)
	enterMode(t, tab, insertMode)
	tab.input.SetValue("sent message")
	tab.input.ArmReply("msg-1", "someone", "hello")

	_, _ = tab.transit(eventSend)

	require.Equal(t, inChatWindow, tab.state)
	require.Empty(t, tab.input.Value(), "sending clears the composer")
	require.False(t, tab.input.ReplyArmed())
}

func TestTabTransitions_CancelFromInspectComposerReturnsToInspect(t *testing.T) {
	t.Parallel()

	tab := newFSMTestTab(t, false)
	enterMode(t, tab, userInspectInsertMode)
	tab.input.SetValue("draft")

	inspect := tab.userInspect
	_, _ = tab.transit(eventCancel)

	require.Equal(t, userInspectMode, tab.state)
	require.Same(t, inspect, tab.userInspect, "cancelling the composer keeps the inspect window")
	assertTabInvariants(t, tab)
}

func TestTabTransitions_LeaveInspectDropsTheWindow(t *testing.T) {
	t.Parallel()

	for _, mode := range []broadcastTabState{userInspectMode, userInspectInsertMode} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()

			tab := newFSMTestTab(t, false)
			enterMode(t, tab, mode)

			_, _ = tab.transit(eventLeaveInspect)

			require.Equal(t, inChatWindow, tab.state)
			require.Nil(t, tab.userInspect)
			assertTabInvariants(t, tab)
		})
	}
}

func TestTabTransitions_UndeclaredEventLeavesModeUntouched(t *testing.T) {
	t.Parallel()

	tab := newFSMTestTab(t, false)
	enterMode(t, tab, emoteOverviewMode)

	applied, cmd := tab.transit(eventCompose)

	require.False(t, applied)
	require.Nil(t, cmd)
	require.Equal(t, emoteOverviewMode, tab.state)
}

// eventName keeps the matrix subtests readable.
func eventName(event tabEvent) string {
	switch event {
	case eventCompose:
		return "compose"
	case eventSend:
		return "send"
	case eventCancel:
		return "cancel"
	case eventInspect:
		return "inspect"
	case eventEmoteOverview:
		return "emote-overview"
	case eventUnbanRequests:
		return "unban-requests"
	case eventLeaveInspect:
		return "leave-inspect"
	}

	return "unknown"
}

// The table refuses to switch into inspect mode without a window: a caller that
// forgot to build one would otherwise leave the tab rendering a nil window.
func TestTabTransitions_InspectRequiresWindow(t *testing.T) {
	t.Parallel()

	tab := newFSMTestTab(t, false)
	require.Nil(t, tab.userInspect)

	applied, _ := tab.transit(eventInspect)

	require.False(t, applied)
	require.Equal(t, inChatWindow, tab.state)
	assertTabInvariants(t, tab)
}

// The emote overview owns the whole tab, so the inspect window and the composer
// must not survive behind it.
func TestTabTransitions_EmoteOverviewDropsInspectWindow(t *testing.T) {
	t.Parallel()

	tab := newFSMTestTab(t, false)
	enterMode(t, tab, userInspectInsertMode)
	tab.input.SetValue("draft")
	require.NotNil(t, tab.userInspect)

	_, _ = tab.transit(eventEmoteOverview)

	require.Equal(t, emoteOverviewMode, tab.state)
	require.Nil(t, tab.userInspect, "the inspect window is closed by the overview")
	require.Equal(t, "draft", tab.input.Value(), "the draft is kept: opening the overview is not a discard")
	require.False(t, tab.input.ReplyArmed(), "the reply target does not survive the mode change")
	assertTabInvariants(t, tab)
}

// Layout and rendering.

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
				prediction: layoutSpan{top: 1, height: 0},
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
				prediction: layoutSpan{top: 2, height: 0},
				poll:       layoutSpan{top: 2, height: 5},
				chat:       layoutSpan{top: 7, height: 8},
				composer:   layoutSpan{top: 15, height: 4},
				status:     layoutSpan{top: 19, height: 1},
			},
		},
		{
			name: "stream info, prediction and poll",
			in:   layoutInputs{height: 24, streamInfo: 2, prediction: 5, poll: 4, composer: 4, status: 1},
			want: broadcastLayout{
				height:     24,
				streamInfo: layoutSpan{top: 0, height: 2},
				prediction: layoutSpan{top: 2, height: 5},
				poll:       layoutSpan{top: 7, height: 4},
				chat:       layoutSpan{top: 11, height: 8},
				composer:   layoutSpan{top: 19, height: 4},
				status:     layoutSpan{top: 23, height: 1},
			},
		},
		{
			name: "anonymous account has no composer",
			in:   layoutInputs{height: 20, streamInfo: 1, status: 1},
			want: broadcastLayout{
				height:     20,
				streamInfo: layoutSpan{top: 0, height: 1},
				prediction: layoutSpan{top: 1, height: 0},
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
				prediction: layoutSpan{top: 1, height: 0},
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
				prediction:  layoutSpan{top: 1, height: 0},
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
			name: "unban requests replace the content area",
			in:   layoutInputs{height: 20, streamInfo: 1, composer: 4, status: 1, unbanRequests: true},
			want: broadcastLayout{
				height:        20,
				unbanRequests: layoutSpan{top: 0, height: 19},
				status:        layoutSpan{top: 19, height: 1},
			},
		},
		{
			name: "short terminal clamps to zero instead of going negative",
			in:   layoutInputs{height: 3, streamInfo: 1, poll: 4, composer: 4, status: 1},
			want: broadcastLayout{
				height:     3,
				streamInfo: layoutSpan{top: 0, height: 1},
				prediction: layoutSpan{top: 1, height: 0},
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

			for _, span := range []layoutSpan{got.streamInfo, got.prediction, got.poll, got.chat, got.userInspect, got.composer, got.status, got.emoteOverview, got.unbanRequests} {
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
		{height: 24, streamInfo: 2, prediction: 5, poll: 4, composer: 4, status: 1},
		{height: 24, streamInfo: 2, poll: 4, composer: 4, status: 1, inspecting: true},
		{height: 30, streamInfo: 1, composer: 6, status: 1, inspecting: true},
		{height: 18, streamInfo: 1, composer: 4, status: 0},
	}

	for i, in := range inputs {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()

			l := allocateBroadcastLayout(in)

			regions := []layoutSpan{l.streamInfo, l.prediction, l.poll, l.chat, l.userInspect, l.composer, l.status}
			if in.emoteOverview {
				regions = []layoutSpan{l.emoteOverview, l.status}
			} else if in.unbanRequests {
				regions = []layoutSpan{l.unbanRequests, l.status}
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

// addTestMessages fills the main chat window with one-row messages.
func addTestMessages(t *testing.T, c *chatWindow, count int) {
	t.Helper()

	for i := 0; i < count; i++ {
		c.handleMessage(chat.ChatEventMessage{Message: &twitchirc.PrivateMessage{
			ID:          fmt.Sprintf("test-%d", i),
			DisplayName: "Alice",
			LoginName:   "alice",
			Message:     fmt.Sprintf("line %d", i),
			TMISentTS:   time.Now(),
		}})
	}

	c.moveToBottom()
}

func selectedCursor(t *testing.T, c *chatWindow) int {
	t.Helper()

	entry, ok := c.Selected()
	require.True(t, ok, "expected a selected entry")

	return c.StartOf(&entry)
}

func addPrivateMessage(t *testing.T, c *chatWindow, id, content string) *chat.Entry {
	t.Helper()

	c.handleMessage(chat.ChatEventMessage{Message: &twitchirc.PrivateMessage{
		ID:          id,
		DisplayName: "Alice",
		LoginName:   "alice",
		Message:     content,
		TMISentTS:   time.Now(),
	}})

	entries := c.Entries()
	require.NotEmpty(t, entries)

	return &entries[len(entries)-1]
}

func entryStart(t *testing.T, c *chatWindow, entry *chat.Entry) int {
	t.Helper()

	return c.StartOf(entry)
}

func entryEnd(t *testing.T, c *chatWindow, entry *chat.Entry) int {
	t.Helper()

	return c.EndOf(entry)
}

func plain(lines []string) string {
	return StripANSI(strings.Join(lines, "\n"))
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

	t.Run("prediction and poll", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, account)
		tab.prediction.enabled = true
		tab.prediction.title = "who wins?"
		tab.prediction.outcomes = []predictionOutcome{{id: "1", title: "yes", points: 1}}
		tab.poll.enabled = true
		tab.poll.title = "best map?"
		tab.poll.items = []pollItem{{id: "1", title: "dust2", votes: 1}}
		tab.HandleResize()

		require.True(t, tab.layout.prediction.visible())
		require.True(t, tab.layout.poll.visible())
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

	t.Run("unban requests", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, account)
		tab.state = unbanRequestMode
		tab.unbanRequests = newUnbanRequestQueue(tab.id, tab.channelID, tab.account.ID, newFakeModerationClient(), tab.width, tab.height, tab.theme, tab.keymap)
		tab.HandleResize()

		require.True(t, tab.layout.unbanRequests.visible())
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

// Mouse interaction.

func leftClick(y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{Button: tea.MouseLeft, X: 1, Y: y}
}

func wheelUp(y int) tea.MouseWheelMsg {
	return tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: 1, Y: y}
}

// Test_Broadcast_MouseClickChatReachesTheMessageUnderTheCursor verifies that every
// row of the chat region maps to the message rendered there, after the resize that
// established the layout.
func Test_Broadcast_MouseClickChatReachesTheMessageUnderTheCursor(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
	addTestMessages(t, tab.chatWindow, 40)
	tab.HandleResize()

	require.True(t, tab.layout.chat.visible())

	for row := 0; row < tab.layout.chat.height; row++ {
		want := tab.chatWindow.viewportRenderStart() + row

		_ = tab.HandleMouseClick(leftClick(tab.layout.chat.top+row), 0, tab.layout.chat.top+row)

		require.Equal(t, want, selectedCursor(t, tab.chatWindow), "row %d", row)
		require.Equal(t, inChatWindow, tab.state)
	}
}

// Test_Broadcast_MouseClickNewMessageBadgeJumpsToTheNewestMessage verifies that the
// badge behaves like a control: clicking it catches up with the chat instead of
// selecting the message underneath it.
func Test_Broadcast_MouseClickNewMessageBadgeJumpsToTheNewestMessage(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})

	for i := range 40 {
		addPrivateMessage(t, tab.chatWindow, fmt.Sprintf("msg-%d", i), fmt.Sprintf("message %d", i))
	}

	tab.HandleResize()

	for range 3 {
		tab.chatWindow.pageUp()
	}

	addPrivateMessage(t, tab.chatWindow, "missed", "missed while scrolled up")

	_, badgeWidth := tab.chatWindow.newMessageBadge()

	row, col, ok := tab.chatWindow.newMessageBadgeSpan(tab.chatWindow.viewportRenderStart(), tab.chatWindow.height, badgeWidth)
	require.True(t, ok, "the badge is visible")

	newest := tab.chatWindow.getNewestEntry()
	require.NotNil(t, newest)
	require.False(t, newest.Selected())

	y := tab.layout.chat.top + row
	cmd := tab.HandleMouseClick(tea.MouseClickMsg{Button: tea.MouseLeft, X: col, Y: y}, col, y)

	require.Nil(t, cmd)
	require.True(t, newest.Selected(), "the click caught up with the chat")
	require.True(t, tab.chatWindow.caughtUp())
	require.Zero(t, tab.chatWindow.NewMessages())
}

// Test_Broadcast_MouseClickUserInspectNewMessageBadge pins that the inspect chat
// behaves like the main chat: the badge catches up with the history instead of
// selecting the message underneath it.
func Test_Broadcast_MouseClickUserInspectNewMessageBadge(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
	addTestMessages(t, tab.chatWindow, 5)
	tab.userInspect = newUserInspect(tab.id, tab.width, tab.height, "someone", tab.channelLogin, tab.account.ID,
		tab.settings, tab.theme, tab.keymap, tab.client.API, tab.messageLogger, tab.emoteReplacer, tab.badgeReplacer)

	tab.state = userInspectMode
	tab.HandleResize()

	history := tab.userInspect.chatWindow

	for i := range 40 {
		addPrivateMessage(t, history, fmt.Sprintf("msg-%d", i), fmt.Sprintf("message %d", i))
	}

	tab.HandleResize()

	for range 3 {
		history.pageUp()
	}

	addPrivateMessage(t, history, "missed", "missed while scrolled up")

	_, badgeWidth := history.newMessageBadge()

	row, col, ok := history.newMessageBadgeSpan(history.viewportRenderStart(), history.height, badgeWidth)
	require.True(t, ok, "the inspect history shows the badge")

	newest := history.getNewestEntry()
	require.NotNil(t, newest)
	require.False(t, newest.Selected())

	y := tab.inspectChatSpan().top + row
	cmd := tab.HandleMouseClick(tea.MouseClickMsg{Button: tea.MouseLeft, X: col, Y: y}, col, y)

	require.Nil(t, cmd)
	require.True(t, newest.Selected(), "the click caught up with the inspect history")
	require.Zero(t, history.NewMessages())
	require.Equal(t, userInspectMode, tab.state, "catching up must not leave inspect mode")
}

// addReplyToChat appends a reply to Alice's most recent message and returns its entry.
func addReplyToChat(t *testing.T, c *chatWindow, parent *chat.Entry, id, body string) *chat.Entry {
	t.Helper()

	parentMsg := parent.Event.Message.(*twitchirc.PrivateMessage)

	c.handleMessage(chat.ChatEventMessage{Message: &twitchirc.PrivateMessage{
		ID:                id,
		DisplayName:       "carol",
		LoginName:         "carol",
		Color:             "#a3be8c",
		Message:           body,
		TMISentTS:         time.Now(),
		ParentMsgID:       parentMsg.ID,
		ParentUserLogin:   parentMsg.LoginName,
		ParentDisplayName: parentMsg.DisplayName,
		ParentMsgBody:     parentMsg.Message,
	}})

	entries := c.Entries()
	require.NotEmpty(t, entries)

	return &entries[len(entries)-1]
}

// Test_Broadcast_MouseClickReplyReferenceJumpsToTheParent verifies that clicking a reply's
// reference row selects the message it answers, while the reply's own rows still select
// the reply.
func Test_Broadcast_MouseClickReplyReferenceJumpsToTheParent(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})

	parent := addPrivateMessage(t, tab.chatWindow, "parent-1", "that was a great play")
	reply := addReplyToChat(t, tab.chatWindow, parent, "reply-1", "@Alice no it wasn't")

	tab.HandleResize()

	referenceRow := tab.layout.chat.top + entryStart(t, tab.chatWindow, reply) - tab.chatWindow.viewportRenderStart()
	_ = tab.HandleMouseClick(leftClick(referenceRow), tab.chatWindow.TimePaddingWidth(), referenceRow)

	require.Equal(t, entryStart(t, tab.chatWindow, parent), selectedCursor(t, tab.chatWindow), "the reference jumped to the parent")

	// The reply's message row still selects the reply itself.
	messageRow := tab.layout.chat.top + entryEnd(t, tab.chatWindow, reply) - tab.chatWindow.viewportRenderStart()
	_ = tab.HandleMouseClick(leftClick(messageRow), tab.chatWindow.TimePaddingWidth()+2, messageRow)

	require.Equal(t, entryStart(t, tab.chatWindow, reply), selectedCursor(t, tab.chatWindow))
}

// Test_Broadcast_MouseClickUserInspectReplyReference pins that the inspect history gets
// the same reply behaviour as the main chat.
func Test_Broadcast_MouseClickUserInspectReplyReference(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
	addTestMessages(t, tab.chatWindow, 5)
	tab.userInspect = newUserInspect(tab.id, tab.width, tab.height, "someone", tab.channelLogin, tab.account.ID,
		tab.settings, tab.theme, tab.keymap, tab.client.API, tab.messageLogger, tab.emoteReplacer, tab.badgeReplacer)

	tab.state = userInspectMode
	tab.HandleResize()

	history := tab.userInspect.chatWindow

	parent := addPrivateMessage(t, history, "parent-1", "that was a great play")
	reply := addReplyToChat(t, history, parent, "reply-1", "@Alice no it wasn't")

	tab.HandleResize()

	chat := tab.inspectChatSpan()
	require.True(t, chat.visible())

	y := chat.top + entryStart(t, tab.chatWindow, reply) - history.viewportRenderStart()
	_ = tab.HandleMouseClick(tea.MouseClickMsg{Button: tea.MouseLeft, X: history.TimePaddingWidth(), Y: y}, history.TimePaddingWidth(), y)

	require.Equal(t, entryStart(t, tab.chatWindow, parent), selectedCursor(t, history))
	require.Equal(t, userInspectMode, tab.state, "jumping to a parent must not leave inspect mode")
}

// Test_Broadcast_MouseClickComposer checks the composer hit box, including after
// the composer grows and the chat window shrinks.
func Test_Broadcast_MouseClickComposer(t *testing.T) {
	t.Parallel()

	account := save.Account{ID: "acc", DisplayName: "me"}

	t.Run("composer top row starts insert mode", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, account)

		y := tab.layout.composer.top
		_ = tab.HandleMouseClick(leftClick(y), 0, y)

		require.Equal(t, insertMode, tab.state)
	})

	t.Run("status bar does not start insert mode", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, account)

		y := tab.layout.status.top
		require.True(t, tab.layout.status.visible())
		_ = tab.HandleMouseClick(leftClick(y), 0, y)

		require.Equal(t, inChatWindow, tab.state, "the status bar is not part of the composer")
	})

	t.Run("hit box follows a wrapped composer", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 40, 24, account)
		addTestMessages(t, tab.chatWindow, 40)
		oldHeight := tab.layout.composer.height

		tab.input.SetValue(strings.Repeat("word ", 60))
		tab.HandleResize()

		require.Greater(t, tab.layout.composer.height, oldHeight, "the composer grew")
		require.Less(t, tab.layout.composer.top, tab.height-tab.layout.composer.height, "the taller composer starts higher")

		// The row just above the composer belongs to the chat window.
		above := tab.layout.composer.top - 1
		require.True(t, tab.layout.chat.contains(above))

		_ = tab.HandleMouseClick(leftClick(above), 0, above)
		require.Equal(t, inChatWindow, tab.state)

		_ = tab.HandleMouseClick(leftClick(tab.layout.composer.top), 0, tab.layout.composer.top)
		require.Equal(t, insertMode, tab.state)
	})

	t.Run("anonymous account has no composer", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "anon", IsAnonymous: true})
		addTestMessages(t, tab.chatWindow, 40)
		tab.HandleResize()

		// The bottom row is the status bar, not a composer.
		y := tab.height - 1
		require.True(t, tab.layout.status.contains(y))
		_ = tab.HandleMouseClick(leftClick(y), 0, y)

		require.Equal(t, inChatWindow, tab.state)
	})
}

// Test_Broadcast_MouseClickUserInspect covers both inspect regions: the inspect
// window scrolls its own history and the main chat still selects messages.
func Test_Broadcast_MouseClickUserInspect(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
	addTestMessages(t, tab.chatWindow, 40)
	tab.userInspect = newUserInspect(tab.id, tab.width, tab.height, "someone", tab.channelLogin, tab.account.ID,
		tab.settings, tab.theme, tab.keymap, tab.client.API, tab.messageLogger, tab.emoteReplacer, tab.badgeReplacer)

	tab.state = userInspectMode
	tab.HandleResize()
	addTestMessages(t, tab.userInspect.chatWindow, 20)
	tab.HandleResize()

	require.True(t, tab.layout.userInspect.visible())

	// Only the history below the info banner is clickable.
	chat := tab.inspectChatSpan()
	require.True(t, chat.visible())
	require.Greater(t, chat.top, tab.layout.userInspect.top, "the info banner sits above the history")

	row := chat.height - 1
	want := tab.userInspect.chatWindow.viewportRenderStart() + row
	_ = tab.HandleMouseClick(leftClick(chat.top+row), 0, chat.top+row)

	require.Equal(t, want, selectedCursor(t, tab.userInspect.chatWindow))
	require.Equal(t, userInspectMode, tab.state)

	// A click on the info banner does not select history.
	before := selectedCursor(t, tab.userInspect.chatWindow)
	_ = tab.HandleMouseClick(leftClick(tab.layout.userInspect.top), 0, tab.layout.userInspect.top)
	require.Equal(t, before, selectedCursor(t, tab.userInspect.chatWindow))

	// Clicking the main chat leaves inspect mode and selects there.
	_ = tab.HandleMouseClick(leftClick(tab.layout.chat.top), 0, tab.layout.chat.top)
	require.Equal(t, inChatWindow, tab.state)
}

// Test_Broadcast_MouseWheel verifies that the wheel scrolls the region under the
// cursor and ignores the composer.
func Test_Broadcast_MouseWheel(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
	addTestMessages(t, tab.chatWindow, 40)
	tab.HandleResize()

	start := tab.chatWindow.CursorLine()
	_ = tab.HandleMouseWheel(wheelUp(tab.layout.chat.top+1), 0, tab.layout.chat.top+1)
	require.Less(t, tab.chatWindow.CursorLine(), start, "the wheel over the chat scrolls it")

	scrolled := tab.chatWindow.CursorLine()
	_ = tab.HandleMouseWheel(wheelUp(tab.layout.composer.top), 0, tab.layout.composer.top)
	require.Equal(t, scrolled, tab.chatWindow.CursorLine(), "the composer does not scroll the chat")

	// The status bar is not a scroll target either.
	_ = tab.HandleMouseWheel(wheelUp(tab.layout.status.top), 0, tab.layout.status.top)
	require.Equal(t, scrolled, tab.chatWindow.CursorLine())
}

// Test_Broadcast_MouseWheelUserInspect scrolls the inspect history when the cursor
// is over it.
func Test_Broadcast_MouseWheelUserInspect(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
	addTestMessages(t, tab.chatWindow, 40)
	tab.userInspect = newUserInspect(tab.id, tab.width, tab.height, "someone", tab.channelLogin, tab.account.ID,
		tab.settings, tab.theme, tab.keymap, tab.client.API, tab.messageLogger, tab.emoteReplacer, tab.badgeReplacer)
	tab.state = userInspectMode
	tab.HandleResize()
	addTestMessages(t, tab.userInspect.chatWindow, 20)
	tab.HandleResize()

	chat := tab.inspectChatSpan()
	start := tab.userInspect.chatWindow.CursorLine()
	_ = tab.HandleMouseWheel(wheelUp(chat.top), 0, chat.top)

	require.Less(t, tab.userInspect.chatWindow.CursorLine(), start)
}

// Test_Broadcast_MouseInVerticalMode verifies that the layout recorded for
// vertical tab mode matches what Root renders: the status bar lives outside the
// tab, and the chat gets the extra rows.
func Test_Broadcast_MouseInVerticalMode(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 70, 24, save.Account{ID: "acc", DisplayName: "me"})
	tab.SetFullWidth(80)
	tab.HandleResize()
	addTestMessages(t, tab.chatWindow, 40)
	tab.HandleResize()

	require.False(t, tab.layout.status.visible())
	require.Equal(t, tab.height, lipgloss.Height(tab.ViewWithoutStatusBar()))

	// The row above the composer is a chat row, not the status bar.
	last := tab.layout.composer.top - 1
	require.True(t, tab.layout.chat.contains(last))

	want := tab.chatWindow.viewportRenderStart() + (last - tab.layout.chat.top)
	_ = tab.HandleMouseClick(leftClick(last), 0, last)
	require.Equal(t, want, selectedCursor(t, tab.chatWindow))

	_ = tab.HandleMouseClick(leftClick(tab.layout.composer.top), 0, tab.layout.composer.top)
	require.Equal(t, insertMode, tab.state)
}

// Test_Broadcast_MouseClickEmoteOverview ignores clicks in the overview.
func Test_Broadcast_MouseClickEmoteOverview(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
	tab.state = emoteOverviewMode
	tab.emoteOverview = NewEmoteOverview(tab.channelID, tab.emoteCache, tab.emoteReplacer, 1, tab.width, tab.height)
	tab.HandleResize()

	require.NotPanics(t, func() {
		_ = tab.HandleMouseClick(leftClick(1), 0, 1)
		_ = tab.HandleMouseWheel(wheelUp(1), 0, 1)
	})
}

// Test_Broadcast_MouseUnloadedTab tolerates events before the children exist.
func Test_Broadcast_MouseUnloadedTab(t *testing.T) {
	t.Parallel()

	tab := newTestTab("tab-1", 80, 24, save.Account{ID: "acc", DisplayName: "me"}, "chan")

	require.NotPanics(t, func() {
		_ = tab.HandleMouseClick(leftClick(1), 0, 1)
		_ = tab.HandleMouseWheel(wheelUp(1), 0, 1)
	})
}

// Test_Broadcast_MouseRowDiagnostics prints nothing but guards the invariant that
// regions never overlap: a row belongs to at most one interactive region.
func Test_Broadcast_MouseRegionsDoNotOverlap(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
	tab.userInspect = newUserInspect(tab.id, tab.width, tab.height, "someone", tab.channelLogin, tab.account.ID,
		tab.settings, tab.theme, tab.keymap, tab.client.API, tab.messageLogger, tab.emoteReplacer, tab.badgeReplacer)
	tab.state = userInspectMode
	tab.HandleResize()

	regions := []struct {
		name string
		span layoutSpan
	}{
		{"chat", tab.layout.chat},
		{"user inspect", tab.layout.userInspect},
		{"composer", tab.layout.composer},
	}

	for y := 0; y < tab.height; y++ {
		hits := make([]string, 0, len(regions))

		for _, region := range regions {
			if region.span.contains(y) {
				hits = append(hits, region.name)
			}
		}

		require.LessOrEqual(t, len(hits), 1, fmt.Sprintf("row %d belongs to %v", y, hits))
	}
}

// Command parsing and duplicate handling.

func Test_parseSlashCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantName string
		wantArgs []string
		wantOK   bool
	}{
		{name: "not a command", input: "hello world", wantOK: false},
		{name: "empty input", input: "", wantOK: false},
		{name: "command without args keeps historical empty arg", input: "/emotes", wantName: "emotes", wantArgs: []string{""}, wantOK: true},
		{name: "command with one arg", input: "/join sodapoppin", wantName: "join", wantArgs: []string{"sodapoppin"}, wantOK: true},
		{name: "command with multiple args", input: "/pyramid word 3", wantName: "pyramid", wantArgs: []string{"word", "3"}, wantOK: true},
		{name: "bare slash", input: "/", wantName: "", wantArgs: []string{""}, wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			name, args, ok := parseSlashCommand(tt.input)

			require.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				return
			}

			require.Equal(t, tt.wantName, name)
			require.Equal(t, tt.wantArgs, args)
		})
	}
}

// TestBroadcast_ModeratorCommandsRequireMod extends the existing permission gate to
// the newly added chat-setting, role, warning, raid, shoutout and AutoMod commands.
func TestBroadcast_ModeratorCommandsRequireMod(t *testing.T) {
	t.Parallel()

	for _, command := range []struct {
		name string
		args []string
	}{
		{"slow", []string{"10"}},
		{"slowoff", []string{""}},
		{"followers", []string{"10m"}},
		{"subscribers", []string{""}},
		{"emoteonly", []string{""}},
		{"uniquechat", []string{""}},
		{"clear", []string{""}},
		{"mod", []string{"alice"}},
		{"unmod", []string{"alice"}},
		{"mods", []string{""}},
		{"vip", []string{"alice"}},
		{"unvip", []string{"alice"}},
		{"vips", []string{""}},
		{"warn", []string{"alice", "spam"}},
		{"raid", []string{"other"}},
		{"unraid", []string{""}},
		{"shoutout", []string{"other"}},
		{"approve", []string{"msg-1"}},
		{"deny", []string{"msg-1"}},
		{"snooze", []string{""}},
	} {
		t.Run(command.name, func(t *testing.T) {
			t.Parallel()

			tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
			tab.isUserMod = false

			cmd := tab.dispatchSlashCommand(command.name, command.args)
			require.NotNil(t, cmd)

			msg, ok := cmd().(chat.ChatEventMessage)
			require.True(t, ok)
			notice, ok := msg.Message.(*twitchirc.Notice)
			require.True(t, ok)
			require.Contains(t, notice.Message, "not a moderator")
		})
	}
}

func Test_applyDuplicateBypass(t *testing.T) {
	t.Parallel()

	require.Equal(t, "hi", applyDuplicateBypass("hi", "different"), "non-duplicate stays unchanged")
	require.Equal(t, "hi "+string(duplicateBypass), applyDuplicateBypass("hi", "hi"), "duplicate gets the bypass character")
	require.Equal(t, "Hi "+string(duplicateBypass), applyDuplicateBypass("Hi", "hI"), "duplicate check is case-insensitive")
	require.Equal(t, ""+" "+string(duplicateBypass), applyDuplicateBypass("", ""), "empty input equals empty last message")
}

func Test_isNearDuplicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message string
		recent  []string
		want    bool
	}{
		{name: "no recent messages", message: "hello there", recent: nil, want: false},
		{name: "single word seen verbatim", message: "Kappa", recent: []string{"kappa"}, want: true},
		{name: "single word not seen", message: "Kappa", recent: []string{"PogChamp"}, want: false},
		{name: "single word never fuzzy matches", message: "Kappas", recent: []string{"Kappa"}, want: false},
		{name: "multi word exact repeat", message: "nice play man", recent: []string{"nice play man"}, want: true},
		{name: "multi word near repeat within distance", message: "nice play man", recent: []string{"nice play men"}, want: true},
		{name: "multi word distinct", message: "nice play man", recent: []string{"completely different message"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, isNearDuplicate(tt.message, tt.recent))
		})
	}
}

// Emote suggestions.

func Test_emoteSuggestions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		userEmotes   []emote.Emote
		channelSet   emote.EmoteSet
		want         []string
		wantContains []string
	}{
		{
			name: "user emotes are always included",
			userEmotes: []emote.Emote{
				{Text: "Kappa", Platform: emote.Twitch},
				{Text: "mySubEmote", Platform: emote.Twitch},
			},
			want: []string{"Kappa", "mySubEmote"},
		},
		{
			name: "channel twitch emotes are excluded",
			channelSet: emote.EmoteSet{
				{Text: "channelSubEmote", Platform: emote.Twitch},
				{Text: "catJAM", Platform: emote.BTTV},
				{Text: "modCheck", Platform: emote.SevenTV},
				{Text: "monkaS", Platform: emote.FFZ},
			},
			want: []string{"catJAM", "modCheck", "monkaS"},
		},
		{
			name:       "duplicates collapse",
			userEmotes: []emote.Emote{{Text: "catJAM", Platform: emote.Twitch}},
			channelSet: emote.EmoteSet{{Text: "catJAM", Platform: emote.BTTV}},
			want:       []string{"catJAM"},
		},
		{
			name: "empty sets yield no suggestions",
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := emoteSuggestions(tt.userEmotes, tt.channelSet)
			require.ElementsMatch(t, tt.want, got)
		})
	}
}

// Reply lifecycle and sending.

// fakeUserClient records the send requests a tab makes and satisfies both the
// APIClient surface used by the dependency container and the user authenticated
// surface used for sending chat messages.
type fakeUserClient struct {
	sentRequests []twitchapi.SendChatMessageRequest
}

func (f *fakeUserClient) GetUsers(ctx context.Context, logins []string, ids []string) (twitchapi.UserResponse, error) {
	return twitchapi.UserResponse{}, nil
}

func (f *fakeUserClient) GetStreamInfo(ctx context.Context, broadcastID []string) (twitchapi.GetStreamsResponse, error) {
	return twitchapi.GetStreamsResponse{}, nil
}

func (f *fakeUserClient) GetChatSettings(ctx context.Context, broadcasterID string, moderatorID string) (twitchapi.GetChatSettingsResponse, error) {
	return twitchapi.GetChatSettingsResponse{}, nil
}

func (f *fakeUserClient) CreateClip(ctx context.Context, broadcastID string, hasDelay bool) (twitchapi.CreatedClip, error) {
	return twitchapi.CreatedClip{}, nil
}

func (f *fakeUserClient) GetUserChatColor(ctx context.Context, userIDs []string) ([]twitchapi.UserChatColor, error) {
	return nil, nil
}

func (f *fakeUserClient) SendChatMessage(ctx context.Context, data twitchapi.SendChatMessageRequest) (twitchapi.SendChatMessageResponse, error) {
	f.sentRequests = append(f.sentRequests, data)

	return twitchapi.SendChatMessageResponse{}, nil
}

// newReplyTab builds the smallest tab that can arm and send a reply.
func newReplyTab(t *testing.T) (*Broadcast, *fakeUserClient) {
	t.Helper()

	client := &fakeUserClient{}

	settings := save.BuildDefaultSettings()
	theme := save.BuildDefaultTheme()
	keymap := save.BuildDefaultKeyMap()

	tab := &Broadcast{
		id:      "tab",
		account: save.Account{ID: "account", DisplayName: "tester"},
		state:   insertMode,
		width:   80,
		height:  24,

		settings: settings,
		theme:    theme,
		keymap:   keymap,
		client:   ClientCapabilities{API: client, Chat: client},
	}

	tab.chatWindow = newChatWindow(tab.width, tab.height, settings, theme, keymap)
	tab.input = newComposer(tab.chatWindow.Palette(), settings.BuildCustomSuggestionMap(), theme, nil)
	tab.input.SetWidth(tab.width)

	return tab, client
}

// newLoadedReplyTab additionally marks the tab as having channel data, so that
// HandleResize performs the real layout calculation instead of bailing out.
func newLoadedReplyTab(t *testing.T) (*Broadcast, *fakeUserClient) {
	t.Helper()

	tab, client := newReplyTab(t)
	tab.channel = "channel"
	tab.channelID = "channel-id"
	tab.channelLogin = "channel"
	tab.channelDataLoaded = true
	tab.statusInfo = newStreamStatus(tab.width, tab.height, tab, tab.account.ID, tab.channelID, tab.theme, tab.client.API, tab.chatSettings)
	tab.streamInfo = newStreamInfo(tab.channelID, client, tab.width)
	tab.prediction = newPrediction(tab.width)
	tab.poll = newPoll(tab.width)
	tab.HandleResize()

	return tab, client
}

// layoutRows is the number of terminal rows the message input and the chat window
// take together. It has to stay constant while the reply indicator appears and
// disappears, otherwise the status bar is pushed away from the bottom edge.
func layoutRows(tab *Broadcast) int {
	return tab.chatWindow.height + lipgloss.Height(tab.renderMessageInput())
}

func Test_composer_SubmitConsumesReply(t *testing.T) {
	t.Parallel()

	tab, _ := newReplyTab(t)

	require.Empty(t, tab.input.Submit().ReplyTo, "no armed reply yields no message ID")

	tab.input.ArmReply("msg-1", "other", "hi")
	require.Equal(t, "msg-1", tab.input.Submit().ReplyTo)
	require.False(t, tab.input.ReplyArmed(), "taking disarms the reply")
	require.Empty(t, tab.input.Submit().ReplyTo, "a reply target is only used once")
}

func Test_broadcastTab_handleMessageSent_sendsArmedReply(t *testing.T) {
	t.Parallel()

	tab, client := newReplyTab(t)
	tab.input.ArmReply("msg-1", "otheruser", "nice play")
	tab.input.SetValue("thanks")

	cmd := tab.handleMessageSent(false)

	require.False(t, tab.input.ReplyArmed(), "sending consumes the armed reply")
	require.NotNil(t, cmd)
	require.Nil(t, cmd(), "a successful send produces no notice")

	require.Len(t, client.sentRequests, 1)
	require.Equal(t, "msg-1", client.sentRequests[0].ReplyMessageID)
	require.Equal(t, "thanks", client.sentRequests[0].Message)
}

func Test_broadcastTab_handleMessageSent_commandDoesNotReuseReply(t *testing.T) {
	t.Parallel()

	tab, client := newReplyTab(t)
	tab.input.ArmReply("msg-1", "otheruser", "nice play")
	tab.input.SetValue("/emotes")

	require.NotNil(t, tab.handleMessageSent(false), "the emote overview command is dispatched")

	require.False(t, tab.input.ReplyArmed(), "using a command disarms the reply")

	tab.input.SetValue("not a reply anymore")

	secondSend := tab.handleMessageSent(false)

	require.NotNil(t, secondSend)
	require.Nil(t, secondSend(), "a successful send produces no notice")

	require.Len(t, client.sentRequests, 1)
	require.Empty(t, client.sentRequests[0].ReplyMessageID, "the next message must not be a reply")
}

func Test_broadcastTab_handleMessageSent_quickSendConsumesReply(t *testing.T) {
	t.Parallel()

	tab, client := newReplyTab(t)
	tab.input.ArmReply("msg-1", "otheruser", "nice play")
	tab.input.SetValue("first")

	firstSend := tab.handleMessageSent(true)

	require.NotNil(t, firstSend)
	require.Nil(t, firstSend(), "a successful send produces no notice")

	tab.input.SetValue("second")

	secondSend := tab.handleMessageSent(true)

	require.NotNil(t, secondSend)
	require.Nil(t, secondSend(), "a successful send produces no notice")

	require.Len(t, client.sentRequests, 2)
	require.Equal(t, "msg-1", client.sentRequests[0].ReplyMessageID)
	require.Empty(t, client.sentRequests[1].ReplyMessageID)
}

func TestBroadcast_PaneResizeAndRefocusPreserveEveryMode(t *testing.T) {
	for _, mode := range fsmModes {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			model := newFSMTestTab(t, false)
			t.Cleanup(model.Close)
			enterMode(t, model, mode)
			if mode == emoteOverviewMode {
				model.emoteOverview = NewEmoteOverview(model.channelID, nil, nil, 1, 80, 24)
			}
			model.input.SetValue("a retained draft 界")
			for _, size := range [][2]int{{0, 0}, {0, 24}, {24, 0}, {1, 1}, {24, 8}, {80, 24}} {
				require.NotPanics(t, func() {
					model.SetSize(size[0], size[1])
					model.SetFullWidth(size[0])
					model.HandleResize()
					model.ViewWithoutStatusBar()
				})
			}
			model.Blur()
			model.Focus()
			require.Equal(t, mode, model.state)
			require.Equal(t, "a retained draft 界", model.input.Value())
			assertTabInvariants(t, model)
		})
	}
}

func TestBroadcast_PaneFocusPreservesDraftReplyAndMode(t *testing.T) {
	t.Parallel()
	model, _ := newLoadedReplyTab(t)
	model.state = insertMode
	model.input.SetValue("unfinished reply")
	model.input.ArmReply("message-1", "alice", "original")
	model.Focus()
	model.HandleResize()
	before := layoutRows(model)
	model.Blur()
	require.Equal(t, "unfinished reply", model.input.Value())
	require.True(t, model.input.ReplyArmed(), "pane focus changes must not discard reply state")
	require.Equal(t, insertMode, model.state)
	model.Focus()
	require.True(t, model.input.Focused())
	require.Equal(t, before, layoutRows(model))
}

func Test_broadcastTab_clearReplyTarget(t *testing.T) {
	t.Parallel()

	tab, _ := newReplyTab(t)
	tab.input.ArmReply("msg-1", "other", "hi")

	tab.clearReplyTarget()

	require.False(t, tab.input.ReplyArmed())
}

// Regression: cancelling an armed reply used to leave the chat window at the height
// it had while the indicator was visible. The view then ended up a row short and the
// status bar was no longer rendered at the bottom of the terminal.
func Test_broadcastTab_cancellingReplyRestoresLayout(t *testing.T) {
	t.Parallel()

	tab, _ := newLoadedReplyTab(t)
	rowsWithoutReply := layoutRows(tab)
	chatHeightWithoutReply := tab.chatWindow.height

	tab.input.ArmReply("msg-1", "otheruser", "nice play")
	tab.HandleResize()

	require.Less(t, tab.chatWindow.height, chatHeightWithoutReply, "the indicator takes a row from the chat window")
	require.Equal(t, rowsWithoutReply, layoutRows(tab), "the indicator only moves rows around")

	tab.state = insertMode
	tab.handleEscapePressed()

	require.False(t, tab.input.ReplyArmed(), "escape disarms the reply")
	require.Equal(t, chatHeightWithoutReply, tab.chatWindow.height, "cancelling gives the row back to the chat window")
	require.Equal(t, rowsWithoutReply, layoutRows(tab))
}

// Regression: alt+enter keeps insert mode, so it skipped the layout recalculation that
// the plain send path performs in its state reset.
func Test_broadcastTab_quickSendRestoresLayout(t *testing.T) {
	t.Parallel()

	tab, _ := newLoadedReplyTab(t)
	rowsWithoutReply := layoutRows(tab)
	chatHeightWithoutReply := tab.chatWindow.height

	tab.input.ArmReply("msg-1", "otheruser", "nice play")
	tab.HandleResize()

	tab.input.SetValue("thanks")

	cmd := tab.handleMessageSent(true)

	require.False(t, tab.input.ReplyArmed(), "quick send consumes the armed reply")
	require.Equal(t, chatHeightWithoutReply, tab.chatWindow.height, "quick send gives the row back to the chat window")
	require.Equal(t, rowsWithoutReply, layoutRows(tab))
	require.NotNil(t, cmd)
}

func TestBroadcastTab_RegisterClick(t *testing.T) {
	t.Parallel()

	c := newChatWindow(80, 10, save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap())
	addTestMessages(t, c, 5)
	entries := c.Entries()
	entry := &entries[0]
	other := &entries[1]
	tab := &Broadcast{chatWindow: c}

	require.False(t, tab.registerClick(entry, c.StartOf(entry)))
	require.True(t, tab.registerClick(entry, c.EndOf(entry)), "second click on the same entry within the window is a double-click")

	require.False(t, tab.registerClick(other, c.StartOf(other)), "a different entry is not a double-click")
	require.True(t, tab.registerClick(other, c.StartOf(other)), "the tracker follows the new entry")

	require.False(t, tab.registerClick(entry, c.EndOf(entry)+10), "a click outside the entry resets the tracker")
	require.False(t, tab.registerClick(entry, c.StartOf(entry)), "the reset click cannot pair with the earlier one")
	require.False(t, tab.registerClick(nil, 0), "nil entries are ignored")
}

func TestBroadcast_EmoteRefreshFailureKeepsTabUsable(t *testing.T) {
	t.Parallel()

	refreshErr := errors.New("could not refresh emote cache: emote data could only be partially fetched")

	t.Run("automatic refresh stays silent", func(t *testing.T) {
		t.Parallel()

		tab := newFSMTestTab(t, false)
		tab.emoteCache = emote.NewCache(zerolog.Nop(), nil, nil, nil, nil)

		// A tab refresh runs on every open; a slow third-party provider is not news.
		_, cmd := tab.Update(emoteSetRefreshedMessage{err: refreshErr})

		require.Nil(t, tab.err, "a cosmetic refresh failure is not a tab error")
		require.Nil(t, cmd, "no notice for the automatic refresh")

		require.NotContains(t, StripANSI(tab.View()), "could not refresh emote cache")
	})

	t.Run("manual refresh reports back", func(t *testing.T) {
		t.Parallel()

		tab := newFSMTestTab(t, false)
		tab.emoteCache = emote.NewCache(zerolog.Nop(), nil, nil, nil, nil)

		_, cmd := tab.Update(emoteSetRefreshedMessage{err: refreshErr, manually: true})

		require.Nil(t, tab.err)
		require.NotNil(t, cmd, "the user asked for it, so the failure is reported")

		msg, ok := cmd().(RequestLocalMessageHandleMessage)
		require.True(t, ok)
		require.Contains(t, msg.Message.(*twitchirc.Notice).Message, "Could not refresh emotes")
	})
}

// capturingPool records the EventSub subscriptions a tab requests. The other
// connection methods are unused by these tests but required by the port.
type capturingPool struct {
	subscriptions []twitchapi.CreateEventSubSubscriptionRequest
}

func (p *capturingPool) ConnectIRC(string) error { return nil }
func (p *capturingPool) DisconnectIRC(string)    {}
func (p *capturingPool) SendIRC(string, twitchirc.IRCer) error {
	return nil
}
func (p *capturingPool) JoinChannel(string, string) error { return nil }
func (p *capturingPool) SubscribeEventSub(_ string, req twitchapi.CreateEventSubSubscriptionRequest, _ wspool.EventSubService) error {
	p.subscriptions = append(p.subscriptions, req)
	return nil
}
func (p *capturingPool) Close() error { return nil }

type fakeEventSubService struct{}

func (fakeEventSubService) CreateEventSubSubscription(context.Context, twitchapi.CreateEventSubSubscriptionRequest) (twitchapi.CreateEventSubSubscriptionResponse, error) {
	return twitchapi.CreateEventSubSubscriptionResponse{}, nil
}

func TestBroadcast_SubscribeChannelEventCmds(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "chan-id", DisplayName: "me"})
	pool := &capturingPool{}
	tab.pool = pool
	tab.client.EventSub = fakeEventSubService{}

	cmds := tab.subscribeChannelEventCmds("chan-id")
	for _, cmd := range cmds {
		require.NotNil(t, cmd)
		_ = cmd()
	}

	types := make([]string, 0, len(pool.subscriptions))
	for _, subscription := range pool.subscriptions {
		types = append(types, subscription.Type)

		if subscription.Type == "channel.raid" {
			if _, ok := subscription.Condition["to_broadcaster_user_id"]; !ok {
				require.Equal(t, "chan-id", subscription.Condition["from_broadcaster_user_id"])
			}
			continue
		}

		require.Equal(t, "chan-id", subscription.Condition["broadcaster_user_id"])
	}

	require.Subset(t, types, []string{
		"channel.prediction.begin",
		"channel.prediction.progress",
		"channel.prediction.lock",
		"channel.prediction.end",
		"channel.channel_points_custom_reward_redemption.add",
		"channel.channel_points_custom_reward_redemption.update",
		"channel.channel_points_automatic_reward_redemption.add",
		"channel.chat_settings.update",
		"channel.warning.send",
		"channel.warning.acknowledge",
		"channel.shoutout.create",
		"channel.shoutout.receive",
		"automod.message.hold",
		"automod.message.update",
		"channel.unban_request.create",
		"channel.unban_request.resolve",
	})

	counts := make(map[string]int, len(pool.subscriptions))
	for _, subscription := range pool.subscriptions {
		counts[subscription.Type]++
	}

	for _, expected := range []struct {
		typeName  string
		version   string
		condition map[string]string
	}{
		{"channel.chat_settings.update", "1", map[string]string{"broadcaster_user_id": "chan-id", "user_id": "chan-id"}},
		{"channel.warning.send", "1", map[string]string{"broadcaster_user_id": "chan-id", "moderator_user_id": "chan-id"}},
		{"channel.warning.acknowledge", "1", map[string]string{"broadcaster_user_id": "chan-id", "moderator_user_id": "chan-id"}},
		{"channel.shoutout.create", "1", map[string]string{"broadcaster_user_id": "chan-id", "moderator_user_id": "chan-id"}},
		{"channel.shoutout.receive", "1", map[string]string{"broadcaster_user_id": "chan-id", "moderator_user_id": "chan-id"}},
		{"automod.message.hold", "2", map[string]string{"broadcaster_user_id": "chan-id", "moderator_user_id": "chan-id"}},
		{"automod.message.update", "2", map[string]string{"broadcaster_user_id": "chan-id", "moderator_user_id": "chan-id"}},
		{"channel.unban_request.create", "1", map[string]string{"broadcaster_user_id": "chan-id", "moderator_user_id": "chan-id"}},
		{"channel.unban_request.resolve", "1", map[string]string{"broadcaster_user_id": "chan-id", "moderator_user_id": "chan-id"}},
	} {
		matched := false
		for _, subscription := range pool.subscriptions {
			if subscription.Type != expected.typeName {
				continue
			}

			matched = true
			require.Equal(t, expected.version, subscription.Version, expected.typeName)
			require.Equal(t, expected.condition, subscription.Condition, expected.typeName)
		}

		require.True(t, matched, "missing subscription %s", expected.typeName)
		require.Equal(t, 1, counts[expected.typeName], "duplicate subscription %s", expected.typeName)
	}
}

func TestBroadcast_SubscribeChannelEventCmdsRequiresOwnChannel(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "other", DisplayName: "me"})
	tab.client.EventSub = fakeEventSubService{}

	require.Empty(t, tab.subscribeChannelEventCmds("chan-id"))
}

func TestBroadcast_SubscribeChannelEventCmdsIncludesUnbanRequestsForModerators(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "other", DisplayName: "me"})
	tab.isUserMod = true
	tab.client.EventSub = fakeEventSubService{}
	pool := &capturingPool{}
	tab.pool = pool

	for _, cmd := range tab.subscribeChannelEventCmds("chan-id") {
		require.NotNil(t, cmd)
		_ = cmd()
	}

	require.Len(t, pool.subscriptions, 2)
	for _, subscription := range pool.subscriptions {
		require.Contains(t, []string{"channel.unban_request.create", "channel.unban_request.resolve"}, subscription.Type)
		require.Equal(t, map[string]string{
			"broadcaster_user_id": "chan-id",
			"moderator_user_id":   "other",
		}, subscription.Condition)
	}
}

func TestBroadcast_HandleEventSubMessageIgnoresOtherChannels(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})

	msg := eventsub.Message[eventsub.NotificationPayload]{
		Payload: eventsub.NotificationPayload{
			Subscription: eventsub.Subscription{
				Type:      "channel.prediction.begin",
				Condition: map[string]string{"broadcaster_user_id": "other-id"},
			},
			Event: eventsub.Event{Title: "not mine"},
		},
	}

	require.Nil(t, tab.handleEventSubMessage(msg))
	require.False(t, tab.prediction.enabled)
}

// requestFromCmd unwraps the local-message request from a plain command or a
// tea.Batch (prediction begin also schedules the countdown tick).
func requestFromCmd(t *testing.T, cmd tea.Cmd) RequestLocalMessageHandleMessage {
	t.Helper()

	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			if child == nil {
				continue
			}

			if request, ok := child().(RequestLocalMessageHandleMessage); ok {
				return request
			}
		}
	}

	request, ok := msg.(RequestLocalMessageHandleMessage)
	require.True(t, ok)
	return request
}

func TestBroadcast_HandlePredictionEvents(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})

	event := func(subType string, event eventsub.Event) eventsub.Message[eventsub.NotificationPayload] {
		return eventsub.Message[eventsub.NotificationPayload]{
			Payload: eventsub.NotificationPayload{
				Subscription: eventsub.Subscription{
					Type:      subType,
					Condition: map[string]string{"broadcaster_user_id": "chan-id"},
				},
				Event: event,
			},
		}
	}

	begin := event("channel.prediction.begin", eventsub.Event{
		ID:      "prediction-1",
		Title:   "Will we win?",
		LocksAt: time.Now().Add(time.Minute),
		Outcomes: []eventsub.Outcome{
			{ID: "yes", Title: "Yes", ChannelPoints: 500},
			{ID: "no", Title: "No", ChannelPoints: 200},
		},
	})

	cmd := tab.handleEventSubMessage(begin)
	require.NotNil(t, cmd)
	require.True(t, tab.prediction.enabled)
	require.False(t, tab.prediction.locked)
	require.True(t, tab.predictionCountdown)
	require.True(t, tab.layout.prediction.visible())

	_, foreignTick := tab.Update(predictionTickMsg{owner: &prediction{}})
	require.Nil(t, foreignTick, "ticks from another tab must not affect this prediction")
	require.True(t, tab.predictionCountdown)

	request := requestFromCmd(t, cmd)
	notice, ok := request.Message.(*twitchirc.Notice)
	require.True(t, ok)
	require.Contains(t, notice.Message, `Prediction "Will we win?" has started`)

	require.Nil(t, tab.handleEventSubMessage(event("channel.prediction.progress", eventsub.Event{
		ID:    "prediction-1",
		Title: "Will we win?",
		Outcomes: []eventsub.Outcome{
			{ID: "yes", Title: "Yes", ChannelPoints: 700},
			{ID: "no", Title: "No", ChannelPoints: 300},
		},
	})))
	require.InDelta(t, 0.7, tab.prediction.outcomes[0].percent, 0.0001)

	lockCmd := tab.handleEventSubMessage(event("channel.prediction.lock", eventsub.Event{
		ID:    "prediction-1",
		Title: "Will we win?",
		Outcomes: []eventsub.Outcome{
			{ID: "yes", Title: "Yes", ChannelPoints: 700},
			{ID: "no", Title: "No", ChannelPoints: 300},
		},
	}))
	require.NotNil(t, lockCmd)
	require.True(t, tab.prediction.locked)
	require.Contains(t, requestFromCmd(t, lockCmd).Message.(*twitchirc.Notice).Message, "is locked")

	_, tickCmd := tab.Update(predictionTickMsg{owner: tab.prediction})
	require.Nil(t, tickCmd, "a locked prediction stops the countdown ticker")
	require.False(t, tab.predictionCountdown)

	endCmd := tab.handleEventSubMessage(event("channel.prediction.end", eventsub.Event{
		ID:               "prediction-1",
		Title:            "Will we win?",
		Status:           "resolved",
		WinningOutcomeID: "no",
		Outcomes: []eventsub.Outcome{
			{ID: "yes", Title: "Yes", ChannelPoints: 700},
			{ID: "no", Title: "No", ChannelPoints: 900},
		},
	}))
	require.NotNil(t, endCmd)
	require.False(t, tab.prediction.enabled)
	require.False(t, tab.layout.prediction.visible())
	require.Contains(t, requestFromCmd(t, endCmd).Message.(*twitchirc.Notice).Message, `"No" won with 900 points`)
}

func TestBroadcast_HandleRedemptionEvents(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})

	event := func(subType string, status string) eventsub.Message[eventsub.NotificationPayload] {
		return eventsub.Message[eventsub.NotificationPayload]{
			Payload: eventsub.NotificationPayload{
				Subscription: eventsub.Subscription{
					Type:      subType,
					Condition: map[string]string{"broadcaster_user_id": "chan-id"},
				},
				Event: eventsub.Event{
					ID:        "redemption-1",
					UserName:  "Alice",
					UserInput: "make it purple",
					Status:    status,
					Reward:    eventsub.Reward{Title: "Highlight My Message", Cost: 500},
				},
			},
		}
	}

	addCmd := tab.handleEventSubMessage(event("channel.channel_points_custom_reward_redemption.add", "unfulfilled"))
	require.NotNil(t, addCmd)

	request, ok := addCmd().(RequestLocalMessageHandleMessage)
	require.True(t, ok)
	tab.chatWindow.handleMessage(chat.ChatEventMessage{Message: request.Message})

	require.Contains(t, strings.Join(strings.Fields(plain(tab.chatWindow.RawRows())), " "), `Alice redeemed "Highlight My Message" (500 points): make it purple`)
	require.Len(t, tab.chatWindow.Entries(), 1)
	require.Len(t, tab.redemptions, 1)

	require.Nil(t, tab.handleEventSubMessage(event("channel.channel_points_custom_reward_redemption.update", "fulfilled")))

	text := plain(tab.chatWindow.RawRows())
	require.Contains(t, text, "[fulfilled]")
	require.Len(t, tab.chatWindow.Entries(), 1, "an update must not append a second notice")
	require.Empty(t, tab.redemptions, "a terminal update stops tracking the redemption")
}

func TestBroadcast_UntrackedRedemptionsDoNotLeak(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})

	event := func(subType, status string) eventsub.Message[eventsub.NotificationPayload] {
		return eventsub.Message[eventsub.NotificationPayload]{
			Payload: eventsub.NotificationPayload{
				Subscription: eventsub.Subscription{
					Type:      subType,
					Condition: map[string]string{"broadcaster_user_id": "chan-id"},
				},
				Event: eventsub.Event{
					ID:     "redemption-1",
					Status: status,
					Reward: eventsub.Reward{Title: "Test Reward", Cost: 500},
				},
			},
		}
	}

	require.NotNil(t, tab.handleEventSubMessage(event("channel.channel_points_automatic_reward_redemption.add", "")))
	require.Empty(t, tab.redemptions, "one-shot automatic rewards are not tracked")

	require.NotNil(t, tab.handleEventSubMessage(event("channel.channel_points_custom_reward_redemption.update", "fulfilled")))
	require.Empty(t, tab.redemptions, "a terminal update without a prior add is not tracked")
}

func TestBroadcast_HandleModerationEventSubMessages(t *testing.T) {
	t.Parallel()

	message := func(subType string, event eventsub.Event) eventsub.Message[eventsub.NotificationPayload] {
		return eventsub.Message[eventsub.NotificationPayload]{
			Payload: eventsub.NotificationPayload{
				Subscription: eventsub.Subscription{
					Type:      subType,
					Condition: map[string]string{"broadcaster_user_id": "chan-id"},
				},
				Event: event,
			},
		}
	}

	t.Run("warning send", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
		cmd := tab.handleEventSubMessage(message("channel.warning.send", eventsub.Event{
			ModeratorUserName: "Mod",
			UserName:          "Alice",
			Reason:            "spam",
		}))

		require.Contains(t, requestFromCmd(t, cmd).Message.(*twitchirc.Notice).Message, "Mod warned Alice: spam")
	})

	t.Run("warning acknowledge", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
		cmd := tab.handleEventSubMessage(message("channel.warning.acknowledge", eventsub.Event{UserName: "Alice"}))

		require.Contains(t, requestFromCmd(t, cmd).Message.(*twitchirc.Notice).Message, "Alice acknowledged")
	})

	t.Run("shoutout create", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
		cmd := tab.handleEventSubMessage(message("channel.shoutout.create", eventsub.Event{
			ToBroadcasterUserName: "Bob",
			ViewerCount:           42,
		}))

		require.Contains(t, requestFromCmd(t, cmd).Message.(*twitchirc.Notice).Message, "Sent a shoutout to Bob with 42 viewers")
	})

	t.Run("shoutout receive", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
		cmd := tab.handleEventSubMessage(message("channel.shoutout.receive", eventsub.Event{
			FromBroadcasterUserName: "Bob",
			ViewerCount:             12,
		}))

		require.Contains(t, requestFromCmd(t, cmd).Message.(*twitchirc.Notice).Message, "Bob sent this channel a shoutout with 12 viewers")
	})
}

func TestBroadcast_HandleAutoModEvents(t *testing.T) {
	t.Parallel()

	tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})

	event := func(subType string, event eventsub.Event) eventsub.Message[eventsub.NotificationPayload] {
		return eventsub.Message[eventsub.NotificationPayload]{
			Payload: eventsub.NotificationPayload{
				Subscription: eventsub.Subscription{
					Type:      subType,
					Condition: map[string]string{"broadcaster_user_id": "chan-id"},
				},
				Event: event,
			},
		}
	}

	held := eventsub.Event{
		MessageID: "msg-1",
		UserName:  "BadUser",
		Reason:    "automod",
		Category:  "aggressive",
		Level:     2,
		Message:   eventsub.MessageContent{Text: "a bad message"},
	}

	cmd := tab.handleEventSubMessage(event("automod.message.hold", held))
	require.NotNil(t, cmd)

	notice := requestFromCmd(t, cmd).Message.(*twitchirc.Notice)
	require.Contains(t, notice.Message, "AutoMod held a message from BadUser")
	require.Contains(t, notice.Message, "a bad message")
	require.Contains(t, notice.Message, "/approve msg-1")
	require.Len(t, tab.automodHeld, 1)

	tab.chatWindow.handleMessage(chat.ChatEventMessage{Message: notice})

	require.Nil(t, tab.handleEventSubMessage(event("automod.message.update", eventsub.Event{
		MessageID: "msg-1",
		UserName:  "BadUser",
		Status:    "approved",
		Reason:    "automod",
		Message:   eventsub.MessageContent{Text: "a bad message"},
	})))

	require.Empty(t, tab.automodHeld, "a terminal update stops tracking the message")
	require.Contains(t, plain(tab.chatWindow.RawRows()), "[approved]")
	require.Len(t, tab.chatWindow.Entries(), 1, "the update rewrites the existing notice")
}

func TestBroadcast_ChatSettingsEventUpdatesStatus(t *testing.T) {
	t.Parallel()

	cache := chatstate.NewCache()
	tab := newStatusTestTab(&settingsAPI{}, cache)

	cmd := tab.handleEventSubMessage(eventsub.Message[eventsub.NotificationPayload]{
		Payload: eventsub.NotificationPayload{
			Subscription: eventsub.Subscription{
				Type:      "channel.chat_settings.update",
				Condition: map[string]string{"broadcaster_user_id": "chan-id"},
			},
			Event: eventsub.Event{
				BroadcasterUserID:    "chan-id",
				SlowMode:             boolValue(true),
				SlowModeWaitTime:     intValue(30),
				FollowerMode:         boolValue(true),
				FollowerModeDuration: intValue(10),
				SubscriberMode:       boolValue(true),
			},
		},
	})

	require.Nil(t, cmd)
	require.True(t, tab.statusInfo.settings.SlowMode)
	require.True(t, tab.statusInfo.settings.SubscriberMode)
	require.Contains(t, tab.statusInfo.View(), "Slow Mode")
	require.Contains(t, tab.statusInfo.View(), "Sub Only")

	cached, ok := cache.Get("chan-id")
	require.True(t, ok)
	require.True(t, cached.FollowerMode)
	require.Equal(t, 10, cached.FollowerModeDuration)
}

// TestBroadcast_AutoModShortcutTypesCommand covers the quick approve/deny keys:
// a held AutoMod notice becomes a ready-to-send command, while other notices and
// terminal AutoMod messages are ignored.
func TestBroadcast_AutoModShortcutTypesCommand(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		key    tea.KeyPressMsg
		action string
	}{
		{name: "approve", key: tea.KeyPressMsg{Code: 'a', Mod: tea.ModAlt}, action: "approve"},
		{name: "deny", key: tea.KeyPressMsg{Code: 'd', Mod: tea.ModAlt}, action: "deny"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
			notice := &twitchirc.Notice{MsgID: "msg-1", Message: "AutoMod held a message"}
			tab.chatWindow.handleMessage(chat.ChatEventMessage{AccountID: "acc", ChannelID: "chan-id", Message: notice})
			tab.automodHeld = map[string]*twitchirc.Notice{"msg-1": notice}

			handled, cmd := tab.handleKeyPress(tt.key)
			require.True(t, handled)
			require.Nil(t, cmd)
			require.Equal(t, insertMode, tab.state)
			require.Equal(t, "/"+tt.action+" msg-1", tab.input.RawValue())
		})
	}

	t.Run("ordinary notice is ignored without losing chat focus", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
		tab.Focus()
		tab.chatWindow.handleMessage(chat.ChatEventMessage{AccountID: "acc", ChannelID: "chan-id", Message: &twitchirc.Notice{
			MsgID:   "notice-1",
			Message: "just a notice",
		}})
		tab.chatWindow.handleMessage(chat.ChatEventMessage{AccountID: "acc", ChannelID: "chan-id", Message: &twitchirc.Notice{
			MsgID:   "notice-2",
			Message: "another notice",
		}})
		entries := tab.chatWindow.Entries()
		tab.chatWindow.selectEntry(&entries[0])

		handled, _ := tab.handleKeyPress(tea.KeyPressMsg{Code: 'd', Mod: tea.ModAlt})
		require.True(t, handled)
		require.Equal(t, inChatWindow, tab.state)
		require.Empty(t, tab.input.RawValue())
		require.True(t, tab.chatWindow.Focused(), "a rejected shortcut must not blur the chat and strand keyboard navigation")

		before, _ := tab.chatWindow.Selected()
		_, _ = tab.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		after, _ := tab.chatWindow.Selected()
		require.NotEqual(t, before.ID, after.ID, "arrow-key navigation keeps working after the rejected shortcut")
	})

	t.Run("finished AutoMod notice is ignored without losing chat focus", func(t *testing.T) {
		t.Parallel()

		tab := newLoadedLayoutTestTab(t, 80, 24, save.Account{ID: "acc", DisplayName: "me"})
		tab.Focus()
		tab.chatWindow.handleMessage(chat.ChatEventMessage{AccountID: "acc", ChannelID: "chan-id", Message: &twitchirc.Notice{
			MsgID:   "msg-1",
			Message: "AutoMod held a message [approved]",
		}})

		handled, _ := tab.handleKeyPress(tea.KeyPressMsg{Code: 'a', Mod: tea.ModAlt})
		require.True(t, handled)
		require.Equal(t, inChatWindow, tab.state)
		require.Empty(t, tab.input.RawValue())
		require.True(t, tab.chatWindow.Focused(), "an already-finished AutoMod notice must keep chat focus")
	})
}
