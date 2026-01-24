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
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	maxMessageSize = 5 * 1024 * 1024 // 5MB
	//eventSubURL    = "wss://eventsub.wss.twitch.tv/ws?keepalive_timeout_seconds=30"
	eventSubURL = "ws://127.0.0.1:8080/ws?keepalive_timeout_seconds=30"
)

type EventSubService interface {
	CreateEventSubSubscription(ctx context.Context, reqData twitchapi.CreateEventSubSubscriptionRequest) (twitchapi.CreateEventSubSubscriptionResponse, error)
}

type InboundMessage struct {
	Req     twitchapi.CreateEventSubSubscriptionRequest
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
		duplicate: ttlcache.New(
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
func (c *Conn) Connect(ctx context.Context, inbound <-chan InboundMessage) error {
	defer func() {
		log.Logger.Info().Msg("Connect(inbound <-chan InboundMessage) drain start")

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

	log.Logger.Info().Str("t", initial.Req.Type).Msg("Connect(inbound <-chan InboundMessage) got intial message")

	subURL := eventSubURL
	var err error
	for {
		log.Logger.Info().Str("t", initial.Req.Type).Msg("Connect(inbound <-chan InboundMessage) for loop start")

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

		log.Logger.Info().Str("t", initial.Req.Type).Msg("Connect(inbound <-chan InboundMessage) startListeningWS()")
		if err = c.startListeningWS(ctx, subURL, inbound, maybeInitial); err != nil {
			var forcedReconnectErr twitchForcedReconnect
			if errors.As(err, &forcedReconnectErr) {
				subURL = forcedReconnectErr.NewWSURL
				continue
			}

			c.logger.Err(err).Msg("failed during startListeningWS")
		}

		log.Logger.Info().Str("t", initial.Req.Type).Msg("Connect(inbound <-chan InboundMessage) for loop end")
	}
}

func (c *Conn) startListeningWS(ctx context.Context, eventSubURL string, inboundChan <-chan InboundMessage, initialInbound *InboundMessage) error {
	dialCtx, dialCancel := context.WithTimeout(ctx, time.Second*30)
	defer dialCancel()

	ws, _, err := websocket.Dial(dialCtx, eventSubURL, &websocket.DialOptions{
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
		log.Logger.Info().Msg("startListeningWS defer done")
	}()

	for {
		log.Logger.Info().Msg("startListeningWS for loop start")

		_, data, err := ws.Read(ctx)
		if err != nil {
			// Check if context was cancelled (clean shutdown)
			if ctx.Err() != nil {
				c.logger.Info().Msg("EventSub connection cancelled by context")
				return nil
			}
			return fmt.Errorf("failed to read message: %w", err)
		}
		log.Logger.Info().Msg("startListeningWS ws.Read done")

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
				log.Logger.Info().Str("session_id", welcome.Payload.Session.ID).Msg("Creating initial subscription")

				_, err := initialInbound.Service.CreateEventSubSubscription(context.Background(), addTransportFunc(initialInbound.Req, welcome.Payload.Session.ID))
				if err != nil {
					err := fmt.Errorf("failed to create initial subscription: %w", err)
					log.Logger.Error().Err(err).Msg("Failed to create initial subscription")
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
		log.Logger.Info().Msg("startListeningWS for loop done")

	}
}

func (c *Conn) listenInboundMessages(wg *sync.WaitGroup, done <-chan struct{}, msg <-chan InboundMessage, sessionID string, ws *websocket.Conn) {
	defer func() {
		log.Logger.Info().Msg("listenInboundMessages defer start")
		wg.Done()
		log.Logger.Info().Msg("listenInboundMessages defer done")
	}()
	for {
		log.Logger.Info().Msg("listenInboundMessages for pre select")

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

		log.Logger.Info().Msg("listenInboundMessages for post select")

	}
}

func addTransportFunc(input twitchapi.CreateEventSubSubscriptionRequest, sessionID string) twitchapi.CreateEventSubSubscriptionRequest {
	input.Transport = twitchapi.EventSubTransportRequest{
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
