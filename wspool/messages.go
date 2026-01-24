package wspool

import (
	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
)

// IRCEvent is sent to UI via tea.Send when an IRC message is received
// or a connection error occurs.
type IRCEvent struct {
	AccountID string
	Message   twitchirc.IRCer // nil if Error is set
	Error     error           // connection error, will attempt reconnect
}

// EventSubEvent is sent to UI via tea.Send when an EventSub notification
// is received or a connection error occurs.
type EventSubEvent struct {
	AccountID string
	Message   eventsub.Message[eventsub.NotificationPayload] // zero value if Error is set
	Error     error
}
