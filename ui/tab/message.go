package tab

import (
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
)

// RequestLocalMessageHandleMessage comes when the program requests a message to be
// handled by the IRC message handler which then converts it into a ChatEventMessage.
type RequestLocalMessageHandleMessage struct {
	Message   twitchirc.IRCer
	AccountID string
	TabID     string
}

// RequestLocalMessageHandleBatchMessage is the same as
// RequestLocalMessageHandleMessage but for multiple messages.
type RequestLocalMessageHandleBatchMessage struct {
	Messages  []twitchirc.IRCer
	AccountID string
	TabID     string
}

// JoinChannelMessage comes when user confirms channel which should be joined.
type JoinChannelMessage struct {
	TabKind TabKind
	Channel string
	Account save.Account
}

// SetStreamInfoMessage comes when new live info about a streamer was fetched.
type SetStreamInfoMessage struct {
	Target   string // the broadcasters ID
	Username string // is broadcasters display name
	Viewer   int
	Title    string
	Game     string
	IsLive   bool
}

// ChannelSuggestionsLoadedMessage delivers channel names for /join autocomplete.
type ChannelSuggestionsLoadedMessage struct {
	Channels []string
}

// RequestNotificationIconMessage comes when app requests an notification icon for a tab.
type RequestNotificationIconMessage struct {
	TabID string
}
