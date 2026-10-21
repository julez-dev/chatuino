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

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/component"
	"github.com/julez-dev/chatuino/wspool"
	"github.com/rs/zerolog/log"
)

func (t *broadcastTab) Update(msg tea.Msg) (tab, tea.Cmd) {
	switch msg := msg.(type) {
	case error:
		if !t.channelDataLoaded {
			return t, nil
		}

		return t, t.localNoticeCmd(msg.Error())
	case setErrorMessage:
		t.err = errors.Join(t.err, msg.err)
		return t, nil
	case setStreamInfoMessage:
		if t.channelDataLoaded {
			if msg.target != t.channelID {
				return t, nil
			}

			var cmd tea.Cmd
			t.streamInfo, cmd = t.streamInfo.Update(msg)
			t.HandleResize()
			return t, cmd
		}
	case setChannelDataMessage:
		return t, t.handleChannelDataLoaded(msg)
	case emoteSetRefreshedMessage:
		return t, t.handleEmoteSetRefreshed(msg)
	case channelSuggestionsLoadedMessage:
		if t.input != nil {
			t.input.SetChannelSuggestions(msg.channels)
		} else {
			t.pendingChannelSuggestions = msg.channels
		}
		return t, nil
	case wspool.EventSubEvent:
		if msg.Error != nil {
			log.Logger.Err(msg.Error).Msg("EventSub error")
			return t, nil
		}
		return t, t.handleEventSubMessage(msg.Message)
	case chatEventMessage: // delegate message event to chat window
		return t, t.handleChatEvent(msg)
	}

	// while the channel data is loading, only the spinner is animated
	if !t.channelDataLoaded {
		var cmd tea.Cmd
		t.spinner, cmd = t.spinner.Update(msg)
		return t, cmd
	}

	var cmds []tea.Cmd

	if t.focused {
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
			if handled, cmd := t.handleKeyPress(keyMsg); handled {
				return t, cmd
			}
		}

		if t.state.isComposing() {
			cmd, heightChanged := t.input.Update(msg)
			cmds = append(cmds, cmd)

			// Recalculate layout if input height changed (text wrapped/unwrapped)
			if heightChanged {
				t.HandleResize()
			}
		}
	}

	// don't update any components when key message but not focused
	if _, ok := msg.(tea.KeyPressMsg); ok && !t.focused {
		return t, nil
	}

	cmds = append(cmds, t.updateChildren(msg))

	return t, tea.Batch(cmds...)
}

// handleChannelDataLoaded finishes the tab setup once the initial channel data has
// been fetched: it creates all child components, replays recent messages, joins the
// IRC channel and subscribes to EventSub events.
func (t *broadcastTab) handleChannelDataLoaded(msg setChannelDataMessage) tea.Cmd {
	var cmds []tea.Cmd

	t.channelDataLoaded = true

	t.channelLogin = msg.channelLogin
	t.channelID = msg.channelID
	t.streamInfo = newStreamInfo(msg.channelID, t.client.API, t.width)
	t.poll = newPoll(t.width)
	t.chatWindow = newChatWindow(t.width, t.height, t.settings, t.theme, t.keymap)

	// enable inline emote replacement; the single line input can only display single row images
	var emoteReplacer component.Replacer
	if graphicRows(t.settings) <= 1 {
		emoteReplacer = t.emoteReplacer
	}
	t.input = newChatInput(t.chatWindow.userColorCache, t.settings.BuildCustomSuggestionMap(), t.theme, emoteReplacer)
	t.input.SetSuggestionKeyMap(t.keymap)

	if len(t.pendingChannelSuggestions) > 0 {
		t.input.SetChannelSuggestions(t.pendingChannelSuggestions)
		t.pendingChannelSuggestions = nil
	}

	t.statusInfo = newStreamStatus(t.width, t.height, t, t.account.ID, msg.channelID, t.theme, t.client.API)

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
			t.input.SetIncludeModeratorCommands(true)
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
		t.pool.ConnectIRC(accountID)
		t.pool.JoinChannel(accountID, channelLogin)
		return nil
	})

	cmds = append(cmds, t.refreshEmotes(msg.channelLogin, msg.channelID, false))
	cmds = append(cmds, t.subscribeChannelEventCmds(msg.channelID)...)

	t.HandleResize()
	cmds = append(cmds, t.streamInfo.Init(), t.statusInfo.Init(), tea.Sequence(ircCmds...))
	return tea.Batch(cmds...)
}

// subscribeChannelEventCmds returns the commands that subscribe to the channel's
// EventSub events (polls, raids, ad breaks).
//
// Subscriptions are only created
//   - if authenticated user
//   - if channel belongs to user
//
// sadly due to cost limits, we only allow this events users channel not other channels
func (t *broadcastTab) subscribeChannelEventCmds(channelID string) []tea.Cmd {
	eventSubAPI := t.client.EventSub
	if eventSubAPI == nil || t.account.ID != channelID {
		return nil
	}

	var cmds []tea.Cmd
	accountID := t.account.ID

	for _, subType := range [...]string{"channel.poll.begin", "channel.poll.progress", "channel.poll.end", "channel.ad_break.begin"} {
		subType := subType // capture for closure
		cmds = append(cmds, func() tea.Msg {
			t.pool.SubscribeEventSub(accountID, twitchapi.CreateEventSubSubscriptionRequest{
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
		t.pool.SubscribeEventSub(accountID, twitchapi.CreateEventSubSubscriptionRequest{
			Type:    "channel.raid",
			Version: "1",
			Condition: map[string]string{
				"to_broadcaster_user_id": channelID, // broadcaster gets raided
			},
		}, eventSubAPI)
		return nil
	})

	cmds = append(cmds, func() tea.Msg {
		t.pool.SubscribeEventSub(accountID, twitchapi.CreateEventSubSubscriptionRequest{
			Type:    "channel.raid",
			Version: "1",
			Condition: map[string]string{
				"from_broadcaster_user_id": channelID, // another channel gets raided from broadcaster
			},
		}, eventSubAPI)
		return nil
	})

	return cmds
}

// handleEmoteSetRefreshed reacts to a finished emote refresh by recomputing the
// input suggestions from the user's and the channel's emote sets.
func (t *broadcastTab) handleEmoteSetRefreshed(msg emoteSetRefreshedMessage) tea.Cmd {
	if t.account.IsAnonymous {
		return nil
	}

	if msg.err != nil && !errors.Is(msg.err, emote.ErrPartialFetch) {
		t.err = errors.Join(t.err, msg.err)
		return nil
	}

	userEmoteSet := t.emoteCache.AllEmotesUsableByUser(t.account.ID)

	log.Info().Str("user-id", t.account.ID).Int("len", len(userEmoteSet)).Msg("fetched emotes for user")

	channelEmoteSet := t.emoteCache.GetAllForChannel(t.channelID) // includes bttv, 7tv

	t.input.SetSuggestions(emoteSuggestions(userEmoteSet, channelEmoteSet))

	// notify user if not all emotes could be fetched
	if errors.Is(msg.err, emote.ErrPartialFetch) {
		return t.localNoticeCmd(msg.err.Error())
	}

	if msg.manually {
		return t.localNoticeCmd("Emotes refreshed manually")
	}

	return nil
}

// emoteSuggestions computes the input suggestion list from the emotes the user can
// use and the channel's third party emotes.
//
// We want to suggest all emotes available to the user. This means we shouldn't
// include twitch sub emotes and others like follower and bits that the user
// doesn't have access to. All twitch emotes for the current channel to which the
// user has access to should be in userEmoteSet, since they are returned from the
// API. So only third party emotes are taken from the channel set.
func emoteSuggestions(userEmoteSet []emote.Emote, channelEmoteSet emote.EmoteSet) []string {
	unique := make(map[string]struct{}, len(userEmoteSet)+len(channelEmoteSet))

	for _, e := range userEmoteSet {
		unique[e.Text] = struct{}{}
	}

	for _, e := range channelEmoteSet {
		if e.Platform == emote.Twitch {
			continue
		}

		unique[e.Text] = struct{}{}
	}

	return slices.Collect(maps.Keys(unique))
}

// handleChatEvent forwards a chat event to the chat window and dependent
// components, unless the message is filtered by the tab's ignore rules.
func (t *broadcastTab) handleChatEvent(msg chatEventMessage) tea.Cmd {
	// ignore all messages that don't target this account and channel
	if t.AccountID() != msg.accountID || t.channelLogin != msg.channel && msg.channel != "" {
		return nil
	}

	if !t.channelDataLoaded {
		return nil
	}

	if t.shouldIgnoreMessage(msg.message) {
		return nil
	}

	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

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

	return tea.Batch(cmds...)
}

// updateChildren fans a message out to all child components that are active in the
// tab's current state.
func (t *broadcastTab) updateChildren(msg tea.Msg) tea.Cmd {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

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

	return tea.Batch(cmds...)
}
