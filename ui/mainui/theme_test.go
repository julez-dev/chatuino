package mainui

import (
	"path/filepath"
	"testing"
	"time"

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

	// Commands may be timers (cursor blink) or async work the test does not
	// need; only wait briefly so a tick can never stall the suite.
	done := make(chan tea.Msg, 1)

	go func() { done <- cmd() }()

	var msg tea.Msg

	select {
	case msg = <-done:
	case <-time.After(50 * time.Millisecond):
		return
	}

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

func TestRoot_ThemePick_EscapeFirstClearsFilterThenCloses(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	root.hasLoadedSession = true

	sendThemeMsg(t, root, themePickerKey())
	require.Equal(t, themePickerRoute, root.stack.Route())

	// A typed rune goes to the picker's filter, not to a chat tab.
	sendThemeMsg(t, root, tea.KeyPressMsg{Code: 'd', Text: "d"})

	picker, ok := root.stack.Top()
	require.True(t, ok)
	themeFilter, ok := picker.(*themePicker)
	require.True(t, ok)
	require.Equal(t, "d", themeFilter.list.FilterInput.Value())

	// The first escape belongs to the filter, so the picker stays open.
	sendThemeMsg(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	require.Equal(t, themePickerRoute, root.stack.Route(), "escape clears the filter before closing")

	// The second escape closes it and reverts the preview.
	sendThemeMsg(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	require.Empty(t, root.stack.Route())
	require.Equal(t, save.DefaultThemeID, root.themeID)
}
