package mainui

import (
	"testing"

	"github.com/julez-dev/chatuino/save"
	"github.com/stretchr/testify/require"
)

// fsmModes is every mode the tab can be in.
var fsmModes = []broadcastTabState{
	inChatWindow,
	insertMode,
	userInspectMode,
	userInspectInsertMode,
	emoteOverviewMode,
}

// fsmEvents is every input that may move the tab.
var fsmEvents = []tabEvent{
	eventCompose,
	eventSend,
	eventCancel,
	eventInspect,
	eventEmoteOverview,
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
		eventLeaveInspect:  {allowed: false, to: inChatWindow},
	},
	insertMode: {
		eventCompose:       {allowed: false, to: insertMode},
		eventSend:          {allowed: true, to: inChatWindow},
		eventCancel:        {allowed: true, to: inChatWindow},
		eventInspect:       {allowed: true, to: userInspectMode},
		eventEmoteOverview: {allowed: true, to: emoteOverviewMode},
		eventLeaveInspect:  {allowed: false, to: insertMode},
	},
	userInspectMode: {
		eventCompose:       {allowed: true, to: userInspectInsertMode},
		eventSend:          {allowed: false, to: userInspectMode},
		eventCancel:        {allowed: true, to: inChatWindow},
		eventInspect:       {allowed: true, to: userInspectMode},
		eventEmoteOverview: {allowed: true, to: emoteOverviewMode},
		eventLeaveInspect:  {allowed: true, to: inChatWindow},
	},
	userInspectInsertMode: {
		eventCompose:       {allowed: false, to: userInspectInsertMode},
		eventSend:          {allowed: true, to: userInspectMode},
		eventCancel:        {allowed: true, to: userInspectMode},
		eventInspect:       {allowed: true, to: userInspectMode},
		eventEmoteOverview: {allowed: true, to: emoteOverviewMode},
		eventLeaveInspect:  {allowed: true, to: inChatWindow},
	},
	emoteOverviewMode: {
		eventCompose:       {allowed: false, to: emoteOverviewMode},
		eventSend:          {allowed: false, to: emoteOverviewMode},
		eventCancel:        {allowed: true, to: inChatWindow},
		eventInspect:       {allowed: false, to: emoteOverviewMode},
		eventEmoteOverview: {allowed: false, to: emoteOverviewMode},
		eventLeaveInspect:  {allowed: false, to: emoteOverviewMode},
	},
}

func newFSMTestTab(t *testing.T, anonymous bool) *broadcastTab {
	t.Helper()

	deps := newTestDeps()

	account := save.Account{ID: "acc", DisplayName: "me"}
	if anonymous {
		account = save.Account{ID: "anon", IsAnonymous: true}
	}

	tab := newBroadcastTab("tab-1", 80, 24, account, "chan", deps)
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
func enterMode(t *testing.T, tab *broadcastTab, mode broadcastTabState) {
	t.Helper()

	inspect := func() {
		tab.userInspect = newUserInspect(tab.id, tab.width, tab.height, "someone", tab.channelLogin, tab.account.ID, tab.deps)
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
	default:
		t.Fatalf("no setup path for mode %v", mode)
	}

	require.Equal(t, mode, tab.state, "test setup must reach the requested mode")
}

// assertTabInvariants checks the guarantees the mode machine is supposed to give.
func assertTabInvariants(t *testing.T, tab *broadcastTab) {
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
					tab.userInspect = newUserInspect(tab.id, tab.width, tab.height, "someone", tab.channelLogin, tab.account.ID, tab.deps)
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
