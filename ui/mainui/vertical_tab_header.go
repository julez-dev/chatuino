package mainui

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/nav"
)

type verticalTabHeader struct {
	width  int
	height int

	borderStyle lipgloss.Style // pre-created for View() border rendering

	list     list.Model
	delegate verticalTabDelegate

	// lastRows is the projection currently pushed into the list. The list is only
	// repopulated when the projection actually changes, so the scroll position and
	// pagination stay stable between frames.
	lastRows []nav.Row
}

type verticalTabDelegate struct {

	// pre-created styles to avoid allocations in Render() (called per visible item per frame)
	bulletStyle       lipgloss.Style
	activeStyle       lipgloss.Style
	notificationStyle lipgloss.Style
}

func (d verticalTabDelegate) Height() int {
	return 1
}

func (d verticalTabDelegate) Spacing() int {
	return 0
}

func (d verticalTabDelegate) Update(tea.Msg, *list.Model) tea.Cmd {
	return nil
}

func (d verticalTabDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(rowItem)
	if !ok {
		return
	}

	selected := m.Index() == index

	var line strings.Builder

	// Bullet for selected tab
	if selected {
		line.WriteString(d.bulletStyle.Render("▸ "))
	} else {
		line.WriteString("  ")
	}

	// Tab content
	content := rowLabel(entry.Row)
	if selected {
		line.WriteString(d.activeStyle.Render(content))
	} else if entry.HasNotification {
		line.WriteString(d.notificationStyle.Render(content))
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

func newVerticalTabHeader(width, height int, theme save.Theme) *verticalTabHeader {
	delegate := verticalTabDelegate{
		bulletStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color(theme.InputPromptColor)),
		activeStyle:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.InputPromptColor)),
		notificationStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ChatNoticeAlertColor)),
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
		width:       width,
		height:      height,
		borderStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.InputPromptColor)),
		list:        l,
		delegate:    delegate,
	}
}

// syncList pushes the registry projection into the bubbles list and selects the
// active row. It is a no-op when the projection did not change.
func (v *verticalTabHeader) syncList(rows []nav.Row) {
	if len(v.lastRows) == len(rows) && slices.Equal(v.lastRows, rows) {
		return
	}

	v.lastRows = slices.Clone(rows)

	items := make([]list.Item, len(rows))
	activeIndex := 0

	for i, row := range rows {
		items[i] = rowItem{Row: row}

		if row.Active {
			activeIndex = i
		}
	}

	v.list.SetItems(items)
	v.list.Select(activeIndex)
}

func (v *verticalTabHeader) View(rows []nav.Row) string {
	v.syncList(rows)

	borderStyle := v.borderStyle

	innerWidth := v.width - 2   // -2 for left/right │
	innerHeight := v.height - 2 // -2 for top/bottom borders

	// Get list content
	view := v.list.View()
	if idx := strings.Index(view, "\n"); idx != -1 {
		view = view[idx+1:]
	}

	// Check if we need scroll indicators
	totalItems := len(rows)
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

// TabAt returns the tab rendered at the given cell, if any.
func (v *verticalTabHeader) TabAt(rows []nav.Row, x, y int) (string, bool) {
	if x < 0 || x >= v.width || y < 1 || len(rows) == 0 {
		return "", false
	}

	// The list renders at most PerPage item rows below the top border.
	perPage := v.list.Paginator.PerPage
	if perPage <= 0 {
		perPage = max(1, v.height-2)
	}

	if y > perPage {
		return "", false
	}

	idx := v.list.Paginator.Page*perPage + (y - 1)
	if idx < 0 || idx >= len(rows) {
		return "", false
	}

	return rows[idx].ID, true
}

func (v *verticalTabHeader) MinWidth(rows []nav.Row) int {
	minWidth := 10

	for _, row := range rows {
		// measure with the notification marker so the width does not jump when one arrives
		row.HasNotification = true
		if w := lipgloss.Width(rowLabel(row)); w > minWidth {
			minWidth = w
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
