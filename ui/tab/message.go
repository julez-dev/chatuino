package tab

import (
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
)

// ChatEventMessage comes when we receive a IRC event.
//
// The fields are exported because the shell assembles the message: it owns the
// emote/badge replacers and the link checker, and it has to build the event before
// it knows whether a tab is there to receive it.
type ChatEventMessage struct {
	// If the event was not created by twitch IRC connection but instead locally by message input chat load etc.
	// This indicates that the root will not start a new wait message command.
	// All messages requested by RequestLocalMessageHandleMessage will have this flag set to true.
	IsFakeEvent             bool
	AccountID               string
	Channel                 string
	ChannelID               string
	ChannelGuestID          string // source-room-id by twitch
	ChannelGuestDisplayName string // set later when broadcast tab reads the message

	Message         twitchirc.IRCer
	DisplayModifier MessageContentModifier // modifier for the original irc message

	// if message should only be sent to a specific tab ID
	// if empty send to all
	TabID string

	// Kitty graphics terminal command that must be written before this event
	// is rendered (e.g. image upload/placement). Emitted as tea.Raw by the shell
	// before forwarding to tabs; not consumed by tabs themselves.
	PrepareCommand string
}

type (
	MessageContentModifier struct {
		WordReplacements WordReplacement
		BadgeReplacement WordReplacement
		MessageSuffix    string
		Strikethrough    bool
		Italic           bool
	}
	WordReplacement map[string]string // og:replacement
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
