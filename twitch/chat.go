package twitch

import (
	"context"
	"errors"
	"fmt"
	"log"
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

func (c *Chat) Connect(ctx context.Context, messages <-chan IRCer, user, oauth string) (<-chan IRCer, error) {
	ctxWS, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	ws, _, err := websocket.DefaultDialer.DialContext(ctxWS, "wss://irc-ws.chat.twitch.tv:443", nil)
	if err != nil {
		return nil, err
	}

	out := make(chan IRCer)
	wg, ctx := errgroup.WithContext(ctx)

	ctx, cancel = context.WithCancel(ctx)

	wg.Go(func() error {
		defer ws.Close()
		<-ctx.Done()
		return ctx.Err()
	})

	wg.Go(func() error {
		ws.SetReadLimit(maxMessageSize)
		ws.SetReadDeadline(time.Time{}) // disable read timeouts

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
				pong := PongMessage{}
				if err := ws.WriteMessage(websocket.TextMessage, []byte(pong.IRC())); err != nil {
					return err
				}
			}

			out <- parsed
		}
	})

	wg.Go(func() error {
		for msg := range messages {
			if err := ws.WriteMessage(websocket.TextMessage, []byte(msg.IRC())); err != nil {
				return err
			}
		}

		return nil
	})

	go func() {
		defer ws.Close()
		defer close(out)
		defer cancel()

		err := wg.Wait()
		if err != nil {
			log.Println(err)
		}
	}()

	if oauth == "" || user == "" {
		oauth = AnonymousOAuth
		user = AnonymousUser
	}

	initMessages := []string{
		fmt.Sprintf("PASS %s", oauth),
		fmt.Sprintf("NICK %s", user),
		"CAP REQ :twitch.tv/membership twitch.tv/tags twitch.tv/commands",
		JoinMessage{Channel: "xqc"}.IRC(),
	}

	for _, m := range initMessages {
		if err := ws.WriteMessage(websocket.TextMessage, []byte(m)); err != nil {
			cancel()
			return nil, err
		}
	}

	return out, nil
}
