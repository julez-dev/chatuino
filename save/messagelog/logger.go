package messagelog

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/mailru/easyjson"
	"github.com/rs/zerolog"
)

type LogEntry struct {
	ID               string
	BroadCastID      int
	UserID           int
	BroadcastChannel string
	SentAt           time.Time
	SenderDisplay    string
	PrivateMessage   *twitchirc.PrivateMessage
}

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
CREATE INDEX IF NOT EXISTS user_in_broadcast_channel_idx ON messages (broadcast_channel, sender_display);
CREATE INDEX IF NOT EXISTS user_in_room_idx ON messages (broadcast_id, sender_display);
CREATE INDEX IF NOT EXISTS user_idx ON messages (user_id);
COMMIT;`

type DB interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
}

const (
	maxBatchWait  = time.Second * 5
	maxBatchItems = 20
)

type BatchedMessageLogger struct {
	logger zerolog.Logger
	db     DB
	roDB   DB

	includeChannels []string
	excludeChannels []string
}

func NewBatchedMessageLogger(logger zerolog.Logger, db DB, roDB DB, includeChannels []string, excludeChannels []string) *BatchedMessageLogger {
	return &BatchedMessageLogger{
		logger:          logger,
		db:              db,
		roDB:            roDB,
		includeChannels: includeChannels,
		excludeChannels: excludeChannels,
	}
}

func (b *BatchedMessageLogger) PrepareDatabase() error {
	queries := [...]string{
		"pragma journal_mode = WAL;",
		"pragma synchronous = normal;",
		"pragma temp_store = memory;",
	}

	for _, query := range queries {
		if _, err := b.db.Exec(query); err != nil {
			return fmt.Errorf("failed running prepare query: %w", err)
		}
	}

	if _, err := b.db.Exec(sqlMigration); err != nil {
		return fmt.Errorf("failed running migration: %w", err)
	}

	return nil
}

func (b *BatchedMessageLogger) LogMessages(twitchMsgChan <-chan *twitchirc.PrivateMessage) error {
	defer b.logger.Info().Msg("batched logger done")

	var batch []*twitchirc.PrivateMessage

	timer := time.NewTimer(maxBatchWait)
	defer timer.Stop()

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

			if !b.isChannelRelevant(twitchMsg.ChannelUserName) {
				continue SELECT_LOOP
			}

			batch = append(batch, twitchMsg)

			if len(batch) != maxBatchItems {
				continue SELECT_LOOP
			}

			cloned := slices.Clone(batch)
			if err := b.createLogEntries(cloned); err != nil {
				return fmt.Errorf("failed to batch insert %d messages after max entries was reached: %w", len(cloned), err)
			}

			// clear batch
			batch = []*twitchirc.PrivateMessage{}

			// reset timer (Go 1.23+ handles channel drain automatically)
			timer.Stop()
			timer.Reset(maxBatchWait)
		case <-timer.C:
			if len(batch) == 0 {
				//b.logger.Info().Msg("logger max wait time was reached but batch is empty, resetting without batching")
				timer.Reset(maxBatchWait)
				continue
			}

			cloned := slices.Clone(batch)
			if err := b.createLogEntries(cloned); err != nil {
				return fmt.Errorf("failed to batch insert %d messages after max wait time was reached: %w", len(cloned), err)
			}

			// clear batch
			batch = []*twitchirc.PrivateMessage{}
			timer.Reset(maxBatchWait)
		}
	}

	return nil
}

func (b *BatchedMessageLogger) MessagesFromUserInChannel(username string, broadcasterChannel string) ([]LogEntry, error) {
	query := `SELECT id, broadcast_id, user_id, broadcast_channel, sent_at, sender_display, payload FROM messages WHERE sender_display = ? AND broadcast_channel = ?`
	rows, err := b.roDB.Query(query, username, broadcasterChannel)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []LogEntry{}, nil
		}

		return nil, err
	}

	return b.scanRows(rows)
}

func (b *BatchedMessageLogger) scanRows(rows *sql.Rows) ([]LogEntry, error) {
	defer rows.Close()

	var logEntries []LogEntry

	for rows.Next() {
		var entry LogEntry
		var rawPayload []byte
		var rawSentAt string
		if err := rows.Scan(
			&entry.ID,
			&entry.BroadCastID,
			&entry.UserID,
			&entry.BroadcastChannel,
			&rawSentAt,
			&entry.SenderDisplay,
			&rawPayload,
		); err != nil {
			return logEntries, err
		}

		var err error
		entry.SentAt, err = time.Parse("2006-01-02 15:04:05-07:00", rawSentAt)
		if err != nil {
			return logEntries, err
		}

		entry.PrivateMessage = &twitchirc.PrivateMessage{}
		if err := easyjson.Unmarshal(rawPayload, entry.PrivateMessage); err != nil {
			return logEntries, err
		}

		logEntries = append(logEntries, entry)
	}

	if err := rows.Err(); err != nil {
		return logEntries, err
	}

	return logEntries, nil
}

func (b *BatchedMessageLogger) createLogEntries(twitchMsgs []*twitchirc.PrivateMessage) error {
	if len(twitchMsgs) == 0 {
		return fmt.Errorf("expected at least 1 element, got %d", len(twitchMsgs))
	}

	query := `INSERT INTO messages (id, broadcast_id, broadcast_channel, sent_at, sender_display, payload, user_id) VALUES %s`

	valueStrings := make([]string, 0, len(twitchMsgs))
	valueArgs := make([]any, 0, len(twitchMsgs)*7) // 7 args per row
	for _, msg := range twitchMsgs {
		payloadJSON, err := easyjson.Marshal(msg)
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

func (b *BatchedMessageLogger) isChannelRelevant(channel string) bool {
	if len(b.includeChannels) == 0 && len(b.excludeChannels) == 0 {
		return true
	}

	// When include channels set, only save messages when channel is in list
	if len(b.includeChannels) > 0 {
		inList := slices.ContainsFunc(b.includeChannels, func(s string) bool { return strings.EqualFold(s, channel) })
		return inList
	}

	// When exclude channels set, don't save messages, when channel in list
	if len(b.excludeChannels) > 0 {
		inList := slices.ContainsFunc(b.excludeChannels, func(s string) bool { return strings.EqualFold(s, channel) })
		return !inList
	}

	return false
}
