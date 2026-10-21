package tab

import (
	"context"

	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/save/messagelog"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/wspool"
)

// The interfaces in this file are the ports a tab consumes. They are declared
// here, by the package that uses them, so a tab states exactly what it needs from
// the rest of the program and the shell only has to supply something that fits.

type AccountProvider interface {
	GetAllAccounts() ([]save.Account, error)
	UpdateTokensFor(id, accessToken, refreshToken string) error
	GetAccountBy(id string) (save.Account, error)
}

type EmoteCache interface {
	GetByText(channelID, text string) (emote.Emote, bool)
	RefreshLocal(ctx context.Context, channelID string) error
	RefreshGlobal(ctx context.Context) error
	GetAllForChannel(id string) emote.EmoteSet
	AddUserEmotes(userID string, emotes []emote.Emote)
	AllEmotesUsableByUser(userID string) []emote.Emote
	RemoveEmoteSetForChannel(channelID string)
	LoadSetForeignEmote(emoteID, emoteText string) emote.Emote
}

type EmoteReplacer interface {
	Replace(channelID, content string, emoteList []twitchirc.Emote) (string, map[string]string, error)
}

type BadgeReplacer interface {
	Replace(broadcasterID string, badgeList []twitchirc.Badge) (string, map[string]string, error)
	InjectContributorBadge(loginName string, badges map[string]string) (string, error)
}

type APIClient interface {
	GetUsers(ctx context.Context, logins []string, ids []string) (twitchapi.UserResponse, error)
	GetStreamInfo(ctx context.Context, broadcastID []string) (twitchapi.GetStreamsResponse, error)
	GetChatSettings(ctx context.Context, broadcasterID string, moderatorID string) (twitchapi.GetChatSettingsResponse, error)
}

type UserEmoteClient interface {
	FetchAllUserEmotes(ctx context.Context, userID string, broadcasterID string) ([]twitchapi.UserEmoteImage, string, error)
}

// UserAuthenticatedAPI is the part of a Twitch client that only works with the
// account's own token: sending messages, making clips, reading chat colours.
type UserAuthenticatedAPI interface {
	CreateClip(ctx context.Context, broadcastID string, hasDelay bool) (twitchapi.CreatedClip, error)
	GetUserChatColor(ctx context.Context, userIDs []string) ([]twitchapi.UserChatColor, error)
	SendChatMessage(ctx context.Context, data twitchapi.SendChatMessageRequest) (twitchapi.SendChatMessageResponse, error)
}

// ModerationAPI is the moderation surface of a Twitch client.
type ModerationAPI interface {
	APIClient
	BanUser(ctx context.Context, broadcasterID string, moderatorID string, data twitchapi.BanUserData) error
	UnbanUser(ctx context.Context, broadcasterID string, moderatorID string, userID string) error
	DeleteMessage(ctx context.Context, broadcasterID string, moderatorID string, messageID string) error
	SendChatAnnouncement(ctx context.Context, broadcasterID string, moderatorID string, req twitchapi.CreateChatAnnouncementRequest) error
	CreateStreamMarker(ctx context.Context, req twitchapi.CreateStreamMarkerRequest) (twitchapi.StreamMarker, error)
}

// FollowedChannelsAPI lists the channels an account follows.
type FollowedChannelsAPI interface {
	FetchUserFollowedChannels(ctx context.Context, userID string, broadcasterID string) ([]twitchapi.FollowedChannel, error)
}

// ClientCapabilities is what one account's API client can actually do.
//
// The shell fills in only the capabilities the account has: an account signed in
// with its own token gets all of them, an anonymous account gets the server proxy
// and nothing else. A nil field means "not available for this account", so callers
// test the field instead of asserting an interface at every call site. Because each
// capability is a separate field, wiring a client into them also makes the compiler
// check it: assigning a client that lacks a method does not compile, where a type
// assertion would only fail once that path was reached.
type ClientCapabilities struct {
	API        APIClient
	Chat       UserAuthenticatedAPI
	Moderation ModerationAPI
	Emotes     UserEmoteClient
	EventSub   wspool.EventSubService
	Follows    FollowedChannelsAPI
}

// ConnectionPool manages WebSocket connections for IRC and EventSub.
type ConnectionPool interface {
	ConnectIRC(accountID string) error
	DisconnectIRC(accountID string)
	SendIRC(accountID string, msg twitchirc.IRCer) error
	JoinChannel(accountID, channel string) error
	SubscribeEventSub(accountID string, req twitchapi.CreateEventSubSubscriptionRequest, service wspool.EventSubService) error
	Close() error
}

type RecentMessageService interface {
	GetRecentMessagesFor(ctx context.Context, channelLogin string) ([]twitchirc.IRCer, error)
}

type MessageLogger interface {
	MessagesFromUserInChannel(username string, broadcasterChannel string) ([]messagelog.LogEntry, error)
}
