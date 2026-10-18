package mainui

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
func (t *broadcastTab) handleKeyPress(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	// Focus message input, when not in insert mode and not in search mode inside chat window, depending on the current active chat window
	if key.Matches(msg, t.deps.Keymap.InsertMode) &&
		(t.state == inChatWindow && t.chatWindow.state != searchChatWindowState || t.state == userInspectMode && t.userInspect.chatWindow.state != searchChatWindowState) {
		return true, t.handleStartInsertMode()
	}

	// Open user inspect mode, where only messages from a specific user are shown
	if key.Matches(msg, t.deps.Keymap.InspectMode) && (t.state == inChatWindow || t.state == userInspectMode) {
		return true, t.handleOpenUserInspectFromMessage()
	}

	// Open chat in browser
	if key.Matches(msg, t.deps.Keymap.ChatPopUp, t.deps.Keymap.ChannelPopUp) && (t.state == inChatWindow || t.state == userInspectMode) {
		return true, t.handleOpenBrowser(msg)
	}

	// Send message
	if key.Matches(msg, t.deps.Keymap.Confirm) && len(t.input.Value()) > 0 && (t.state == insertMode || t.state == userInspectInsertMode) {
		t.input.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // commit history entry
		return true, t.handleMessageSent(false)
	}

	// Send message - quick send
	if key.Matches(msg, t.deps.Keymap.QuickSent) && len(t.input.Value()) > 0 && (t.state == insertMode || t.state == userInspectInsertMode) {
		t.input.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // commit history entry
		return true, t.handleMessageSent(true)
	}

	// Message Accept Suggestion Template Replace
	// always allow accept suggestion key so even new texts can be templated
	if t.input.IsAcceptSuggestionKey(msg) && len(t.input.Value()) > 0 && (t.state == insertMode || t.state == userInspectInsertMode) {
		_, heightChanged := t.input.Update(msg)
		cmd := t.replaceInputTemplate()
		if heightChanged {
			t.HandleResize()
		}
		return true, cmd
	}

	// Set quick time out message to message input
	if key.Matches(msg, t.deps.Keymap.QuickTimeout) && (t.state == inChatWindow || t.state == userInspectMode) {
		t.handleTimeoutShortcut()
		return true, nil
	}

	// Copy selected message to message input
	if key.Matches(msg, t.deps.Keymap.CopyMessage) && (t.state == inChatWindow || t.state == userInspectMode) {
		t.handleCopyMessage()
		return true, nil
	}

	// Reply to selected message
	if key.Matches(msg, t.deps.Keymap.ReplyMessage) && (t.state == inChatWindow || t.state == userInspectMode) {
		t.handleReplyMessage()
		return true, nil
	}

	// Close overlay windows
	if key.Matches(msg, t.deps.Keymap.Escape) {
		var cmd tea.Cmd

		// first end search in user inspect sub window
		if t.userInspect != nil && t.userInspect.chatWindow.state == searchChatWindowState {
			t.userInspect.chatWindow, cmd = t.userInspect.chatWindow.Update(msg)
			return true, cmd
		}

		// second case, end inspect mode or end insert mode in inspect window
		if t.state == userInspectMode || t.state == userInspectInsertMode {
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

func (t *broadcastTab) handleEscapePressed() {
	if t.state == userInspectMode || t.state == emoteOverviewMode {
		t.state = inChatWindow
		t.userInspect = nil
		t.chatWindow.Focus()
		t.HandleResize()
		t.chatWindow.updatePort()
		return
	}

	if t.state == userInspectInsertMode {
		t.state = userInspectMode
		t.userInspect.chatWindow.Focus()
		t.input.Blur()
		t.clearReplyTarget()
		return
	}

	if !t.account.IsAnonymous {
		t.state = inChatWindow
		t.chatWindow.Focus()
		t.input.Blur()
		t.clearReplyTarget()
	}
}

func (t *broadcastTab) handleOpenBrowser(msg tea.KeyPressMsg) tea.Cmd {
	return func() tea.Msg {
		// open popup chat if modifier is pressed
		if key.Matches(msg, t.deps.Keymap.ChatPopUp) {
			t.handleOpenBrowserChatPopUp()()
			return nil
		}

		t.handleOpenBrowserChannel()()
		return nil
	}
}

func (t *broadcastTab) handleOpenBrowserChatPopUp() tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf(streamChatPopUpFmt, t.channelLogin)

		if err := browser.OpenURL(url); err != nil {
			log.Logger.Error().Err(err).Msg("error while opening twitch channel in browser")
		}
		return nil
	}
}

func (t *broadcastTab) handleOpenBrowserChannel() tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf(streamWebFmt, t.channelLogin)

		if err := browser.OpenURL(url); err != nil {
			log.Logger.Error().Err(err).Msg("error while opening twitch channel in browser")
		}
		return nil
	}
}

func (t *broadcastTab) handleStartInsertMode() tea.Cmd {
	if !t.account.IsAnonymous && (t.state == inChatWindow || t.state == userInspectMode) {
		if t.state == inChatWindow {
			t.state = insertMode
		} else {
			t.state = userInspectInsertMode
			t.userInspect.chatWindow.Blur()
		}

		t.chatWindow.Blur()

		return t.input.Focus()
	}

	return nil
}

func (t *broadcastTab) handleCopyMessage() {
	if t.account.IsAnonymous {
		return
	}

	var entry *chatEntry

	if t.state == inChatWindow {
		_, entry = t.chatWindow.entryForCurrentCursor()

		if entry == nil {
			return
		}

		if t.chatWindow.state == searchChatWindowState {
			t.chatWindow.handleStopSearchMode()
		}
		t.chatWindow.Blur()
	} else {
		_, entry = t.userInspect.chatWindow.entryForCurrentCursor()

		if entry == nil {
			return
		}

		if t.userInspect.chatWindow.state == searchChatWindowState {
			t.userInspect.chatWindow.handleStopSearchMode()
		}
		t.userInspect.chatWindow.Blur()
	}

	msg, ok := entry.Event.message.(*twitchirc.PrivateMessage)

	if !ok {
		return
	}

	if t.state == userInspectMode {
		t.state = userInspectInsertMode
	} else {
		t.state = insertMode
	}
	t.clearReplyTarget() // copying replaces the armed reply with different content

	t.input.Focus()
	t.input.SetValue(strings.ReplaceAll(msg.Message, string(duplicateBypass), ""))
	t.HandleResize() // Recalculate layout after copying message
}

// handleReplyMessage arms a reply to the message under the cursor and starts insert mode.
func (t *broadcastTab) handleReplyMessage() {
	if t.account.IsAnonymous {
		return
	}

	var entry *chatEntry

	if t.state == inChatWindow {
		_, entry = t.chatWindow.entryForCurrentCursor()

		if entry == nil {
			return
		}

		if t.chatWindow.state == searchChatWindowState {
			t.chatWindow.handleStopSearchMode()
		}
		t.chatWindow.Blur()
	} else {
		_, entry = t.userInspect.chatWindow.entryForCurrentCursor()

		if entry == nil {
			return
		}

		if t.userInspect.chatWindow.state == searchChatWindowState {
			t.userInspect.chatWindow.handleStopSearchMode()
		}
		t.userInspect.chatWindow.Blur()
	}

	msg, ok := entry.Event.message.(*twitchirc.PrivateMessage)

	// Without an original message ID there is nothing to thread onto, so ignore the
	// keypress instead of arming a reply Twitch would reject.
	if !ok || msg.ID == "" {
		return
	}

	if t.state == userInspectMode {
		t.state = userInspectInsertMode
	} else {
		t.state = insertMode
	}

	t.input.ArmReply(msg.ID, msg.DisplayName, msg.Message)

	t.input.Focus()
	t.HandleResize()
}

func (t *broadcastTab) handleOpenUserInspectFromMessage() tea.Cmd {
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
	switch msg := e.Event.message.(type) {
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

func (t *broadcastTab) handleTimeoutShortcut() {
	if t.account.IsAnonymous {
		return
	}

	var entry *chatEntry

	switch t.state {
	case inChatWindow:
		_, entry = t.chatWindow.entryForCurrentCursor()
	case userInspectMode:
		_, entry = t.userInspect.chatWindow.entryForCurrentCursor()
	}

	if entry == nil {
		return
	}

	msg, ok := entry.Event.message.(*twitchirc.PrivateMessage)

	if !ok {
		return
	}

	if t.state == userInspectMode {
		t.state = userInspectInsertMode
		t.userInspect.chatWindow.handleStopSearchMode()
		t.userInspect.chatWindow.Blur()
	} else {
		t.state = insertMode
		t.chatWindow.handleStopSearchMode()
		t.chatWindow.Blur()
	}

	t.input.Focus()
	t.input.SetValue("/timeout " + msg.DisplayName + " 600")
	t.HandleResize() // Recalculate layout after setting timeout command
}

func (t *broadcastTab) handleOpenEmoteOverview() tea.Cmd {
	if t.account.IsAnonymous {
		return nil
	}

	t.state = emoteOverviewMode

	if t.emoteOverview != nil {
		t.HandleResize()
		return nil
	}

	t.emoteOverview = NewEmoteOverview(t.channelID, t.deps.EmoteCache, t.deps.EmoteReplacer, graphicRows(t.deps.UserConfig.Settings), t.width, t.height)
	t.HandleResize()
	return t.emoteOverview.Init()
}

func (t *broadcastTab) handleManualRefreshEmotes() tea.Cmd {
	if t.account.IsAnonymous {
		return nil
	}

	t.deps.EmoteCache.RemoveEmoteSetForChannel(t.channelID)

	return t.refreshEmotes(t.channelLogin, t.channelID, true)
}
