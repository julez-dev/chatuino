package mainui

import tea "charm.land/bubbletea/v2"

// A broadcast tab is a small state machine: which widget owns the keyboard, which
// overlay is open, and what gets rendered all follow from its mode.
//
// Only the transitions declared in tabTransitions are legal. Every mode change
// goes through transit, so the table is the single place that says what may
// follow what, and the entry and exit actions are the single place that moves
// focus. Before this existed the same information was spread over ~30 guards and
// 15 assignments across five files, and "which child is focused" was a convention
// rather than an invariant.

// tabEvent is an input that may move the tab to another mode.
type tabEvent uint8

const (
	// eventCompose starts writing a message. The insert key, reply, copy and the
	// timeout shortcut all mean "compose in this tab".
	eventCompose tabEvent = iota
	// eventSend sends the composed message and returns to viewing.
	eventSend
	// eventCancel is Escape.
	eventCancel
	// eventInspect opens the user inspect window for the selected message.
	eventInspect
	// eventEmoteOverview opens the emote overview.
	eventEmoteOverview
	// eventLeaveInspect closes the user inspect window without returning to the
	// composer, used when the user clicks outside of it.
	eventLeaveInspect
)

// tabTransition is one row of the mode table.
type tabTransition struct {
	to    broadcastTabState
	guard func(*broadcastTab) bool
	exit  func(*broadcastTab)
	enter func(*broadcastTab) tea.Cmd
}

// tabTransitions is the mode table. A pair that is not listed cannot happen: the
// matching key press or click is reported as unhandled instead.
var tabTransitions = map[broadcastTabState]map[tabEvent]tabTransition{
	inChatWindow: {
		eventCompose:       {to: insertMode, guard: canInteract, exit: blurChatWindow, enter: focusMessageInput},
		eventInspect:       {to: userInspectMode, guard: hasInspectWindow, exit: blurChatWindow, enter: focusInspectWindow},
		eventEmoteOverview: {to: emoteOverviewMode, guard: canInteract, exit: blurChatWindow},
	},
	insertMode: {
		eventSend:          {to: inChatWindow, exit: clearMessageInput, enter: focusChatWindow},
		eventCancel:        {to: inChatWindow, exit: cancelMessageInput, enter: focusChatWindow},
		eventInspect:       {to: userInspectMode, guard: hasInspectWindow, exit: cancelMessageInput, enter: focusInspectWindow},
		eventEmoteOverview: {to: emoteOverviewMode, guard: canInteract, exit: cancelMessageInput},
	},
	userInspectMode: {
		eventCompose:       {to: userInspectInsertMode, guard: canInteract, exit: blurChatWindows, enter: focusMessageInput},
		eventCancel:        {to: inChatWindow, exit: dropInspectWindow, enter: focusChatWindow},
		eventInspect:       {to: userInspectMode, guard: hasInspectWindow, exit: blurChatWindow, enter: focusInspectWindow},
		eventEmoteOverview: {to: emoteOverviewMode, guard: canInteract, exit: leaveInspectForOverview},
		eventLeaveInspect:  {to: inChatWindow, exit: leaveInspect, enter: focusChatWindow},
	},
	userInspectInsertMode: {
		eventSend:          {to: userInspectMode, exit: clearMessageInput, enter: focusInspectWindow},
		eventCancel:        {to: userInspectMode, exit: cancelMessageInput, enter: focusInspectWindow},
		eventInspect:       {to: userInspectMode, guard: hasInspectWindow, exit: cancelMessageInput, enter: focusInspectWindow},
		eventLeaveInspect:  {to: inChatWindow, exit: leaveInspect, enter: focusChatWindow},
		eventEmoteOverview: {to: emoteOverviewMode, guard: canInteract, exit: leaveInspectForOverview},
	},
	emoteOverviewMode: {
		eventCancel: {to: inChatWindow, exit: dropInspectWindow, enter: focusChatWindow},
	},
}

// transit applies the declared transition for (mode, event). It reports whether
// the event was accepted and returns the command produced by the entry action.
func (t *broadcastTab) transit(event tabEvent) (bool, tea.Cmd) {
	transition, ok := tabTransitions[t.state][event]
	if !ok {
		return false, nil
	}

	if transition.guard != nil && !transition.guard(t) {
		return false, nil
	}

	if transition.exit != nil {
		transition.exit(t)
	}

	t.state = transition.to

	if transition.enter != nil {
		return true, transition.enter(t)
	}

	return true, nil
}

// canGo reports whether event is legal in the current mode, without applying it.
func (t *broadcastTab) canGo(event tabEvent) bool {
	transition, ok := tabTransitions[t.state][event]
	if !ok {
		return false
	}

	return transition.guard == nil || transition.guard(t)
}

// --- guards ---

// canInteract blocks the transitions that need an authenticated account: anonymous
// accounts can read chat but cannot compose messages or open the emote overview.
func canInteract(t *broadcastTab) bool {
	return !t.account.IsAnonymous
}

// hasInspectWindow rejects entering inspect mode without an inspect window. The
// caller builds the window first (it needs the username to do so); without this
// guard a caller that forgot would leave the tab in a mode that renders a nil
// window.
func hasInspectWindow(t *broadcastTab) bool {
	return t.userInspect != nil
}

// --- exit actions ---

func blurChatWindow(t *broadcastTab) {
	t.chatWindow.Blur()
}

func blurChatWindows(t *broadcastTab) {
	t.chatWindow.Blur()

	if t.userInspect != nil {
		t.userInspect.chatWindow.Blur()
	}
}

// clearMessageInput drops the sent message, its reply target and the focus that
// came with it.
func clearMessageInput(t *broadcastTab) {
	t.input.Blur()
	t.input.Clear()
	t.clearReplyTarget()
}

// cancelMessageInput keeps the typed text but drops focus and the reply target.
func cancelMessageInput(t *broadcastTab) {
	t.input.Blur()
	t.clearReplyTarget()
}

func dropInspectWindow(t *broadcastTab) {
	t.userInspect = nil
}

// leaveInspectForOverview drops the inspect window when the emote overview takes
// over the whole tab.
func leaveInspectForOverview(t *broadcastTab) {
	blurChatWindows(t)
	dropInspectWindow(t)
	cancelMessageInput(t)
}

// leaveInspect closes the inspect window, including its composer.
func leaveInspect(t *broadcastTab) {
	t.userInspect = nil
	t.input.Blur()
	t.clearReplyTarget()
}

// --- entry actions ---

func focusMessageInput(t *broadcastTab) tea.Cmd {
	return t.input.Focus()
}

func focusChatWindow(t *broadcastTab) tea.Cmd {
	t.chatWindow.Focus()
	return nil
}

func focusInspectWindow(t *broadcastTab) tea.Cmd {
	if t.userInspect != nil {
		t.userInspect.chatWindow.Focus()
	}

	return nil
}

// --- mode predicates ---

// isComposing reports whether the message composer owns the keyboard.
func (s broadcastTabState) isComposing() bool {
	return s == insertMode || s == userInspectInsertMode
}

// isInspecting reports whether the user inspect window is open, with or without
// the composer.
func (s broadcastTabState) isInspecting() bool {
	return s == userInspectMode || s == userInspectInsertMode
}
