package mainui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/tab"
)

// themePreviewMessage applies a theme without saving it, while the user moves
// the cursor through the theme picker.
type themePreviewMessage struct {
	id string
}

// themeChoiceMessage commits the theme the user confirmed in the picker.
type themeChoiceMessage struct {
	id string
}

// themeRejectedMessage reports why a theme could not be selected.
type themeRejectedMessage struct {
	message string
}

// applyTheme repaints the whole UI with theme. Tabs are updated in place, so
// drafts, scroll positions, focus and layout survive the switch.
func (r *Root) applyTheme(theme save.Theme, id string) tea.Cmd {
	r.theme = theme
	r.themeID = id
	r.splash.theme = theme
	r.header.SetTheme(theme)

	return r.tabs.UpdateAll(tab.ThemeChangedMessage{Theme: theme}, updateTab)
}

// previewTheme switches to id without persisting anything.
func (r *Root) previewTheme(id string) tea.Cmd {
	entry, ok := r.themes.Lookup(id)
	if !ok || !entry.Selectable() {
		return nil
	}

	return r.applyTheme(entry.Theme, entry.ID)
}

// revertThemePreview restores the saved theme. Cancelling the picker must never
// leave a preview behind.
func (r *Root) revertThemePreview() tea.Cmd {
	return r.applyTheme(r.themes.Active(), r.themes.ActiveID())
}

// commitTheme persists the selection and keeps the already previewed colors.
func (r *Root) commitTheme(id string) tea.Cmd {
	if err := r.themes.Select(id); err != nil {
		r.setNotice(fmt.Sprintf("could not save theme: %s", err))

		return r.revertThemePreview()
	}

	cmds := []tea.Cmd{r.applyTheme(r.themes.Active(), r.themes.ActiveID())}

	if r.stack.Route() == themePickerRoute {
		cmds = append(cmds, r.stack.Pop())

		if r.stack.Len() == 0 {
			r.tabs.FocusActive()
		}

		r.handleResize()
	}

	r.setNotice(fmt.Sprintf("theme set to %s", r.themes.ActiveID()))

	return tea.Batch(cmds...)
}

// openThemePicker reloads the catalog, so a theme file dropped into the themes
// directory shows up without a restart, and lists every known theme.
func (r *Root) openThemePicker() tea.Cmd {
	if err := r.themes.Reload(); err != nil {
		r.setNotice(fmt.Sprintf("could not read themes: %s", err))
	}

	entries := r.themes.Entries()
	if len(entries) == 0 {
		r.setNotice("no themes found")

		return nil
	}

	r.tabs.BlurActive()

	picker := newThemePicker(r.width, r.height, r.theme, r.keymap, entries, r.themeID)
	picker.Resize(r.width, r.height)

	return r.stack.Push(picker)
}

func (r *Root) setNotice(message string) {
	r.notice = message
}

// noticeLine renders the one-line notice, clipped to the terminal width.
func (r *Root) noticeLine() string {
	if r.notice == "" || r.width <= 0 {
		return ""
	}

	text := r.notice
	if runes := []rune(text); len(runes) > r.width {
		text = string(runes[:r.width])
	}

	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(r.theme.ListBackgroundColor)).
		Background(lipgloss.Color(r.theme.ChatNoticeAlertColor)).
		Render(text)
}

// withNotice replaces the bottom screen row with the notice, so nothing shifts
// and the message cannot push the layout past the terminal height.
func (r *Root) withNotice(view string) string {
	line := r.noticeLine()
	if line == "" || view == "" {
		return view
	}

	lines := strings.Split(view, "\n")
	lines[len(lines)-1] = line

	return strings.Join(lines, "\n")
}
