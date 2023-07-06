package twitch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/sync/errgroup"
)

const (
	maxMessageSize = 4096
	AnonymousUser  = "justinfan123123"
	AnonymousOAuth = "oauth:123123123"
)

// IRCer are types that can be turned into an IRC command
type IRCer interface {
	IRC() string
}

type Chat struct{}

func NewChat() *Chat {
	return &Chat{}
}

func (c *Chat) Connect(ctx context.Context, messages <-chan IRCer, user, oauth string) (<-chan IRCer, <-chan error, error) {
	ctxWS, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	ws, _, err := websocket.DefaultDialer.DialContext(ctxWS, "wss://irc-ws.chat.twitch.tv:443", nil)
	if err != nil {
		return nil, nil, err
	}

	out := make(chan IRCer)
	outErr := make(chan error)
	wg, ctx := errgroup.WithContext(ctx)

	ctx, cancel = context.WithCancel(ctx)

	wg.Go(func() error {
		defer ws.Close()
		<-ctx.Done()
		return ctx.Err()
	})

	ws.SetReadLimit(maxMessageSize)
	ws.SetReadDeadline(time.Time{}) // disable read timeouts
	ws.SetWriteDeadline(time.Time{})

	wg.Go(func() error {
		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				return err
			}

			parsed, err := parseIRC(string(message))
			if err != nil {
				if errors.Is(err, ErrUnhandledCommand) {
					continue
				}

				return err
			}

			// automatically respond with pong
			if _, ok := parsed.(PingMessage); ok {
				if err := ws.WriteMessage(websocket.TextMessage, []byte("PONG tmi.twitch.tv\r\n")); err != nil {
					return err
				}
			}

			out <- parsed
		}
	})

	wg.Go(func() error {
		initMessages := []string{
			fmt.Sprintf("PASS %s", oauth),
			fmt.Sprintf("NICK %s", user),
			"CAP REQ :twitch.tv/membership twitch.tv/tags twitch.tv/commands",
		}

		for _, m := range initMessages {
			if err := ws.WriteMessage(websocket.TextMessage, []byte(m)); err != nil {
				cancel()
				return err
			}
		}

		for {
			select {
			case msg, ok := <-messages:
				if !ok {
					return nil
				}

				if err := ws.WriteMessage(websocket.TextMessage, []byte(msg.IRC())); err != nil {
					return err
				}
			case <-ctx.Done():
				return nil
			}
		}
	})

	go func() {
		defer ws.Close()
		defer close(out)
		defer close(outErr)
		defer cancel()

		err := wg.Wait()
		if err != nil {
			outErr <- err
		}
	}()

	if oauth == "" || user == "" {
		oauth = AnonymousOAuth
		user = AnonymousUser
	}

	return out, outErr, nil
}
