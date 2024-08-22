package mainui

import (
	"slices"
	"testing"

	"github.com/julez-dev/chatuino/twitch/command"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/stretchr/testify/assert"
)

func Test_selectWordAtIndex(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		sentence := "a new and nice sentence"

		res := selectWordAtIndex(sentence, 7)
		assert.Equal(t, "and", res)
	})

	t.Run("one word", func(t *testing.T) {
		sentence := "new"

		res := selectWordAtIndex(sentence, 1)
		assert.Equal(t, "new", res)
	})

	t.Run("bounds", func(t *testing.T) {
		sentence := "new bla"

		res := selectWordAtIndex(sentence, len(sentence)+1)
		assert.Equal(t, "", res)
	})
}

func Test_filter(t *testing.T) {
	entries := []*chatEntry{
		{
			Event: chatEventMessage{
				message: &command.PrivateMessage{
					DisplayName: "abc",
					Message:     "hello test message",
				},
			},
		},
		{
			Event: chatEventMessage{
				message: &command.PrivateMessage{
					DisplayName: "xyz",
					Message:     "hello test message",
				},
			},
		},
	}

	filtered := slices.Collect(filter(entries, func(e *chatEntry) bool {
		cast, ok := e.Event.message.(*command.PrivateMessage)

		if !ok {
			return false
		}

		if fuzzy.MatchFold("bc", cast.DisplayName) || fuzzy.MatchFold("bc", cast.Message) {
			return true
		}

		return false
	}))

	assert.Len(t, filtered, 1)
}
