package tab

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
)

func (t *Broadcast) handleEventSubMessage(msg eventsub.Message[eventsub.NotificationPayload]) tea.Cmd {
	if msg.Payload.Subscription.Condition["broadcaster_user_id"] != t.channelID &&
		msg.Payload.Subscription.Condition["from_broadcaster_user_id"] != t.channelID &&
		msg.Payload.Subscription.Condition["to_broadcaster_user_id"] != t.channelID {
		return nil
	}

	createCMDFunc := func(ircer twitchirc.IRCer) tea.Cmd {
		return func() tea.Msg {
			return RequestLocalMessageHandleMessage{
				Message:   ircer,
				AccountID: t.AccountID(),
			}
		}
	}

	switch msg.Payload.Subscription.Type {
	case "channel.poll.begin":
		t.poll.setPollData(msg)
		t.poll.enabled = true
		t.HandleResize()
		return createCMDFunc(
			&twitchirc.Notice{
				FakeTimestamp:   time.Now(),
				ChannelUserName: t.channelLogin,
				MsgID:           twitchirc.MsgID(uuid.NewString()),
				Message:         fmt.Sprintf("Poll %q has started!", msg.Payload.Event.Title),
			},
		)
	case "channel.poll.progress":
		heightBefore := lipgloss.Height(t.poll.View())
		t.poll.setPollData(msg)
		t.poll.enabled = true
		heightAfter := lipgloss.Height(t.poll.View())

		if heightAfter != heightBefore {
			t.HandleResize()
		}
	case "channel.poll.end":
		winner := msg.Payload.Event.Choices[0]

		for _, choice := range msg.Payload.Event.Choices {
			if choice.Votes > winner.Votes {
				winner = choice
			}
		}

		t.poll.enabled = false
		t.HandleResize()

		return createCMDFunc(
			&twitchirc.Notice{
				FakeTimestamp:   time.Now(),
				ChannelUserName: t.channelLogin,
				MsgID:           twitchirc.MsgID(uuid.NewString()),
				Message:         fmt.Sprintf("Poll %q has ended, %q has won with %d votes!", msg.Payload.Event.Title, winner.Title, winner.Votes),
			},
		)
	case "channel.raid":
		// broadcaster raided another channel
		if msg.Payload.Event.FromBroadcasterUserID == t.channelID {
			return createCMDFunc(
				&twitchirc.Notice{
					FakeTimestamp:   time.Now(),
					ChannelUserName: t.channelLogin,
					MsgID:           twitchirc.MsgID(uuid.NewString()),
					Message:         fmt.Sprintf("Raiding %s with %d Viewers!", msg.Payload.Event.ToBroadcasterUserName, msg.Payload.Event.Viewers),
				},
			)
		}

		// broadcaster gets raided
		return createCMDFunc(
			&twitchirc.Notice{
				FakeTimestamp:   time.Now(),
				ChannelUserName: t.channelLogin,
				MsgID:           twitchirc.MsgID(uuid.NewString()),
				Message:         fmt.Sprintf("You are getting raided by %s with %d Viewers!", msg.Payload.Event.FromBroadcasterUserName, msg.Payload.Event.Viewers),
			},
		)
	case "channel.ad_break.begin":
		var chatMsg string

		adDuration := humanizeDuration(time.Duration(msg.Payload.Event.DurationInSeconds) * time.Second)
		if msg.Payload.Event.IsAutomatic {
			chatMsg = fmt.Sprintf("An automatic %s ad just started!", adDuration)
		} else {
			chatMsg = fmt.Sprintf("A %s ad, requested by %s, just started!", adDuration, msg.Payload.Event.RequesterUserName)
		}

		return createCMDFunc(
			&twitchirc.Notice{
				FakeTimestamp:   time.Now(),
				ChannelUserName: t.channelLogin,
				MsgID:           twitchirc.MsgID(uuid.NewString()),
				Message:         chatMsg,
			},
		)
	}

	return nil
}
