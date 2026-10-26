package eventsub

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMessage_PredictionPayload(t *testing.T) {
	t.Parallel()

	raw := `{
		"metadata": {
			"message_id": "msg-1",
			"message_type": "notification",
			"message_timestamp": "2026-01-02T03:04:05Z",
			"subscription_type": "channel.prediction.begin",
			"subscription_version": "1"
		},
		"payload": {
			"subscription": {
				"id": "sub-1",
				"status": "enabled",
				"type": "channel.prediction.begin",
				"version": "1",
				"cost": 0,
				"condition": {"broadcaster_user_id": "123"},
				"transport": {"method": "websocket", "session_id": "session-1"},
				"created_at": "2026-01-02T03:00:00Z"
			},
			"event": {
				"id": "prediction-1",
				"broadcaster_user_id": "123",
				"broadcaster_user_login": "streamer",
				"broadcaster_user_name": "Streamer",
				"title": "Will we win?",
				"outcomes": [
					{"id": "yes", "title": "Yes", "color": "blue", "users": 5, "channel_points": 500},
					{"id": "no", "title": "No", "color": "pink", "users": 2, "channel_points": 200}
				],
				"started_at": "2026-01-02T03:04:05Z",
				"locks_at": "2026-01-02T03:05:05Z"
			}
		}
	}`

	var msg Message[NotificationPayload]
	require.NoError(t, json.Unmarshal([]byte(raw), &msg))

	event := msg.Payload.Event
	require.Equal(t, "prediction-1", event.ID)
	require.Equal(t, "Will we win?", event.Title)
	require.Len(t, event.Outcomes, 2)
	require.Equal(t, "yes", event.Outcomes[0].ID)
	require.Equal(t, 500, event.Outcomes[0].ChannelPoints)
	require.Equal(t, 5, event.Outcomes[0].Users)
	require.Equal(t, time.Date(2026, 1, 2, 3, 5, 5, 0, time.UTC), event.LocksAt)
}

func TestMessage_CustomRedemptionPayload(t *testing.T) {
	t.Parallel()

	raw := `{
		"payload": {
			"subscription": {
				"id": "sub-2",
				"status": "enabled",
				"type": "channel.channel_points_custom_reward_redemption.update",
				"version": "1",
				"cost": 0,
				"condition": {"broadcaster_user_id": "123"},
				"transport": {"method": "websocket", "session_id": "session-1"},
				"created_at": "2026-01-02T03:00:00Z"
			},
			"event": {
				"id": "redemption-1",
				"broadcaster_user_id": "123",
				"broadcaster_user_login": "streamer",
				"broadcaster_user_name": "Streamer",
				"user_id": "456",
				"user_login": "alice",
				"user_name": "Alice",
				"user_input": "make it purple",
				"status": "fulfilled",
				"reward": {
					"id": "reward-1",
					"title": "Highlight My Message",
					"prompt": "Choose a message to highlight",
					"cost": 500
				},
				"redeemed_at": "2026-01-02T03:04:05Z"
			}
		}
	}`

	var msg Message[NotificationPayload]
	require.NoError(t, json.Unmarshal([]byte(raw), &msg))

	event := msg.Payload.Event
	require.Equal(t, "redemption-1", event.ID)
	require.Equal(t, "Alice", event.UserName)
	require.Equal(t, "make it purple", event.UserInput)
	require.Equal(t, "fulfilled", event.Status)
	require.Equal(t, "Highlight My Message", event.Reward.Title)
	require.Equal(t, 500, event.Reward.Cost)
	require.Equal(t, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), event.RedeemedAt)
}

func TestMessage_AutomaticRedemptionPayload(t *testing.T) {
	t.Parallel()

	raw := `{
		"payload": {
			"subscription": {
				"id": "sub-3",
				"status": "enabled",
				"type": "channel.channel_points_automatic_reward_redemption.add",
				"version": "1",
				"cost": 0,
				"condition": {"broadcaster_user_id": "123"},
				"transport": {"method": "websocket", "session_id": "session-1"},
				"created_at": "2026-01-02T03:00:00Z"
			},
			"event": {
				"id": "redemption-2",
				"broadcaster_user_id": "123",
				"user_id": "456",
				"user_login": "alice",
				"user_name": "Alice",
				"reward": {"id": "reward-2", "title": "Message Effect", "cost": 200},
				"message": {"text": "sparkles"},
				"redeemed_at": "2026-01-02T03:04:05Z"
			}
		}
	}`

	var msg Message[NotificationPayload]
	require.NoError(t, json.Unmarshal([]byte(raw), &msg))

	require.Equal(t, "sparkles", msg.Payload.Event.RewardText.Text)
}
