package wspool

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/coder/websocket"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type mockAccountProvider struct {
	account save.Account
	err     error
}

func (m *mockAccountProvider) GetAccountBy(id string) (save.Account, error) {
	return m.account, m.err
}

func newTestIRCServer(t *testing.T, handler func(ws *websocket.Conn)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Logf("websocket accept error: %v", err)
			return
		}
		defer ws.Close(websocket.StatusNormalClosure, "")
		handler(ws)
	}))
}

func wsURL(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

func TestIRCConn_Connect_Auth(t *testing.T) {
	t.Parallel()

	authReceived := make(chan []string, 3)

	server := newTestIRCServer(t, func(ws *websocket.Conn) {
		// Read auth messages
		for i := 0; i < 3; i++ {
			_, data, err := ws.Read(context.Background())
			if err != nil {
				return
			}
			authReceived <- strings.Split(string(data), "\r\n")
		}

		// Send welcome
		ws.Write(context.Background(), websocket.MessageText, []byte(":tmi.twitch.tv 001 testuser :Welcome\r\n"))

		// Keep connection open briefly
		<-time.After(100 * time.Millisecond)
	})
	defer server.Close()

	accounts := &mockAccountProvider{
		account: save.Account{
			ID:          "123",
			DisplayName: "testuser",
			AccessToken: "test-token",
		},
	}

	conn := newIRCConn("123", accounts, zerolog.Nop(), func(tea.Msg) {})
	conn.WSURL = wsURL(server)

	go conn.Run()
	defer conn.Close()

	// Verify auth messages
	select {
	case msgs := <-authReceived:
		require.Contains(t, msgs[0], "PASS oauth:test-token")
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PASS")
	}

	select {
	case msgs := <-authReceived:
		require.Contains(t, msgs[0], "NICK testuser")
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for NICK")
	}

	select {
	case msgs := <-authReceived:
		require.Contains(t, msgs[0], "CAP REQ")
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for CAP REQ")
	}
}

func TestIRCConn_ReceiveMessage(t *testing.T) {
	t.Parallel()

	server := newTestIRCServer(t, func(ws *websocket.Conn) {
		// Read auth (3 messages)
		for i := 0; i < 3; i++ {
			ws.Read(context.Background())
		}

		// Send a PRIVMSG
		msg := "@badge-info=;badges=;color=#FF0000;display-name=TestUser;emotes=;id=abc123;mod=0;room-id=456;subscriber=0;tmi-sent-ts=1234567890;turbo=0;user-id=789;user-type= :testuser!testuser@testuser.tmi.twitch.tv PRIVMSG #channel :Hello World\r\n"
		ws.Write(context.Background(), websocket.MessageText, []byte(msg))

		<-time.After(100 * time.Millisecond)
	})
	defer server.Close()

	accounts := &mockAccountProvider{
		account: save.Account{ID: "123", DisplayName: "testuser", AccessToken: "token"},
	}

	var received []tea.Msg
	var mu sync.Mutex
	sendFn := func(msg tea.Msg) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	conn := newIRCConn("123", accounts, zerolog.Nop(), sendFn)
	conn.WSURL = wsURL(server)

	go conn.Run()
	defer conn.Close()

	// Wait for message
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Find PRIVMSG event
	var foundPrivmsg bool
	for _, msg := range received {
		if evt, ok := msg.(IRCEvent); ok {
			if _, ok := evt.Message.(*twitchirc.PrivateMessage); ok {
				foundPrivmsg = true
				require.Equal(t, "123", evt.AccountID)
				break
			}
		}
	}
	require.True(t, foundPrivmsg, "should receive PRIVMSG")
}

func TestIRCConn_PingPong(t *testing.T) {
	t.Parallel()

	pongReceived := make(chan struct{}, 1)

	server := newTestIRCServer(t, func(ws *websocket.Conn) {
		// Read auth
		for i := 0; i < 3; i++ {
			ws.Read(context.Background())
		}

		// Send PING
		ws.Write(context.Background(), websocket.MessageText, []byte("PING :tmi.twitch.tv\r\n"))

		// Wait for PONG
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, data, err := ws.Read(ctx)
		if err == nil && strings.Contains(string(data), "PONG") {
			pongReceived <- struct{}{}
		}
	})
	defer server.Close()

	accounts := &mockAccountProvider{
		account: save.Account{ID: "123", DisplayName: "testuser", AccessToken: "token"},
	}

	conn := newIRCConn("123", accounts, zerolog.Nop(), func(tea.Msg) {})
	conn.WSURL = wsURL(server)

	go conn.Run()
	defer conn.Close()

	select {
	case <-pongReceived:
		// Success
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PONG")
	}
}

func TestIRCConn_Reconnect(t *testing.T) {
	t.Parallel()

	var connectCount atomic.Int32

	server := newTestIRCServer(t, func(ws *websocket.Conn) {
		count := connectCount.Add(1)

		// Read auth
		for i := 0; i < 3; i++ {
			ws.Read(context.Background())
		}

		if count == 1 {
			// First connection: close immediately to trigger reconnect
			ws.Close(websocket.StatusGoingAway, "bye")
			return
		}

		// Second connection: stay open
		<-time.After(200 * time.Millisecond)
	})
	defer server.Close()

	accounts := &mockAccountProvider{
		account: save.Account{ID: "123", DisplayName: "testuser", AccessToken: "token"},
	}

	var errorCount atomic.Int32
	sendFn := func(msg tea.Msg) {
		if evt, ok := msg.(IRCEvent); ok && evt.Error != nil {
			errorCount.Add(1)
		}
	}

	conn := newIRCConn("123", accounts, zerolog.Nop(), sendFn)
	conn.WSURL = wsURL(server)

	go conn.Run()
	defer conn.Close()

	// Wait for reconnect (5s delay + connection time)
	time.Sleep(6 * time.Second)

	require.GreaterOrEqual(t, connectCount.Load(), int32(2), "should reconnect after disconnect")
	require.GreaterOrEqual(t, errorCount.Load(), int32(1), "should emit error on disconnect")
}

func TestIRCConn_ChannelRejoin(t *testing.T) {
	t.Parallel()

	var connectCount atomic.Int32
	joinReceived := make(chan string, 10)

	server := newTestIRCServer(t, func(ws *websocket.Conn) {
		count := connectCount.Add(1)

		// Read messages until connection closes
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			_, data, err := ws.Read(ctx)
			cancel()
			if err != nil {
				break
			}
			msg := string(data)
			if strings.Contains(msg, "JOIN") {
				joinReceived <- msg
			}
		}

		if count == 1 {
			// Close first connection
			return
		}

		// Keep second connection open
		<-time.After(200 * time.Millisecond)
	})
	defer server.Close()

	accounts := &mockAccountProvider{
		account: save.Account{ID: "123", DisplayName: "testuser", AccessToken: "token"},
	}

	conn := newIRCConn("123", accounts, zerolog.Nop(), func(tea.Msg) {})
	conn.WSURL = wsURL(server)

	// Pre-populate channels for rejoin via JoinChannel
	// Note: JoinChannel will send JOIN message immediately, but we call it before Run()
	// so it will be queued and sent on first connect, then rejoined on reconnect
	go conn.Run()
	// Small delay to ensure connection starts before we join
	time.Sleep(50 * time.Millisecond)
	conn.JoinChannel("channel1")
	conn.JoinChannel("channel2")
	defer conn.Close()

	// Collect JOINs (wait for reconnect: 5s delay + connection time)
	var joins []string
	timeout := time.After(6 * time.Second)
loop:
	for {
		select {
		case j := <-joinReceived:
			joins = append(joins, j)
			if len(joins) >= 4 { // 2 channels * 2 connections
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	// Should have JOINs from both connections
	var channel1Joins, channel2Joins int
	for _, j := range joins {
		if strings.Contains(j, "channel1") {
			channel1Joins++
		}
		if strings.Contains(j, "channel2") {
			channel2Joins++
		}
	}

	require.GreaterOrEqual(t, channel1Joins, 2, "channel1 should be joined on reconnect")
	require.GreaterOrEqual(t, channel2Joins, 2, "channel2 should be joined on reconnect")
}

func TestIRCConn_SendMessage(t *testing.T) {
	t.Parallel()

	msgReceived := make(chan string, 10)

	server := newTestIRCServer(t, func(ws *websocket.Conn) {
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			_, data, err := ws.Read(ctx)
			cancel()
			if err != nil {
				return
			}
			msg := string(data)
			if strings.Contains(msg, "PRIVMSG") {
				msgReceived <- msg
			}
		}
	})
	defer server.Close()

	accounts := &mockAccountProvider{
		account: save.Account{ID: "123", DisplayName: "testuser", AccessToken: "token"},
	}

	conn := newIRCConn("123", accounts, zerolog.Nop(), func(tea.Msg) {})
	conn.WSURL = wsURL(server)

	go conn.Run()
	defer conn.Close()

	// Wait for connection to establish
	time.Sleep(100 * time.Millisecond)

	// Send a message
	err := conn.Send(&twitchirc.PrivateMessage{
		ChannelUserName: "testchannel",
		Message:         "Hello World",
	})
	require.NoError(t, err)

	select {
	case msg := <-msgReceived:
		require.Contains(t, msg, "PRIVMSG #testchannel :Hello World")
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PRIVMSG")
	}
}

func TestIRCConn_RefCounting(t *testing.T) {
	t.Parallel()

	conn := &ircConn{}

	conn.incRef()
	require.Equal(t, 1, conn.refs)

	conn.incRef()
	require.Equal(t, 2, conn.refs)

	refs := conn.decRef()
	require.Equal(t, 1, refs)
	require.Equal(t, 1, conn.refs)

	refs = conn.decRef()
	require.Equal(t, 0, refs)
	require.Equal(t, 0, conn.refs)
}
