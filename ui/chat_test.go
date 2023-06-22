package ui

import (
	"testing"
	"time"

	"github.com/julez-dev/chatuino/twitch"
	"github.com/stretchr/testify/assert"
)

func Test_messageToText(t *testing.T) {
	t.Run("priv-message", func(t *testing.T) {
		time := time.Now()
		message := &twitch.PrivateMessage{
			From:    "test-user",
			In:      "test-channel",
			Message: "my super cool message",
			SentAt:  time,
		}

		lines := messageToText(message, 50)
		assert.Len(t, lines, 1)
	})
	t.Run("priv-message-multi", func(t *testing.T) {
		time := time.Now()
		message := &twitch.PrivateMessage{
			From:    "test-user",
			In:      "test-channel",
			Message: "my super cool message which is super long and super nice",
			SentAt:  time,
		}

		lines := messageToText(message, 50)
		assert.Equal(t, "", lines)
	})
}
