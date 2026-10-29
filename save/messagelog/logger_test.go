package messagelog

import (
	"database/sql"
	"encoding/json/v2"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type fakeDB struct{}

func (f fakeDB) Exec(query string, args ...any) (sql.Result, error) {
	return nil, nil
}

func (f fakeDB) Query(query string, args ...any) (*sql.Rows, error) {
	return nil, nil
}

// capturingDB records the statement and arguments of the last Exec call instead
// of asserting on them through a mock framework.
type capturingDB struct {
	query string
	args  []any
}

func (c *capturingDB) Exec(query string, args ...any) (sql.Result, error) {
	c.query = query
	c.args = args

	return nil, nil
}

func (c *capturingDB) Query(query string, args ...any) (*sql.Rows, error) {
	return nil, nil
}

func TestBatchedMessageLogger_LogMessages(t *testing.T) {
	t.Run("max-batch-size", func(t *testing.T) {
		messageLogger := NewBatchedMessageLogger(zerolog.Nop(), fakeDB{}, fakeDB{}, nil, nil)
		in := make(chan *twitchirc.PrivateMessage, maxBatchItems)
		for i := range maxBatchItems {
			msg := &twitchirc.PrivateMessage{
				ID:              fmt.Sprintf("%d", i),
				RoomID:          fmt.Sprintf("room-%d", i),
				ChannelUserName: fmt.Sprintf("room-name-%d", i),
				TMISentTS:       time.Now(),
				DisplayName:     fmt.Sprintf("sender-%d", i),
			}

			in <- msg
		}

		close(in)

		err := messageLogger.LogMessages(in)
		require.Nil(t, err)
	})

	t.Run("timeout-reached", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
		}
		defer db.Close()

		messageLogger := NewBatchedMessageLogger(zerolog.Nop(), db, db, nil, nil)

		in := make(chan *twitchirc.PrivateMessage, 1)
		msg := &twitchirc.PrivateMessage{
			ID:              "1",
			RoomID:          "room-1",
			ChannelUserName: "room-name-1",
			TMISentTS:       time.Now(),
			DisplayName:     "sender-1",
		}

		mock.ExpectExec("INSERT INTO messages").
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		in <- msg
		time.AfterFunc(time.Second*6, func() {
			close(in)
		})

		err = messageLogger.LogMessages(in)
		require.Nil(t, err)

		// we make sure that all expectations were met
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})
}

func TestBatchedMessageLogger_createLogEntries(t *testing.T) {
	t.Run("dynamic-rows", func(t *testing.T) {
		db := &capturingDB{}

		msgs := []*twitchirc.PrivateMessage{
			{
				ID:              "first",
				RoomID:          "room-id",
				ChannelUserName: "room-name",
				TMISentTS:       time.Now(),
				DisplayName:     "sender",
				UserID:          "sender-id",
			},
			{
				ID:              "second",
				RoomID:          "room-id-2",
				ChannelUserName: "room-name-2",
				TMISentTS:       time.Now(),
				DisplayName:     "sender-2",
				UserID:          "sender-id-2",
			},
		}

		messageLogger := NewBatchedMessageLogger(zerolog.Nop(), db, db, nil, nil)
		err := messageLogger.createLogEntries(msgs)
		require.Nil(t, err)

		require.Equal(t, "INSERT INTO messages (id, broadcast_id, broadcast_channel, sent_at, sender_display, payload, user_id) VALUES (?, ?, ?, ?, ?, ?, ?),(?, ?, ?, ?, ?, ?, ?)", db.query)
		require.Len(t, db.args, 14)
		require.Equal(t, []any{
			msgs[0].ID,
			msgs[0].RoomID,
			msgs[0].ChannelUserName,
			msgs[0].TMISentTS,
			msgs[0].DisplayName,
			db.args[5],
			msgs[0].UserID,

			msgs[1].ID,
			msgs[1].RoomID,
			msgs[1].ChannelUserName,
			msgs[1].TMISentTS,
			msgs[1].DisplayName,
			db.args[12],
			msgs[1].UserID,
		}, db.args)

		// The stored payload keeps the snake_case JSON names older binaries read.
		for _, column := range []int{5, 12} {
			payload, ok := db.args[column].([]byte)
			require.True(t, ok, "payload is stored as raw JSON bytes")

			var members map[string]any
			require.NoError(t, json.Unmarshal(payload, &members))
			require.Contains(t, members, "display_name")
			require.Contains(t, members, "channel_user_name")
		}
	})
}

func TestBatchedMessageLogger_scanRowsReadsStoredPayload(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	// A payload as written before the JSON v2 migration: snake_case names and
	// null for the empty slices.
	const payload = `{"badge_info":null,"badges":null,"bits":0,"channel_user_name":"chan","color":"",` +
		`"display_name":"Alice","emotes":null,"first_msg":false,"id":"msg-1","login_name":"alice",` +
		`"message":"hi there","mod":false,"room_id":"7","subscriber":false,` +
		`"tmi_sent_ts":"2024-05-06T07:08:09Z","turbo":false,"user_id":"3","user_type":"","vip":false}`

	rows := sqlmock.NewRows([]string{
		"id", "broadcast_id", "user_id", "broadcast_channel", "sent_at", "sender_display", "payload",
	}).AddRow("msg-1", 7, 3, "chan", "2024-05-06 07:08:09+00:00", "Alice", []byte(payload))
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	messageLogger := NewBatchedMessageLogger(zerolog.Nop(), db, db, nil, nil)

	rowsResult, err := db.Query("SELECT id, broadcast_id, user_id, broadcast_channel, sent_at, sender_display, payload FROM messages")
	require.NoError(t, err)

	entries, err := messageLogger.scanRows(rowsResult)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "msg-1", entries[0].ID)
	require.Equal(t, "Alice", entries[0].SenderDisplay)
	require.Equal(t, "chan", entries[0].BroadcastChannel)
	require.True(t, entries[0].SentAt.Equal(time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)), "sent_at is parsed from SQLite's text layout")
	require.Equal(t, "hi there", entries[0].PrivateMessage.Message)
	require.Equal(t, "alice", entries[0].PrivateMessage.LoginName)
	require.Equal(t, twitchirc.UserType(""), entries[0].PrivateMessage.UserType)
}
