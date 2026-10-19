package save

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSettings_MouseDefaultAndOverride(t *testing.T) {
	t.Parallel()

	require.True(t, BuildDefaultSettings().Mouse, "mouse capture is on by default")

	settings := BuildDefaultSettings()
	require.NoError(t, yaml.Unmarshal([]byte("mouse: false\n"), &settings))
	require.False(t, settings.Mouse, "explicit false disables mouse capture")

	settings = BuildDefaultSettings()
	require.NoError(t, yaml.Unmarshal([]byte("{}\n"), &settings))
	require.True(t, settings.Mouse, "a settings file without the key keeps the default")
}

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
