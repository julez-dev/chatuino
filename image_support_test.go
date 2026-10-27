package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnavailableErrorNamesSettingsAndRemedies(t *testing.T) {
	t.Parallel()

	support := graphicsSupport{
		Terminal: "Alacritty",
		Reason:   "Alacritty has no inline image protocol",
		Hint:     "use kitty, Ghostty or iTerm2",
	}
	message := support.unavailableError("/home/me/.config/chatuino/settings.yaml").Error()

	require.Contains(t, message, "Alacritty")
	require.Contains(t, message, "no inline image protocol")
	require.Contains(t, message, "use kitty, Ghostty or iTerm2")
	require.Contains(t, message, "/home/me/.config/chatuino/settings.yaml")
	require.Contains(t, message, "graphic_emotes")
	require.NotContains(t, message, "\n\n", "the message is one compact block")
}
