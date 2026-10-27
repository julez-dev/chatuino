package tab

import (
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
)

// ThemeChangedMessage tells a tab that the user selected a different theme.
//
// Tabs are not rebuilt on a theme switch: they re-derive the styles they cached
// at construction and re-render their transcripts. Everything else — drafts,
// scroll positions, filters, focus, in-flight requests — stays untouched, so a
// theme switch is invisible apart from the colors.
type ThemeChangedMessage struct {
	Theme save.Theme
}

// SetTheme applies a new theme to the tab and everything below it.
func (t *Broadcast) SetTheme(theme save.Theme) {
	t.theme = theme

	if t.chatWindow != nil {
		t.chatWindow.SetTheme(theme)
	}

	if t.input != nil {
		t.input.SetTheme(theme)
	}

	if t.statusInfo != nil {
		t.statusInfo.SetTheme(theme)
	}

	if t.userInspect != nil {
		t.userInspect.SetTheme(theme)
	}

	if t.unbanRequests != nil {
		t.unbanRequests.SetTheme(theme)
	}
}

// SetTheme applies a new theme to the mention tab.
func (m *mentionTab) SetTheme(theme save.Theme) {
	m.theme = theme

	if m.chatWindow != nil {
		m.chatWindow.SetTheme(theme)
	}
}

// SetTheme applies a new theme to the live notification tab.
func (l *liveNotificationTab) SetTheme(theme save.Theme) {
	l.theme = theme

	if l.chatWindow != nil {
		l.chatWindow.SetTheme(theme)
	}
}

// SetTheme repaints the transcript and the search prompt.
func (c *chatWindow) SetTheme(theme save.Theme) {
	c.Buffer.SetTheme(theme)

	styles := c.searchInput.Styles()
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.InputPromptColor))
	c.searchInput.SetStyles(styles)
}

// SetTheme repaints the composer frame and keeps the text input prompt in sync.
func (c *composer) SetTheme(theme save.Theme) {
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.InputPromptColor))

	styles := c.input.InputModel.Styles()
	styles.Focused.Prompt = accent
	c.input.InputModel.SetStyles(styles)

	c.borderStyle = accent
	c.accentStyle = accent
	c.dimmedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.DimmedTextColor))
}

// SetTheme repaints the status bar highlights.
func (s *streamStatus) SetTheme(theme save.Theme) {
	s.statusHighlight = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.StatusColor))
	s.updateHighlight = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.SplashHighlightColor))
}

// SetTheme repaints the inspect window and its history.
func (u *userInspect) SetTheme(theme save.Theme) {
	u.theme = theme

	if u.chatWindow != nil {
		u.chatWindow.SetTheme(theme)
	}
}

// SetTheme repaints the unban request queue. Its styles are derived at render
// time, so the theme field is all it needs.
func (q *unbanRequestQueue) SetTheme(theme save.Theme) {
	q.theme = theme
}
