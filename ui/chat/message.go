package chat

import "github.com/julez-dev/chatuino/twitch/twitchirc"

// ChatEventMessage is one event shown in a chat transcript. The fields are
// exported because the shell assembles the message: it owns the emote/badge
// replacers and the link checker, and it builds the event before it knows which
// transcript will receive it.
type ChatEventMessage struct {
	// IsFakeEvent marks an event created by local input or history load rather
	// than a live IRC connection.
	IsFakeEvent             bool
	AccountID               string
	Channel                 string
	ChannelID               string
	ChannelGuestID          string // source-room-id by twitch
	ChannelGuestDisplayName string // set later when the tab reads the message

	Message         twitchirc.IRCer
	DisplayModifier MessageContentModifier // modifier for the original irc message

	// TabID addresses the event to one tab. Empty means every tab.
	TabID string

	// PrepareCommand is a kitty graphics command the shell writes before the
	// event is rendered. The transcript itself never consumes it.
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
