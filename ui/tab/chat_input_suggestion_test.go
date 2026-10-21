package tab

import (
	"testing"

	"github.com/julez-dev/chatuino/save"
	"github.com/stretchr/testify/require"
)

func TestChatInput_SetSuggestionKeyMap(t *testing.T) {
	t.Parallel()

	keymap := save.BuildDefaultKeyMap()
	keymap.AcceptSuggestion.SetKeys("ctrl+space")

	input := newChatInput(map[string]func(...string) string{}, nil, save.BuildDefaultTheme(), nil)
	input.SetSuggestionKeyMap(keymap)

	require.Equal(t, []string{"ctrl+space"}, input.input.KeyMap.AcceptSuggestion.Keys())
}
