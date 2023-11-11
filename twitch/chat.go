package twitch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
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
	if !strings.HasPrefix(oauth, "oauth:") {
		oauth = "oauth:" + oauth
	}

	if oauth == "" || user == "" {
		oauth = AnonymousOAuth
		user = AnonymousUser
	}

	ctxWS, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	ws, _, err := websocket.DefaultDialer.DialContext(ctxWS, "wss://irc-ws.chat.twitch.tv:443", nil)
	if err != nil {
		return nil, nil, err
	}

	out := make(chan IRCer)
	outErr := make(chan error, 1)
	wg, ctx := errgroup.WithContext(ctx)

	ctx, cancel = context.WithCancel(ctx)

	wg.Go(func() error {
		defer ws.Close()
		<-ctx.Done()
		return ctx.Err()
	})

	ws.SetReadLimit(maxMessageSize)
	ws.SetWriteDeadline(time.Time{})

	wg.Go(func() error {
		for {
			if err := ws.SetReadDeadline(time.Now().Add(time.Minute * 6)); err != nil {
				return err
			}

			_, message, err := ws.ReadMessage()
			if err != nil {
				return err
			}

			// sometimes we get two messages for whatever reason, seperated by \r\n
			messages := bytes.Split(message, []byte("\r\n"))

			for _, message := range messages {
				if len(message) == 0 {
					continue
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

	return out, outErr, nil
}
