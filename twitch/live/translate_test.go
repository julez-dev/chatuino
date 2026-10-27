package live

import (
	"testing"
	"time"

	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/stretchr/testify/require"
)

func notification(topic string, event eventsub.Event) eventsub.Message[eventsub.NotificationPayload] {
	return eventsub.Message[eventsub.NotificationPayload]{
		Payload: eventsub.NotificationPayload{
			Subscription: eventsub.Subscription{Type: topic},
			Event:        event,
		},
	}
}

func TestTranslate_UnknownTopicIsIgnored(t *testing.T) {
	t.Parallel()

	event, ok := Translate(notification("channel.follow", eventsub.Event{}))
	require.False(t, ok)
	require.Nil(t, event)
}

func TestTranslate_PollEvents(t *testing.T) {
	t.Parallel()

	event, ok := Translate(notification("channel.poll.begin", eventsub.Event{Title: "Best game"}))
	require.True(t, ok)
	started, ok := event.(PollStarted)
	require.True(t, ok)
	require.Equal(t, "Best game", started.Poll.Title)

	event, ok = Translate(notification("channel.poll.progress", eventsub.Event{
		Title: "Best game",
		Choices: []eventsub.Choice{
			{ID: "a", Title: "A", Votes: 3},
			{ID: "b", Title: "B", Votes: 7},
		},
	}))
	require.True(t, ok)
	progress, ok := event.(PollProgress)
	require.True(t, ok)
	require.Equal(t, "Best game", progress.Poll.Title)
	require.Len(t, progress.Poll.Choices, 2)

	event, ok = Translate(notification("channel.poll.end", eventsub.Event{
		Title: "Best game",
		Choices: []eventsub.Choice{
			{ID: "a", Title: "A", Votes: 3},
			{ID: "b", Title: "B", Votes: 7},
		},
	}))
	require.True(t, ok)
	ended, ok := event.(PollEnded)
	require.True(t, ok)
	require.NotNil(t, ended.Winner)
	require.Equal(t, "b", ended.Winner.ID)
	require.Equal(t, 7, ended.Winner.Votes)
}

func TestTranslate_PollEndWithoutChoicesDoesNotPanic(t *testing.T) {
	t.Parallel()

	event, ok := Translate(notification("channel.poll.end", eventsub.Event{Title: "Empty"}))
	require.True(t, ok)

	ended, ok := event.(PollEnded)
	require.True(t, ok)
	require.Nil(t, ended.Winner)
}

func TestTranslate_PredictionEvents(t *testing.T) {
	t.Parallel()

	locksAt := time.Now().Add(time.Minute)
	payload := eventsub.Event{
		Title:   "Will we win?",
		LocksAt: locksAt,
		Outcomes: []eventsub.Outcome{
			{ID: "yes", Title: "Yes", ChannelPoints: 10},
			{ID: "no", Title: "No", ChannelPoints: 5},
		},
	}

	for _, topic := range []string{
		"channel.prediction.begin",
		"channel.prediction.progress",
		"channel.prediction.lock",
	} {
		event, ok := Translate(notification(topic, payload))
		require.True(t, ok, topic)

		var prediction Prediction
		switch typed := event.(type) {
		case PredictionStarted:
			prediction = typed.Prediction
		case PredictionProgress:
			prediction = typed.Prediction
		case PredictionLocked:
			prediction = typed.Prediction
		}

		require.Equal(t, "Will we win?", prediction.Title, topic)
		require.Equal(t, locksAt, prediction.LocksAt, topic)
		require.Len(t, prediction.Outcomes, 2, topic)
	}

	event, ok := Translate(notification("channel.prediction.end", eventsub.Event{
		Title:            "Will we win?",
		Status:           "resolved",
		WinningOutcomeID: "yes",
	}))
	require.True(t, ok)
	ended, ok := event.(PredictionEnded)
	require.True(t, ok)
	require.Equal(t, "resolved", ended.Status)
	require.Equal(t, "yes", ended.WinningOutcomeID)
}

func TestTranslate_RedemptionEvents(t *testing.T) {
	t.Parallel()

	payload := eventsub.Event{
		ID:        "redemption-1",
		UserName:  "carol",
		UserInput: "hello",
		Status:    "unfulfilled",
		Reward:    eventsub.Reward{Title: "Highlight", Cost: 500},
	}

	event, ok := Translate(notification("channel.channel_points_custom_reward_redemption.add", payload))
	require.True(t, ok)
	added, ok := event.(RedemptionAdded)
	require.True(t, ok)
	require.True(t, added.Custom)
	require.Equal(t, "carol", added.Redemption.User)
	require.Equal(t, "Highlight", added.Redemption.Reward)
	require.Equal(t, 500, added.Redemption.Cost)

	event, ok = Translate(notification("channel.channel_points_automatic_reward_redemption.add", payload))
	require.True(t, ok)
	added, ok = event.(RedemptionAdded)
	require.True(t, ok)
	require.False(t, added.Custom)

	event, ok = Translate(notification("channel.channel_points_custom_reward_redemption.update", payload))
	require.True(t, ok)
	updated, ok := event.(RedemptionUpdated)
	require.True(t, ok)
	require.Equal(t, "unfulfilled", updated.Redemption.Status)
}

func TestTranslate_UnbanRequestEvents(t *testing.T) {
	t.Parallel()

	payload := eventsub.Event{
		ID:                  "unban-1",
		BroadcasterUserID:   "chan",
		BroadcasterUserName: "streamer",
		UserID:              "user",
		UserLogin:           "carol",
		UserName:            "Carol",
		Text:                "please",
		Status:              "approved",
		ResolutionText:      "ok",
		ModeratorID:         "mod",
		ModeratorLogin:      "modlogin",
		ModeratorName:       "Mod",
		CreatedAt:           time.Now(),
	}

	event, ok := Translate(notification("channel.unban_request.create", payload))
	require.True(t, ok)
	created, ok := event.(UnbanRequestCreated)
	require.True(t, ok)
	require.Equal(t, "pending", created.Request.Status)
	require.Equal(t, "please", created.Request.Text)

	event, ok = Translate(notification("channel.unban_request.resolve", payload))
	require.True(t, ok)
	resolved, ok := event.(UnbanRequestResolved)
	require.True(t, ok)
	require.Equal(t, "approved", resolved.Request.Status)
	require.Equal(t, "Mod", resolved.Request.ModeratorName)
}

func TestTranslate_RaidCarriesBothDirections(t *testing.T) {
	t.Parallel()

	event, ok := Translate(notification("channel.raid", eventsub.Event{
		FromBroadcasterUserID:   "from",
		FromBroadcasterUserName: "From",
		ToBroadcasterUserID:     "to",
		ToBroadcasterUserName:   "To",
		Viewers:                 42,
	}))
	require.True(t, ok)
	raid, ok := event.(Raid)
	require.True(t, ok)
	require.Equal(t, "from", raid.FromID)
	require.Equal(t, "From", raid.FromName)
	require.Equal(t, "To", raid.ToName)
	require.Equal(t, 42, raid.Viewers)
}

func TestTranslate_ChatSettings(t *testing.T) {
	t.Parallel()

	slow := true
	event, ok := Translate(notification("channel.chat_settings.update", eventsub.Event{
		BroadcasterUserID: "chan",
		SlowMode:          &slow,
	}))
	require.True(t, ok)
	settings, ok := event.(ChatSettingsChanged)
	require.True(t, ok)
	require.True(t, settings.Settings.SlowMode)
	require.Equal(t, "chan", settings.Settings.BroadcasterID)
}

func TestTranslate_NoticeEvents(t *testing.T) {
	t.Parallel()

	event, ok := Translate(notification("channel.warning.send", eventsub.Event{
		ModeratorUserName: "Mod",
		UserName:          "Carol",
		Reason:            "be nice",
	}))
	require.True(t, ok)
	require.Equal(t, WarningSent{Moderator: "Mod", User: "Carol", Reason: "be nice"}, event)

	event, ok = Translate(notification("channel.shoutout.create", eventsub.Event{
		ToBroadcasterUserName: "Target",
		ViewerCount:           12,
	}))
	require.True(t, ok)
	require.Equal(t, ShoutoutSent{Target: "Target", Viewers: 12}, event)

	event, ok = Translate(notification("channel.ad_break.begin", eventsub.Event{
		DurationInSeconds: 30,
		IsAutomatic:       true,
	}))
	require.True(t, ok)
	require.Equal(t, AdBreakStarted{Duration: 30 * time.Second, Automatic: true}, event)
}

func TestTranslate_AutoModEvents(t *testing.T) {
	t.Parallel()

	payload := eventsub.Event{
		MessageID: "msg-1",
		UserName:  "Carol",
		Category:  "harassment",
		Level:     2,
		Status:    "pending",
		Message:   eventsub.MessageContent{Text: "text"},
	}

	event, ok := Translate(notification("automod.message.hold", payload))
	require.True(t, ok)
	held, ok := event.(AutoModHeld)
	require.True(t, ok)
	require.Empty(t, held.AutoMod.Reason)
	require.Equal(t, "harassment", held.AutoMod.Category)
	require.Equal(t, "text", held.AutoMod.Message)
	require.Equal(t, 2, held.AutoMod.Level)

	event, ok = Translate(notification("automod.message.update", payload))
	require.True(t, ok)
	_, ok = event.(AutoModUpdated)
	require.True(t, ok)
}
