package eventsub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/jellydator/ttlcache/v3"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/rs/zerolog"
)

const (
	maxMessageSize = 5 * 1024 * 1024 // 5MB
	eventSubURL    = "wss://eventsub.wss.twitch.tv/ws?keepalive_timeout_seconds=30"
	// eventSubURL = "ws://127.0.0.1:8080/ws?keepalive_timeout_seconds=30"
)

type EventSubService interface {
	CreateEventSubSubscription(ctx context.Context, reqData twitch.CreateEventSubSubscriptionRequest) (twitch.CreateEventSubSubscriptionResponse, error)
}

type InboundMessage struct {
	Req     twitch.CreateEventSubSubscriptionRequest
	Service EventSubService
}

type Conn struct {
	inboundWasClosed bool
	httpClient       *http.Client
	logger           zerolog.Logger
	m                *sync.Mutex

	// twitch may send duplicate messages (detectable by id), we need to filter them out
	// keep all ids in store for 15 minutes
	duplicate *ttlcache.Cache[string, struct{}]

	HandleMessage func(msg Message[NotificationPayload])
	HandleError   func(err error)
}

func NewConn(logger zerolog.Logger, httpClient *http.Client) *Conn {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Conn{
		httpClient:    httpClient,
		m:             &sync.Mutex{},
		HandleMessage: func(msg Message[NotificationPayload]) {},
		HandleError:   func(err error) {},
		duplicate: ttlcache.New[string, struct{}](
			ttlcache.WithTTL[string, struct{}](15 * time.Minute),
		),
	}
}

type twitchForcedReconnect struct {
	NewWSURL string
}

func (t twitchForcedReconnect) Error() string {
	return "twitch forced reconnect"
}

// Connect connects to the twitch eventsub websocket
// It will wait util a message is send to the inbound channel before connecting this is because twitch
// will close the connection if we don't send a event sub subscription within 10 seconds after connecting.
// Connect will try to reconnect if the connection is dropped until inboud is closed.
// If twitch sends reconnect messsage, Connect will reconnect to the session.
// Duplicate messages are filtered out.
func (c *Conn) Connect(inbound <-chan InboundMessage) error {
	defer func() {
		go func() {
			// drain the inbound channel
			for range inbound {
			}
		}()
		c.duplicate.Stop()
	}()

	go c.duplicate.Start()

	// only start once we have a message
	// twitch will close the eventsub WS if we don't send a event sub subscription within 10 seconds after connecting
	initial, ok := <-inbound
	if !ok {
		return fmt.Errorf("inbound channel was closed early")
	}

	subURL := eventSubURL
	var err error
	for {
		// If old connection was closed by application, don't reconnect
		c.m.Lock()
		if c.inboundWasClosed {
			c.m.Unlock()
			return nil
		}
		c.m.Unlock()

		maybeInitial := &initial

		// if err is not nil, this is a reconnect loop, skip initial message
		if err != nil {
			maybeInitial = nil

			// wait 5 seconds before reconnecting
			select {
			case <-time.After(time.Second * 5):
			case _, ok := <-inbound:
				if !ok {
					return fmt.Errorf("inbound channel was closed while waiting for reconnect")
				}
			}
		}

		if err = c.startListeningWS(subURL, inbound, maybeInitial); err != nil {
			var forcedReconnectErr twitchForcedReconnect
			if errors.As(err, &forcedReconnectErr) {
				subURL = forcedReconnectErr.NewWSURL
				continue
			}

			c.logger.Err(err).Msg("failed during startListeningWS")
		}
	}
}

func (c *Conn) startListeningWS(eventSubURL string, inboundChan <-chan InboundMessage, initialInbound *InboundMessage) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	ws, _, err := websocket.Dial(ctx, eventSubURL, &websocket.DialOptions{
		HTTPClient: c.httpClient,
	})
	if err != nil {
		return fmt.Errorf("failed to dial %s: %w", eventSubURL, err)
	}

	defer ws.Close(websocket.StatusNormalClosure, "consuming done")

	done := make(chan struct{})
	wg := &sync.WaitGroup{}

	defer func() {
		close(done)
		wg.Wait()
	}()

	for {
		_, data, err := ws.Read(context.Background())
		if err != nil {
			return fmt.Errorf("failed to read message: %w", err)
		}

		var untypedData untypedMessagePayload
		if err := json.Unmarshal(data, &untypedData); err != nil {
			continue
		}

		switch untypedData.Metadata.MessageType {
		case "session_welcome":
			welcome, err := convertUntyped[SessionPayload](untypedData)
			if err != nil {
				return fmt.Errorf("failed to convert to session welcome: %w", err)
			}

			if initialInbound != nil {
				_, err := initialInbound.Service.CreateEventSubSubscription(context.Background(), addTransportFunc(initialInbound.Req, welcome.Payload.Session.ID))
				if err != nil {
					err := fmt.Errorf("failed to create initial subscription: %w", err)
					c.HandleError(err)
					continue
				}
			}

			wg.Add(1)
			go c.listenInboundMessages(wg, done, inboundChan, welcome.Payload.Session.ID, ws)
		case "session_reconnect":
			reconnect, err := convertUntyped[SessionPayload](untypedData)
			if err != nil {
				err := fmt.Errorf("failed to convert to session reconnect: %w", err)
				c.HandleError(err)
				return err
			}

			return twitchForcedReconnect{NewWSURL: reconnect.Payload.Session.ReconnectURL}
		case "session_keepalive":
			c.logger.Info().Any("event-message", untypedData).Msg("session_keepalive")
			continue
		case "notification":
			// skip if duplicate
			if c.duplicate.Has(untypedData.Metadata.MessageID) {
				continue
			}

			c.duplicate.Set(untypedData.Metadata.MessageID, struct{}{}, ttlcache.DefaultTTL)

			typedData, err := convertUntyped[NotificationPayload](untypedData)
			if err != nil {
				err := fmt.Errorf("failed to convert to notification: %w", err)
				c.HandleError(err)
				return err
			}

			c.HandleMessage(typedData)
		default:
			c.logger.Info().Any("event-message", untypedData).Msg("unhandled message type")
		}
	}
}

func (c *Conn) listenInboundMessages(wg *sync.WaitGroup, done <-chan struct{}, msg <-chan InboundMessage, sessionID string, ws *websocket.Conn) {
	defer wg.Done()
	for {
		select {
		case <-done:
			return
		case inboundReq, ok := <-msg:
			if !ok {
				c.m.Lock()
				c.inboundWasClosed = true
				c.m.Unlock()
				ws.Close(websocket.StatusNormalClosure, "consuming done")
				return
			}
			resp, err := inboundReq.Service.CreateEventSubSubscription(context.Background(), addTransportFunc(inboundReq.Req, sessionID))
			if err != nil {
				c.HandleError(err)
				c.logger.Error().Err(err).Msg("failed to create subscription")
			} else {
				c.logger.Info().Any("resp-event", resp).Msg("subscription created")
			}
		}
	}
}

func addTransportFunc(input twitch.CreateEventSubSubscriptionRequest, sessionID string) twitch.CreateEventSubSubscriptionRequest {
	input.Transport = twitch.EventSubTransportRequest{
		Method:    "websocket",
		SessionID: sessionID,
	}
	return input
}

func convertUntyped[T any](untyped untypedMessagePayload) (Message[T], error) {
	typedMessage := Message[T]{
		Metadata: untyped.Metadata,
	}

	if err := json.Unmarshal(untyped.Payload, &typedMessage.Payload); err != nil {
		return Message[T]{}, err
	}

	return typedMessage, nil
}
