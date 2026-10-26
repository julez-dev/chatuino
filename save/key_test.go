package save

import (
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestKeyMap_MarshalYAML(t *testing.T) {
	t.Parallel()

	keyMap := &KeyMap{
		Up: key.NewBinding(key.WithKeys("w", "q"), key.WithHelp("w", "test")),
	}

	doc, err := yaml.Marshal(keyMap)

	require.NoError(t, err)
	require.Equal(t, "up:\n    - w\n    - q\n", string(doc))

}

func TestBuildDefaultKeyMap_IncludesVimAndModernAliases(t *testing.T) {
	t.Parallel()

	keyMap := BuildDefaultKeyMap()

	require.Contains(t, keyMap.Quit.Keys(), "ctrl+c")
	require.Contains(t, keyMap.Quit.Keys(), "q")
	require.Contains(t, keyMap.GoToTop.Keys(), "t")
	require.Contains(t, keyMap.GoToTop.Keys(), "g")
	require.Contains(t, keyMap.GoToTop.Keys(), "home")
	require.Contains(t, keyMap.GoToBottom.Keys(), "b")
	require.Contains(t, keyMap.GoToBottom.Keys(), "G")
	require.Contains(t, keyMap.GoToBottom.Keys(), "end")
	require.Contains(t, keyMap.PageUp.Keys(), "pgup")
	require.Contains(t, keyMap.PageUp.Keys(), "ctrl+u")
	require.Contains(t, keyMap.PageDown.Keys(), "pgdown")
	require.Contains(t, keyMap.PageDown.Keys(), "ctrl+d")
	require.Contains(t, keyMap.InsertMode.Keys(), "enter")
	require.Contains(t, keyMap.CopyMessage.Keys(), "y")
	require.Contains(t, keyMap.CopyToClipboard.Keys(), "Y")
	require.Contains(t, keyMap.ReplyMessage.Keys(), "r")
	require.Contains(t, keyMap.QuickApprove.Keys(), "alt+a")
	require.Contains(t, keyMap.QuickDeny.Keys(), "alt+d")
	require.Contains(t, keyMap.InspectMode.Keys(), "u")
	require.Contains(t, keyMap.SearchMode.Keys(), "ctrl+f")
	require.Contains(t, keyMap.Next.Keys(), "]")
	require.Contains(t, keyMap.Previous.Keys(), "[")
	require.Contains(t, keyMap.AcceptSuggestion.Keys(), "tab")
	require.Contains(t, keyMap.AcceptSuggestion.Keys(), "ctrl+space")
	require.Contains(t, keyMap.AcceptSuggestion.Keys(), "right")
	require.Contains(t, keyMap.NextSuggestion.Keys(), "ctrl+n")
	require.Contains(t, keyMap.PrevSuggestion.Keys(), "ctrl+p")
}

// Existing keymap.yaml files are intentionally never rewritten. Fields present
// in the file keep their keys; fields added later fall back to their defaults.
func TestKeyMap_UnmarshalLegacyConfigKeepsNewFields(t *testing.T) {
	t.Parallel()

	legacy := []byte("go_to_top:\n    - t\nquit:\n    - ctrl+c\n")

	keyMap := BuildDefaultKeyMap()
	require.NoError(t, yaml.Unmarshal(legacy, &keyMap))

	require.Equal(t, []string{"t"}, keyMap.GoToTop.Keys(), "existing field is preserved")
	require.Equal(t, []string{"ctrl+c"}, keyMap.Quit.Keys(), "existing field is preserved")
	require.Equal(t, []string{"pgup", "ctrl+u"}, keyMap.PageUp.Keys(), "new field uses the default")
}

func TestKeyMap_PaneBindingsKeepDefaultsAndExplicitOverrides(t *testing.T) {
	t.Parallel()
	mapping := BuildDefaultKeyMap()
	require.Equal(t, []string{"f2"}, mapping.PaneMode.Keys())
	require.Equal(t, []string{"left", "h"}, mapping.PaneFocusLeft.Keys())
	require.Equal(t, []string{"H"}, mapping.PaneSwapLeft.Keys())
	require.Equal(t, []string{"v"}, mapping.PaneSplitColumns.Keys())
	require.Equal(t, []string{"s"}, mapping.PaneSplitRows.Keys())
	require.NoError(t, yaml.Unmarshal([]byte("quit: [ctrl+c]\npane_mode: [ctrl+g]\npane_focus_left: []\npane_close: [ctrl+x]\n"), &mapping))
	require.Equal(t, []string{"ctrl+g"}, mapping.PaneMode.Keys())
	require.Empty(t, mapping.PaneFocusLeft.Keys(), "explicitly empty bindings disable that action")
	require.Equal(t, []string{"ctrl+x"}, mapping.PaneClose.Keys())
	require.Equal(t, []string{"tab"}, mapping.PaneNext.Keys(), "omitted new actions retain defaults")
	require.Equal(t, []string{"ctrl+c"}, mapping.Quit.Keys())
	encoded, err := yaml.Marshal(&mapping)
	require.NoError(t, err)
	roundtrip := BuildDefaultKeyMap()
	require.NoError(t, yaml.Unmarshal(encoded, &roundtrip))
	require.Empty(t, roundtrip.PaneFocusLeft.Keys())
	require.Equal(t, mapping.PaneMode.Keys(), roundtrip.PaneMode.Keys())
}

func TestKeyMap_UnmarshalYAML(t *testing.T) {
	t.Parallel()

	gotKeyMap := &KeyMap{
		Up: key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "test-help")),
	}

	err := yaml.Unmarshal([]byte("up:\n    - w\n    - q\n"), &gotKeyMap)
	require.NoError(t, err)
	require.Equal(t, []string{"w", "q"}, gotKeyMap.Up.Keys())
	require.Equal(t, "test-help", gotKeyMap.Up.Help().Desc)   // should not be overwritten
	require.Equal(t, []string{"w", "q"}, gotKeyMap.Up.Keys()) // should be overwritten

}
