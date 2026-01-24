package wspool

import (
	"context"
	"errors"
	"net/http"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/rs/zerolog"
)

// AccountProvider retrieves account credentials for authentication.
type AccountProvider interface {
	GetAccountBy(id string) (save.Account, error)
}

// EventSubService creates EventSub subscriptions via the Twitch API.
type EventSubService interface {
	CreateEventSubSubscription(ctx context.Context, reqData twitchapi.CreateEventSubSubscriptionRequest) (twitchapi.CreateEventSubSubscriptionResponse, error)
}

// Pool manages WebSocket connections for IRC chat and EventSub.
// Connections are lazily created per account and reference-counted.
type Pool struct {
	mu       sync.RWMutex
	send     func(tea.Msg)
	accounts AccountProvider
	logger   zerolog.Logger

	ircConns   map[string]*ircConn
	eventConns map[string]*eventConn

	closed bool

	// For testing: override default WebSocket URLs
	ircWSURL      string
	eventSubWSURL string
}

// NewPool creates a new connection pool.
// Call SetSend() before using Connect/Subscribe methods.
func NewPool(accounts AccountProvider, logger zerolog.Logger) *Pool {
	return &Pool{
		accounts:   accounts,
		logger:     logger.With().Str("component", "wspool").Logger(),
		ircConns:   make(map[string]*ircConn),
		eventConns: make(map[string]*eventConn),
	}
}

// SetSend sets the callback for sending messages to the UI.
// Must be called after tea.NewProgram() is created, before any connections.
// Typically: pool.SetSend(program.Send)
func (p *Pool) SetSend(send func(tea.Msg)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.send = send
}

// ConnectIRC increments the reference count for an account's IRC connection.
// Creates a new connection if one doesn't exist.
func (p *Pool) ConnectIRC(accountID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return errors.New("pool is closed")
	}

	if p.send == nil {
		return errors.New("SetSend not called")
	}

	conn, exists := p.ircConns[accountID]
	if exists {
		refs := conn.incRef()
		p.logger.Debug().Str("account_id", accountID).Int("refs", refs).Msg("incremented IRC ref count")
		return nil
	}

	// Create new connection
	conn = newIRCConn(accountID, p.accounts, p.logger, p.send)
	if p.ircWSURL != "" {
		conn.WSURL = p.ircWSURL
	}
	p.ircConns[accountID] = conn
	_ = conn.incRef()

	go conn.Run()

	p.logger.Info().Str("account_id", accountID).Msg("created new IRC connection")
	return nil
}

// DisconnectIRC decrements the reference count for an account's IRC connection.
// Closes the connection when the count reaches zero.
func (p *Pool) DisconnectIRC(accountID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	conn, exists := p.ircConns[accountID]
	if !exists {
		return
	}

	refs := conn.decRef()
	p.logger.Debug().Str("account_id", accountID).Int("refs", refs).Msg("decremented IRC ref count")

	if refs <= 0 {
		conn.Close()
		delete(p.ircConns, accountID)
		p.logger.Info().Str("account_id", accountID).Msg("closed IRC connection")
	}
}

// SendIRC sends a message through an account's IRC connection.
func (p *Pool) SendIRC(accountID string, msg twitchirc.IRCer) error {
	p.mu.RLock()
	conn, exists := p.ircConns[accountID]
	p.mu.RUnlock()

	if !exists {
		return errors.New("no IRC connection for account")
	}

	return conn.Send(msg)
}

// JoinChannel joins an IRC channel and tracks it for rejoin on reconnect.
func (p *Pool) JoinChannel(accountID, channel string) error {
	p.mu.RLock()
	conn, exists := p.ircConns[accountID]
	p.mu.RUnlock()

	if !exists {
		return errors.New("no IRC connection for account")
	}

	return conn.JoinChannel(channel)
}

// SubscribeEventSub creates an EventSub subscription.
// Creates a new connection for the account if one doesn't exist.
func (p *Pool) SubscribeEventSub(accountID string, req twitchapi.CreateEventSubSubscriptionRequest, service EventSubService) error {
	p.mu.Lock()

	if p.closed {
		p.mu.Unlock()
		return errors.New("pool is closed")
	}

	if p.send == nil {
		p.mu.Unlock()
		return errors.New("SetSend not called")
	}

	conn, exists := p.eventConns[accountID]
	if !exists {
		conn = newEventConn(accountID, p.logger, http.DefaultClient, p.send)
		if p.eventSubWSURL != "" {
			conn.WSURL = p.eventSubWSURL
		}
		p.eventConns[accountID] = conn
		go conn.Run()
		p.logger.Info().Str("account_id", accountID).Msg("created new EventSub connection")
	}

	// Call subscribe while still holding lock to prevent Close() from
	// closing the connection before we subscribe. The subscribe method
	// checks its own closed flag and won't block.
	conn.subscribe(req, service)

	p.mu.Unlock()
	return nil
}

// Close closes all connections and prevents new ones.
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true

	for id, conn := range p.ircConns {
		conn.Close()
		delete(p.ircConns, id)
	}

	for id, conn := range p.eventConns {
		conn.Close()
		delete(p.eventConns, id)
	}

	p.logger.Info().Msg("pool closed")
	return nil
}
