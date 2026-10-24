package mainui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/nav"
)

type paneItem struct{ id, label string }

func (i paneItem) Title() string       { return i.label }
func (i paneItem) Description() string { return "" }
func (i paneItem) FilterValue() string { return i.label }

// panePicker owns only a modal selection. No pane exists until Root accepts its
// ticketed result, so cancellation does not require undoing a split.
type panePicker struct {
	list   list.Model
	keymap save.KeyMap
	ticket uint64
	width  int
	height int
}

func newPanePicker(width, height int, theme save.Theme, keymap save.KeyMap, ticket uint64, rows []nav.Row) *panePicker {
	items := make([]list.Item, 0, len(rows)+1)
	for _, row := range rows {
		items = append(items, paneItem{id: row.ID, label: rowLabel(row)})
	}
	items = append(items, paneItem{label: "Open a new tab…"})
	model := createDefaultList(height, theme.ListSelectedColor)
	model.SetItems(items)
	model.SetFilteringEnabled(false)
	model.KeyMap.CursorUp = keymap.Up
	model.KeyMap.CursorDown = keymap.Down
	p := &panePicker{list: model, ticket: ticket, keymap: keymap}
	p.Resize(width, height)
	return p
}

func (p *panePicker) Init() tea.Cmd { return nil }

func (p *panePicker) Update(msg tea.Msg) (nav.Screen, tea.Cmd) {
	if pressed, ok := msg.(tea.KeyPressMsg); ok && key.Matches(pressed, p.keymap.Confirm) {
		if item, ok := p.list.SelectedItem().(paneItem); ok {
			choice := paneChoiceMessage{ticket: p.ticket, tabID: item.id}
			return p, func() tea.Msg { return choice }
		}
	}
	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return p, cmd
}

func (p *panePicker) View() string {
	content := "Choose a tab for the new pane\n\n" + p.list.View() + "\n" +
		p.keymap.Confirm.Help().Key + ": select • " + p.keymap.Escape.Help().Key + ": cancel"
	return cellBlock(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Render(content), p.width, p.height)
}

func (p *panePicker) Route() string            { return panePickerRoute }
func (p *panePicker) OverlaysBackground() bool { return true }
func (p *panePicker) Resize(width, height int) {
	p.width, p.height = min(64, max(0, width)), min(18, max(0, height))
	p.list.SetSize(max(0, p.width-4), max(0, p.height-6))
}
