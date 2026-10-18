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
		t.messageInput.Blur()
		t.clearReplyTarget()
		return
	}

	if !t.account.IsAnonymous {
		t.state = inChatWindow
		t.chatWindow.Focus()
		t.messageInput.Blur()
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

		t.messageInput.Focus()
		t.chatWindow.Blur()

		return t.messageInput.InputModel.Focus()
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

	t.messageInput.Focus()
	t.messageInput.SetValue(strings.ReplaceAll(msg.Message, string(duplicateBypass), ""))
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

	t.replyTo = &replyTarget{
		MessageID:   msg.ID,
		DisplayName: msg.DisplayName,
		Snippet:     replySnippet(msg.Message),
	}

	t.messageInput.Focus()
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

	t.messageInput.Focus()
	t.messageInput.SetValue("/timeout " + msg.DisplayName + " 600")
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
