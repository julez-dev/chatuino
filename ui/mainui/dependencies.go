package mainui

import (
	"context"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/julez-dev/chatuino/wspool"
)

// The ports a tab consumes are declared in the tab package. What is left here is
// what only the shell uses, plus the composite types that wire concrete clients
// into the capability record.

// ChatuinoServer is the chatuino server API plus the endpoints only the shell
// calls: token refresh, link checking and the version check.
type ChatuinoServer interface {
	tab.APIClient
	RefreshToken(ctx context.Context, refreshToken string) (string, string, error)
	CheckLink(ctx context.Context, targetURL string) (server.CheckLinkResponse, error)
	GetLatestVersion(ctx context.Context) (string, error)
}

// fullClient is the widest capability set a client can have. Taking it as the
// parameter type is what turns the capability list into a compile-time check.
type fullClient interface {
	tab.APIClient
	tab.UserAuthenticatedAPI
	tab.ModerationAPI
	tab.UserEmoteClient
	tab.FollowedChannelsAPI
	wspool.EventSubService
}

// NewClientCapabilities records every capability of a fully authenticated client.
func NewClientCapabilities(client fullClient) tab.ClientCapabilities {
	return tab.ClientCapabilities{
		API:        client,
		Chat:       client,
		Moderation: client,
		Emotes:     client,
		EventSub:   client,
		Follows:    client,
	}
}

type AppStateManager interface {
	LoadAppState() (save.AppState, error)
	SaveAppState(save.AppState) error
}

// ChannelHistory tracks recently visited channels.
type ChannelHistory interface {
	LoadHistory() ([]save.ChannelHistoryEntry, error)
	RecordChannel(login string) error
}
