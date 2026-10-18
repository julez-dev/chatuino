package mainui

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jellydator/ttlcache/v3"

	"github.com/julez-dev/chatuino/emote"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/component"
	"github.com/julez-dev/chatuino/wspool"
	"github.com/rs/zerolog/log"
)

func (t *broadcastTab) Update(msg tea.Msg) (tab, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case error:
		if !t.channelDataLoaded {
			return t, nil
		}

		return t, func() tea.Msg {
			return requestLocalMessageHandleMessage{
				tabID:     t.id,
				accountID: t.AccountID(),
				message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       msg.Error(),
				},
			}
		}
	case setErrorMessage:
		if msg.targetID != t.id {
			return t, nil
		}

		t.err = errors.Join(t.err, msg.err)
		return t, nil
	case setStreamInfoMessage:
		if t.channelDataLoaded {
			if msg.target != t.channelID {
				return t, nil
			}

			t.streamInfo, cmd = t.streamInfo.Update(msg)
			t.HandleResize()
			return t, cmd
		}
	case setChannelDataMessage:
		if msg.targetID != t.id {
			return t, nil
		}

		t.channelDataLoaded = true

		t.channelLogin = msg.channelLogin
		t.channelID = msg.channelID
		t.streamInfo = newStreamInfo(msg.channelID, t.deps.APIUserClients[t.account.ID], t.width)
		t.poll = newPoll(t.width)
		t.chatWindow = newChatWindow(t.width, t.height, t.deps)

		t.messageInput = component.NewSuggestionTextInput(t.chatWindow.userColorCache, t.deps.UserConfig.Settings.BuildCustomSuggestionMap())
		// enable inline emote replacement; the single line input can only display single row images
		if graphicRows(t.deps.UserConfig.Settings) <= 1 {
			t.messageInput.EmoteReplacer = t.deps.EmoteReplacer
		}
		msgInputStyles := t.messageInput.InputModel.Styles()
		msgInputStyles.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color(t.deps.UserConfig.Theme.InputPromptColor))
		t.messageInput.InputModel.SetStyles(msgInputStyles)
		t.messageInput.SetMaxVisibleLines(3) // allow input to grow up to 3 lines

		if len(t.pendingChannelSuggestions) > 0 {
			t.messageInput.SetChannelSuggestions(t.pendingChannelSuggestions)
			t.pendingChannelSuggestions = nil
		}

		t.statusInfo = newStreamStatus(t.width, t.height, t, t.account.ID, msg.channelID, t.deps)

		// set chat suggestions if non-anonymous user
		if !t.account.IsAnonymous {
			t.isUserMod = msg.isUserMod
			t.isModStatusAssumed = msg.modFetchErr != nil

			// if user is broadcaster, allow mod commands
			if t.account.ID == msg.channelID {
				t.isUserMod = true
				t.isModStatusAssumed = false
			}

			// user is mod or broadcaster, include mod commands
			if t.isUserMod {
				t.messageInput.IncludeModeratorCommands = true
			}
		}

		if t.focused {
			t.chatWindow.Focus()
		}

		ircCmds := make([]tea.Cmd, 0, 3)

		if msg.modFetchErr != nil {
			msg.initialMessages = append(msg.initialMessages, &twitchirc.Notice{
				FakeTimestamp:   time.Now(),
				ChannelUserName: t.channelLogin,
				MsgID:           twitchirc.MsgID(uuid.NewString()),
				Message:         fmt.Sprintf("Could not fetch mod status, assuming mod. (%s)", msg.modFetchErr),
			})
		}

		// notify user about loaded messages
		msg.initialMessages = append(msg.initialMessages, &twitchirc.Notice{
			FakeTimestamp:   time.Now(),
			ChannelUserName: t.channelLogin,
			MsgID:           twitchirc.MsgID(uuid.NewString()),
			Message:         fmt.Sprintf("Loaded %d recent messages; powered by https://recent-messages.robotty.de", len(msg.initialMessages)),
		})

		// Pass recent messages, recorded before the application was started, to chat window
		// all irc commands will be processed as a sequence. This means all remote messages should be handled before the join irc command
		// is sent. This should keep the message order consistent.
		ircCmds = append(ircCmds, func() tea.Msg {
			return requestLocalMessageHandleBatchMessage{
				messages:  msg.initialMessages,
				tabID:     t.id,
				accountID: t.account.ID,
			}
		})

		// Connect to IRC and join channel via Pool
		accountID := t.account.ID
		channelLogin := msg.channelLogin
		ircCmds = append(ircCmds, func() tea.Msg {
			t.deps.Pool.ConnectIRC(accountID)
			t.deps.Pool.JoinChannel(accountID, channelLogin)
			return nil
		})

		cmds = append(cmds, t.refreshEmotes(msg.channelLogin, msg.channelID, false))

		// subscribe to channel events
		//  - if authenticated user
		//  - if channel belongs to user
		// sadly due to cost limits, we only allow this events users channel not other channels
		if eventSubAPI, ok := t.deps.APIUserClients[t.account.ID].(wspool.EventSubService); ok && t.account.ID == msg.channelID {
			accountID := t.account.ID
			channelID := msg.channelID

			for _, subType := range [...]string{"channel.poll.begin", "channel.poll.progress", "channel.poll.end", "channel.ad_break.begin"} {
				subType := subType // capture for closure
				cmds = append(cmds, func() tea.Msg {
					t.deps.Pool.SubscribeEventSub(accountID, twitchapi.CreateEventSubSubscriptionRequest{
						Type:    subType,
						Version: "1",
						Condition: map[string]string{
							"broadcaster_user_id": channelID,
						},
					}, eventSubAPI)
					return nil
				})
			}

			cmds = append(cmds, func() tea.Msg {
				t.deps.Pool.SubscribeEventSub(accountID, twitchapi.CreateEventSubSubscriptionRequest{
					Type:    "channel.raid",
					Version: "1",
					Condition: map[string]string{
						"to_broadcaster_user_id": channelID, // broadcaster gets raided
					},
				}, eventSubAPI)
				return nil
			})

			cmds = append(cmds, func() tea.Msg {
				t.deps.Pool.SubscribeEventSub(accountID, twitchapi.CreateEventSubSubscriptionRequest{
					Type:    "channel.raid",
					Version: "1",
					Condition: map[string]string{
						"from_broadcaster_user_id": channelID, // another channel gets raided from broadcaster
					},
				}, eventSubAPI)
				return nil
			})
		}

		t.HandleResize()
		cmds = append(cmds, t.streamInfo.Init(), t.statusInfo.Init(), tea.Sequence(ircCmds...))
		return t, tea.Batch(cmds...)
	case emoteSetRefreshedMessage:
		if !t.account.IsAnonymous && msg.targetID == t.id {
			if msg.err != nil && !errors.Is(msg.err, emote.ErrPartialFetch) {
				t.err = errors.Join(t.err, msg.err)
				return t, nil
			}

			userEmoteSet := t.deps.EmoteCache.AllEmotesUsableByUser(t.account.ID)

			log.Info().Str("user-id", t.account.ID).Int("len", len(userEmoteSet)).Msg("fetched emotes for user")

			channelEmoteSet := t.deps.EmoteCache.GetAllForChannel(t.channelID) // includes bttv, 7tv

			unique := make(map[string]struct{}, len(userEmoteSet)+len(channelEmoteSet))

			for _, emote := range userEmoteSet {
				unique[emote.Text] = struct{}{}
			}

			for _, e := range channelEmoteSet {
				// We want to set all emotes available to the user. This means we shouldn't set twitch sub emotes and others like follower and bits that
				// the user doesn't have access to.
				// All twitch emotes for the current channel to which the users has access to should be in the userEmoteSet, since they are returned from the API.
				// So we only include all 3rd pary emotes here
				if e.Platform == emote.Twitch {
					continue
				}

				unique[e.Text] = struct{}{}
			}

			suggestions := slices.Collect(maps.Keys(unique))
			t.messageInput.SetSuggestions(suggestions)

			// notify user if not all emotes could be fetched
			if errors.Is(msg.err, emote.ErrPartialFetch) {
				return t, func() tea.Msg {
					return requestLocalMessageHandleMessage{
						tabID:     t.id,
						accountID: t.AccountID(),
						message: &twitchirc.Notice{
							FakeTimestamp: time.Now(),
							Message:       msg.err.Error(),
						},
					}
				}
			}

			if msg.manually {
				return t, func() tea.Msg {
					return requestLocalMessageHandleMessage{
						tabID:     t.id,
						accountID: t.AccountID(),
						message: &twitchirc.Notice{
							FakeTimestamp: time.Now(),
							Message:       "Emotes refreshed manually",
						},
					}
				}
			}
		}

		return t, nil
	case channelSuggestionsLoadedMessage:
		if msg.targetID == t.id {
			if t.messageInput != nil {
				t.messageInput.SetChannelSuggestions(msg.channels)
			} else {
				t.pendingChannelSuggestions = msg.channels
			}
		}
		return t, nil
	case wspool.EventSubEvent:
		if msg.Error != nil {
			log.Logger.Err(msg.Error).Msg("EventSub error")
			return t, nil
		}
		cmd = t.handleEventSubMessage(msg.Message)
		return t, cmd
	case chatEventMessage: // delegate message event to chat window
		// ignore all messages that don't target this account and channel

		if t.AccountID() != msg.accountID || t.channelLogin != msg.channel && msg.channel != "" {
			return t, nil
		}

		if t.channelDataLoaded {
			if t.shouldIgnoreMessage(msg.message) {
				return t, nil
			}

			if msg, ok := msg.message.(*twitchirc.PrivateMessage); ok {
				if messageContainsCaseInsensitive(msg, t.account.DisplayName) {
					cmds = append(cmds, func() tea.Msg {
						return requestNotificationIconMessage{
							tabID: t.id,
						}
					})
				}
			}

			t.chatWindow, cmd = t.chatWindow.Update(msg)
			cmds = append(cmds, cmd)

			// if room state update, update status info
			if _, ok := msg.message.(*twitchirc.RoomState); ok {
				cmds = append(cmds, t.statusInfo.Init()) // resend init command
			}

			if t.state == userInspectMode {
				t.userInspect, cmd = t.userInspect.Update(msg)
				cmds = append(cmds, cmd)
			}

			// add message content to cache
			if cast, ok := msg.message.(*twitchirc.PrivateMessage); ok {
				t.lastMessages.Set(cast.Message, struct{}{}, ttlcache.DefaultTTL)
			}

		}

		return t, tea.Batch(cmds...)
	}

	if t.channelDataLoaded {
		if t.focused {
			switch msg := msg.(type) {
			case tea.KeyPressMsg:
				// Focus message input, when not in insert mode and not in search mode inside chat window, depending on the current active chat window
				if key.Matches(msg, t.deps.Keymap.InsertMode) &&
					(t.state == inChatWindow && t.chatWindow.state != searchChatWindowState || t.state == userInspectMode && t.userInspect.chatWindow.state != searchChatWindowState) {
					cmd := t.handleStartInsertMode()
					cmds = append(cmds, cmd)
					return t, tea.Batch(cmds...)
				}

				// Open user inspect mode, where only messages from a specific user are shown
				if key.Matches(msg, t.deps.Keymap.InspectMode) && (t.state == inChatWindow || t.state == userInspectMode) {
					cmd := t.handleOpenUserInspectFromMessage()
					cmds = append(cmds, cmd)
					return t, tea.Batch(cmds...)
				}

				// Open chat in browser
				if key.Matches(msg, t.deps.Keymap.ChatPopUp, t.deps.Keymap.ChannelPopUp) && (t.state == inChatWindow || t.state == userInspectMode) {
					return t, t.handleOpenBrowser(msg)
				}

				// Send message
				if key.Matches(msg, t.deps.Keymap.Confirm) && len(t.messageInput.Value()) > 0 && (t.state == insertMode || t.state == userInspectInsertMode) {
					t.messageInput, _ = t.messageInput.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
					return t, t.handleMessageSent(false)
				}

				// Send message - quick send
				if key.Matches(msg, t.deps.Keymap.QuickSent) && len(t.messageInput.Value()) > 0 && (t.state == insertMode || t.state == userInspectInsertMode) {
					t.messageInput, _ = t.messageInput.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
					return t, t.handleMessageSent(true)
				}

				// Message Accept Suggestion Template Replace
				// always allow accept suggestion key so even new texts can be templated
				if key.Matches(msg, t.messageInput.KeyMap.AcceptSuggestion) && len(t.messageInput.Value()) > 0 && (t.state == insertMode || t.state == userInspectInsertMode) {
					lineCountBefore := t.messageInput.LineCount()
					t.messageInput, _ = t.messageInput.Update(msg)
					cmds = append(cmds, t.replaceInputTemplate())
					if t.messageInput.LineCount() != lineCountBefore {
						t.HandleResize()
					}
					return t, tea.Batch(cmds...)
				}

				// Set quick time out message to message input
				if key.Matches(msg, t.deps.Keymap.QuickTimeout) && (t.state == inChatWindow || t.state == userInspectMode) {
					t.handleTimeoutShortcut()
					return t, nil
				}

				// Copy selected message to message input
				if key.Matches(msg, t.deps.Keymap.CopyMessage) && (t.state == inChatWindow || t.state == userInspectMode) {
					t.handleCopyMessage()
					return t, nil
				}

				// Reply to selected message
				if key.Matches(msg, t.deps.Keymap.ReplyMessage) && (t.state == inChatWindow || t.state == userInspectMode) {
					t.handleReplyMessage()
					return t, nil
				}

				// Close overlay windows
				if key.Matches(msg, t.deps.Keymap.Escape) {
					// first end search in user inspect sub window
					if t.userInspect != nil && t.userInspect.chatWindow.state == searchChatWindowState {
						t.userInspect.chatWindow, cmd = t.userInspect.chatWindow.Update(msg)
						cmds = append(cmds, cmd)
						return t, tea.Batch(cmds...)
					}

					// second case, end inspect mode or end insert mode in inspect window
					if t.state == userInspectMode || t.state == userInspectInsertMode {
						t.handleEscapePressed()
						return t, nil
					}

					// third case, end search in 'main' chat window
					if t.chatWindow.state == searchChatWindowState {
						t.chatWindow, cmd = t.chatWindow.Update(msg)
						cmds = append(cmds, cmd)
						return t, tea.Batch(cmds...)
					}

					t.handleEscapePressed()
					return t, nil
				}
			}

			if t.state == insertMode || t.state == userInspectInsertMode {
				// Track line count before update to detect changes
				lineCountBefore := t.messageInput.LineCount()

				t.messageInput, cmd = t.messageInput.Update(msg)
				cmds = append(cmds, cmd)

				// Recalculate layout if input line count changed (text wrapped/unwrapped)
				if t.messageInput.LineCount() != lineCountBefore {
					t.HandleResize()
				}
			}
		}

		// don't update any components when key message but not focused
		if _, ok := msg.(tea.KeyPressMsg); ok && !t.focused {
			return t, nil
		}

		if t.state != emoteOverviewMode {
			t.chatWindow, cmd = t.chatWindow.Update(msg)
			cmds = append(cmds, cmd)
		}

		t.streamInfo, cmd = t.streamInfo.Update(msg)
		cmds = append(cmds, cmd)

		t.statusInfo, cmd = t.statusInfo.Update(msg)
		cmds = append(cmds, cmd)

		if t.emoteOverview != nil {
			_, ok := msg.(emoteOverviewSetDataMessage)

			// allow emoteOverviewSetDataMessage even when no longer in state
			if ok || t.state == emoteOverviewMode {
				t.emoteOverview, cmd = t.emoteOverview.Update(msg)
				cmds = append(cmds, cmd)
			}
		}

		if t.state == userInspectMode {
			t.userInspect, cmd = t.userInspect.Update(msg)
			cmds = append(cmds, cmd)
		}
	} else {
		t.spinner, cmd = t.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return t, tea.Batch(cmds...)
}
