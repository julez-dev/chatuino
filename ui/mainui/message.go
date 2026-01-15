package mainui

import (
	"github.com/julez-dev/chatuino/multiplex"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
)

// persistedDataLoadedMessage comes when state and user data is loaded
type persistedDataLoadedMessage struct {
	err      error
	ttvUsers map[string]twitchapi.UserData
	state    save.AppState
}

// chatEventMessage comes when we receive a IRC event
type chatEventMessage struct {
	// If the event was not created by twitch IRC connection but instead locally by message input chat load etc.
	// This indicates that the root will not start a new wait message command.
	// All messages requested by requestLocalMessageHandleMessage will have this flag set to true.
	isFakeEvent             bool
	accountID               string
	channel                 string
	channelID               string
	channelGuestID          string // source-room-id by twitch
	channelGuestDisplayName string // set later when broadcast tab reads the message

	message         twitchirc.IRCer
	displayModifier messageContentModifier // modifier for the original irc message

	// if message should only be sent to a specific tab ID
	// if empty send to all
	tabID string
}

type (
	messageContentModifier struct {
		wordReplacements wordReplacement
		badgeReplacement wordReplacement
		messageSuffix    string
		strikethrough    bool
		italic           bool
	}
	wordReplacement map[string]string // og:replacement
)

// requestLocalMessageHandleMessage comes when the program requests a message to be handled by the IRC message handler which
// then converts it into a chatEventMessage
type requestLocalMessageHandleMessage struct {
	message   twitchirc.IRCer
	accountID string
	tabID     string
}

// requestLocalMessageHandleBatchMessage is the same as requestLocalMessageHandleMessage but for multiple message
type requestLocalMessageHandleBatchMessage struct {
	messages  []twitchirc.IRCer
	accountID string
	tabID     string
}

// requestLocalMessageHandleBatchMessage comes when program requests a message to be sent to the IRC stream
type forwardChatMessage struct {
	msg multiplex.InboundMessage
}

// forwardEventSubMessage comes when program requests a message to be sent to the event sub stream
type forwardEventSubMessage struct {
	accountID string
	msg       eventsub.InboundMessage
}

// EventSubMessage comes when we get a message from the event sub
type EventSubMessage struct {
	Payload eventsub.Message[eventsub.NotificationPayload]
}

// polledStreamInfoMessage comes when current stream info is refreshed
type polledStreamInfoMessage struct {
	streamInfos []setStreamInfoMessage
}

// appStateSaveMessage comes when current app state was saved
type appStateSaveMessage struct{}

// imageCleanupTickMessage comes when images should be cleaned up
type imageCleanupTickMessage struct {
	deletionCommand string
}

// joinChannelMessage comes when user confirms channel which should be joined
type joinChannelMessage struct {
	tabKind tabKind
	channel string
	account save.Account
}

// setStreamInfoMessage comes when new live info about a streamer was fetched
type setStreamInfoMessage struct {
	target   string // the broadcasters ID
	username string // is broadcasters display name
	viewer   int
	title    string
	game     string
	isLive   bool
}

// requestNotificationIconMessage comes when app requests an notification icon for a tab
type requestNotificationIconMessage struct {
	tabID string
}
