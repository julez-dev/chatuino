package save

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

const (
	themeFileName = "theme.yaml"
	themeDirName  = "themes"

	// DefaultThemeID is the theme used when nothing else is configured. It is a
	// built-in, so the default never depends on files the user has on disk.
	DefaultThemeID = "nord"

	// LegacyThemeID is the id the pre-themes/ theme.yaml file is exposed under.
	// Keeping it addressable means an existing customization keeps working and
	// can still be selected from the picker, without ever moving the file.
	LegacyThemeID = "custom"

	themeExtYAML = ".yaml"
	themeExtYML  = ".yml"
)

// builtinThemes holds the shipped themes. They are parsed by the same loader as
// user files, so a built-in and a user file can never diverge in behavior.
//
//go:embed themes/*.yaml
var builtinThemes embed.FS

// ThemeSource reports where a theme was loaded from.
type ThemeSource int

const (
	ThemeSourceBuiltin ThemeSource = iota
	ThemeSourceUser
	ThemeSourceLegacy
)

func (s ThemeSource) String() string {
	switch s {
	case ThemeSourceUser:
		return "user"
	case ThemeSourceLegacy:
		return "legacy"
	default:
		return "built-in"
	}
}

// Theme is the complete set of colors one chatuino theme controls.
//
// Every field is optional in a theme file: a file is merged over the default
// theme, so a theme only has to list the colors it actually changes.
type Theme struct {
	// Name is the display name shown in the theme picker. It is optional; the
	// filename (without extension) is the theme id.
	Name string `yaml:"name"`

	SevenTVEmoteColor   string `yaml:"seven_tv_emote_color"`
	TwitchTVEmoteColor  string `yaml:"twitch_tv_emote_color"`
	BetterTTVEmoteColor string `yaml:"better_ttv_emote_color"`
	FFZEmoteColor       string `yaml:"ffz_emote_color"`

	InputPromptColor string `yaml:"input_prompt_color"`

	ChatStreamerColor    string `yaml:"chat_streamer_color"`
	ChatVIPColor         string `yaml:"chat_vip_color"`
	ChatSubColor         string `yaml:"chat_sub_color"`
	ChatTurboColor       string `yaml:"chat_turbo_color"`
	ChatModeratorColor   string `yaml:"chat_moderator_color"`
	ChatIndicatorColor   string `yaml:"chat_indicator_color"`
	ChatSubAlertColor    string `yaml:"chat_sub_alert_color"`
	ChatNoticeAlertColor string `yaml:"chat_notice_alert_color"`
	ChatClearChatColor   string `yaml:"chat_clear_chat_color"`
	ChatErrorColor       string `yaml:"chat_error_color"`

	ListSelectedColor string `yaml:"list_selected_color"`
	ListLabelColor    string `yaml:"list_label_color"`
	ActiveLabelColor  string `yaml:"active_label_color"`

	StatusColor string `yaml:"status_color"`

	ChatuinoSplashColor  string `yaml:"chatuino_splash_color"`
	SplashHighlightColor string `yaml:"splash_highlight_color"`

	TabHeaderBackgroundColor       string `yaml:"tab_header_background_color"`
	TabHeaderActiveBackgroundColor string `yaml:"tab_header_active_background_color"`

	InspectBorderColor string `yaml:"inspect_border_color"`

	ListBackgroundColor string `yaml:"list_background_color"`
	ListFontColor       string `yaml:"list_font_color"`

	// UI chrome
	DimmedTextColor string `yaml:"dimmed_text_color"`
}

// BuildDefaultTheme returns the hardcoded fallback theme. It is inspired by the
// Nord color scheme and matches the embedded nord theme, but it exists so a
// damaged or missing theme catalog can never leave the UI without colors.
func BuildDefaultTheme() Theme {
	return Theme{
		// Emote provider colors - slightly more vibrant
		SevenTVEmoteColor:   "#88c0d0",
		TwitchTVEmoteColor:  "#b48ead",
		BetterTTVEmoteColor: "#bf616a",
		FFZEmoteColor:       "#a3be8c",

		// Input
		InputPromptColor: "#88c0d0",

		// Chat user colors - improved contrast
		ChatStreamerColor:  "#d08770",
		ChatVIPColor:       "#b48ead",
		ChatSubColor:       "#a3be8c",
		ChatTurboColor:     "#5e81ac",
		ChatModeratorColor: "#a3be8c",
		ChatIndicatorColor: "#88c0d0",

		// Alert colors - better visibility
		ChatSubAlertColor:    "#b48ead",
		ChatNoticeAlertColor: "#ebcb8b",
		ChatClearChatColor:   "#d08770",
		ChatErrorColor:       "#bf616a",

		// List/selection colors
		ListSelectedColor: "#88c0d0",
		ListLabelColor:    "#81a1c1",
		ActiveLabelColor:  "#ebcb8b",

		// Status
		StatusColor: "#88c0d0",

		// Splash screen
		ChatuinoSplashColor:  "#fd00eb",
		SplashHighlightColor: "#88c0d0",

		// Tab headers - deeper contrast
		TabHeaderBackgroundColor:       "#3b4252",
		TabHeaderActiveBackgroundColor: "#2e3440",

		// Borders
		InspectBorderColor: "#5e81ac",

		// List styling
		ListBackgroundColor: "#2e3440",
		ListFontColor:       "#d8dee9",

		// UI chrome
		DimmedTextColor: "#4c566a",
	}
}

// ParseTheme decodes one theme file. Missing keys keep their default value and
// unknown keys are an error, so a typo is reported instead of silently ignored.
func ParseTheme(data []byte) (Theme, error) {
	theme := BuildDefaultTheme()

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	if err := decoder.Decode(&theme); err != nil {
		if errors.Is(err, io.EOF) {
			// An empty file is a valid theme: it is simply all defaults.
			return theme, nil
		}

		return theme, err
	}

	return theme, nil
}

// ThemeEntry is one theme known to the catalog. Err is set when the file was
// found but could not be loaded; the entry is still listed so the picker can
// show what is wrong with it.
type ThemeEntry struct {
	ID     string
	Source ThemeSource
	Path   string
	Theme  Theme
	Err    error
}

// Name returns the display name of the theme, falling back to its id.
func (e ThemeEntry) Name() string {
	if e.Theme.Name != "" {
		return e.Theme.Name
	}

	return e.ID
}

// Selectable reports whether the theme can be activated.
func (e ThemeEntry) Selectable() bool { return e.Err == nil }

// ThemeCatalog is the set of built-in and user themes known to this process.
type ThemeCatalog struct {
	entries []ThemeEntry
}

// Entries returns the themes in picker order: built-ins alphabetically first,
// then user and legacy themes alphabetically.
func (c *ThemeCatalog) Entries() []ThemeEntry {
	entries := make([]ThemeEntry, len(c.entries))
	copy(entries, c.entries)

	return entries
}

// Lookup returns the entry for id, matching case-insensitively.
func (c *ThemeCatalog) Lookup(id string) (ThemeEntry, bool) {
	if c == nil {
		return ThemeEntry{}, false
	}

	for _, entry := range c.entries {
		if entry.ID == id {
			return entry, true
		}
	}

	lower := strings.ToLower(id)

	for _, entry := range c.entries {
		if entry.ID == lower {
			return entry, true
		}
	}

	return ThemeEntry{}, false
}

// ThemeResolution is the outcome of resolving a configured theme id. Warning is
// non-empty when the requested theme could not be used, so the caller can tell
// the user why they are looking at a fallback.
type ThemeResolution struct {
	ID      string
	Theme   Theme
	Warning string
}

// Resolve picks the theme to use for a configured theme id.
//
// An empty id means "nothing configured": the legacy theme.yaml stays active
// while it exists, otherwise the built-in default is used. A requested id that
// is missing or invalid falls back to the default and reports a warning.
func (c *ThemeCatalog) Resolve(requested string) ThemeResolution {
	if requested == "" {
		if entry, ok := c.Lookup(LegacyThemeID); ok && entry.Source == ThemeSourceLegacy {
			if entry.Err != nil {
				fallback := c.defaultResolution()
				fallback.Warning = fmt.Sprintf("%s is invalid (%s), using %s", entry.Path, entry.Err, fallback.ID)

				return fallback
			}

			return ThemeResolution{ID: entry.ID, Theme: entry.Theme}
		}

		return c.defaultResolution()
	}

	entry, ok := c.Lookup(requested)
	switch {
	case !ok:
		fallback := c.defaultResolution()
		fallback.Warning = fmt.Sprintf("unknown theme %q, using %s", requested, fallback.ID)

		return fallback
	case entry.Err != nil:
		fallback := c.defaultResolution()
		fallback.Warning = fmt.Sprintf("theme %q is invalid (%s), using %s", requested, entry.Err, fallback.ID)

		return fallback
	}

	return ThemeResolution{ID: entry.ID, Theme: entry.Theme}
}

func (c *ThemeCatalog) defaultResolution() ThemeResolution {
	if entry, ok := c.Lookup(DefaultThemeID); ok && entry.Err == nil {
		return ThemeResolution{ID: entry.ID, Theme: entry.Theme}
	}

	return ThemeResolution{ID: DefaultThemeID, Theme: BuildDefaultTheme()}
}

// ConfigDir returns the chatuino configuration directory.
func ConfigDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return chatuinoConfigDir
	}

	return filepath.Join(dir, chatuinoConfigDir)
}

func loadThemeCatalog(fsys afero.Fs, configDir string) (*ThemeCatalog, error) {
	entries := map[string]ThemeEntry{}

	builtin, err := loadBuiltinThemes()
	if err != nil {
		return nil, err
	}

	for _, entry := range builtin {
		entries[entry.ID] = entry
	}

	userEntries, err := loadUserThemes(fsys, configDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range userEntries {
		entries[entry.ID] = entry
	}

	if entry, ok, err := loadLegacyTheme(fsys, configDir); err != nil {
		return nil, err
	} else if ok {
		// A themes/custom.yaml intentionally wins over the legacy file.
		if existing, exists := entries[entry.ID]; !exists || existing.Source == ThemeSourceBuiltin {
			entries[entry.ID] = entry
		}
	}

	result := make([]ThemeEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry)
	}

	sortThemeEntries(result)

	return &ThemeCatalog{entries: result}, nil
}

func loadBuiltinThemes() ([]ThemeEntry, error) {
	dirEntries, err := fs.ReadDir(builtinThemes, themeDirName)
	if err != nil {
		return nil, fmt.Errorf("failed to read built-in themes: %w", err)
	}

	entries := make([]ThemeEntry, 0, len(dirEntries))

	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			continue
		}

		id, ok := ThemeIDFromFileName(dirEntry.Name())
		if !ok {
			continue
		}

		path := themeDirName + "/" + dirEntry.Name()
		data, err := builtinThemes.ReadFile(path)

		theme, parseErr := ParseTheme(data)
		entries = append(entries, ThemeEntry{
			ID:     id,
			Source: ThemeSourceBuiltin,
			Path:   path,
			Theme:  theme,
			Err:    themeError(path, errors.Join(err, parseErr)),
		})
	}

	return entries, nil
}

func loadUserThemes(fsys afero.Fs, configDir string) ([]ThemeEntry, error) {
	dir := filepath.Join(configDir, themeDirName)

	dirEntries, err := afero.ReadDir(fsys, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to read themes directory %s: %w", dir, err)
	}

	entries := make([]ThemeEntry, 0, len(dirEntries))
	byID := map[string]string{} // id -> first file that claimed it

	for _, dirEntry := range dirEntries {
		// Only top-level theme files count. Dotfiles, backups, editor swap files
		// and subdirectories are ignored without comment.
		if dirEntry.IsDir() || strings.HasPrefix(dirEntry.Name(), ".") {
			continue
		}

		id, ok := ThemeIDFromFileName(dirEntry.Name())
		if !ok {
			continue
		}

		path := filepath.Join(dir, dirEntry.Name())

		if first, duplicate := byID[id]; duplicate {
			entries = append(entries, ThemeEntry{
				ID:     id,
				Source: ThemeSourceUser,
				Path:   path,
				Err:    fmt.Errorf("duplicate id %q: %s and %s", id, first, path),
			})

			continue
		}

		byID[id] = path

		data, readErr := afero.ReadFile(fsys, path)

		theme, parseErr := ParseTheme(data)
		entries = append(entries, ThemeEntry{
			ID:     id,
			Source: ThemeSourceUser,
			Path:   path,
			Theme:  theme,
			Err:    themeError(path, errors.Join(readErr, parseErr)),
		})
	}

	return entries, nil
}

func loadLegacyTheme(fsys afero.Fs, configDir string) (ThemeEntry, bool, error) {
	path := filepath.Join(configDir, themeFileName)

	data, err := afero.ReadFile(fsys, path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ThemeEntry{}, false, nil
		}

		return ThemeEntry{}, false, fmt.Errorf("failed to read %s: %w", path, err)
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return ThemeEntry{}, false, nil
	}

	theme, parseErr := ParseTheme(data)
	theme.Name = "custom (legacy)"

	return ThemeEntry{
		ID:     LegacyThemeID,
		Source: ThemeSourceLegacy,
		Path:   path,
		Theme:  theme,
		Err:    themeError(path, parseErr),
	}, true, nil
}

// ThemeIDFromFileName derives a theme id from a file name. It reports false for
// anything that is not a .yaml/.yml file.
func ThemeIDFromFileName(name string) (string, bool) {
	lower := strings.ToLower(name)

	var id string

	switch {
	case strings.HasSuffix(lower, themeExtYAML):
		id = name[:len(name)-len(themeExtYAML)]
	case strings.HasSuffix(lower, themeExtYML):
		id = name[:len(name)-len(themeExtYML)]
	default:
		return "", false
	}

	if id == "" {
		return "", false
	}

	return strings.ToLower(id), true
}

// themeError names the file a parse error came from. The picker already knows
// the path, but a bare yaml error ("field x not found") is useless on its own.
func themeError(path string, err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%s: %w", path, err)
}

func sortThemeEntries(entries []ThemeEntry) {
	group := func(source ThemeSource) int {
		if source == ThemeSourceBuiltin {
			return 0
		}

		return 1
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if gi, gj := group(entries[i].Source), group(entries[j].Source); gi != gj {
			return gi < gj
		}

		return entries[i].ID < entries[j].ID
	})
}

// ThemeManager owns the active theme of the running process. Selecting a theme
// persists the choice to settings.yaml and swaps the in-memory theme.
type ThemeManager struct {
	fs        afero.Fs
	configDir string
	catalog   *ThemeCatalog
	theme     Theme
	activeID  string
	warnings  []string
}

// NewThemeManager loads the theme catalog and resolves requested (the persisted
// or command line theme id) to the theme this process starts with.
func NewThemeManager(fsys afero.Fs, configDir, requested string) (*ThemeManager, error) {
	catalog, err := loadThemeCatalog(fsys, configDir)
	if err != nil {
		return nil, err
	}

	resolution := catalog.Resolve(requested)

	manager := &ThemeManager{
		fs:        fsys,
		configDir: configDir,
		catalog:   catalog,
		theme:     resolution.Theme,
		activeID:  resolution.ID,
	}

	if resolution.Warning != "" {
		manager.warnings = append(manager.warnings, resolution.Warning)
	}

	// Every unusable file is reported, not just the one that happens to be
	// selected: a theme someone dropped in and never saw again is the usual
	// reason a color change "does not work".
	for _, entry := range catalog.Entries() {
		if entry.Err == nil {
			continue
		}

		manager.warnings = append(manager.warnings, fmt.Sprintf("theme %q can't be used (%s)", entry.ID, entry.Err))
	}

	// A legacy theme.yaml next to a saved selection is dead weight; say so once.
	if requested != "" && requested != LegacyThemeID {
		if entry, ok := catalog.Lookup(LegacyThemeID); ok && entry.Source == ThemeSourceLegacy {
			manager.warnings = append(manager.warnings,
				fmt.Sprintf("%s is no longer used, move it to %s to keep it", entry.Path, filepath.Join(configDir, themeDirName, LegacyThemeID+".yaml")))
		}
	}

	return manager, nil
}

// Active returns the theme currently in use.
func (m *ThemeManager) Active() Theme { return m.theme }

// ActiveID returns the id of the theme currently in use.
func (m *ThemeManager) ActiveID() string { return m.activeID }

// Entries returns every known theme in picker order.
func (m *ThemeManager) Entries() []ThemeEntry { return m.catalog.Entries() }

// Lookup returns one known theme by id.
func (m *ThemeManager) Lookup(id string) (ThemeEntry, bool) { return m.catalog.Lookup(id) }

// Warnings returns the problems found while resolving the startup theme.
func (m *ThemeManager) Warnings() []string { return m.warnings }

// SettingsPath returns the settings file a selection is persisted to.
func (m *ThemeManager) SettingsPath() string {
	return filepath.Join(m.configDir, settingsFileName)
}

// Reload re-reads the theme catalog, so a theme file copied into the themes
// directory while chatuino runs shows up the next time the picker opens. The
// active theme is left alone.
func (m *ThemeManager) Reload() error {
	catalog, err := loadThemeCatalog(m.fs, m.configDir)
	if err != nil {
		return err
	}

	m.catalog = catalog

	return nil
}

// Select makes id the active theme and persists the choice. An unknown or
// invalid theme is rejected and leaves the current theme untouched.
func (m *ThemeManager) Select(id string) error {
	entry, ok := m.catalog.Lookup(id)
	if !ok {
		return fmt.Errorf("unknown theme %q", id)
	}

	if entry.Err != nil {
		return fmt.Errorf("theme %q is invalid: %w", id, entry.Err)
	}

	if err := SetThemeSetting(m.fs, m.SettingsPath(), entry.ID); err != nil {
		return err
	}

	m.theme, m.activeID = entry.Theme, entry.ID

	return nil
}
