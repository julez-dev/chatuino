package mainui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func themePickerKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl | tea.ModAlt}
}

// newTestManagerOn wires a theme manager whose config lives in an in-memory fs,
// so a test can assert what was persisted.
func newTestManagerOn(t *testing.T, fsys afero.Fs, configDir string) *save.ThemeManager {
	t.Helper()

	manager, err := save.NewThemeManager(fsys, configDir, "")
	require.NoError(t, err)

	return manager
}

// sendThemeMsg feeds one message to the root and drains every command it
// produces, which is what the bubbletea runtime does for the real UI.
func sendThemeMsg(t *testing.T, root *Root, msg tea.Msg) {
	t.Helper()

	_, cmd := root.Update(msg)
	drainThemeCmd(t, root, cmd)
}

func drainThemeCmd(t *testing.T, root *Root, cmd tea.Cmd) {
	t.Helper()

	if cmd == nil {
		return
	}

	msg := cmd()
	if msg == nil {
		return
	}

	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range batch {
			drainThemeCmd(t, root, sub)
		}

		return
	}

	_, next := root.Update(msg)
	drainThemeCmd(t, root, next)
}

// readThemeSetting parses the persisted theme id out of a settings file.
func readThemeSetting(t *testing.T, fsys afero.Fs, path string) string {
	t.Helper()

	data, err := afero.ReadFile(fsys, path)
	require.NoError(t, err)

	settings := save.BuildDefaultSettings()
	require.NoError(t, yaml.Unmarshal(data, &settings))

	return settings.Theme
}

func TestRoot_ThemePick_PreviewsWithoutPersisting(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	configDir := t.TempDir()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	root.hasLoadedSession = true
	root.themes = newTestManagerOn(t, fsys, configDir)
	root.theme = root.themes.Active()
	root.themeID = root.themes.ActiveID()

	require.Equal(t, save.DefaultThemeID, root.themeID)

	sendThemeMsg(t, root, themePickerKey())
	require.Equal(t, themePickerRoute, root.stack.Route(), "the key opens the picker")

	sendThemeMsg(t, root, tea.KeyPressMsg{Code: tea.KeyDown})
	require.Equal(t, "one-dark", root.themeID, "moving the cursor previews the next theme")
	require.Equal(t, save.DefaultThemeID, root.themes.ActiveID(), "previewing never persists")
	require.Equal(t, "#21252b", root.theme.TabHeaderBackgroundColor)

	exists, err := afero.Exists(fsys, filepath.Join(configDir, "settings.yaml"))
	require.NoError(t, err)
	require.False(t, exists, "a preview must not touch settings.yaml")

	sendThemeMsg(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	require.Empty(t, root.stack.Route())
	require.Equal(t, save.DefaultThemeID, root.themeID, "escaping reverts the preview")
}

func TestRoot_ThemePick_ConfirmPersistsSelection(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	configDir := t.TempDir()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	root.hasLoadedSession = true
	root.themes = newTestManagerOn(t, fsys, configDir)
	root.theme = root.themes.Active()
	root.themeID = root.themes.ActiveID()

	sendThemeMsg(t, root, themePickerKey())
	sendThemeMsg(t, root, tea.KeyPressMsg{Code: tea.KeyDown})

	previewed := root.themeID
	require.Equal(t, "one-dark", previewed)

	sendThemeMsg(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})

	require.Equal(t, previewed, root.themeID)
	require.Empty(t, root.stack.Route(), "confirming closes the picker")
	require.Equal(t, previewed, root.themes.ActiveID())
	require.Equal(t, previewed, readThemeSetting(t, fsys, filepath.Join(configDir, "settings.yaml")))
}

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

func TestRoot_ApplyTheme_RepaintsOpenTabs(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, []save.Account{{ID: "acc", DisplayName: "me"}}, map[string]tab.ClientCapabilities{}, nil)
	root.hasLoadedSession = true

	newTab := newTestTab("tab-1", 40, 20, save.Account{ID: "acc", DisplayName: "me"}, "channel")
	root.tabs.Add(newTab)
	root.tabs.SetActive(newTab.ID())

	entry, ok := root.themes.Lookup("dracula")
	require.True(t, ok)

	drainThemeCmd(t, root, root.applyTheme(entry.Theme, entry.ID))

	require.Equal(t, "dracula", root.themeID)
	require.Equal(t, "#21222c", root.theme.TabHeaderBackgroundColor)
}

func TestRoot_NoticeIsTransient(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	root.hasLoadedSession = true

	root.setNotice("something happened")
	require.Equal(t, "something happened", root.notice)

	sendThemeMsg(t, root, tea.KeyPressMsg{Code: 'x', Text: "x"})
	require.Empty(t, root.notice, "the next key press dismisses the notice")

	// The notice replaces the bottom row instead of pushing the layout taller.
	root.setNotice("themes/broken.yaml: field chat_streamer_colour not found")
	view := root.withNotice("row-1\nrow-2")
	lines := strings.Split(view, "\n")

	require.Len(t, lines, 2, "the notice must not add a row")
	require.Contains(t, lines[1], "chat_streamer_colour")
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
