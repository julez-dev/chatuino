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
	width  int
	height int
	deps   *DependencyContainer

	list     list.Model
	delegate verticalTabDelegate
}

type verticalTabDelegate struct {
	deps *DependencyContainer
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

	bulletStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(d.deps.UserConfig.Theme.InputPromptColor))
	activeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(d.deps.UserConfig.Theme.InputPromptColor))
	notificationStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(d.deps.UserConfig.Theme.ChatNoticeAlertColor))

	selected := m.Index() == index

	var line strings.Builder

	// Bullet for selected tab
	if selected {
		line.WriteString(bulletStyle.Render("▸ "))
	} else {
		line.WriteString("  ")
	}

	// Tab content
	content := entry.render()
	if selected {
		line.WriteString(activeStyle.Render(content))
	} else if entry.hasNotification {
		line.WriteString(notificationStyle.Render(content))
	} else {
		line.WriteString(content)
	}

	// Pad to width
	currentWidth := lipgloss.Width(line.String())
	if diff := m.Width() - currentWidth; diff > 0 {
		line.WriteString(strings.Repeat(" ", diff))
	}

	fmt.Fprint(w, line.String())
}

func newVerticalTabHeader(width, height int, deps *DependencyContainer) *verticalTabHeader {
	delegate := verticalTabDelegate{
		deps: deps,
	}

	// Adjust dimensions for border: -2 width for left/right │, -2 height for top/bottom borders
	listWidth := max(1, width-2)
	listHeight := max(1, height-2)

	l := list.New(nil, delegate, listWidth, listHeight)
	l.SetShowPagination(false)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetWidth(listWidth)
	l.SetHeight(listHeight)
	l.SetDelegate(delegate)
	l.Title = ""
	l.InfiniteScrolling = true

	l.KeyMap = list.KeyMap{}

	return &verticalTabHeader{
		width:    width,
		height:   height,
		deps:     deps,
		list:     l,
		delegate: delegate,
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

	// Add 4 for left/right border characters (2) + bullet prefix (2)
	return minWidth + 4
}

func (v *verticalTabHeader) Resize(width, height int) {
	v.width = width
	v.height = height
	// Adjust for border: -2 width for left/right │, -2 height for top/bottom borders
	v.list.SetWidth(max(1, width-2))
	v.list.SetHeight(max(1, height-2))
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
	borderColor := lipgloss.Color(v.deps.UserConfig.Theme.InputPromptColor)
	borderStyle := lipgloss.NewStyle().Foreground(borderColor)

	innerWidth := v.width - 2   // -2 for left/right │
	innerHeight := v.height - 2 // -2 for top/bottom borders

	// Get list content
	view := v.list.View()
	if idx := strings.Index(view, "\n"); idx != -1 {
		view = view[idx+1:]
	}

	// Check if we need scroll indicators
	totalItems := len(v.list.Items())
	visibleItems := innerHeight
	currentIndex := v.list.Index()

	showUpArrow := currentIndex > 0 && totalItems > visibleItems
	showDownArrow := currentIndex < totalItems-1 && totalItems > visibleItems

	// Build top label with optional scroll indicator
	topLabel := "[ Channels ]"
	if totalItems > visibleItems {
		// Show position indicator
		topLabel = fmt.Sprintf("[ Channels %d/%d ]", currentIndex+1, totalItems)
	}

	// Top border: ┌─[ Channels ]───...─┐ or with ▲
	topFill := innerWidth - lipgloss.Width(topLabel) - 2
	if showUpArrow {
		topFill -= 2 // space for " ▲"
	}
	if topFill < 0 {
		topFill = 0
	}
	topBorder := "┌─" + topLabel + strings.Repeat("─", topFill)
	if showUpArrow {
		topBorder += " ▲"
	}
	topBorder += "─┐"

	// Bottom border: └───...───┘ or with ▼
	bottomFill := innerWidth
	if showDownArrow {
		bottomFill -= 2 // space for "▼ "
	}
	if bottomFill < 0 {
		bottomFill = 0
	}
	bottomBorder := "└"
	if showDownArrow {
		bottomBorder += "▼ "
	}
	bottomBorder += strings.Repeat("─", bottomFill) + "┘"

	// Wrap each line with │ borders
	lines := strings.Split(view, "\n")
	var borderedLines []string
	for _, line := range lines {
		padNeeded := max(0, innerWidth-lipgloss.Width(line))
		borderedLines = append(borderedLines, borderStyle.Render("│")+line+strings.Repeat(" ", padNeeded)+borderStyle.Render("│"))
	}

	result := borderStyle.Render(topBorder) + "\n"
	result += strings.Join(borderedLines, "\n") + "\n"
	result += borderStyle.Render(bottomBorder)

	return result
}
