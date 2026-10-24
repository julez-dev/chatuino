package save_test

import (
	"encoding/json"
	"testing"

	"github.com/adrg/xdg"
	"github.com/julez-dev/chatuino/save"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func TestAppState_LayoutRoundTripsAndLegacyTabsRemainCompatible(t *testing.T) {
	// AppStateManager uses XDG's process-global data directory. Keep this test
	// sequential and redirect actual save/load IO to the test's temporary tree.
	previous := xdg.DataHome
	xdg.DataHome = t.TempDir()
	t.Cleanup(func() { xdg.DataHome = previous })
	manager := save.NewAppStateManager(afero.NewOsFs())
	var legacy save.AppState
	require.NoError(t, json.Unmarshal([]byte(`{"tabs":[{"channel":"alpha","identity_id":"main","kind":0,"is_focused":true}]}`), &legacy))
	require.Empty(t, legacy.Layout)
	for _, layout := range []json.RawMessage{
		nil,
		json.RawMessage(`{"version":1,"root":{"tab":"0"},"focus":"0"}`),
		json.RawMessage(`"unusable layout metadata"`),
		json.RawMessage(`{"version":99,"root":null}`),
	} {
		state := legacy
		state.Layout = layout
		require.NoError(t, manager.SaveAppState(state))
		restored, err := manager.LoadAppState()
		require.NoError(t, err, "layout structure must not invalidate the surrounding tab records")
		require.Equal(t, legacy.Tabs, restored.Tabs)
		if layout == nil {
			require.Empty(t, restored.Layout)
		} else {
			require.JSONEq(t, string(layout), string(restored.Layout))
		}
	}
}
