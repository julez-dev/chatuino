package wspool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/coder/websocket"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type mockEventSubService struct {
	mu    sync.Mutex
	calls []twitchapi.CreateEventSubSubscriptionRequest
	err   error
}

func (m *mockEventSubService) CreateEventSubSubscription(ctx context.Context, req twitchapi.CreateEventSubSubscriptionRequest) (twitchapi.CreateEventSubSubscriptionResponse, error) {
	m.mu.Lock()
	m.calls = append(m.calls, req)
	m.mu.Unlock()
	return twitchapi.CreateEventSubSubscriptionResponse{}, m.err
}

func (m *mockEventSubService) getCalls() []twitchapi.CreateEventSubSubscriptionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]twitchapi.CreateEventSubSubscriptionRequest(nil), m.calls...)
}

func newTestEventSubServer(t *testing.T, handler func(ws *websocket.Conn)) *httptest.Server {
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

func sendWelcome(ws *websocket.Conn, sessionID string) error {
	welcome := map[string]any{
		"metadata": map[string]string{
			"message_id":   "test-id",
			"message_type": "session_welcome",
		},
		"payload": map[string]any{
			"session": map[string]any{
				"id":     sessionID,
				"status": "connected",
			},
		},
	}
	data, _ := json.Marshal(welcome)
	return ws.Write(context.Background(), websocket.MessageText, data)
}

func sendNotification(ws *websocket.Conn, messageID, subType string) error {
	notification := map[string]any{
		"metadata": map[string]string{
			"message_id":   messageID,
			"message_type": "notification",
		},
		"payload": map[string]any{
			"subscription": map[string]any{
				"id":   "sub-123",
				"type": subType,
			},
			"event": map[string]any{
				"broadcaster_user_id": "12345",
			},
		},
	}
	data, _ := json.Marshal(notification)
	return ws.Write(context.Background(), websocket.MessageText, data)
}

func TestEventConn_WaitsForSubscription(t *testing.T) {
	t.Parallel()

	welcomeSent := make(chan struct{})

	server := newTestEventSubServer(t, func(ws *websocket.Conn) {
		sendWelcome(ws, "session-123")
		close(welcomeSent)
		<-time.After(100 * time.Millisecond)
	})
	defer server.Close()

	conn := newEventConn("acc-123", zerolog.Nop(), http.DefaultClient, func(tea.Msg) {})
	conn.WSURL = wsURL(server)

	// Start connection but don't send subscription yet
	go conn.Run()
	defer conn.Close()

	// Welcome should not be sent yet
	select {
	case <-welcomeSent:
		t.Fatal("should wait for subscription before connecting")
	case <-time.After(100 * time.Millisecond):
		// Expected
	}

	// Now send subscription
	mockService := &mockEventSubService{}
	conn.subscribe(twitchapi.CreateEventSubSubscriptionRequest{
		Type: "channel.follow",
	}, mockService)

	// Now welcome should be sent
	select {
	case <-welcomeSent:
		// Expected
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for connection after subscription")
	}
}

func TestEventConn_CreatesSubscription(t *testing.T) {
	t.Parallel()

	server := newTestEventSubServer(t, func(ws *websocket.Conn) {
		sendWelcome(ws, "session-abc")
		<-time.After(200 * time.Millisecond)
	})
	defer server.Close()

	conn := newEventConn("acc-123", zerolog.Nop(), http.DefaultClient, func(tea.Msg) {})
	conn.WSURL = wsURL(server)

	go conn.Run()
	defer conn.Close()

	mockService := &mockEventSubService{}
	conn.subscribe(twitchapi.CreateEventSubSubscriptionRequest{
		Type:    "channel.follow",
		Version: "2",
		Condition: map[string]string{
			"broadcaster_user_id": "12345",
		},
	}, mockService)

	// Wait for subscription call
	time.Sleep(200 * time.Millisecond)

	calls := mockService.getCalls()
	require.Len(t, calls, 1)
	require.Equal(t, "channel.follow", calls[0].Type)
	require.Equal(t, "websocket", calls[0].Transport.Method)
	require.Equal(t, "session-abc", calls[0].Transport.SessionID)
}

func TestEventConn_ReceiveNotification(t *testing.T) {
	t.Parallel()

	server := newTestEventSubServer(t, func(ws *websocket.Conn) {
		sendWelcome(ws, "session-123")

		// Wait a bit then send notification
		time.Sleep(50 * time.Millisecond)
		sendNotification(ws, "notif-123", "channel.poll.begin")

		<-time.After(200 * time.Millisecond)
	})
	defer server.Close()

	var received []tea.Msg
	var mu sync.Mutex
	sendFn := func(msg tea.Msg) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	conn := newEventConn("acc-123", zerolog.Nop(), http.DefaultClient, sendFn)
	conn.WSURL = wsURL(server)

	go conn.Run()
	defer conn.Close()

	mockService := &mockEventSubService{}
	conn.subscribe(twitchapi.CreateEventSubSubscriptionRequest{Type: "channel.poll.begin"}, mockService)

	// Wait for notification
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	var foundNotification bool
	for _, msg := range received {
		if evt, ok := msg.(EventSubEvent); ok && evt.Error == nil {
			foundNotification = true
			require.Equal(t, "acc-123", evt.AccountID)
			break
		}
	}
	require.True(t, foundNotification, "should receive notification")
}

func TestEventConn_DuplicateFiltering(t *testing.T) {
	t.Parallel()

	server := newTestEventSubServer(t, func(ws *websocket.Conn) {
		sendWelcome(ws, "session-123")

		// Send same notification twice
		time.Sleep(50 * time.Millisecond)
		sendNotification(ws, "duplicate-id", "channel.poll.begin")
		sendNotification(ws, "duplicate-id", "channel.poll.begin")
		// Send different notification
		sendNotification(ws, "unique-id", "channel.poll.end")

		<-time.After(200 * time.Millisecond)
	})
	defer server.Close()

	var notificationCount atomic.Int32
	sendFn := func(msg tea.Msg) {
		if evt, ok := msg.(EventSubEvent); ok && evt.Error == nil {
			notificationCount.Add(1)
		}
	}

	conn := newEventConn("acc-123", zerolog.Nop(), http.DefaultClient, sendFn)
	conn.WSURL = wsURL(server)

	go conn.Run()
	defer conn.Close()

	mockService := &mockEventSubService{}
	conn.subscribe(twitchapi.CreateEventSubSubscriptionRequest{Type: "test"}, mockService)

	time.Sleep(200 * time.Millisecond)

	// Should only receive 2 notifications (duplicate filtered out)
	require.Equal(t, int32(2), notificationCount.Load(), "duplicates should be filtered")
}

func TestEventConn_ForcedReconnect(t *testing.T) {
	t.Parallel()

	var connectCount atomic.Int32
	newURL := make(chan string, 1)

	server := newTestEventSubServer(t, func(ws *websocket.Conn) {
		count := connectCount.Add(1)

		if count == 1 {
			sendWelcome(ws, "session-1")

			// Wait then send reconnect
			time.Sleep(50 * time.Millisecond)

			reconnect := map[string]any{
				"metadata": map[string]string{
					"message_id":   "reconnect-id",
					"message_type": "session_reconnect",
				},
				"payload": map[string]any{
					"session": map[string]any{
						"reconnect_url": <-newURL,
					},
				},
			}
			data, _ := json.Marshal(reconnect)
			ws.Write(context.Background(), websocket.MessageText, data)
			return
		}

		// Second connection
		sendWelcome(ws, "session-2")
		<-time.After(200 * time.Millisecond)
	})
	defer server.Close()

	// Send the new URL to the channel
	newURL <- wsURL(server)

	conn := newEventConn("acc-123", zerolog.Nop(), http.DefaultClient, func(tea.Msg) {})
	conn.WSURL = wsURL(server)

	go conn.Run()
	defer conn.Close()

	mockService := &mockEventSubService{}
	conn.subscribe(twitchapi.CreateEventSubSubscriptionRequest{Type: "test"}, mockService)

	// Wait for reconnection
	time.Sleep(500 * time.Millisecond)

	require.GreaterOrEqual(t, connectCount.Load(), int32(2), "should reconnect on forced reconnect")
}

func TestEventConn_ResubscribeOnReconnect(t *testing.T) {
	t.Parallel()

	var connectCount atomic.Int32

	server := newTestEventSubServer(t, func(ws *websocket.Conn) {
		count := connectCount.Add(1)
		sendWelcome(ws, "session-"+string(rune('0'+count)))

		if count == 1 {
			// First connection: close to trigger reconnect
			time.Sleep(50 * time.Millisecond)
			ws.Close(websocket.StatusGoingAway, "bye")
			return
		}

		// Second connection: stay open
		<-time.After(300 * time.Millisecond)
	})
	defer server.Close()

	conn := newEventConn("acc-123", zerolog.Nop(), http.DefaultClient, func(tea.Msg) {})
	conn.WSURL = wsURL(server)

	go conn.Run()
	defer conn.Close()

	mockService := &mockEventSubService{}
	conn.subscribe(twitchapi.CreateEventSubSubscriptionRequest{
		Type:    "channel.poll.begin",
		Version: "1",
	}, mockService)

	// Wait for reconnection and resubscription
	time.Sleep(7 * time.Second)

	calls := mockService.getCalls()
	require.GreaterOrEqual(t, len(calls), 2, "should resubscribe on reconnect")

	// Verify both calls are for the same subscription type
	for _, call := range calls {
		require.Equal(t, "channel.poll.begin", call.Type)
	}
}

func TestEventConn_MultipleSubscriptions(t *testing.T) {
	t.Parallel()

	server := newTestEventSubServer(t, func(ws *websocket.Conn) {
		sendWelcome(ws, "session-123")
		<-time.After(500 * time.Millisecond)
	})
	defer server.Close()

	conn := newEventConn("acc-123", zerolog.Nop(), http.DefaultClient, func(tea.Msg) {})
	conn.WSURL = wsURL(server)

	go conn.Run()
	defer conn.Close()

	mockService := &mockEventSubService{}

	// Send multiple subscriptions
	conn.subscribe(twitchapi.CreateEventSubSubscriptionRequest{Type: "channel.poll.begin"}, mockService)
	conn.subscribe(twitchapi.CreateEventSubSubscriptionRequest{Type: "channel.poll.end"}, mockService)
	conn.subscribe(twitchapi.CreateEventSubSubscriptionRequest{Type: "channel.raid"}, mockService)

	// Wait for all subscriptions
	time.Sleep(300 * time.Millisecond)

	calls := mockService.getCalls()
	require.Len(t, calls, 3)

	types := make(map[string]bool)
	for _, call := range calls {
		types[call.Type] = true
	}
	require.True(t, types["channel.poll.begin"])
	require.True(t, types["channel.poll.end"])
	require.True(t, types["channel.raid"])
}
