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

type untypedMessagePayload struct {
	Metadata metadata        `json:"metadata"`
	Payload  json.RawMessage `json:"payload"`
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
	UserID               string    `json:"user_id"`
	UserLogin            string    `json:"user_login"`
	UserName             string    `json:"user_name"`
	BroadcasterUserID    string    `json:"broadcaster_user_id"`
	BroadcasterUserLogin string    `json:"broadcaster_user_login"`
	BroadcasterUserName  string    `json:"broadcaster_user_name"`
	FollowedAt           time.Time `json:"followed_at"`

	// Poll releated
	Title               string    `json:"title"`
	Choices             []Choice  `json:"choices"`
	BitsVoting          Voting    `json:"bits_voting"`
	ChannelPointsVoting Voting    `json:"channel_points_voting"`
	StartedAt           time.Time `json:"started_at"`
	EndsAt              time.Time `json:"ends_at"`  // empty if done
	EndedAt             time.Time `json:"ended_at"` // empty until done
	Status              string    `json:"status"`   // completed when done, else empty
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
