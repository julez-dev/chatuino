package mainui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
)

type verticalTabHeader struct {
	width             int
	height            int
	userConfiguration UserConfiguration

	list     list.Model
	delegate verticalTabDelegate
}

type verticalTabDelegate struct {
	tabHeaderStyle       lipgloss.Style
	tabHeaderActiveStyle lipgloss.Style
}

func (d verticalTabDelegate) Height() int {
	return 1
}

func (d verticalTabDelegate) Spacing() int {
	return 1
}

func (d verticalTabDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d verticalTabDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(tabHeaderEntry)
	if !ok {
		return
	}

	var (
		name     = entry.name
		identity = entry.identity
	)

	title := fmt.Sprintf("%s (%s)", name, identity)

	diff := m.Width() - lipgloss.Width(title)
	if diff > 0 {
		title += strings.Repeat(" ", diff)
	}

	selected := m.Index() == index
	if selected {
		fmt.Fprintf(w, d.tabHeaderActiveStyle.Render(title))
		return
	}

	fmt.Fprintf(w, d.tabHeaderStyle.Render(title))
}

func newVerticalTabHeader(width, height int, userConfiguration UserConfiguration) *verticalTabHeader {
	delegate := verticalTabDelegate{
		tabHeaderStyle:       lipgloss.NewStyle().Background(lipgloss.Color(userConfiguration.Theme.TabHeaderBackgroundColor)),
		tabHeaderActiveStyle: lipgloss.NewStyle().Background(lipgloss.Color(userConfiguration.Theme.TabHeaderActiveBackgroundColor)),
	}

	l := createDefaultList(0, "#FFFFFF")
	l.SetShowPagination(false)
	l.SetShowTitle(false)
	l.SetWidth(width)
	l.SetHeight(height)
	l.SetDelegate(delegate)
	l.InfiniteScrolling = true

	l.KeyMap = list.KeyMap{}

	return &verticalTabHeader{
		width:             width,
		height:            height,
		list:              l,
		userConfiguration: userConfiguration,
		delegate:          delegate,
	}
}

func (v *verticalTabHeader) addTab(channel, identity string) (string, tea.Cmd) {
	entry := tabHeaderEntry{
		id:       uuid.New().String(),
		name:     channel,
		identity: identity,
	}

	// append new entry to the end of list
	i := len(v.list.Items())
	cmd := v.list.InsertItem(i, entry)

	return entry.id, cmd
}

func (v *verticalTabHeader) selectTab(id string) {
	for i, item := range v.list.Items() {
		if item.(tabHeaderEntry).id == id {
			v.list.Select(i)
		}
	}
}

func (v *verticalTabHeader) removeTab(id string) {
	for i, item := range v.list.Items() {
		if item.(tabHeaderEntry).id == id {
			v.list.RemoveItem(i)
		}
	}
}

func (v *verticalTabHeader) minWidth() int {
	minWidth := 10

	for i, e := range v.list.Items() {
		out := strings.Builder{}

		v.delegate.Render(&out, v.list, i, e)

		if lipgloss.Width(out.String()) > minWidth {
			minWidth = lipgloss.Width(out.String())
		}
	}

	return minWidth
}

func (v *verticalTabHeader) resize(width, height int) {
	v.width = width
	v.height = height
	v.list.SetWidth(width)
	v.list.SetHeight(height)
}

func (v *verticalTabHeader) Init() tea.Cmd {
	return nil
}

func (v *verticalTabHeader) Update(msg tea.Msg) (*verticalTabHeader, tea.Cmd) {
	var cmd tea.Cmd
	v.list, cmd = v.list.Update(msg)
	return v, cmd
}

func (v *verticalTabHeader) View() string {
	return v.list.View()
}
