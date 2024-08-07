package messagelog

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/julez-dev/chatuino/twitch/command"
	"github.com/rs/zerolog"
)

const sqlMigration = `BEGIN;
CREATE TABLE IF NOT EXISTS messages (
	id TEXT PRIMARY KEY,
	broadcast_id INTEGER NOT NULL,
	user_id INTEGER NOT NULL,
	broadcast_channel TEXT NOT NULL collate nocase,
	sent_at TEXT NOT NULL,
	sender_display TEXT NOT NULL collate nocase,
	payload JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS user_in_room_idx ON messages (broadcast_id, sender_display);
CREATE INDEX IF NOT EXISTS user_idx ON messages (user_id);
COMMIT;`

type DB interface {
	Exec(query string, args ...any) (sql.Result, error)
}

const (
	maxBatchWait  = time.Second * 5
	maxBatchItems = 20
)

type BatchedMessageLogger struct {
	logger zerolog.Logger
	db     DB
}

func NewBatchedMessageLogger(logger zerolog.Logger, db DB) *BatchedMessageLogger {
	return &BatchedMessageLogger{
		logger: logger,
		db:     db,
	}
}

func (b *BatchedMessageLogger) MigrateDatabase() error {
	if _, err := b.db.Exec(sqlMigration); err != nil {
		return fmt.Errorf("failed running migration: %w", err)
	}

	return nil
}

func (b *BatchedMessageLogger) LogMessages(twitchMsgChan <-chan *command.PrivateMessage) error {
	defer b.logger.Info().Msg("batched logger done")

	var batch []*command.PrivateMessage

	timer := time.NewTimer(maxBatchWait)
	defer func() {
		if !timer.Stop() {
			<-timer.C
		}
	}()

SELECT_LOOP:
	for {
		select {
		case twitchMsg, ok := <-twitchMsgChan:
			// when channel is closed write all items currently in batch
			if !ok {
				cloned := slices.Clone(batch)
				if len(cloned) == 0 {
					break SELECT_LOOP
				}

				b.logger.Info().Int("len-batch", len(cloned)).Msg("twitch message channel closed; batching open entries")

				if err := b.createLogEntries(cloned); err != nil {
					return fmt.Errorf("failed to batch insert %d messages after channel was closed: %w", len(cloned), err)
				}

				break SELECT_LOOP
			}

			batch = append(batch, twitchMsg)

			if len(batch) != maxBatchItems {
				continue SELECT_LOOP
			}

			cloned := slices.Clone(batch)
			b.logger.Info().Int("len-batch", len(cloned)).Msg("batching logger messages after max retry reached")

			if err := b.createLogEntries(cloned); err != nil {
				return fmt.Errorf("failed to batch insert %d messages after max entries was reached: %w", len(cloned), err)
			}

			// clear batch
			batch = []*command.PrivateMessage{}

			// reset timer, drain channel if needed
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(maxBatchWait)
		case <-timer.C:
			if len(batch) == 0 {
				b.logger.Info().Msg("logger max wait time was reached but batch is empty, resetting without batching")
				timer.Reset(maxBatchWait)
				continue
			}

			cloned := slices.Clone(batch)
			b.logger.Info().Int("len-batch", len(cloned)).Dur("max-time", maxBatchWait).Msg("batching logger messages after max wait time reached")
			if err := b.createLogEntries(cloned); err != nil {
				return fmt.Errorf("failed to batch insert %d messages after max wait time was reached: %w", len(cloned), err)
			}

			// clear batch
			batch = []*command.PrivateMessage{}
			timer.Reset(maxBatchWait)
		}
	}

	return nil
}

func (b *BatchedMessageLogger) createLogEntries(twitchMsgs []*command.PrivateMessage) error {
	if len(twitchMsgs) == 0 {
		return fmt.Errorf("expected at least 1 element, got %d", len(twitchMsgs))
	}

	query := `INSERT INTO messages (id, broadcast_id, broadcast_channel, sent_at, sender_display, payload, user_id) VALUES %s`

	valueStrings := make([]string, 0, len(twitchMsgs))
	valueArgs := make([]any, 0, len(twitchMsgs)*7) // 7 args per row
	for _, msg := range twitchMsgs {
		payloadJSON, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal JSON payload for message %s: %w", msg.ID, err)
		}

		valueStrings = append(valueStrings, "(?, ?, ?, ?, ?, ?, ?)")
		valueArgs = append(valueArgs, msg.ID)
		valueArgs = append(valueArgs, msg.RoomID)
		valueArgs = append(valueArgs, msg.ChannelUserName)
		valueArgs = append(valueArgs, msg.TMISentTS)
		valueArgs = append(valueArgs, msg.DisplayName)
		valueArgs = append(valueArgs, payloadJSON)
		valueArgs = append(valueArgs, msg.UserID)
	}

	query = fmt.Sprintf(query, strings.Join(valueStrings, ","))

	if _, err := b.db.Exec(query, valueArgs...); err != nil {
		return fmt.Errorf("failed inserting data: %w", err)
	}

	return nil
}
