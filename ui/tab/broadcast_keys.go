package tab

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/cli/browser"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/rs/zerolog/log"
)

// handleKeyPress dispatches a key press while the tab is focused and the channel
// data is loaded. It reports whether the key was handled; unhandled keys fall
// through to the message input and child components.
func (t *Broadcast) handleKeyPress(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	// Focus message input, when not in insert mode and not in search mode inside chat window, depending on the current active chat window
	if key.Matches(msg, t.keymap.InsertMode) &&
		(t.state == inChatWindow && t.chatWindow.state != searchChatWindowState || t.state == userInspectMode && t.userInspect.chatWindow.state != searchChatWindowState) {
		return true, t.handleStartInsertMode()
	}

	// Plain-letter shortcuts must not fire while a chat search input is focused.
	searching := t.isChatSearching()

	// Open user inspect mode, where only messages from a specific user are shown
	if key.Matches(msg, t.keymap.InspectMode) && !searching && t.state.isViewing() {
		return true, t.handleOpenUserInspectFromMessage()
	}

	// Open chat in browser
	if key.Matches(msg, t.keymap.ChatPopUp, t.keymap.ChannelPopUp) && !searching && t.state.isViewing() {
		return true, t.handleOpenBrowser(msg)
	}

	// Send message
	if key.Matches(msg, t.keymap.Confirm) && len(t.input.Value()) > 0 && t.state.isComposing() {
		t.input.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // commit history entry
		return true, t.handleMessageSent(false)
	}

	// Send message - quick send
	if key.Matches(msg, t.keymap.QuickSent) && len(t.input.Value()) > 0 && t.state.isComposing() {
		t.input.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // commit history entry
		return true, t.handleMessageSent(true)
	}

	// Message Accept Suggestion Template Replace
	// always allow accept suggestion key so even new texts can be templated
	if t.input.IsAcceptSuggestionKey(msg) && len(t.input.Value()) > 0 && t.state.isComposing() {
		_, heightChanged := t.input.Update(msg)
		cmd := t.replaceInputTemplate()
		if heightChanged {
			t.HandleResize()
		}
		return true, cmd
	}

	// Set quick time out message to message input
	if key.Matches(msg, t.keymap.QuickTimeout) && !searching && t.state.isViewing() {
		t.handleTimeoutShortcut()
		return true, nil
	}

	// Copy selected message to message input
	if key.Matches(msg, t.keymap.CopyMessage) && !searching && t.state.isViewing() {
		t.handleCopyMessage()
		return true, nil
	}

	// Reply to selected message
	if key.Matches(msg, t.keymap.ReplyMessage) && !searching && t.state.isViewing() {
		t.handleReplyMessage()
		return true, nil
	}

	// Close overlay windows
	if key.Matches(msg, t.keymap.Escape) {
		var cmd tea.Cmd

		// first end search in user inspect sub window
		if t.userInspect != nil && t.userInspect.chatWindow.state == searchChatWindowState {
			t.userInspect.chatWindow, cmd = t.userInspect.chatWindow.Update(msg)
			return true, cmd
		}

		// second case, end inspect mode or end insert mode in inspect window
		if t.state.isInspecting() {
			t.handleEscapePressed()
			return true, nil
		}

		// third case, end search in 'main' chat window
		if t.chatWindow.state == searchChatWindowState {
			t.chatWindow, cmd = t.chatWindow.Update(msg)
			return true, cmd
		}

		t.handleEscapePressed()
		return true, nil
	}

	return false, nil
}

// isChatSearching reports whether the active chat window is capturing text for
// search. Plain-letter shortcuts are suppressed while this is true so they can
// be typed into the search input instead.
func (t *Broadcast) isChatSearching() bool {
	if t.state.isInspecting() {
		return t.userInspect != nil && t.userInspect.chatWindow.state == searchChatWindowState
	}

	return t.chatWindow != nil && t.chatWindow.state == searchChatWindowState
}

// leaveUserInspect exits user inspect mode (including its insert mode) and
// returns focus to the main chat window.
func (t *Broadcast) leaveUserInspect() {
	changed, _ := t.transit(eventLeaveInspect)
	if !changed {
		return
	}

	t.HandleResize()
	t.chatWindow.updatePort()
}

func (t *Broadcast) handleEscapePressed() {
	// Leaving a mode that replaced the chat window with a full-sized one changes
	// the layout; leaving the composer does not.
	fullWindow := t.state.isInspecting() || t.state == emoteOverviewMode

	changed, _ := t.transit(eventCancel)
	if !changed {
		return
	}

	if fullWindow && t.state == inChatWindow {
		t.HandleResize()
		t.chatWindow.updatePort()
	}
}

func (t *Broadcast) handleOpenBrowser(msg tea.KeyPressMsg) tea.Cmd {
	return func() tea.Msg {
		// open popup chat if modifier is pressed
		if key.Matches(msg, t.keymap.ChatPopUp) {
			t.handleOpenBrowserChatPopUp()()
			return nil
		}

		t.handleOpenBrowserChannel()()
		return nil
	}
}

func (t *Broadcast) handleOpenBrowserChatPopUp() tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf(streamChatPopUpFmt, t.channelLogin)

		if err := browser.OpenURL(url); err != nil {
			log.Logger.Error().Err(err).Msg("error while opening twitch channel in browser")
		}
		return nil
	}
}

func (t *Broadcast) handleOpenBrowserChannel() tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf(streamWebFmt, t.channelLogin)

		if err := browser.OpenURL(url); err != nil {
			log.Logger.Error().Err(err).Msg("error while opening twitch channel in browser")
		}
		return nil
	}
}

func (t *Broadcast) handleStartInsertMode() tea.Cmd {
	_, cmd := t.transit(eventCompose)
	return cmd
}

func (t *Broadcast) handleCopyMessage() {
	entry, ok := t.cursorEntry()
	if !ok {
		return
	}

	msg, ok := entry.Event.Message.(*twitchirc.PrivateMessage)
	if !ok {
		return
	}

	// The table rejects this for anonymous accounts, which cannot compose.
	if accepted, _ := t.transit(eventCompose); !accepted {
		return
	}

	t.clearReplyTarget() // copying replaces the armed reply with different content
	t.input.SetValue(strings.ReplaceAll(msg.Message, string(duplicateBypass), ""))
	t.HandleResize() // Recalculate layout after copying message
}

// handleReplyMessage arms a reply to the message under the cursor and starts insert mode.
func (t *Broadcast) handleReplyMessage() {
	if t.account.IsAnonymous {
		return
	}

	entry, ok := t.cursorEntry()
	if !ok {
		return
	}

	msg, ok := entry.Event.Message.(*twitchirc.PrivateMessage)

	// Without an original message ID there is nothing to thread onto, so ignore the
	// keypress instead of arming a reply Twitch would reject.
	if !ok || msg.ID == "" {
		return
	}

	if accepted, _ := t.transit(eventCompose); !accepted {
		return
	}

	t.input.ArmReply(msg.ID, msg.DisplayName, msg.Message)
	t.HandleResize()
}

func (t *Broadcast) handleOpenUserInspectFromMessage() tea.Cmd {
	var e *chatEntry

	if t.state == inChatWindow {
		_, e = t.chatWindow.entryForCurrentCursor()
	} else {
		_, e = t.userInspect.chatWindow.entryForCurrentCursor()
	}

	if e == nil {
		return nil
	}

	var username string
	switch msg := e.Event.Message.(type) {
	case *twitchirc.PrivateMessage:
		username = msg.LoginName
	case *twitchirc.ClearChat:
		if msg.UserName == nil {
			return nil
		}

		username = *msg.UserName
	default:
		return nil
	}

	return t.handleOpenUserInspect([]string{username})
}

func (t *Broadcast) handleTimeoutShortcut() {
	if t.account.IsAnonymous {
		return
	}

	entry, ok := t.cursorEntry()
	if !ok {
		return
	}

	msg, ok := entry.Event.Message.(*twitchirc.PrivateMessage)
	if !ok {
		return
	}

	if accepted, _ := t.transit(eventCompose); !accepted {
		return
	}

	t.input.SetValue("/timeout " + msg.DisplayName + " 600")
	t.HandleResize() // Recalculate layout after setting timeout command
}

// activeChatWindow returns the chat window that owns the selection, or nil when
// the tab shows none.
func (t *Broadcast) activeChatWindow() *chatWindow {
	if t.state.isInspecting() {
		if t.userInspect == nil {
			return nil
		}

		return t.userInspect.chatWindow
	}

	return t.chatWindow
}

// cursorEntry returns the entry under the cursor of the active chat window. An
// active search is stopped first so the cursor resolves against the full list.
func (t *Broadcast) cursorEntry() (*chatEntry, bool) {
	window := t.activeChatWindow()
	if window == nil {
		return nil, false
	}

	if window.state == searchChatWindowState {
		window.handleStopSearchMode()
	}

	_, entry := window.entryForCurrentCursor()
	if entry == nil {
		return nil, false
	}

	t.chatWindow.Blur()

	if t.userInspect != nil {
		t.userInspect.chatWindow.Blur()
	}

	return entry, true
}

func (t *Broadcast) handleOpenEmoteOverview() tea.Cmd {
	if accepted, _ := t.transit(eventEmoteOverview); !accepted {
		return nil
	}

	if t.emoteOverview != nil {
		t.HandleResize()
		return nil
	}

	t.emoteOverview = NewEmoteOverview(t.channelID, t.emoteCache, t.emoteReplacer, graphicRows(t.settings), t.width, t.height)
	t.HandleResize()
	return t.emoteOverview.Init()
}

func (t *Broadcast) handleManualRefreshEmotes() tea.Cmd {
	if t.account.IsAnonymous {
		return nil
	}

	t.emoteCache.RemoveEmoteSetForChannel(t.channelID)

	return t.refreshEmotes(t.channelLogin, t.channelID, true)
}
