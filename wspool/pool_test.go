package wspool

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/coder/websocket"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestPool_RequiresSetSend(t *testing.T) {
	t.Parallel()

	accounts := &mockAccountProvider{
		account: save.Account{ID: "123", DisplayName: "test", AccessToken: "token"},
	}

	pool := NewPool(accounts, zerolog.Nop())

	err := pool.ConnectIRC("123")
	require.Error(t, err)
	require.Contains(t, err.Error(), "SetSend not called")
}

func TestPool_IRCRefCounting(t *testing.T) {
	t.Parallel()

	var connectCount atomic.Int32
	server := newTestIRCServer(t, func(ws *websocket.Conn) {
		connectCount.Add(1)
		// Read auth
		for range 3 {
			ws.Read(context.Background())
		}
		<-time.After(2 * time.Second)
	})
	defer server.Close()

	accounts := &mockAccountProvider{
		account: save.Account{ID: "123", DisplayName: "test", AccessToken: "token"},
	}

	pool := NewPool(accounts, zerolog.Nop())
	pool.SetSend(func(tea.Msg) {})

	// Set URL before connecting
	pool.ircWSURL = wsURL(server)

	// First connect
	err := pool.ConnectIRC("123")
	require.NoError(t, err)

	// Second connect same account - should reuse
	err = pool.ConnectIRC("123")
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Should only have one connection
	pool.mu.RLock()
	connCount := len(pool.ircConns)
	refs := pool.ircConns["123"].refs
	pool.mu.RUnlock()

	require.Equal(t, 1, connCount)
	require.Equal(t, 2, refs)

	// First disconnect - should keep connection
	pool.DisconnectIRC("123")

	pool.mu.RLock()
	connCount = len(pool.ircConns)
	pool.mu.RUnlock()

	require.Equal(t, 1, connCount, "connection should remain after first disconnect")

	// Second disconnect - should close connection
	pool.DisconnectIRC("123")

	pool.mu.RLock()
	connCount = len(pool.ircConns)
	pool.mu.RUnlock()

	require.Equal(t, 0, connCount, "connection should close after all refs disconnected")
}

func TestPool_MultipleAccounts(t *testing.T) {
	t.Parallel()

	var connectCount atomic.Int32
	server := newTestIRCServer(t, func(ws *websocket.Conn) {
		connectCount.Add(1)
		// Read auth
		for range 3 {
			ws.Read(context.Background())
		}
		<-time.After(time.Second)
	})
	defer server.Close()

	accounts := &mockAccountProvider{
		account: save.Account{ID: "acc", DisplayName: "test", AccessToken: "token"},
	}

	pool := NewPool(accounts, zerolog.Nop())
	pool.SetSend(func(tea.Msg) {})
	pool.ircWSURL = wsURL(server)

	// Connect two different accounts
	err := pool.ConnectIRC("acc1")
	require.NoError(t, err)

	err = pool.ConnectIRC("acc2")
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	pool.mu.RLock()
	connCount := len(pool.ircConns)
	pool.mu.RUnlock()

	require.Equal(t, 2, connCount, "should have separate connections per account")
}

func TestPool_SendIRC(t *testing.T) {
	t.Parallel()

	msgReceived := make(chan string, 10)
	server := newTestIRCServer(t, func(ws *websocket.Conn) {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_, data, err := ws.Read(ctx)
			cancel()
			if err != nil {
				return
			}
			msgReceived <- string(data)
		}
	})
	defer server.Close()

	accounts := &mockAccountProvider{
		account: save.Account{ID: "123", DisplayName: "test", AccessToken: "token"},
	}

	pool := NewPool(accounts, zerolog.Nop())
	pool.SetSend(func(tea.Msg) {})
	pool.ircWSURL = wsURL(server)

	err := pool.ConnectIRC("123")
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(200 * time.Millisecond)

	// Send message
	err = pool.SendIRC("123", &twitchirc.PrivateMessage{
		ChannelUserName: "testchan",
		Message:         "Hello",
	})
	require.NoError(t, err)

	// Verify message received
	timeout := time.After(time.Second)
	for {
		select {
		case msg := <-msgReceived:
			if msg == "PRIVMSG #testchan :Hello" {
				return // Success
			}
		case <-timeout:
			t.Fatal("timeout waiting for message")
		}
	}
}

func TestPool_JoinChannel(t *testing.T) {
	t.Parallel()

	joinReceived := make(chan string, 10)
	server := newTestIRCServer(t, func(ws *websocket.Conn) {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_, data, err := ws.Read(ctx)
			cancel()
			if err != nil {
				return
			}
			msg := string(data)
			if len(msg) > 4 && msg[:4] == "JOIN" {
				joinReceived <- msg
			}
		}
	})
	defer server.Close()

	accounts := &mockAccountProvider{
		account: save.Account{ID: "123", DisplayName: "test", AccessToken: "token"},
	}

	pool := NewPool(accounts, zerolog.Nop())
	pool.SetSend(func(tea.Msg) {})
	pool.ircWSURL = wsURL(server)

	err := pool.ConnectIRC("123")
	require.NoError(t, err)

	// Wait for connection
	time.Sleep(200 * time.Millisecond)

	// Join channel
	err = pool.JoinChannel("123", "testchannel")
	require.NoError(t, err)

	// Verify join received
	select {
	case msg := <-joinReceived:
		require.Equal(t, "JOIN #testchannel", msg)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for JOIN")
	}
}

func TestPool_Close(t *testing.T) {
	t.Parallel()

	var connectCount atomic.Int32
	server := newTestIRCServer(t, func(ws *websocket.Conn) {
		connectCount.Add(1)
		// Read auth
		for range 3 {
			ws.Read(context.Background())
		}
		<-time.After(5 * time.Second)
	})
	defer server.Close()

	accounts := &mockAccountProvider{
		account: save.Account{ID: "123", DisplayName: "test", AccessToken: "token"},
	}

	pool := NewPool(accounts, zerolog.Nop())
	pool.SetSend(func(tea.Msg) {})
	pool.ircWSURL = wsURL(server)

	// Connect
	err := pool.ConnectIRC("123")
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Close pool
	err = pool.Close()
	require.NoError(t, err)

	// Verify connections cleaned up
	pool.mu.RLock()
	ircCount := len(pool.ircConns)
	eventCount := len(pool.eventConns)
	pool.mu.RUnlock()

	require.Equal(t, 0, ircCount)
	require.Equal(t, 0, eventCount)

	// Verify can't connect after close
	err = pool.ConnectIRC("123")
	require.Error(t, err)
	require.Contains(t, err.Error(), "pool is closed")
}

func TestPool_EventsEmittedToSend(t *testing.T) {
	t.Parallel()

	server := newTestIRCServer(t, func(ws *websocket.Conn) {
		// Read auth
		for range 3 {
			ws.Read(context.Background())
		}

		// Send a message
		msg := "@badge-info=;badges=;color=#FF0000;display-name=TestUser;emotes=;id=abc123;mod=0;room-id=456;subscriber=0;tmi-sent-ts=1234567890;turbo=0;user-id=789;user-type= :testuser!testuser@testuser.tmi.twitch.tv PRIVMSG #channel :Hello World\r\n"
		ws.Write(context.Background(), websocket.MessageText, []byte(msg))

		<-time.After(500 * time.Millisecond)
	})
	defer server.Close()

	accounts := &mockAccountProvider{
		account: save.Account{ID: "123", DisplayName: "test", AccessToken: "token"},
	}

	var received []tea.Msg
	var mu sync.Mutex

	pool := NewPool(accounts, zerolog.Nop())
	pool.SetSend(func(msg tea.Msg) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	})
	pool.ircWSURL = wsURL(server)

	err := pool.ConnectIRC("123")
	require.NoError(t, err)

	// Wait for message
	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Find IRC event
	var foundEvent bool
	for _, msg := range received {
		if evt, ok := msg.(IRCEvent); ok {
			if _, ok := evt.Message.(*twitchirc.PrivateMessage); ok {
				foundEvent = true
				require.Equal(t, "123", evt.AccountID)
				break
			}
		}
	}
	require.True(t, foundEvent, "should receive IRC event via send function")
}

func TestPool_SubscribeEventSub(t *testing.T) {
	t.Parallel()

	server := newTestEventSubServer(t, func(ws *websocket.Conn) {
		sendWelcome(ws, "session-123")
		<-time.After(500 * time.Millisecond)
	})
	defer server.Close()

	accounts := &mockAccountProvider{
		account: save.Account{ID: "123", DisplayName: "test", AccessToken: "token"},
	}

	pool := NewPool(accounts, zerolog.Nop())
	pool.SetSend(func(tea.Msg) {})
	pool.eventSubWSURL = wsURL(server)

	mockService := &mockEventSubService{}

	// Subscribe creates connection automatically
	err := pool.SubscribeEventSub("123", twitchapi.CreateEventSubSubscriptionRequest{
		Type: "channel.poll.begin",
	}, mockService)
	require.NoError(t, err)

	// Wait for subscription
	time.Sleep(300 * time.Millisecond)

	calls := mockService.getCalls()
	require.GreaterOrEqual(t, len(calls), 1)
	require.Equal(t, "channel.poll.begin", calls[0].Type)
}
