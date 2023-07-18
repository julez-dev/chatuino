package chatui

import (
	"testing"

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
