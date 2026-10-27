package mainui

import (
	"testing"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/chat"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestHelp_ListsConfigurablePaneControlsAndDistinctClosures(t *testing.T) {
	t.Parallel()
	mapping := save.BuildDefaultKeyMap()
	require.NoError(t, yaml.Unmarshal([]byte("pane_mode: [ctrl+g]\npane_close: [ctrl+x]\n"), &mapping))
	view := chat.StripANSI(newHelp(220, 140, mapping).View())
	require.Contains(t, view, "Pane Controls")
	require.Contains(t, view, "ctrl+g")
	require.Contains(t, view, "ctrl+x")
	require.Contains(t, view, "hide pane (keep tab)")
	require.Contains(t, view, "close current tab")
	require.Contains(t, view, "split left/right")
	require.Contains(t, view, "increase pane width")
}
