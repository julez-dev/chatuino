package mainui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/nav"
)

const themePickerRoute = "theme-picker"

// themeItem is one row of the theme picker. Broken themes are listed but cannot
// be selected, so the user can see what is wrong with a file they dropped in the
// themes directory instead of wondering where it went.
type themeItem struct {
	entry  save.ThemeEntry
	active bool
}

func (i themeItem) Title() string {
	if i.active {
		return i.entry.Name() + " ✓"
	}

	return i.entry.Name()
}

func (i themeItem) Description() string {
	description := i.entry.Source.String()
	if i.entry.Err != nil {
		description = "⚠ " + i.entry.Err.Error()
	}

	return description
}

func (i themeItem) FilterValue() string { return i.entry.Name() + " " + i.entry.ID }

// themePicker is the modal that lists every known theme. It only selects: the
// live preview and the persistence are Root's decision, so cancelling a preview
// never has to undo anything inside the picker.
type themePicker struct {
	list     list.Model
	keymap   save.KeyMap
	entries  []save.ThemeEntry
	activeID string

	// previewed is the id currently shown in the UI. It differs from activeID
	// while the cursor moves, and Root reverts to it on cancel.
	previewed string

	width, height int
}

func newThemePicker(width, height int, theme save.Theme, keymap save.KeyMap, entries []save.ThemeEntry, activeID string) *themePicker {
	model := createDefaultList(height, theme.ListSelectedColor)
	model.SetFilteringEnabled(true)
	model.SetShowTitle(false)
	model.SetShowStatusBar(false)
	model.KeyMap.CursorUp = keymap.Up
	model.KeyMap.CursorDown = keymap.Down

	// Rows carry a description: where the theme comes from, or why it could not
	// be loaded. The default list delegate hides descriptions.
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetSpacing(0)
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color(theme.ListSelectedColor)).Bold(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedTitle
	delegate.Styles.NormalDesc = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.DimmedTextColor))
	model.SetDelegate(delegate)

	p := &themePicker{
		list:     model,
		keymap:   keymap,
		entries:  entries,
		activeID: activeID,
		width:    width,
		height:   height,
	}

	p.setItems()
	p.Resize(width, height)

	if index := p.indexOf(activeID); index >= 0 {
		p.list.Select(index)
	}

	p.previewed = activeID

	return p
}

func (p *themePicker) setItems() {
	items := make([]list.Item, 0, len(p.entries))

	for _, entry := range p.entries {
		items = append(items, themeItem{entry: entry, active: entry.ID == p.activeID})
	}

	p.list.SetItems(items)
}

func (p *themePicker) indexOf(id string) int {
	for i, entry := range p.entries {
		if entry.ID == id {
			return i
		}
	}

	return -1
}

func (p *themePicker) selected() (save.ThemeEntry, bool) {
	item, ok := p.list.SelectedItem().(themeItem)
	if !ok {
		return save.ThemeEntry{}, false
	}

	return item.entry, true
}

func (p *themePicker) Init() tea.Cmd { return nil }

// Appeared previews the theme under the cursor as soon as the picker opens.
func (p *themePicker) Appeared() tea.Cmd {
	return p.previewCmd()
}

func (p *themePicker) Disappeared() {}

func (p *themePicker) previewCmd() tea.Cmd {
	entry, ok := p.selected()
	if !ok || entry.ID == p.previewed {
		return nil
	}

	p.previewed = entry.ID

	return func() tea.Msg { return themePreviewMessage{id: entry.ID} }
}

func (p *themePicker) Update(msg tea.Msg) (nav.Screen, tea.Cmd) {
	if pressed, ok := msg.(tea.KeyPressMsg); ok {
		// While the filter is being edited, enter applies the filter instead of
		// committing a theme; escape clears the filter (see BlocksEscape).
		if !p.list.SettingFilter() {
			if key.Matches(pressed, p.keymap.Confirm) {
				if entry, ok := p.selected(); ok && entry.Selectable() {
					return p, func() tea.Msg { return themeChoiceMessage{id: entry.ID} }
				}

				return p, func() tea.Msg {
					return themeRejectedMessage{message: "this theme file is broken and can't be selected"}
				}
			}

			switch {
			case key.Matches(pressed, p.keymap.Up):
				p.list.CursorUp()

				return p, p.previewCmd()
			case key.Matches(pressed, p.keymap.Down):
				p.list.CursorDown()

				return p, p.previewCmd()
			}

			// Type-to-filter: the first printable key starts the filter and is
			// consumed by it, so no key has to be learned to search the list.
			if _, ok := printableText(pressed); ok {
				p.list.SetFilterState(list.Filtering)
			}
		}
	}

	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)

	// The filter can move the selection (accepting or clearing it), so preview
	// whatever the list now points at.
	return p, tea.Batch(cmd, p.previewCmd())
}

// BlocksEscape keeps the shell from closing the picker while the filter input
// is capturing keys: the first escape clears the filter, the second one cancels.
func (p *themePicker) BlocksEscape() bool {
	return p.list.SettingFilter()
}

// printableText returns the text of a key press that should be typed into the
// filter, i.e. a printable rune without modifiers.
func printableText(msg tea.KeyPressMsg) (string, bool) {
	if msg.Text == "" || msg.Mod.Contains(tea.ModCtrl) || msg.Mod.Contains(tea.ModAlt) {
		return "", false
	}

	return msg.Text, true
}

func (p *themePicker) View() string {
	title := lipgloss.NewStyle().Bold(true).Render("Themes")

	hints := strings.Join([]string{
		p.keymap.Up.Help().Key + "/" + p.keymap.Down.Help().Key + ": preview",
		p.keymap.Confirm.Help().Key + ": save",
		p.keymap.Escape.Help().Key + ": cancel",
		"type: filter",
	}, " • ")

	content := title + "\n\n" + p.list.View() + "\n" + hints
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Render(content)

	width, height := min(p.width, lipgloss.Width(box)), min(p.height, lipgloss.Height(box))

	return strings.Join(blockLines(box, width, height), "\n")
}

func (p *themePicker) Route() string            { return themePickerRoute }
func (p *themePicker) OverlaysBackground() bool { return true }

func (p *themePicker) Resize(width, height int) {
	p.width, p.height = min(72, max(0, width)), min(20, max(0, height))
	p.list.SetSize(max(0, p.width-4), max(0, p.height-8))
}
