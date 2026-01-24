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
)

const (
	DefaultEventSubURL = "wss://eventsub.wss.twitch.tv/ws?keepalive_timeout_seconds=30"
	//DefaultEventSubURL     = "ws://127.0.0.1:8080/ws"
	eventSubDialTimeout    = 30 * time.Second
	eventSubReconnectDelay = 5 * time.Second
	eventSubDuplicateTTL   = 15 * time.Minute
	subscriptionTimeout    = 10 * time.Second
)

// ConnEventSubService creates EventSub subscriptions via the Twitch API.
type ConnEventSubService interface {
	CreateEventSubSubscription(ctx context.Context, reqData twitchapi.CreateEventSubSubscriptionRequest) (twitchapi.CreateEventSubSubscriptionResponse, error)
}

// twitchForcedReconnect signals that Twitch wants us to reconnect to a new URL.
type twitchForcedReconnect struct {
	newURL string
}

func (t twitchForcedReconnect) Error() string {
	return "twitch forced reconnect"
}

// SubRequest represents a subscription request with its service.
type SubRequest struct {
	Req     twitchapi.CreateEventSubSubscriptionRequest
	Service ConnEventSubService
}

// Conn manages a single EventSub WebSocket connection with automatic reconnection.
type Conn struct {
	accountID  string
	logger     zerolog.Logger
	httpClient *http.Client
	sendFn     func(msg Message[NotificationPayload], err error)

	ctx    context.Context
	cancel context.CancelFunc

	subReqCh  chan SubRequest
	duplicate *ttlcache.Cache[string, struct{}]

	mu         sync.Mutex
	activeSubs []SubRequest // tracked for resubscription on reconnect
	closed     bool

	// WSURL allows overriding the WebSocket URL for testing
	WSURL string
}

// NewConnEventSub creates a new EventSub connection for the given account.
// sendFn is called for each received notification or error.
func NewConnEventSub(accountID string, logger zerolog.Logger, httpClient *http.Client, sendFn func(msg Message[NotificationPayload], err error)) *Conn {
	ctx, cancel := context.WithCancel(context.Background())

	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Conn{
		accountID:  accountID,
		logger:     logger.With().Str("account_id", accountID).Str("conn", "eventsub").Logger(),
		httpClient: httpClient,
		sendFn:     sendFn,
		ctx:        ctx,
		cancel:     cancel,
		subReqCh:   make(chan SubRequest, 16),
		duplicate: ttlcache.New(
			ttlcache.WithTTL[string, struct{}](eventSubDuplicateTTL),
		),
		WSURL: DefaultEventSubURL,
	}
}

// Close stops the connection and all goroutines.
func (c *Conn) Close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.cancel()
}

// Subscribe queues a subscription request to be created.
func (c *Conn) Subscribe(req twitchapi.CreateEventSubSubscriptionRequest, service ConnEventSubService) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	select {
	case c.subReqCh <- SubRequest{Req: req, Service: service}:
	case <-c.ctx.Done():
	}
}

func (c *Conn) emit(msg Message[NotificationPayload]) {
	c.sendFn(msg, nil)
}

func (c *Conn) emitError(err error) {
	c.sendFn(Message[NotificationPayload]{}, err)
}

func (c *Conn) trackSubscription(req SubRequest) {
	c.mu.Lock()
	c.activeSubs = append(c.activeSubs, req)
	c.mu.Unlock()
}

func (c *Conn) getActiveSubscriptions() []SubRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]SubRequest, len(c.activeSubs))
	copy(result, c.activeSubs)
	return result
}

// Run is the main loop. It waits for the first subscription before connecting,
// then maintains the connection with automatic reconnect.
// It blocks until Close is called or the context is cancelled.
func (c *Conn) Run() {
	defer func() {
		c.duplicate.Stop()
		close(c.subReqCh)
	}()

	go c.duplicate.Start()

	// Wait for first subscription before connecting
	// Twitch closes connection if no subscription within 10s
	var initial *SubRequest
	select {
	case req, ok := <-c.subReqCh:
		if !ok {
			return
		}
		initial = &req
	case <-c.ctx.Done():
		return
	}

	url := c.WSURL
	for {
		if c.ctx.Err() != nil {
			c.logger.Info().Msg("connection stopped (context cancelled)")
			return
		}

		err := c.connectOnce(url, initial)
		initial = nil // only use initial on first connect

		if c.ctx.Err() != nil {
			return
		}

		var forced twitchForcedReconnect
		if errors.As(err, &forced) {
			c.logger.Info().Str("new_url", forced.newURL).Msg("forced reconnect to new URL")
			url = forced.newURL
			continue
		}

		if err != nil {
			c.logger.Warn().Err(err).Msg("connection error, will reconnect")
			c.emitError(fmt.Errorf("EventSub disconnected: %w", err))
		}

		select {
		case <-c.ctx.Done():
			return
		case <-time.After(eventSubReconnectDelay):
			c.logger.Info().Msg("reconnecting...")
			url = c.WSURL // reset to default URL on error reconnect
		}
	}
}

func (c *Conn) connectOnce(wsURL string, initial *SubRequest) error {
	dialCtx, dialCancel := context.WithTimeout(c.ctx, eventSubDialTimeout)
	defer dialCancel()

	ws, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{
		HTTPClient: c.httpClient,
	})
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}
	defer ws.Close(websocket.StatusNormalClosure, "closing")

	// Wait for session_welcome to get session ID
	sessionID, _, err := c.waitForWelcome(ws)
	if err != nil {
		return err
	}

	// Create initial subscription if provided (first connection only)
	if initial != nil {
		if err := c.createSubscription(sessionID, *initial); err != nil {
			c.logger.Error().Err(err).Msg("failed to create initial subscription")
			c.emitError(err)
		} else {
			c.trackSubscription(*initial)
		}
	} else {
		// Resubscribe all tracked subscriptions (reconnect case)
		for _, sub := range c.getActiveSubscriptions() {
			if err := c.createSubscription(sessionID, sub); err != nil {
				c.logger.Error().Err(err).Str("type", sub.Req.Type).Msg("failed to resubscribe")
				c.emitError(err)
			}
		}
	}

	// Start subscription listener in background
	subDone := make(chan struct{})
	go c.subscriptionListener(sessionID, subDone)
	defer close(subDone)

	// Main read loop
	return c.readLoop(ws)
}

func (c *Conn) waitForWelcome(ws *websocket.Conn) (sessionID, reconnectURL string, err error) {
	_, data, err := ws.Read(c.ctx)
	if err != nil {
		return "", "", fmt.Errorf("read welcome: %w", err)
	}

	var msg struct {
		Metadata struct {
			MessageType string `json:"message_type"`
		} `json:"metadata"`
		Payload struct {
			Session struct {
				ID           string `json:"id"`
				ReconnectURL string `json:"reconnect_url"`
			} `json:"session"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(data, &msg); err != nil {
		return "", "", fmt.Errorf("parse welcome: %w", err)
	}

	if msg.Metadata.MessageType != "session_welcome" {
		return "", "", fmt.Errorf("expected session_welcome, got %s", msg.Metadata.MessageType)
	}

	c.logger.Info().Str("session_id", msg.Payload.Session.ID).Msg("received session_welcome")
	return msg.Payload.Session.ID, msg.Payload.Session.ReconnectURL, nil
}

func (c *Conn) createSubscription(sessionID string, sub SubRequest) error {
	req := sub.Req
	req.Transport = twitchapi.EventSubTransportRequest{
		Method:    "websocket",
		SessionID: sessionID,
	}

	ctx, cancel := context.WithTimeout(c.ctx, subscriptionTimeout)
	defer cancel()

	_, err := sub.Service.CreateEventSubSubscription(ctx, req)
	if err != nil {
		return fmt.Errorf("create subscription %s: %w", req.Type, err)
	}

	c.logger.Info().Str("type", req.Type).Msg("subscription created")
	return nil
}

func (c *Conn) subscriptionListener(sessionID string, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case <-c.ctx.Done():
			return
		case req, ok := <-c.subReqCh:
			if !ok {
				return
			}
			if err := c.createSubscription(sessionID, req); err != nil {
				c.logger.Error().Err(err).Str("type", req.Req.Type).Msg("failed to create subscription")
				c.emitError(err)
			} else {
				c.trackSubscription(req)
			}
		}
	}
}

func (c *Conn) readLoop(ws *websocket.Conn) error {
	for {
		_, data, err := ws.Read(c.ctx)
		if err != nil {
			if c.ctx.Err() != nil {
				return nil
			}
			return err
		}

		var msg struct {
			Metadata struct {
				MessageID   string `json:"message_id"`
				MessageType string `json:"message_type"`
			} `json:"metadata"`
			Payload json.RawMessage `json:"payload"`
		}

		if err := json.Unmarshal(data, &msg); err != nil {
			c.logger.Warn().Err(err).Msg("failed to parse message")
			continue
		}

		switch msg.Metadata.MessageType {
		case "session_keepalive":
			// Nothing to do
			continue

		case "session_reconnect":
			var payload struct {
				Session struct {
					ReconnectURL string `json:"reconnect_url"`
				} `json:"session"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				return fmt.Errorf("parse reconnect: %w", err)
			}
			return twitchForcedReconnect{newURL: payload.Session.ReconnectURL}

		case "notification":
			// Check for duplicate
			if c.duplicate.Has(msg.Metadata.MessageID) {
				continue
			}
			c.duplicate.Set(msg.Metadata.MessageID, struct{}{}, ttlcache.DefaultTTL)

			// Parse the full message to get proper Message[NotificationPayload]
			var typedMsg Message[NotificationPayload]
			if err := json.Unmarshal(data, &typedMsg); err != nil {
				c.logger.Warn().Err(err).Msg("failed to parse notification")
				continue
			}

			c.emit(typedMsg)

		default:
			c.logger.Debug().Str("type", msg.Metadata.MessageType).Msg("unhandled message type")
		}
	}
}
