package mainui

import (
	"context"

	"github.com/julez-dev/chatuino/badge"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/save/messagelog"
	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/wspool"
)

type UserConfiguration struct {
	Settings save.Settings
	Theme    save.Theme
}

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
}

type APIClient interface {
	GetUsers(ctx context.Context, logins []string, ids []string) (twitchapi.UserResponse, error)
	GetStreamInfo(ctx context.Context, broadcastID []string) (twitchapi.GetStreamsResponse, error)
	GetChatSettings(ctx context.Context, broadcasterID string, moderatorID string) (twitchapi.GetChatSettingsResponse, error)
}

type ChatuinoServer interface {
	APIClient
	RefreshToken(ctx context.Context, refreshToken string) (string, string, error)
	CheckLink(ctx context.Context, targetURL string) (server.CheckLinkResponse, error)
}

type UserEmoteClient interface {
	FetchAllUserEmotes(ctx context.Context, userID string, broadcasterID string) ([]twitchapi.UserEmoteImage, string, error)
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

type AppStateManager interface {
	LoadAppState() (save.AppState, error)
	SaveAppState(save.AppState) error
}

type DependencyContainer struct {
	UserConfig UserConfiguration
	Keymap     save.KeyMap
	Accounts   []save.Account

	ServerAPI      ChatuinoServer
	APIUserClients map[string]APIClient

	AccountProvider      AccountProvider
	EmoteCache           EmoteCache
	BadgeCache           *badge.Cache
	EmoteReplacer        EmoteReplacer
	BadgeReplacer        BadgeReplacer
	ImageDisplayManager  *kittyimg.DisplayManager
	RecentMessageService RecentMessageService
	MessageLogger        MessageLogger
	Pool                 ConnectionPool
	AppStateManager      AppStateManager
}
