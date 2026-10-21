package mainui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/rs/zerolog/log"
)

func (t *broadcastTab) handleJoinCommand(args []string) tea.Cmd {
	if len(args) < 1 || args[0] == "" {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: t.account.ID,
				channel:   t.channelLogin,
				channelID: t.channelID,
				tabID:     t.id,
				message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Expected Usage: /join <channel>",
				},
			}
		}
	}

	channel := strings.ToLower(args[0])
	account := t.account
	client := t.client.API

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := client.GetUsers(ctx, []string{channel}, nil)
		if err == nil && len(resp.Data) > 0 {
			channel = resp.Data[0].Login
		}

		return joinChannelMessage{
			tabKind: BroadcastTabKind,
			channel: channel,
			account: account,
		}
	}
}

// handlePyramidMessagesCommand build a message pyramid with the given word and count
// like this:
// word
// word word
// word word word
// word word
// word
func (t *broadcastTab) handlePyramidMessagesCommand(args []string) tea.Cmd {
	accountIsStreamer := t.account.ID == t.channelID

	if !accountIsStreamer && t.statusInfo != nil && t.statusInfo.settings.SlowMode {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: t.account.ID,
				channel:   t.channelLogin,
				channelID: t.channelID,
				tabID:     t.id,
				message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Pyramid command is disabled in slow mode",
				},
			}
		}
	}

	if len(args) < 2 {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: t.account.ID,
				channel:   t.channelLogin,
				channelID: t.channelID,
				tabID:     t.id,
				message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Expected Usage: /pyramid <word> <count>",
				},
			}
		}
	}

	word := args[0]
	count, err := strconv.Atoi(args[1])
	if err != nil {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: t.account.ID,
				channel:   t.channelLogin,
				channelID: t.channelID,
				tabID:     t.id,
				message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Failed to convert count to integer",
				},
			}
		}
	}

	client := t.client.Chat
	if client == nil {
		log.Logger.Warn().Str("account", t.account.DisplayName).Msg("account has no authenticated API client")
		return nil
	}

	broadcasterID := t.channelID
	userID := t.account.ID

	return func() tea.Msg {
		var msgs []string

		for i := 1; i <= count; i++ {
			msgs = append(msgs, strings.Repeat(word+" ", i))
		}

		for i := count - 1; i > 0; i-- {
			msgs = append(msgs, strings.Repeat(word+" ", i))
		}

		var delay time.Duration
		if accountIsStreamer {
			delay = time.Millisecond * 500
		} else {
			delay = time.Millisecond * 1050
		}

		notice := &twitchirc.Notice{
			FakeTimestamp: time.Now(),
		}

		resp := chatEventMessage{
			isFakeEvent: true,
			accountID:   userID,
			channel:     t.channelLogin,
			channelID:   t.channelID,
			tabID:       t.id,
			message:     notice,
		}

		for i, msg := range msgs {
			if i > 0 {
				time.Sleep(delay)
			}

			if i%2 == 0 {
				msg += string(duplicateBypass)
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()

			r, err := client.SendChatMessage(ctx, twitchapi.SendChatMessageRequest{
				BroadcasterID: broadcasterID,
				SenderID:      userID,
				Message:       msg,
			})
			if err != nil {
				notice.Message = fmt.Sprintf("Could not send message: %s", err.Error())
				return resp
			}

			if len(r.Data) > 0 && !r.Data[0].IsSent {
				notice.Message = fmt.Sprintf("Could not send message: %s", r.Data[0].DropReason.Message)
				return resp
			}
		}

		return nil
	}
}

func (t *broadcastTab) handleLocalSubCommand(enable bool) tea.Cmd {
	if enable && t.isLocalSub {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: t.account.ID,
				channel:   t.channelLogin,
				channelID: t.channelID,
				tabID:     t.id,
				message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Already in local submode",
				},
			}
		}
	}

	if !enable && !t.isLocalSub {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: t.account.ID,
				channel:   t.channelLogin,
				channelID: t.channelID,
				tabID:     t.id,
				message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Already out of local submode",
				},
			}
		}
	}

	t.isLocalSub = enable

	return nil
}

func (t *broadcastTab) handleUniqueOnlyChatCommand(enable bool) tea.Cmd {
	if enable && t.isUniqueOnlyChat {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: t.account.ID,
				channel:   t.channelLogin,
				channelID: t.channelID,
				tabID:     t.id,
				message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Already in unique only chat",
				},
			}
		}
	}

	if !enable && !t.isUniqueOnlyChat {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: t.account.ID,
				channel:   t.channelLogin,
				channelID: t.channelID,
				tabID:     t.id,
				message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Already out of unique only chat",
				},
			}
		}
	}

	t.isUniqueOnlyChat = enable

	return nil
}

func (t *broadcastTab) handleCreateClipMessage() tea.Cmd {
	return func() tea.Msg {
		api := t.client.Chat
		if api == nil {
			log.Logger.Warn().Str("broadcast", t.channelLogin).Str("account", t.account.DisplayName).Msg("provided API does not support user authenticated API")
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
		defer cancel()

		clip, err := api.CreateClip(ctx, t.channelID, false)

		notice := &twitchirc.Notice{
			FakeTimestamp: time.Now(),
		}

		resp := chatEventMessage{
			isFakeEvent: true,
			accountID:   t.account.ID,
			channel:     t.channelLogin,
			channelID:   t.channelID,
			tabID:       t.id,
			message:     notice,
		}

		if err != nil {
			apiErr := twitchapi.APIError{}
			if errors.As(err, &apiErr) {
				switch apiErr.Status {
				case http.StatusUnauthorized:
					notice.Message = fmt.Sprintf("@%s Failed to create clip because you are unauthenticated or missing a auth scope; please authenticate again", t.account.DisplayName)
					return resp
				case http.StatusForbidden:
					notice.Message = fmt.Sprintf("@%s Failed to create clip because broadcaster restricted the ability to capture clips", t.account.DisplayName)
					return resp
				case http.StatusNotFound:
					notice.Message = fmt.Sprintf("@%s Failed to create clip because broadcaster is not live", t.account.DisplayName)
					return resp
				}
			}

			notice.Message = fmt.Sprintf("@%s Failed to create clip: %s", t.account.DisplayName, err)
			return resp
		}

		notice.Message = fmt.Sprintf("@%s Created clip can be edited here: %s", t.account.DisplayName, clip.EditURL)
		return resp
	}
}

func (t *broadcastTab) handleOpenUserInspect(args []string) tea.Cmd {
	var cmds []tea.Cmd

	if len(args) < 1 {
		return nil
	}

	username := args[0]

	t.userInspect = newUserInspect(t.id, t.width, t.height, username, t.channelLogin, t.account.ID,
		t.settings, t.theme, t.keymap, t.client.API, t.messageLogger, t.emoteReplacer, t.badgeReplacer)
	t.userInspect.chatWindow.userColorCache = t.chatWindow.userColorCache

	// The table blurs the chat window and focuses the new inspect window.
	_, _ = t.transit(eventInspect)

	initialEvents := make([]chatEventMessage, 0, 15)
	for e := range slices.Values(t.chatWindow.entries) {
		initialEvents = append(initialEvents, chatEventMessage{
			isFakeEvent:             true,
			accountID:               t.account.ID,
			channel:                 t.channelLogin,
			channelID:               t.channelID,
			message:                 e.Event.message,
			displayModifier:         e.Event.displayModifier,
			channelGuestID:          e.Event.channelGuestID,
			channelGuestDisplayName: e.Event.channelGuestDisplayName,
		})
	}

	cmds = append(cmds, t.userInspect.init(initialEvents))
	t.HandleResize()

	return tea.Batch(cmds...)
}
