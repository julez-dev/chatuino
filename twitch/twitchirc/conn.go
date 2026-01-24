package twitchirc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/julez-dev/chatuino/save"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

const (
	DefaultIRCWSURL   = "wss://irc-ws.chat.twitch.tv:443"
	ircDialTimeout    = 5 * time.Second
	ircPingInterval   = 10 * time.Second
	ircPingTimeout    = 5 * time.Second
	ircReconnectDelay = 5 * time.Second
	ircMaxMessageSize = 1 * 1024 * 1024 // 1MiB
	ircSendBufferSize = 64
)

// ConnAccountProvider retrieves account credentials for IRC authentication.
type ConnAccountProvider interface {
	GetAccountBy(id string) (save.Account, error)
}

// Conn manages a single IRC WebSocket connection with automatic reconnection.
type Conn struct {
	accountID string
	accounts  ConnAccountProvider
	logger    zerolog.Logger
	sendFn    func(msg IRCer, err error)

	ctx    context.Context
	cancel context.CancelFunc

	sendCh chan IRCer

	mu       sync.Mutex
	channels []string
	refs     int
	closed   bool

	// WSURL allows overriding the WebSocket URL for testing
	WSURL string
}

// NewConn creates a new IRC connection for the given account.
// sendFn is called for each received message or error.
func NewConn(accountID string, accounts ConnAccountProvider, logger zerolog.Logger, sendFn func(msg IRCer, err error)) *Conn {
	ctx, cancel := context.WithCancel(context.Background())
	return &Conn{
		accountID: accountID,
		accounts:  accounts,
		logger:    logger.With().Str("account_id", accountID).Str("conn", "irc").Logger(),
		sendFn:    sendFn,
		ctx:       ctx,
		cancel:    cancel,
		sendCh:    make(chan IRCer, ircSendBufferSize),
		WSURL:     DefaultIRCWSURL,
	}
}

// IncRef increments the reference count and returns the new count.
func (c *Conn) IncRef() int {
	c.mu.Lock()
	c.refs++
	refs := c.refs
	c.mu.Unlock()
	return refs
}

// DecRef decrements the reference count and returns the new count.
func (c *Conn) DecRef() int {
	c.mu.Lock()
	c.refs--
	refs := c.refs
	c.mu.Unlock()
	return refs
}

// Close stops the connection and all goroutines.
func (c *Conn) Close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.cancel()
}

// Send queues a message to be sent over the connection.
func (c *Conn) Send(msg IRCer) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("connection closed")
	}
	c.mu.Unlock()

	select {
	case c.sendCh <- msg:
		return nil
	case <-c.ctx.Done():
		return errors.New("connection closed")
	}
}

// JoinChannel joins a channel and tracks it for rejoin on reconnect.
func (c *Conn) JoinChannel(channel string) error {
	c.mu.Lock()
	if !slices.Contains(c.channels, channel) {
		c.channels = append(c.channels, channel)
	}
	c.mu.Unlock()

	return c.Send(JoinMessage{Channel: channel})
}

func (c *Conn) getChannels() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.channels)
}

func (c *Conn) emit(msg IRCer) {
	c.sendFn(msg, nil)
}

func (c *Conn) emitError(err error) {
	c.sendFn(nil, err)
}

// Run is the main loop that maintains the connection with automatic reconnect.
// It blocks until Close is called or the context is cancelled.
func (c *Conn) Run() {
	defer close(c.sendCh)

	for {
		err := c.connectOnce()
		if c.ctx.Err() != nil {
			c.logger.Info().Msg("connection stopped (context cancelled)")
			return
		}

		if err != nil {
			c.logger.Warn().Err(err).Msg("connection error, will reconnect")
			c.emitError(fmt.Errorf("disconnected from chat server: %w", err))
		}

		select {
		case <-c.ctx.Done():
			return
		case <-time.After(ircReconnectDelay):
			c.logger.Info().Msg("reconnecting...")
		}
	}
}

func (c *Conn) connectOnce() error {
	dialCtx, dialCancel := context.WithTimeout(c.ctx, ircDialTimeout)
	defer dialCancel()

	ws, _, err := websocket.Dial(dialCtx, c.WSURL, &websocket.DialOptions{
		HTTPClient: &http.Client{Timeout: ircDialTimeout * 2},
	})
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}
	defer ws.Close(websocket.StatusNormalClosure, "closing")

	ws.SetReadLimit(ircMaxMessageSize)

	if err := c.authenticate(ws); err != nil {
		return fmt.Errorf("auth failed: %w", err)
	}

	// Rejoin channels after reconnect
	for _, ch := range c.getChannels() {
		msg := fmt.Sprintf("JOIN #%s", ch)
		if err := ws.Write(c.ctx, websocket.MessageText, []byte(msg)); err != nil {
			return fmt.Errorf("rejoin failed: %w", err)
		}
	}

	// Run reader/writer/pinger concurrently
	g, ctx := errgroup.WithContext(c.ctx)

	// Internal channel for PONG messages (reader → writer)
	pongCh := make(chan struct{}, 1)

	g.Go(func() error {
		return c.readLoop(ctx, ws, pongCh)
	})

	g.Go(func() error {
		return c.writeLoop(ctx, ws, pongCh)
	})

	g.Go(func() error {
		return c.pingLoop(ctx, ws)
	})

	return g.Wait()
}

func (c *Conn) authenticate(ws *websocket.Conn) error {
	account, err := c.accounts.GetAccountBy(c.accountID)
	if err != nil {
		return fmt.Errorf("get account: %w", err)
	}

	oauth := account.AccessToken
	if !strings.HasPrefix(oauth, "oauth:") {
		oauth = "oauth:" + oauth
	}

	authMsgs := []string{
		fmt.Sprintf("PASS %s", oauth),
		fmt.Sprintf("NICK %s", account.DisplayName),
		"CAP REQ :twitch.tv/membership twitch.tv/tags twitch.tv/commands",
	}

	for _, msg := range authMsgs {
		if err := ws.Write(c.ctx, websocket.MessageText, []byte(msg)); err != nil {
			return err
		}
	}

	return nil
}

func (c *Conn) readLoop(ctx context.Context, ws *websocket.Conn, pongCh chan<- struct{}) error {
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			return err
		}

		// Twitch may send multiple messages in one frame
		for _, line := range strings.Split(string(data), "\r\n") {
			if line == "" {
				continue
			}

			parsed, err := ParseIRC(line)
			if err != nil {
				if errors.Is(err, ErrUnhandledCommand) {
					// Ignore PART, JOIN, tmi.twitch.tv notices
					if !strings.Contains(line, "PART") &&
						!strings.Contains(line, "JOIN") &&
						!strings.HasPrefix(line, ":tmi.twitch.tv") {
						c.logger.Debug().Str("line", line).Msg("unhandled IRC command")
					}
					continue
				}
				return fmt.Errorf("parse error: %w", err)
			}

			// Handle PING → signal writer to send PONG
			if _, ok := parsed.(PingMessage); ok {
				select {
				case pongCh <- struct{}{}:
				default:
				}
				continue
			}

			c.emit(parsed)
		}
	}
}

func (c *Conn) writeLoop(ctx context.Context, ws *websocket.Conn, pongCh <-chan struct{}) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pongCh:
			if err := ws.Write(ctx, websocket.MessageText, []byte("PONG")); err != nil {
				return err
			}
		case msg, ok := <-c.sendCh:
			if !ok {
				return nil
			}
			if err := ws.Write(ctx, websocket.MessageText, []byte(msg.IRC())); err != nil {
				return err
			}
		}
	}
}

func (c *Conn) pingLoop(ctx context.Context, ws *websocket.Conn) error {
	ticker := time.NewTicker(ircPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, ircPingTimeout)
			err := ws.Ping(pingCtx)
			cancel()
			if err != nil {
				return fmt.Errorf("ping timeout: %w", err)
			}
		}
	}
}
