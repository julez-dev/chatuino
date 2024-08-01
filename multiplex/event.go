package multiplex

import (
	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

type EventSub interface {
	Connect(inbound <-chan eventsub.InboundMessage) error
}

type EventMultiplexer struct {
	logger        zerolog.Logger
	BuildEventSub func() EventSub
}

// EventSubInboundMessage wraps an eventsub.InboundMessage with the accountID of the account that sends the message.
type EventSubInboundMessage struct {
	AccountID string
	Msg       eventsub.InboundMessage
}

func NewEventMultiplexer(logger zerolog.Logger) *EventMultiplexer {
	return &EventMultiplexer{
		logger: logger,
		BuildEventSub: func() EventSub {
			return eventsub.NewConn(logger, nil)
		},
	}
}

func (e *EventMultiplexer) ListenAndServe(inbound <-chan EventSubInboundMessage) error {
	internalInbounds := map[string]chan<- eventsub.InboundMessage{}
	doneAgg := make(chan string) // signals when a ws connection is done (value is account ID) / mostly for errors
	connWG := &errgroup.Group{}

SELECT:
	for {
		select {
		case accountID := <-doneAgg:
			close(internalInbounds[accountID])
			delete(internalInbounds, accountID)
			e.logger.Info().Str("account-id", accountID).Msg("removing event sub connection early")
		case msg, ok := <-inbound:
			if !ok {
				e.logger.Info().Msg("event multiplex inbound channel closed")
				break SELECT
			}
			var internalInbound chan<- eventsub.InboundMessage
			internalInbound, ok = internalInbounds[msg.AccountID]

			// ws connection for accountID does not exist
			if !ok {
				e.logger.Info().Str("account-id", msg.AccountID).Msg("creating new event sub connection")
				var doneChan <-chan struct{}
				internalInbound, doneChan = e.startEventSub(connWG)
				internalInbounds[msg.AccountID] = internalInbound

				connWG.Go(func() error {
					id := msg.AccountID
					<-doneChan
					doneAgg <- id
					return nil
				})
			}

			// forward message to ws connection
			internalInbound <- msg.Msg
		}
	}

	// close all ws connections
	for id, internalInbound := range internalInbounds {
		e.logger.Info().Str("account-id", id).Msg("closing internal inbound event sub channel")
		close(internalInbound)
		e.logger.Info().Str("account-id", id).Msg("closed internal inbound event sub channel")
	}

	// drain dones
	go func() {
		connWG.Wait() // wait for all done collector routines to be done
		close(doneAgg)
	}()

	for range doneAgg {
	}

	for range inbound {
	}

	err := connWG.Wait()

	if err != nil {
		return err
	}

	return nil
}

func (e *EventMultiplexer) startEventSub(wg *errgroup.Group) (chan<- eventsub.InboundMessage, <-chan struct{}) {
	internalInbound := make(chan eventsub.InboundMessage)
	done := make(chan struct{})
	wg.Go(func() error {
		defer close(done)
		conn := e.BuildEventSub()
		return conn.Connect(internalInbound)
	})
	return internalInbound, done
}
