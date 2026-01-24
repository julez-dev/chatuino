package wspool

import (
	"net/http"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/rs/zerolog"
)

// eventConn wraps eventsub.Conn for pool management.
type eventConn struct {
	*eventsub.Conn
}

func newEventConn(accountID string, logger zerolog.Logger, httpClient *http.Client, sendFn func(tea.Msg)) *eventConn {
	conn := &eventConn{}

	// Create the underlying connection with a callback that wraps messages in EventSubEvent
	conn.Conn = eventsub.NewConnEventSub(accountID, logger, httpClient, func(msg eventsub.Message[eventsub.NotificationPayload], err error) {
		if err != nil {
			sendFn(EventSubEvent{AccountID: accountID, Error: err})
		} else {
			sendFn(EventSubEvent{AccountID: accountID, Message: msg})
		}
	})

	return conn
}

// subscribe wraps the Subscribe method to adapt the EventSubService interface
func (c *eventConn) subscribe(req twitchapi.CreateEventSubSubscriptionRequest, service EventSubService) {
	c.Subscribe(req, service)
}
