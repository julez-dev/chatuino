package mainui

import (
	"testing"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/stretchr/testify/require"
)

func TestJoin_SuggestionKeys(t *testing.T) {
	t.Parallel()

	settings := save.BuildDefaultSettings()
	theme := save.BuildDefaultTheme()
	keymap := save.BuildDefaultKeyMap()

	j := newJoin(120, settings, theme, keymap, map[string]tab.ClientCapabilities{}, nil)

	keys := j.input.KeyMap.AcceptSuggestion.Keys()
	require.Contains(t, keys, "ctrl+space")
	require.Contains(t, keys, "right")
	require.Contains(t, keys, "space", "space stays a join-only alias")

	require.NotContains(t, keymap.AcceptSuggestion.Keys(), "space", "the join alias must not mutate the shared keymap")
}
