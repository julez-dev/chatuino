// Package live owns the channel-live subscription catalog and the translation
// from EventSub wire notifications into typed domain events.
//
// The package deliberately does not know about Bubble Tea, tabs or notices: it
// describes what to subscribe to and what happened. The UI decides how to issue
// the subscriptions and how to render the events.
package live

import "github.com/julez-dev/chatuino/twitch/twitchapi"

// Role is the account's relationship to the channel being watched. It decides
// which EventSub topics the account is allowed to subscribe to.
type Role int

const (
	// RoleViewer may not subscribe to any channel-live topic.
	RoleViewer Role = iota
	// RoleModerator may subscribe to unban request topics for the channel.
	RoleModerator
	// RoleBroadcaster may subscribe to the full channel-live topic set.
	RoleBroadcaster
)

// Subscription is an opaque EventSub subscription. The request is constructed by
// this package so callers never assemble topic strings or condition keys.
type Subscription struct {
	request twitchapi.CreateEventSubSubscriptionRequest
}

// Request returns the transport payload a connection pool accepts. Callers pass
// it through without inspecting or modifying it.
func (s Subscription) Request() twitchapi.CreateEventSubSubscriptionRequest {
	return s.request
}

// Type returns the EventSub topic, for diagnostics and tests.
func (s Subscription) Type() string {
	return s.request.Type
}

// Watch returns the subscriptions an account with the given role should hold for
// a channel. A viewer holds none. A moderator holds only the unban request topics
// that Twitch allows moderators of other channels to use. A broadcaster holds the
// full channel-live set, including unban requests.
func Watch(role Role, channelID, accountID string) []Subscription {
	switch role {
	case RoleBroadcaster:
		return broadcasterSubscriptions(channelID, accountID)
	case RoleModerator:
		return unbanRequestSubscriptions(channelID, accountID)
	default:
		return nil
	}
}

// broadcasterSubscriptions is 22 subscriptions across 21 distinct topics:
// channel.raid is subscribed twice, once per direction.
func broadcasterSubscriptions(channelID, accountID string) []Subscription {
	subscriptions := make([]Subscription, 0, 22)

	for _, topic := range [...]string{
		"channel.poll.begin", "channel.poll.progress", "channel.poll.end",
		"channel.prediction.begin", "channel.prediction.progress", "channel.prediction.lock", "channel.prediction.end",
		"channel.channel_points_custom_reward_redemption.add", "channel.channel_points_custom_reward_redemption.update",
		"channel.channel_points_automatic_reward_redemption.add",
		"channel.ad_break.begin",
	} {
		subscriptions = append(subscriptions, subscription(topic, "1", map[string]string{
			"broadcaster_user_id": channelID,
		}))
	}

	// The broadcaster can be raided or raid another channel, and the two
	// directions use different condition keys.
	subscriptions = append(subscriptions,
		subscription("channel.raid", "1", map[string]string{
			"to_broadcaster_user_id": channelID,
		}),
		subscription("channel.raid", "1", map[string]string{
			"from_broadcaster_user_id": channelID,
		}),
	)

	// Chat settings events identify the reader with user_id, while the
	// moderation topics below identify them with moderator_user_id.
	subscriptions = append(subscriptions, subscription("channel.chat_settings.update", "1", map[string]string{
		"broadcaster_user_id": channelID,
		"user_id":             accountID,
	}))

	for _, topic := range [...]struct {
		name    string
		version string
	}{
		{"channel.warning.send", "1"},
		{"channel.warning.acknowledge", "1"},
		{"channel.shoutout.create", "1"},
		{"channel.shoutout.receive", "1"},
		{"automod.message.hold", "2"},
		{"automod.message.update", "2"},
	} {
		subscriptions = append(subscriptions, subscription(topic.name, topic.version, map[string]string{
			"broadcaster_user_id": channelID,
			"moderator_user_id":   accountID,
		}))
	}

	return append(subscriptions, unbanRequestSubscriptions(channelID, accountID)...)
}

func unbanRequestSubscriptions(channelID, accountID string) []Subscription {
	return []Subscription{
		subscription("channel.unban_request.create", "1", map[string]string{
			"broadcaster_user_id": channelID,
			"moderator_user_id":   accountID,
		}),
		subscription("channel.unban_request.resolve", "1", map[string]string{
			"broadcaster_user_id": channelID,
			"moderator_user_id":   accountID,
		}),
	}
}

func subscription(topic, version string, condition map[string]string) Subscription {
	return Subscription{request: twitchapi.CreateEventSubSubscriptionRequest{
		Type:      topic,
		Version:   version,
		Condition: condition,
	}}
}
