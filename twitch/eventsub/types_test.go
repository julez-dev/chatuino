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

	require.Equal(t, "sparkles", msg.Payload.Event.Message.Text)
}

// TestMessage_AutoModPayload covers both message shapes Twitch uses: AutoMod v1
// sends a string, v2 sends an object, and neither may break the event.
func TestMessage_AutoModPayload(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		message string
	}{
		{name: "auto mod v1 string", message: `"This is a bad message"`},
		{name: "auto mod v2 object", message: `{"text": "This is a bad message"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			raw := `{
				"payload": {
					"subscription": {
						"type": "automod.message.hold",
						"version": "2",
						"condition": {"broadcaster_user_id": "123", "moderator_user_id": "456"}
					},
					"event": {
						"broadcaster_user_id": "123",
						"user_id": "789",
						"user_login": "baduser",
						"user_name": "BadUser",
						"message_id": "msg-1",
						"message": ` + tt.message + `,
						"reason": "automod",
						"held_at": "2026-01-02T03:04:05Z"
					}
				}
			}`

			var msg Message[NotificationPayload]
			require.NoError(t, json.Unmarshal([]byte(raw), &msg))

			event := msg.Payload.Event
			require.Equal(t, "msg-1", event.MessageID)
			require.Equal(t, "BadUser", event.UserName)
			require.Equal(t, "automod", event.Reason)
			require.Equal(t, "This is a bad message", event.Message.Text)
		})
	}
}

func TestMessage_UnbanRequestPayloads(t *testing.T) {
	t.Parallel()

	created := `{"payload":{"subscription":{"type":"channel.unban_request.create"},"event":{"id":"request-1","broadcaster_user_id":"123","broadcaster_user_login":"streamer","broadcaster_user_name":"Streamer","user_id":"456","user_login":"alice","user_name":"Alice","text":"I was banned by mistake","created_at":"2026-01-02T03:04:05Z"}}}`
	resolved := `{"payload":{"subscription":{"type":"channel.unban_request.resolve"},"event":{"id":"request-1","broadcaster_user_id":"123","moderator_id":"789","moderator_login":"mod","moderator_name":"Mod","user_id":"456","user_login":"alice","user_name":"Alice","resolution_text":"apology accepted","status":"approved"}}}`

	var createMsg Message[NotificationPayload]
	require.NoError(t, json.Unmarshal([]byte(created), &createMsg))
	createEvent := createMsg.Payload.Event
	require.Equal(t, "request-1", createEvent.ID)
	require.Equal(t, "Alice", createEvent.UserName)
	require.Equal(t, "I was banned by mistake", createEvent.Text)
	require.Equal(t, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), createEvent.CreatedAt)

	var resolveMsg Message[NotificationPayload]
	require.NoError(t, json.Unmarshal([]byte(resolved), &resolveMsg))
	resolveEvent := resolveMsg.Payload.Event
	require.Equal(t, "request-1", resolveEvent.ID)
	require.Equal(t, "approved", resolveEvent.Status)
	require.Equal(t, "apology accepted", resolveEvent.ResolutionText)
	require.Equal(t, "789", resolveEvent.ModeratorID)
	require.Equal(t, "Mod", resolveEvent.ModeratorName)
}

// TestMessage_ShoutoutAndChatSettingsPayload pins the fields the UI handlers read.
func TestMessage_ShoutoutAndChatSettingsPayload(t *testing.T) {
	parallel := []string{
		`{"payload":{"subscription":{"type":"channel.chat_settings.update","condition":{"broadcaster_user_id":"123"}},"event":{"broadcaster_user_id":"123","emote_mode":true,"follower_mode":false,"follower_mode_duration_minutes":null,"slow_mode":true,"slow_mode_wait_time_seconds":10,"subscriber_mode":false,"unique_chat_mode":true}}}`,
		`{"payload":{"subscription":{"type":"channel.shoutout.create","condition":{"broadcaster_user_id":"123"}},"event":{"to_broadcaster_user_name":"Bob","viewer_count":42}}}`,
		`{"payload":{"subscription":{"type":"channel.warning.send","condition":{"broadcaster_user_id":"123"}},"event":{"moderator_user_name":"Mod","user_name":"Alice","reason":"spam"}}}`,
	}

	for _, raw := range parallel {
		var msg Message[NotificationPayload]
		require.NoError(t, json.Unmarshal([]byte(raw), &msg))

		event := msg.Payload.Event
		switch msg.Payload.Subscription.Type {
		case "channel.chat_settings.update":
			require.NotNil(t, event.SlowMode)
			require.True(t, *event.SlowMode)
			require.Equal(t, 10, *event.SlowModeWaitTime)
			require.True(t, *event.EmoteMode)
			require.True(t, *event.UniqueChatMode)
		case "channel.shoutout.create":
			require.Equal(t, "Bob", event.ToBroadcasterUserName)
			require.Equal(t, 42, event.ViewerCount)
		case "channel.warning.send":
			require.Equal(t, "Mod", event.ModeratorUserName)
			require.Equal(t, "Alice", event.UserName)
			require.Equal(t, "spam", event.Reason)
		}
	}
}
