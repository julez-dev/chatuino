package mainui

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/nav"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func TestRoot_ThemePick_BrokenThemeIsNotSelectable(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	configDir := t.TempDir()
	require.NoError(t, fsys.MkdirAll(filepath.Join(configDir, "themes"), 0o755))
	require.NoError(t, afero.WriteFile(fsys, filepath.Join(configDir, "themes", "broken.yaml"),
		[]byte("chat_streamer_colour: \"#fff\"\n"), 0o600))

	manager := newTestManagerOn(t, fsys, configDir)

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	root.hasLoadedSession = true
	root.themes = manager
	root.theme = manager.Active()
	root.themeID = manager.ActiveID()

	picker := newThemePicker(80, 24, save.BuildDefaultTheme(), save.BuildDefaultKeyMap(), manager.Entries(), manager.ActiveID())
	drainThemeCmd(t, root, root.stack.Push(picker))

	// Walk the cursor to the broken row.
	for {
		entry, ok := picker.selected()
		require.True(t, ok)

		if entry.ID == "broken" {
			require.False(t, entry.Selectable(), "a broken theme is listed but not selectable")
			require.Error(t, entry.Err)

			break
		}

		picker.list.CursorDown()
	}

	_, cmd := picker.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.IsType(t, themeRejectedMessage{}, cmd())
	require.Equal(t, save.DefaultThemeID, root.themeID)
}

func TestThemePicker_ListsSourceAndMarksActive(t *testing.T) {
	t.Parallel()

	manager := newTestManagerOn(t, afero.NewMemMapFs(), t.TempDir())
	picker := newThemePicker(80, 30, save.BuildDefaultTheme(), save.BuildDefaultKeyMap(), manager.Entries(), save.DefaultThemeID)

	view := picker.View()

	require.Contains(t, view, "One Dark", "the list is scrolled to the active theme")
	require.Contains(t, view, "built-in", "rows show where a theme comes from")
	require.Contains(t, view, "Nord ✓", "the active theme is marked")
	require.Contains(t, view, "enter", "the picker explains its keys")
}

// Typing filters the list without a leading slash, and escape leaves the picker
// open while the filter is being edited.
func TestThemePicker_TypeToFilterAndEscapeClears(t *testing.T) {
	t.Parallel()

	manager := newTestManagerOn(t, afero.NewMemMapFs(), t.TempDir())
	picker := newThemePicker(80, 30, save.BuildDefaultTheme(), save.BuildDefaultKeyMap(), manager.Entries(), save.DefaultThemeID)

	var screen nav.Screen = picker

	for _, r := range "drac" {
		screen, _ = screen.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	picker = screen.(*themePicker)

	require.Equal(t, "drac", picker.list.FilterInput.Value(), "typed runes go to the filter, not to the list cursor")
	require.True(t, picker.BlocksEscape(), "the filter owns escape while it is edited")

	// The first escape clears the filter and keeps the picker open, so cancelling
	// the search does not cancel the theme switch.
	screen, _ = screen.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	picker = screen.(*themePicker)

	require.False(t, picker.BlocksEscape())
	require.Equal(t, "", picker.list.FilterInput.Value(), "escape clears the filter")
	require.Equal(t, themePickerRoute, picker.Route())
}

// A filter narrows the list to its matches.
func TestThemePicker_FilterSelectsMatch(t *testing.T) {
	t.Parallel()

	manager := newTestManagerOn(t, afero.NewMemMapFs(), t.TempDir())
	picker := newThemePicker(80, 30, save.BuildDefaultTheme(), save.BuildDefaultKeyMap(), manager.Entries(), save.DefaultThemeID)

	picker.list.SetFilterText("drac")

	entry, ok := picker.selected()
	require.True(t, ok)
	require.Equal(t, "dracula", entry.ID)
}
