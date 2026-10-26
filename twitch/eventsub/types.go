package eventsub

import (
	"encoding/json"
	"time"
)

type metadata struct {
	MessageID           string    `json:"message_id"`
	MessageType         string    `json:"message_type"`
	MessageTimeStamp    time.Time `json:"message_timestamp"`
	SubscriptionType    string    `json:"subscription_type"`
	SubscriptionVersion string    `json:"subscription_version"`
}

type Message[T any] struct {
	Metadata metadata `json:"metadata"`
	Payload  T        `json:"payload"`
}

type (
	SessionPayload struct {
		Session Session `json:"session"`
	}
	Session struct {
		ID                      string    `json:"id"`
		Satus                   string    `json:"status"`
		ConnectedAt             time.Time `json:"connected_at"`
		KeepAliveTimeoutSeconds int       `json:"keepalive_timeout_seconds"`
		ReconnectURL            string    `json:"reconnect_url"`
	}
)

type NotificationPayload struct {
	Subscription Subscription `json:"subscription"`
	Event        Event        `json:"event"`
}

type Transport struct {
	Method    string `json:"method"`
	SessionID string `json:"session_id"`
}

type Subscription struct {
	ID        string            `json:"id"`
	Status    string            `json:"status"`
	Type      string            `json:"type"`
	Version   string            `json:"version"`
	Cost      int               `json:"cost"`
	Condition map[string]string `json:"condition"`
	Transport Transport         `json:"transport"`
	CreatedAt time.Time         `json:"created_at"`
}

type Event struct {
	ID                   string    `json:"id"`
	UserID               string    `json:"user_id"`
	UserLogin            string    `json:"user_login"`
	UserName             string    `json:"user_name"`
	BroadcasterUserID    string    `json:"broadcaster_user_id"`
	BroadcasterUserLogin string    `json:"broadcaster_user_login"`
	BroadcasterUserName  string    `json:"broadcaster_user_name"`
	FollowedAt           time.Time `json:"followed_at"`

	// Poll and prediction related
	Title               string    `json:"title"`
	Choices             []Choice  `json:"choices"`
	Outcomes            []Outcome `json:"outcomes"`
	BitsVoting          Voting    `json:"bits_voting"`
	ChannelPointsVoting Voting    `json:"channel_points_voting"`
	StartedAt           time.Time `json:"started_at"`
	LocksAt             time.Time `json:"locks_at"`
	LockedAt            time.Time `json:"locked_at"`
	EndsAt              time.Time `json:"ends_at"`  // empty if done
	EndedAt             time.Time `json:"ended_at"` // empty until done
	WinningOutcomeID    string    `json:"winning_outcome_id"`
	Status              string    `json:"status"` // poll: completed; prediction: resolved/canceled; redemption: unfulfilled/fulfilled/canceled

	// Channel point redemption related
	UserInput  string         `json:"user_input"`
	Reward     Reward         `json:"reward"`
	Message    MessageContent `json:"message"` // redemption text or AutoMod message
	RedeemedAt time.Time      `json:"redeemed_at"`

	// Raid related
	FromBroadcasterUserID    string `json:"from_broadcaster_user_id"`
	FromBroadcasterUserLogin string `json:"from_broadcaster_user_login"`
	FromBroadcasterUserName  string `json:"from_broadcaster_user_name"`

	ToBroadcasterUserID    string `json:"to_broadcaster_user_id"`
	ToBroadcasterUserLogin string `json:"to_broadcaster_user_login"`
	ToBroadcasterUserName  string `json:"to_broadcaster_user_name"`

	Viewers int `json:"viewers"`

	// Ad related
	IsAutomatic       bool `json:"is_automatic"`
	DurationInSeconds int  `json:"duration_seconds"`

	RequesterUserID    string `json:"requester_user_id"`
	RequesterUserLogin string `json:"requester_user_login"`
	RequesterUserName  string `json:"requester_user_name"`

	// Chat settings related. The booleans are pointers because a chat settings
	// event sends all of them, while the same Event struct is used by events that
	// do not mention these fields at all.
	EmoteMode            *bool `json:"emote_mode"`
	FollowerMode         *bool `json:"follower_mode"`
	FollowerModeDuration *int  `json:"follower_mode_duration_minutes"`
	SlowMode             *bool `json:"slow_mode"`
	SlowModeWaitTime     *int  `json:"slow_mode_wait_time_seconds"`
	SubscriberMode       *bool `json:"subscriber_mode"`
	UniqueChatMode       *bool `json:"unique_chat_mode"`

	// Warning and AutoMod related
	ModeratorUserID    string `json:"moderator_user_id"`
	ModeratorUserLogin string `json:"moderator_user_login"`
	ModeratorUserName  string `json:"moderator_user_name"`
	MessageID          string `json:"message_id"`
	Reason             string `json:"reason"`
	Category           string `json:"category"`
	Level              int    `json:"level"`

	// Shoutout related
	ViewerCount int `json:"viewer_count"`
}

type Voting struct {
	IsEnabled     bool `json:"is_enabled"`
	AmountPerVote int  `json:"amount_per_vote"`
}

type Choice struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	BitsVotes          int    `json:"bits_votes"`
	ChannelPointsVotes int    `json:"channel_points_votes"`
	Votes              int    `json:"votes"`
}

type Outcome struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Color         string `json:"color"`
	Users         int    `json:"users"`
	ChannelPoints int    `json:"channel_points"`
}

type Reward struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
	Cost   int    `json:"cost"`
}

type MessageContent struct {
	Text string `json:"text"`
}

// UnmarshalJSON accepts both shapes Twitch uses for an event's message field:
// redemptions send an object ({"text": "..."}) while some AutoMod versions send
// a bare string. Decoding one shape into the other used to fail the whole event.
func (m *MessageContent) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &m.Text)
	}

	type messageContent MessageContent
	return json.Unmarshal(data, (*messageContent)(m))
}
