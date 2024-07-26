package twitch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/julez-dev/chatuino/twitch/command"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"nhooyr.io/websocket"
)

const (
	maxMessageSize = 32 * 1024 // 32KB
	ircWSURL       = "wss://irc-ws.chat.twitch.tv:443"
)

// IRCer are types that can be turned into an IRC command
type IRCer interface {
	IRC() string
}

type RetryReachedError struct {
	err error
}

func (e RetryReachedError) Error() string {
	return fmt.Sprintf("max retries reached: %s", e.err)
}

func (e RetryReachedError) Unwrap() error {
	return e.err
}

type Chat struct {
	channels []string
	m        *sync.Mutex

	accountProvider AccountProvider
	accountID       string

	logger zerolog.Logger
}

func NewChat(logger zerolog.Logger, accountProvider AccountProvider, accountID string) *Chat {
	return &Chat{
		m:               &sync.Mutex{},
		logger:          logger,
		accountProvider: accountProvider,
		accountID:       accountID,
	}
}

func (c *Chat) ConnectWithRetry(ctx context.Context, messages <-chan IRCer) (<-chan IRCer, <-chan error) {
	out := make(chan IRCer)
	outErr := make(chan error, 1)

	// outerWG is done, once all retries have failed
	outerWG, outerCtx := errgroup.WithContext(ctx)

	outerWG.Go(func() error {
		return retry.Do(func() error {
			ctxWS, cancel := context.WithTimeout(outerCtx, time.Second*5)
			defer cancel()

			ws, _, err := websocket.Dial(ctxWS, ircWSURL, &websocket.DialOptions{
				HTTPClient: &http.Client{
					Transport: http.DefaultClient.Transport,
					Timeout:   time.Second * 10,
				},
			})

			if err != nil {
				return err
			}

			ws.SetReadLimit(maxMessageSize)

			// innerWG is done, once either the writer or reader returns an error
			innerWG, innerCtx := errgroup.WithContext(outerCtx)

			// sometimes the reader needs to write to the websocket
			// if the reader writes to the websocket we may get a data race,
			// so we send internal messages from the reader to the writer
			innerMessages := make(chan IRCer, 10)

			// close the websocket, once the context is done
			innerWG.Go(func() error {
				defer ws.Close(websocket.StatusNormalClosure, "closing websocket")
				defer close(innerMessages)
				<-innerCtx.Done() // processes once a other routine has failed
				return ctx.Err()
			})

			// ping twitch every 10 seconds
			innerWG.Go(func() error {
				t := time.NewTicker(time.Second * 10)
				defer t.Stop()

				for {
					select {
					case <-innerCtx.Done():
						return nil
					case <-t.C:
						pingCtx, cancel := context.WithTimeout(innerCtx, time.Second*5)
						if err := ws.Ping(pingCtx); err != nil {
							cancel()
							return err
						}
						cancel()
					}
				}
			})

			innerWG.Go(func() error {
				for {
					// this deadline just tracks how much time can pass without getting a new message
					// not to check if the connection is still up, so not a keep-alive
					_, wsMessage, err := ws.Read(innerCtx)

					if err != nil {
						return err
					}

					// sometimes twitch sends multiple messages in one response
					messages := bytes.Split(wsMessage, []byte("\r\n"))

					for _, message := range messages {
						if len(message) == 0 {
							continue
						}

						parsed, err := ParseIRC(string(message))
						if err != nil {
							if errors.Is(err, ErrUnhandledCommand) {
								if !strings.Contains(string(message), "PART") && !strings.Contains(string(message), "JOIN") {
									c.logger.Info().Str("unhandled", string(message)).Send()
								}

								continue
							}

							return err
						}

						if _, ok := parsed.(command.PingMessage); ok {
							select {
							case innerMessages <- command.PongMessage{}:
							case <-innerCtx.Done():
								return nil
							}
						}

						out <- parsed
					}
				}
			})

			innerWG.Go(func() error {
				account, err := c.accountProvider.GetAccountBy(c.accountID)
				if err != nil {
					return retry.Unrecoverable(err)
				}

				oauth := account.AccessToken
				user := account.DisplayName

				if !strings.HasPrefix(oauth, "oauth:") {
					oauth = "oauth:" + oauth
				}

				initMessages := []string{
					fmt.Sprintf("PASS %s", oauth),
					fmt.Sprintf("NICK %s", user),
					"CAP REQ :twitch.tv/membership twitch.tv/tags twitch.tv/commands",
				}

				// rejoin channels that were joined before, in case of a reconnect
				c.m.Lock()
				for _, channel := range c.channels {
					initMessages = append(initMessages, fmt.Sprintf("JOIN #%s", channel))
				}
				c.m.Unlock()

				for _, m := range initMessages {
					if err := ws.Write(innerCtx, websocket.MessageText, []byte(m)); err != nil {
						return err
					}
				}

				for {
					select {
					case msg, ok := <-innerMessages:
						if !ok {
							return nil
						}

						if err := ws.Write(innerCtx, websocket.MessageText, []byte(msg.IRC())); err != nil {
							return err
						}
					case msg, ok := <-messages:
						if !ok {
							return retry.Unrecoverable(errors.New("messages channel closed"))
						}

						if join, ok := msg.(command.JoinMessage); ok {
							c.m.Lock()
							has := slices.ContainsFunc(c.channels, func(s string) bool {
								return s == join.Channel
							})

							if !has {
								c.channels = append(c.channels, join.Channel)
								c.m.Unlock()
							} else {
								c.m.Unlock()
								continue
							}
						}

						if err := ws.Write(innerCtx, websocket.MessageText, []byte(msg.IRC())); err != nil {
							return err
						}
					case <-innerCtx.Done():
						return nil
					}
				}
			})

			// before we retry, send error to consumer
			if err := innerWG.Wait(); err != nil {
				return err
			}

			return nil
		}, retry.Attempts(0), retry.DelayType(retry.FixedDelay),
			retry.Delay(time.Second*5), retry.Context(ctx),
			retry.LastErrorOnly(true), retry.OnRetry(func(_ uint, err error) {
				outErr <- fmt.Errorf("will retry after error: %w", err)
			}))
	})

	go func() {
		err := outerWG.Wait()
		if err != nil {
			outErr <- RetryReachedError{err: err}
		}

		close(outErr)
		close(out)
	}()

	return out, outErr
}
