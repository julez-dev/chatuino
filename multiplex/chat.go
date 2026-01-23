package multiplex

import (
	"context"
	"sync"

	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/rs/zerolog"
)

type Chat interface {
	ConnectWithRetry(ctx context.Context, messages <-chan twitchirc.IRCer) (<-chan twitchirc.IRCer, <-chan error)
}

type OutboundMessage struct {
	ID  string
	Msg twitchirc.IRCer
	Err error
}

type InboundMessage struct {
	AccountID string
	Msg       any // Type of IncrementCounter, DecrementCounter, or twitchirc.IRCer
}

type IncrementTabCounter struct{}

type DecrementTabCounter struct{}

type ChatMultiplexer struct {
	logger          zerolog.Logger
	provider        twitchapi.AccountProvider
	BuildChatClient func(logger zerolog.Logger, provider twitchapi.AccountProvider, accountID string) Chat
}

func NewChatMultiplexer(logger zerolog.Logger, provider twitchapi.AccountProvider) *ChatMultiplexer {
	return &ChatMultiplexer{
		logger:   logger,
		provider: provider,
		BuildChatClient: func(logger zerolog.Logger, provider twitchapi.AccountProvider, accountID string) Chat {
			return twitchirc.NewChat(logger, provider, accountID)
		},
	}
}

func (m *ChatMultiplexer) ListenAndServe(inbound <-chan InboundMessage) <-chan OutboundMessage {
	out := make(chan OutboundMessage, 10)

	go func() {
		cancels := make(map[string]context.CancelFunc)
		chatIns := make(map[string]chan twitchirc.IRCer)
		chatDones := make(map[string]chan struct{}) // to unblock pending sends
		numListeners := make(map[string]int)

		chatWG := sync.WaitGroup{}

		for msg := range inbound {
			accountID := msg.AccountID

			in, ok := chatIns[accountID]

			// if not exists, create new chat for the ID
			if !ok {
				m.logger.Warn().Str("account-id", accountID).Msgf("received message for unknown channel creating new chat connection for this account")
				chat := m.BuildChatClient(m.logger, m.provider, accountID)
				ctx, cancel := context.WithCancel(context.Background())

				cancels[accountID] = cancel
				chatIns[accountID] = make(chan twitchirc.IRCer)
				chatDones[accountID] = make(chan struct{})

				in = chatIns[accountID]
				done := chatDones[accountID]

				outChat, outErrChat := chat.ConnectWithRetry(ctx, in)

				chatWG.Go(func() {
					defer close(done)

					for {
						select {
						case ircMessage, ok := <-outChat:
							if !ok {
								m.logger.Warn().Str("account-id", accountID).Msgf("irc message channel closed")
								return
							}

							out <- OutboundMessage{
								ID:  accountID,
								Msg: ircMessage,
							}
						case err, ok := <-outErrChat:
							if !ok {
								m.logger.Warn().Str("account-id", accountID).Msgf("irc out err channel closed")
								return
							}

							out <- OutboundMessage{
								ID:  accountID,
								Err: err,
							}
						}
					}
				})
			} else {
				m.logger.Info().Str("account-id", accountID).Msg("channel already exists, no need to start new one")
			}

			// if message is IncrementCounter or DecrementCounter, handle it
			switch msg.Msg.(type) {
			case IncrementTabCounter:
				numListeners[accountID]++
				m.logger.Info().Str("account-id", accountID).Int("num-listeners", numListeners[accountID]).Msgf("incremented tab listener counter")
				continue // don't forward message
			case DecrementTabCounter:
				numListeners[accountID]--
				m.logger.Info().Str("account-id", accountID).Int("num-listeners", numListeners[accountID]).Msgf("decremented tab listener counter")

				if numListeners[accountID] == 0 {
					m.logger.Info().Str("account-id", accountID).Int("num-listeners", numListeners[accountID]).Msgf("closing because no more listeners")
					cancels[accountID]()
					close(chatIns[accountID])
					<-chatDones[accountID]

					delete(cancels, accountID)
					delete(chatIns, accountID)
					delete(chatDones, accountID)
					delete(numListeners, accountID)
					m.logger.Info().Str("account-id", accountID).Int("num-listeners", numListeners[accountID]).Msgf("done closing because no more listeners")
				} else {
					m.logger.Info().Str("account-id", accountID).Int("num-listeners", numListeners[accountID]).Msgf("still listeners for this account, not closing connection")
				}

				continue // don't forward message
			}

			ircer, ok := msg.Msg.(twitchirc.IRCer)
			if !ok {
				m.logger.Error().Str("account-id", accountID).Type("msg-type", msg.Msg).Msg("unexpected message type, expected IRCer")
				continue
			}

			select {
			case in <- ircer:
			case <-chatDones[accountID]:
				// cancels[accountID]()
				// close(chatIns[accountID])

				// delete(cancels, accountID)
				// delete(chatIns, accountID)
				// delete(chatDones, accountID)
				// delete(numListeners, accountID)
				m.logger.Warn().Str("account-id", accountID).Int("num-listeners", numListeners[accountID]).Msgf("done channel for account closed, aborting sending")
			}
		}

		for _, cancel := range cancels {
			cancel()
		}
		clear(cancels)

		for _, in := range chatIns {
			close(in)
		}
		clear(chatIns)

		chatWG.Wait()
		close(out)
	}()

	return out
}
