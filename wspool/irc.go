package wspool

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/rs/zerolog"
)

// ircConn wraps twitchirc.Conn with reference counting for pool management.
type ircConn struct {
	*twitchirc.Conn

	mu   sync.Mutex
	refs int
}

func newIRCConn(accountID string, accounts AccountProvider, logger zerolog.Logger, sendFn func(tea.Msg)) *ircConn {
	conn := &ircConn{}

	// Create the underlying connection with a callback that wraps messages in IRCEvent
	conn.Conn = twitchirc.NewConn(accountID, accounts, logger, func(msg twitchirc.IRCer, err error) {
		if err != nil {
			sendFn(IRCEvent{AccountID: accountID, Error: err})
		} else {
			sendFn(IRCEvent{AccountID: accountID, Message: msg})
		}
	})

	return conn
}

func (c *ircConn) incRef() int {
	c.mu.Lock()
	c.refs++
	refs := c.refs
	c.mu.Unlock()
	return refs
}

func (c *ircConn) decRef() int {
	c.mu.Lock()
	c.refs--
	refs := c.refs
	c.mu.Unlock()
	return refs
}
