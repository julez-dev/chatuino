package save

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettings_ValidateGraphicScale(t *testing.T) {
	t.Parallel()

	for _, scale := range []int{0, 1, 4} {
		s := BuildDefaultSettings()
		s.Chat.GraphicScale = scale
		require.NoError(t, s.validate(), "scale %d", scale)
	}

	for _, scale := range []int{-1, 5} {
		s := BuildDefaultSettings()
		s.Chat.GraphicScale = scale
		require.ErrorContains(t, s.validate(), "chat.graphic_scale")
	}
}
