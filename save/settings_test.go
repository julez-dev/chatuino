package save

import (
	"path/filepath"
	"testing"

	"github.com/spf13/afero"

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

func TestSettingsPath(t *testing.T) {
	t.Parallel()

	path := SettingsPath()
	require.Equal(t, settingsFileName, filepath.Base(path))
	require.Equal(t, chatuinoConfigDir, filepath.Base(filepath.Dir(path)))
	require.True(t, filepath.IsAbs(path), "the path must be usable in a user-facing message")
}

func TestSetThemeSetting_PreservesDocument(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	path := filepath.Join(t.TempDir(), "settings.yaml")
	writeFile(t, fsys, path, "# my settings\nvertical_tab_list: true # left tabs\nunknown_future_key: 42\ntheme: nord # current\n")

	require.NoError(t, SetThemeSetting(fsys, path, "dracula"))

	written, err := afero.ReadFile(fsys, path)
	require.NoError(t, err)

	require.Contains(t, string(written), "# my settings")
	require.Contains(t, string(written), "# left tabs")
	require.Contains(t, string(written), "unknown_future_key: 42")
	require.Contains(t, string(written), "theme: dracula")
	require.NotContains(t, string(written), "theme: nord")
}

func TestSetThemeSetting_AddsKeysAndCreatesFile(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	path := filepath.Join(t.TempDir(), "settings.yaml")

	require.NoError(t, SetThemeSetting(fsys, path, "dracula"))

	written, err := afero.ReadFile(fsys, path)
	require.NoError(t, err)
	require.Equal(t, "theme: dracula\n", string(written))

	// A second selection replaces the value instead of appending a new key.
	writeFile(t, fsys, path, "mouse: true\ntheme: dracula\n")
	require.NoError(t, SetThemeSetting(fsys, path, "nord"))

	written, err = afero.ReadFile(fsys, path)
	require.NoError(t, err)
	require.Equal(t, "mouse: true\ntheme: nord\n", string(written))
}

func TestSetThemeSetting_RejectsNonMappingDocument(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	path := filepath.Join(t.TempDir(), "settings.yaml")
	writeFile(t, fsys, path, "- just\n- a\n- list\n")

	require.Error(t, SetThemeSetting(fsys, path, "nord"))
}

func TestSetThemeSetting_KeepsNestedIndentation(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	path := filepath.Join(t.TempDir(), "settings.yaml")
	writeFile(t, fsys, path, "moderation:\n  store_chat_logs: false\nchat:\n  time_format: \"15:04\"\n")

	require.NoError(t, SetThemeSetting(fsys, path, "dracula"))

	written, err := afero.ReadFile(fsys, path)
	require.NoError(t, err)
	require.Contains(t, string(written), "moderation:\n  store_chat_logs: false", "nested blocks keep their two-space indentation")
	require.Contains(t, string(written), "theme: dracula")
}
