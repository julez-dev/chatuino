package multiplexer

import (
	"context"
	"sync"

	"github.com/julez-dev/chatuino/twitch"
	"github.com/rs/zerolog"
)

type OutboundMessage struct {
	ID  string
	Msg twitch.IRCer
	Err error
}

type InboundMessage struct {
	AccountID string
	Msg       any // Type of IncrementCounter, DecrementCounter, or twitch.IRCer
}

type IncrementTabCounter struct{}

type DecrementTabCounter struct{}

type Multiplexer struct {
	logger   zerolog.Logger
	provider twitch.AccountProvider
}

func NewMultiplexer(logger zerolog.Logger, provider twitch.AccountProvider) *Multiplexer {
	return &Multiplexer{
		logger:   logger,
		provider: provider,
	}
}

func (m *Multiplexer) ListenAndServe(inbound <-chan InboundMessage) <-chan OutboundMessage {
	out := make(chan OutboundMessage, 10)

	go func() {
		cancels := make(map[string]context.CancelFunc)
		chatIns := make(map[string]chan twitch.IRCer)
		chatDones := make(map[string]chan struct{}) // to unblock pending sends
		numListeners := make(map[string]int)

		chatWG := sync.WaitGroup{}

		for msg := range inbound {
			accountID := msg.AccountID

			in, ok := chatIns[accountID]

			// if not exists, create new chat for the ID
			if !ok {
				m.logger.Warn().Msgf("received message for unknown channel %s joining channel", accountID)
				chat := twitch.NewChat(m.logger, m.provider, accountID)
				ctx, cancel := context.WithCancel(context.Background())

				cancels[accountID] = cancel
				chatIns[accountID] = make(chan twitch.IRCer)
				chatDones[accountID] = make(chan struct{})

				in = chatIns[accountID]
				done := chatDones[accountID]

				outChat, outErrChat := chat.ConnectWithRetry(ctx, in)

				chatWG.Add(1)
				go func() {
					defer chatWG.Done()
					defer close(done)

					for {
						select {
						case ircMessage, ok := <-outChat:
							if !ok {
								m.logger.Warn().Msgf("channel %s closed", msg.AccountID)
								return
							}

							out <- OutboundMessage{
								ID:  accountID,
								Msg: ircMessage,
							}
						case err, ok := <-outErrChat:
							if !ok {
								m.logger.Warn().Msgf("channel %s closed", msg.AccountID)
								return
							}

							out <- OutboundMessage{
								ID:  accountID,
								Err: err,
							}
						}
					}
				}()
			} else {
				m.logger.Info().Msg("channel already exists, no need to start new one")
			}

			// if message is IncrementCounter or DecrementCounter, handle it
			switch msg.Msg.(type) {
			case IncrementTabCounter:
				numListeners[accountID]++
				m.logger.Info().Msgf("incremented counter for %s to %d", accountID, numListeners[accountID])
				continue // don't forward message
			case DecrementTabCounter:
				numListeners[accountID]--
				m.logger.Info().Msgf("decremented counter for %s to %d", accountID, numListeners[accountID])

				if numListeners[accountID] == 0 {
					m.logger.Info().Msgf("no more listeners for %s, closing channel", accountID)
					cancels[accountID]()
					close(chatIns[accountID])
					<-chatDones[accountID]

					delete(cancels, accountID)
					delete(chatIns, accountID)
					delete(chatDones, accountID)
					delete(numListeners, accountID)
				} else {
					m.logger.Info().Msgf("still %d listeners for %s, not closing channel", numListeners[accountID], accountID)
				}

				continue // don't forward message
			}

			m.logger.Info().Msg("forwarding message to channel")
			select {
			case in <- msg.Msg.(twitch.IRCer): // we know it's an IRCer
			case <-chatDones[accountID]:
				cancels[accountID]()
				close(chatIns[accountID])

				delete(cancels, accountID)
				delete(chatIns, accountID)
				delete(chatDones, accountID)
				delete(numListeners, accountID)
				m.logger.Warn().Msgf("done for %s is closed", msg.AccountID)
			}
			m.logger.Info().Msg("forwarded message to channel")
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
