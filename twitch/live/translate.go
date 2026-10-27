package live

import (
	"strings"
	"time"

	"github.com/julez-dev/chatuino/chatstate"
	"github.com/julez-dev/chatuino/twitch/eventsub"
)

// Translate turns one EventSub notification into a typed domain event. It is a
// pure function of the payload: routing, notices and child state belong to the
// caller. A notification this package does not model is ignored.
func Translate(msg eventsub.Message[eventsub.NotificationPayload]) (Event, bool) {
	subscription := msg.Payload.Subscription
	event := msg.Payload.Event

	switch subscription.Type {
	case "channel.poll.begin":
		return PollStarted{Poll: pollFrom(event)}, true
	case "channel.poll.progress":
		return PollProgress{Poll: pollFrom(event)}, true
	case "channel.poll.end":
		poll := pollFrom(event)
		return PollEnded{Poll: poll, Winner: pollWinner(poll)}, true
	case "channel.prediction.begin":
		return PredictionStarted{Prediction: predictionFrom(event)}, true
	case "channel.prediction.progress":
		return PredictionProgress{Prediction: predictionFrom(event)}, true
	case "channel.prediction.lock":
		return PredictionLocked{Prediction: predictionFrom(event)}, true
	case "channel.prediction.end":
		return PredictionEnded{
			Prediction:       predictionFrom(event),
			Status:           event.Status,
			WinningOutcomeID: event.WinningOutcomeID,
		}, true
	case "channel.channel_points_custom_reward_redemption.add":
		return RedemptionAdded{Redemption: redemptionFrom(event), Custom: true}, true
	case "channel.channel_points_automatic_reward_redemption.add":
		return RedemptionAdded{Redemption: redemptionFrom(event), Custom: false}, true
	case "channel.channel_points_custom_reward_redemption.update":
		return RedemptionUpdated{Redemption: redemptionFrom(event)}, true
	case "channel.unban_request.create":
		request := unbanRequestFrom(event)
		request.Status = "pending"
		return UnbanRequestCreated{Request: request}, true
	case "channel.unban_request.resolve":
		return UnbanRequestResolved{Request: unbanRequestFrom(event)}, true
	case "channel.raid":
		return Raid{
			FromID:   event.FromBroadcasterUserID,
			FromName: preferredDisplayName(event.FromBroadcasterUserName, event.FromBroadcasterUserLogin),
			ToName:   preferredDisplayName(event.ToBroadcasterUserName, event.ToBroadcasterUserLogin),
			Viewers:  event.Viewers,
		}, true
	case "channel.chat_settings.update":
		return ChatSettingsChanged{Settings: chatstate.SettingsFromUpdate(event)}, true
	case "channel.warning.send":
		return WarningSent{
			Moderator: preferredDisplayName(event.ModeratorUserName, event.ModeratorUserLogin),
			User:      preferredDisplayName(event.UserName, event.UserLogin),
			Reason:    event.Reason,
		}, true
	case "channel.warning.acknowledge":
		return WarningAcknowledged{
			User: preferredDisplayName(event.UserName, event.UserLogin),
		}, true
	case "channel.shoutout.create":
		return ShoutoutSent{
			Target:  preferredDisplayName(event.ToBroadcasterUserName, event.ToBroadcasterUserLogin),
			Viewers: event.ViewerCount,
		}, true
	case "channel.shoutout.receive":
		return ShoutoutReceived{
			From:    preferredDisplayName(event.FromBroadcasterUserName, event.FromBroadcasterUserLogin),
			Viewers: event.ViewerCount,
		}, true
	case "automod.message.hold":
		return AutoModHeld{AutoMod: autoModFrom(event)}, true
	case "automod.message.update":
		return AutoModUpdated{AutoMod: autoModFrom(event)}, true
	case "channel.ad_break.begin":
		return AdBreakStarted{
			Duration:  time.Duration(event.DurationInSeconds) * time.Second,
			Automatic: event.IsAutomatic,
			Requester: event.RequesterUserName,
		}, true
	}

	return nil, false
}

func pollFrom(event eventsub.Event) Poll {
	poll := Poll{
		Title:   event.Title,
		Choices: make([]PollChoice, 0, len(event.Choices)),
	}

	for _, choice := range event.Choices {
		poll.Choices = append(poll.Choices, PollChoice{
			ID:    choice.ID,
			Title: choice.Title,
			Votes: choice.Votes,
		})
	}

	return poll
}

// pollWinner returns the choice with the most votes, or nil when the poll has no
// choices. A malformed end event must not turn into an index panic.
func pollWinner(poll Poll) *PollChoice {
	if len(poll.Choices) == 0 {
		return nil
	}

	winner := poll.Choices[0]
	for _, choice := range poll.Choices[1:] {
		if choice.Votes > winner.Votes {
			winner = choice
		}
	}

	return &winner
}

func predictionFrom(event eventsub.Event) Prediction {
	prediction := Prediction{
		Title:    event.Title,
		LocksAt:  event.LocksAt,
		Outcomes: make([]PredictionOutcome, 0, len(event.Outcomes)),
	}

	for _, outcome := range event.Outcomes {
		prediction.Outcomes = append(prediction.Outcomes, PredictionOutcome{
			ID:     outcome.ID,
			Title:  outcome.Title,
			Points: outcome.ChannelPoints,
		})
	}

	return prediction
}

func redemptionFrom(event eventsub.Event) Redemption {
	user := event.UserName
	if user == "" {
		user = event.UserLogin
	}

	return Redemption{
		ID:          event.ID,
		User:        user,
		Reward:      event.Reward.Title,
		Cost:        event.Reward.Cost,
		UserInput:   event.UserInput,
		MessageText: event.Message.Text,
		Status:      event.Status,
	}
}

func unbanRequestFrom(event eventsub.Event) UnbanRequest {
	return UnbanRequest{
		ID:              event.ID,
		BroadcasterID:   event.BroadcasterUserID,
		BroadcasterName: event.BroadcasterUserName,
		UserID:          event.UserID,
		UserLogin:       event.UserLogin,
		UserName:        event.UserName,
		Text:            event.Text,
		Status:          event.Status,
		ResolutionText:  event.ResolutionText,
		ModeratorID:     event.ModeratorID,
		ModeratorLogin:  event.ModeratorLogin,
		ModeratorName:   event.ModeratorName,
		CreatedAt:       event.CreatedAt,
	}
}

func autoModFrom(event eventsub.Event) AutoMod {
	reason := event.Reason
	if event.Category != "" {
		reason = event.Category
	}

	return AutoMod{
		MessageID: event.MessageID,
		User:      preferredDisplayName(event.UserName, event.UserLogin),
		Reason:    reason,
		Category:  event.Category,
		Status:    event.Status,
		Message:   strings.TrimSpace(event.Message.Text),
		Level:     event.Level,
	}
}

// preferredDisplayName keeps the display name when Twitch sent one, then the
// login, and finally a readable fallback.
func preferredDisplayName(name, login string) string {
	if name != "" {
		return name
	}

	if login != "" {
		return login
	}

	return "someone"
}
