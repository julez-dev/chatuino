package messagelog

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/julez-dev/chatuino/mocks"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type fakeDB struct{}

func (f fakeDB) Exec(query string, args ...any) (sql.Result, error) {
	return nil, nil
}

func (f fakeDB) Query(query string, args ...any) (*sql.Rows, error) {
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
		assert.Nil(t, err)
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
		assert.Nil(t, err)

		// we make sure that all expectations were met
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("there were unfulfilled expectations: %s", err)
		}
	})
}

func TestBatchedMessageLogger_createLogEntries(t *testing.T) {
	t.Run("dynamic-rows", func(t *testing.T) {
		db := mocks.NewDBVariadic(t)

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

		db.EXPECT().Exec("INSERT INTO messages (id, broadcast_id, broadcast_channel, sent_at, sender_display, payload, user_id) VALUES (?, ?, ?, ?, ?, ?, ?),(?, ?, ?, ?, ?, ?, ?)",
			msgs[0].ID,
			msgs[0].RoomID,
			msgs[0].ChannelUserName,
			msgs[0].TMISentTS,
			msgs[0].DisplayName,
			mock.AnythingOfType("[]uint8"),
			msgs[0].UserID,

			msgs[1].ID,
			msgs[1].RoomID,
			msgs[1].ChannelUserName,
			msgs[1].TMISentTS,
			msgs[1].DisplayName,
			mock.AnythingOfType("[]uint8"),
			msgs[1].UserID,
		).Return(nil, nil)

		messageLogger := NewBatchedMessageLogger(zerolog.Nop(), db, db, nil, nil)
		err := messageLogger.createLogEntries(msgs)
		assert.Nil(t, err)
	})
}
