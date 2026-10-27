package save

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestParseTheme_PartialFileKeepsDefaults(t *testing.T) {
	t.Parallel()

	theme, err := ParseTheme([]byte("name: Mine\nchat_streamer_color: \"#ff79c6\"\n"))
	require.NoError(t, err)

	require.Equal(t, "Mine", theme.Name)
	require.Equal(t, "#ff79c6", theme.ChatStreamerColor)
	require.Equal(t, BuildDefaultTheme().StatusColor, theme.StatusColor, "unset keys keep the default")
}

func TestParseTheme_UnknownKeyIsAnError(t *testing.T) {
	t.Parallel()

	_, err := ParseTheme([]byte("chat_streamer_colour: \"#ff79c6\"\n"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "chat_streamer_colour")
}

func TestParseTheme_EmptyFileIsAllDefaults(t *testing.T) {
	t.Parallel()

	theme, err := ParseTheme(nil)
	require.NoError(t, err)
	require.Equal(t, BuildDefaultTheme(), theme)
}

func TestLoadThemeCatalog_BuiltinsLoad(t *testing.T) {
	t.Parallel()

	catalog, err := loadThemeCatalog(afero.NewMemMapFs(), t.TempDir())
	require.NoError(t, err)

	entries := catalog.Entries()
	require.Len(t, entries, 12)

	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		require.NoError(t, entry.Err, "built-in %s must parse", entry.ID)
		require.Equal(t, ThemeSourceBuiltin, entry.Source)
		ids = append(ids, entry.ID)
	}

	require.Equal(t, []string{
		"ansi16-high-contrast",
		"catppuccin-latte",
		"catppuccin-mocha",
		"dracula",
		"gruvbox-dark",
		"gruvbox-light",
		"nord",
		"one-dark",
		"one-light",
		"solarized-dark",
		"solarized-light",
		"tokyo-night",
	}, ids)

	nord, ok := catalog.Lookup(DefaultThemeID)
	require.True(t, ok)
	require.Equal(t, "Nord", nord.Name())
	expected := BuildDefaultTheme()
	expected.Name = "Nord"
	require.Equal(t, expected, nord.Theme, "the embedded default matches the hardcoded fallback")
}

func TestLoadThemeCatalog_UserFileShadowsBuiltin(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	configDir := t.TempDir()
	writeFile(t, fsys, filepath.Join(configDir, "themes", "nord.yaml"), "chat_streamer_color: \"#ff0000\"\n")

	catalog, err := loadThemeCatalog(fsys, configDir)
	require.NoError(t, err)

	nord, ok := catalog.Lookup("nord")
	require.True(t, ok)
	require.Equal(t, ThemeSourceUser, nord.Source)
	require.Equal(t, "#ff0000", nord.Theme.ChatStreamerColor)
}

func TestLoadThemeCatalog_IgnoresNonThemeFiles(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	configDir := t.TempDir()
	dir := filepath.Join(configDir, "themes")

	writeFile(t, fsys, filepath.Join(dir, "mine.yml"), "name: Mine\n")
	writeFile(t, fsys, filepath.Join(dir, ".hidden.yaml"), "name: Hidden\n")
	writeFile(t, fsys, filepath.Join(dir, "notes.txt"), "not a theme")
	writeFile(t, fsys, filepath.Join(dir, "backup.yaml.bak"), "name: Backup\n")
	writeFile(t, fsys, filepath.Join(dir, "nested", "nested.yaml"), "name: Nested\n")

	catalog, err := loadThemeCatalog(fsys, configDir)
	require.NoError(t, err)

	user := userEntries(catalog.Entries())
	require.Len(t, user, 1)
	require.Equal(t, "mine", user[0].ID)
	require.Equal(t, "Mine", user[0].Name())
}

func TestLoadThemeCatalog_FlagsDuplicateIDs(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	configDir := t.TempDir()
	dir := filepath.Join(configDir, "themes")

	writeFile(t, fsys, filepath.Join(dir, "nord.yaml"), "name: Mine\n")
	writeFile(t, fsys, filepath.Join(dir, "Nord.yaml"), "name: Also mine\n")

	catalog, err := loadThemeCatalog(fsys, configDir)
	require.NoError(t, err)

	entry, ok := catalog.Lookup("nord")
	require.True(t, ok)
	require.Error(t, entry.Err)
	require.Contains(t, entry.Err.Error(), "duplicate id")
}

func TestLoadThemeCatalog_LegacyThemeFile(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	configDir := t.TempDir()
	writeFile(t, fsys, filepath.Join(configDir, "theme.yaml"), "status_color: \"#abcdef\"\n")

	catalog, err := loadThemeCatalog(fsys, configDir)
	require.NoError(t, err)

	entry, ok := catalog.Lookup(LegacyThemeID)
	require.True(t, ok)
	require.Equal(t, ThemeSourceLegacy, entry.Source)
	require.Equal(t, "custom (legacy)", entry.Name())
	require.Equal(t, "#abcdef", entry.Theme.StatusColor)
}

func TestLoadThemeCatalog_DoesNotCreateFiles(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	configDir := t.TempDir()

	_, err := loadThemeCatalog(fsys, configDir)
	require.NoError(t, err)

	exists, err := afero.Exists(fsys, filepath.Join(configDir, "theme.yaml"))
	require.NoError(t, err)
	require.False(t, exists, "loading themes must not create a legacy theme file")
}

func TestThemeCatalog_Resolve(t *testing.T) {
	t.Parallel()

	t.Run("unset prefers the legacy file", func(t *testing.T) {
		t.Parallel()

		fsys := afero.NewMemMapFs()
		configDir := t.TempDir()
		writeFile(t, fsys, filepath.Join(configDir, "theme.yaml"), "status_color: \"#abcdef\"\n")

		catalog, err := loadThemeCatalog(fsys, configDir)
		require.NoError(t, err)

		resolution := catalog.Resolve("")
		require.Equal(t, LegacyThemeID, resolution.ID)
		require.Equal(t, "#abcdef", resolution.Theme.StatusColor)
		require.Empty(t, resolution.Warning)
	})

	t.Run("unset without a legacy file uses the default", func(t *testing.T) {
		t.Parallel()

		catalog, err := loadThemeCatalog(afero.NewMemMapFs(), t.TempDir())
		require.NoError(t, err)

		resolution := catalog.Resolve("")
		require.Equal(t, DefaultThemeID, resolution.ID)
		require.Empty(t, resolution.Warning)
	})

	t.Run("unknown id falls back with a warning", func(t *testing.T) {
		t.Parallel()

		catalog, err := loadThemeCatalog(afero.NewMemMapFs(), t.TempDir())
		require.NoError(t, err)

		resolution := catalog.Resolve("draculaa")
		require.Equal(t, DefaultThemeID, resolution.ID)
		require.Contains(t, resolution.Warning, "unknown theme \"draculaa\"")
		require.Contains(t, resolution.Warning, DefaultThemeID)
	})

	t.Run("broken file falls back with a warning", func(t *testing.T) {
		t.Parallel()

		fsys := afero.NewMemMapFs()
		configDir := t.TempDir()
		writeFile(t, fsys, filepath.Join(configDir, "themes", "broken.yaml"), "chat_streamer_colour: \"#fff\"\n")

		catalog, err := loadThemeCatalog(fsys, configDir)
		require.NoError(t, err)

		resolution := catalog.Resolve("broken")
		require.Equal(t, DefaultThemeID, resolution.ID)
		require.Contains(t, resolution.Warning, "chat_streamer_colour")
	})

	t.Run("built-in id resolves", func(t *testing.T) {
		t.Parallel()

		catalog, err := loadThemeCatalog(afero.NewMemMapFs(), t.TempDir())
		require.NoError(t, err)

		resolution := catalog.Resolve("Dracula")
		require.Equal(t, "dracula", resolution.ID, "lookup is case-insensitive")
		require.Equal(t, "#bd93f9", resolution.Theme.InputPromptColor)
	})
}

func TestThemeManager_SelectPersistsAndSwaps(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	configDir := t.TempDir()
	writeFile(t, fsys, filepath.Join(configDir, "settings.yaml"),
		"vertical_tab_list: true # keep me\nmouse: false\n")

	manager, err := NewThemeManager(fsys, configDir, "")
	require.NoError(t, err)
	require.Equal(t, DefaultThemeID, manager.ActiveID())
	require.Empty(t, manager.Warnings())

	require.NoError(t, manager.Select("dracula"))
	require.Equal(t, "dracula", manager.ActiveID())
	require.Equal(t, "#bd93f9", manager.Active().InputPromptColor)

	written, err := afero.ReadFile(fsys, filepath.Join(configDir, "settings.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(written), "theme: dracula")
	require.Contains(t, string(written), "# keep me", "existing comments survive the write")

	settings := BuildDefaultSettings()
	require.NoError(t, yaml.Unmarshal(written, &settings))
	require.Equal(t, "dracula", settings.Theme)
	require.True(t, settings.VerticalTabList)
	require.False(t, settings.Mouse)
}

func TestThemeManager_SelectRejectsUnknownAndInvalid(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	configDir := t.TempDir()
	writeFile(t, fsys, filepath.Join(configDir, "themes", "broken.yaml"), "chat_streamer_colour: \"#fff\"\n")

	manager, err := NewThemeManager(fsys, configDir, "")
	require.NoError(t, err)

	require.Error(t, manager.Select("nope"))
	require.Error(t, manager.Select("broken"))
	require.Equal(t, DefaultThemeID, manager.ActiveID())

	exists, err := afero.Exists(fsys, filepath.Join(configDir, "settings.yaml"))
	require.NoError(t, err)
	require.False(t, exists, "a rejected selection must not create a settings file")
}

func TestThemeManager_ReloadPicksUpNewFiles(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	configDir := t.TempDir()

	manager, err := NewThemeManager(fsys, configDir, "")
	require.NoError(t, err)

	_, ok := manager.catalog.Lookup("fresh")
	require.False(t, ok)

	writeFile(t, fsys, filepath.Join(configDir, "themes", "fresh.yaml"), "name: Fresh\n")
	require.NoError(t, manager.Reload())

	entry, ok := manager.catalog.Lookup("fresh")
	require.True(t, ok)
	require.Equal(t, "Fresh", entry.Name())
}

func TestThemeIDFromFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
		ok   bool
	}{
		{name: "dracula.yaml", id: "dracula", ok: true},
		{name: "My-Warm.yml", id: "my-warm", ok: true},
		{name: "nord.YAML", id: "nord", ok: true},
		{name: ".yaml", ok: false},
		{name: "theme.json", ok: false},
		{name: "notes.txt", ok: false},
	}

	for _, tt := range tests {
		id, ok := ThemeIDFromFileName(tt.name)
		require.Equal(t, tt.ok, ok, tt.name)

		if tt.ok {
			require.Equal(t, tt.id, id, tt.name)
		}
	}
}

func writeFile(t *testing.T, fsys afero.Fs, path, content string) {
	t.Helper()
	require.NoError(t, fsys.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, afero.WriteFile(fsys, path, []byte(content), 0o600))
}

func userEntries(entries []ThemeEntry) []ThemeEntry {
	var result []ThemeEntry

	for _, entry := range entries {
		if entry.Source != ThemeSourceBuiltin {
			result = append(result, entry)
		}
	}

	return result
}

func TestThemeManager_WarnsAboutEveryUnusableFile(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	configDir := t.TempDir()
	writeFile(t, fsys, filepath.Join(configDir, "themes", "broken.yaml"), "chat_streamer_colour: \"#fff\"\n")
	writeFile(t, fsys, filepath.Join(configDir, "themes", "nord.yaml"), "name: Mine\n")
	writeFile(t, fsys, filepath.Join(configDir, "themes", "Nord.yaml"), "name: Also mine\n")

	manager, err := NewThemeManager(fsys, configDir, "")
	require.NoError(t, err)

	warnings := strings.Join(manager.Warnings(), "\n")
	require.Contains(t, warnings, "chat_streamer_colour", "a broken file is reported even when it is not selected")
	require.Contains(t, warnings, "duplicate id", "a duplicate id is reported")
	require.Contains(t, warnings, "broken.yaml", "the warning names the file")
}

func TestThemeManager_WarnsWhenLegacyFileIsIgnored(t *testing.T) {
	t.Parallel()

	fsys := afero.NewMemMapFs()
	configDir := t.TempDir()
	writeFile(t, fsys, filepath.Join(configDir, "theme.yaml"), "status_color: \"#abcdef\"\n")

	manager, err := NewThemeManager(fsys, configDir, "dracula")
	require.NoError(t, err)
	require.Equal(t, "dracula", manager.ActiveID())
	require.Contains(t, strings.Join(manager.Warnings(), "\n"), "theme.yaml is no longer used")

	// Selecting the legacy theme explicitly keeps it usable and is not a warning.
	manager, err = NewThemeManager(fsys, configDir, LegacyThemeID)
	require.NoError(t, err)
	require.Empty(t, manager.Warnings())
	require.Equal(t, "#abcdef", manager.Active().StatusColor)
}
