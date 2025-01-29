package mainui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
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
	return 0
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
	if entry.hasNotification {
		title = fmt.Sprintf("%s%s", bellEmojiPrefix, title)
	}

	diff := m.Width() - lipgloss.Width(title)
	if diff > 0 {
		title += strings.Repeat(" ", diff)
	}

	selected := m.Index() == index
	if selected {
		fmt.Fprint(w, d.tabHeaderActiveStyle.Render(title))
		return
	}

	fmt.Fprint(w, d.tabHeaderStyle.Render(title))
}

func newVerticalTabHeader(width, height int, userConfiguration UserConfiguration) *verticalTabHeader {
	delegate := verticalTabDelegate{
		tabHeaderStyle:       lipgloss.NewStyle().Background(lipgloss.Color(userConfiguration.Theme.TabHeaderBackgroundColor)),
		tabHeaderActiveStyle: lipgloss.NewStyle().Background(lipgloss.Color(userConfiguration.Theme.TabHeaderActiveBackgroundColor)),
	}

	l := list.New(nil, delegate, width, height)
	l.SetShowPagination(false)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetWidth(width)
	l.SetHeight(height)
	l.SetDelegate(delegate)
	l.Title = ""
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

func (v *verticalTabHeader) AddTab(channel, identity string) (string, tea.Cmd) {
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

func (v *verticalTabHeader) SelectTab(id string) {
	for i, item := range v.list.Items() {
		e := item.(tabHeaderEntry)
		if e.id == id {
			// reset notification flag on select
			if e.hasNotification {
				e.hasNotification = false
				v.list.SetItem(i, e)
			}

			v.list.Select(i)
		}
	}
}

func (v *verticalTabHeader) RemoveTab(id string) {
	for i, item := range v.list.Items() {
		if item != nil && item.(tabHeaderEntry).id == id {
			v.list.RemoveItem(i)
		}
	}
}

func (v *verticalTabHeader) MinWidth() int {
	minWidth := 10

	for _, e := range v.list.Items() {
		e := e.(tabHeaderEntry)
		e.hasNotification = true
		r := e.render()
		if lipgloss.Width(r) > minWidth {
			minWidth = lipgloss.Width(r)
		}
	}

	return minWidth
}

func (v *verticalTabHeader) Resize(width, height int) {
	v.width = width
	v.height = height
	v.list.SetWidth(width)
	v.list.SetHeight(height)
}

func (v *verticalTabHeader) Init() tea.Cmd {
	return nil
}

func (v *verticalTabHeader) Update(msg tea.Msg) (header, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	v.list, cmd = v.list.Update(msg)
	cmds = append(cmds, cmd)

	if req, ok := msg.(requestNotificationIconMessage); ok {
		log.Logger.Info().Str("id", req.tabID).Msg("got noti request")
		for i, e := range v.list.Items() {
			e := e.(tabHeaderEntry)
			// add bell prefix if tab id matched, and tab is not already active
			if e.id == req.tabID && v.list.Index() != i {
				e.hasNotification = true
				v.list.SetItem(i, e)
			}
		}
	}

	return v, tea.Batch(cmds...)
}

func (v *verticalTabHeader) View() string {
	view := v.list.View()
	if idx := strings.Index(view, "\n"); idx != -1 {
		view = view[idx+1:]
	}
	return view
}
