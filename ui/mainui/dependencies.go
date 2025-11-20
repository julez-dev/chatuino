package mainui

import (
	"context"

	"github.com/julez-dev/chatuino/badge"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/multiplex"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/save/messagelog"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/twitch/command"
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
	Replace(channelID, content string, emoteList []command.Emote) (string, string, error)
}

type BadgeReplacer interface {
	Replace(broadcasterID string, badgeList []command.Badge) (string, []string, error)
}

type APIClient interface {
	GetUsers(ctx context.Context, logins []string, ids []string) (twitch.UserResponse, error)
	GetStreamInfo(ctx context.Context, broadcastID []string) (twitch.GetStreamsResponse, error)
	GetChatSettings(ctx context.Context, broadcasterID string, moderatorID string) (twitch.GetChatSettingsResponse, error)
}

type APIClientWithRefresh interface {
	APIClient
	RefreshToken(ctx context.Context, refreshToken string) (string, string, error)
}

type UserEmoteClient interface {
	FetchAllUserEmotes(ctx context.Context, userID string, broadcasterID string) ([]twitch.UserEmoteImage, string, error)
}

type ChatPool interface {
	ListenAndServe(inbound <-chan multiplex.InboundMessage) <-chan multiplex.OutboundMessage
}

type EventSubPool interface {
	ListenAndServe(inbound <-chan multiplex.EventSubInboundMessage) error
}

type RecentMessageService interface {
	GetRecentMessagesFor(ctx context.Context, channelLogin string) ([]twitch.IRCer, error)
}

type MessageLogger interface {
	MessagesFromUserInChannel(username string, broadcasterChannel string) ([]messagelog.LogEntry, error)
}

type DependencyContainer struct {
	UserConfig UserConfiguration
	Keymap     save.KeyMap
	Accounts   []save.Account

	ServerAPI      APIClientWithRefresh
	APIUserClients map[string]APIClient

	AccountProvider      AccountProvider
	EmoteCache           EmoteCache
	BadgeCache           *badge.Cache
	EmoteReplacer        EmoteReplacer
	BadgeReplacer        BadgeReplacer
	ImageDisplayManager  *kittyimg.DisplayManager
	RecentMessageService RecentMessageService
	MessageLogger        MessageLogger
	ChatPool             ChatPool
	EventSubPool         EventSubPool
}
