package mainui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/google/uuid"
)

// tabHitSpan is a clickable column range on the tab header content row.
type tabHitSpan struct {
	start, end int // [start, end) columns, including the leading border column
	id         string
}

type horizontalTabHeader struct {
	width    int
	entries  []tabHeaderEntry
	deps     *DependencyContainer
	hitSpans []tabHitSpan // rebuilt on render and used for mouse hit-testing

	// pre-created styles to avoid allocations in View()/renderTabContent() (called every frame)
	borderStyle       lipgloss.Style
	bulletStyle       lipgloss.Style
	separatorStyle    lipgloss.Style
	activeStyle       lipgloss.Style
	notificationStyle lipgloss.Style
}

func newHorizontalTabHeader(width int, deps *DependencyContainer) *horizontalTabHeader {
	return &horizontalTabHeader{
		width:             width,
		deps:              deps,
		borderStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.InputPromptColor)),
		bulletStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.InputPromptColor)),
		separatorStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.DimmedTextColor)),
		activeStyle:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(deps.UserConfig.Theme.InputPromptColor)),
		notificationStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.ChatNoticeAlertColor)),
	}
}

func (h *horizontalTabHeader) Init() tea.Cmd {
	return nil
}

func (h *horizontalTabHeader) Update(msg tea.Msg) (header, tea.Cmd) {
	if req, ok := msg.(requestNotificationIconMessage); ok {
		for i, e := range h.entries {
			if e.id == req.tabID && !e.selected {
				h.entries[i].hasNotification = true
				break
			}
		}
	}

	return h, nil
}

func (h *horizontalTabHeader) View() string {
	if len(h.entries) == 0 {
		return ""
	}

	borderStyle := h.borderStyle

	// Calculate pages with variable width
	pages := h.calculatePages()
	currentPage, totalPages := h.findCurrentPage(pages)

	// Top label with page indicator
	topLabel := "[ Tabs ]"
	if totalPages > 1 {
		topLabel = fmt.Sprintf("[ Tabs %d/%d ]", currentPage+1, totalPages)
	}

	innerWidth := h.width - 2 // -2 for left/right │

	// Build borders - total width should be h.width
	// Top: ┌─ + label + fill + ─┐ = 2 + label + fill + 2 = innerWidth + 2 = h.width
	topFill := innerWidth - lipgloss.Width(topLabel) - 2 // -2 for "─" before and after label
	if topFill < 0 {
		topFill = 0
	}
	topBorder := "┌─" + topLabel + strings.Repeat("─", topFill) + "─┐"

	// Bottom: └ + fill + ┘ = 1 + fill + 1, so fill = innerWidth
	bottomBorder := "└" + strings.Repeat("─", innerWidth) + "┘"

	h.hitSpans = h.buildHitSpans(pages, currentPage, totalPages)

	// Build content with tabs, separators, arrows
	content := h.renderTabContent(pages, currentPage, totalPages)

	// Pad and wrap with │ - content row should match border width
	padNeeded := innerWidth - lipgloss.Width(content)
	if padNeeded < 0 {
		padNeeded = 0
	}
	// Only apply borderStyle to the │ characters, not the content
	contentRow := borderStyle.Render("│") + content + strings.Repeat(" ", padNeeded) + borderStyle.Render("│")

	return borderStyle.Render(topBorder) + "\n" +
		contentRow + "\n" +
		borderStyle.Render(bottomBorder)
}

// calculatePages groups entries into pages based on variable width
func (h *horizontalTabHeader) calculatePages() [][]int {
	if len(h.entries) == 0 {
		return nil
	}

	// Available width for tab content:
	// total - 2 (border │) - 2 (padding) - 4 (arrows "< " and " >")
	availableWidth := h.width - 8
	if availableWidth < 10 {
		availableWidth = 10
	}

	var pages [][]int
	var currentPage []int
	currentWidth := 0

	for i, entry := range h.entries {
		// Width = bullet(2 "▸ ") + content + separator(3 " │ ")
		// For first item on page, no leading separator needed
		entryWidth := 2 + lipgloss.Width(entry.render())
		if len(currentPage) > 0 {
			entryWidth += 3 // add separator width
		}

		if currentWidth+entryWidth > availableWidth && len(currentPage) > 0 {
			// Start new page
			pages = append(pages, currentPage)
			currentPage = []int{i}
			currentWidth = 2 + lipgloss.Width(entry.render()) // reset without separator
		} else {
			currentPage = append(currentPage, i)
			currentWidth += entryWidth
		}
	}

	if len(currentPage) > 0 {
		pages = append(pages, currentPage)
	}

	return pages
}

// findCurrentPage returns the page index containing the selected entry
func (h *horizontalTabHeader) findCurrentPage(pages [][]int) (pageIndex, totalPages int) {
	totalPages = len(pages)
	if totalPages == 0 {
		return 0, 0
	}

	// Find selected entry index
	selectedIndex := -1
	for i, e := range h.entries {
		if e.selected {
			selectedIndex = i
			break
		}
	}

	if selectedIndex == -1 {
		return 0, totalPages
	}

	// Find which page contains the selected entry
	for pi, page := range pages {
		for _, idx := range page {
			if idx == selectedIndex {
				return pi, totalPages
			}
		}
	}

	return 0, totalPages
}

// renderTabContent renders the tab row with bullets, separators, and arrows
func (h *horizontalTabHeader) renderTabContent(pages [][]int, currentPage, totalPages int) string {
	if len(pages) == 0 || currentPage >= len(pages) {
		return ""
	}

	bulletStyle := h.bulletStyle
	separatorStyle := h.separatorStyle
	activeStyle := h.activeStyle
	notificationStyle := h.notificationStyle

	var b strings.Builder

	// Left arrow (if not on first page)
	showPrev := currentPage > 0
	showNext := currentPage < totalPages-1

	if showPrev {
		// Check for notifications on previous pages
		hasNotification := h.hasNotificationInRange(0, pages[currentPage][0])
		if hasNotification {
			b.WriteString(notificationStyle.Render("<"))
		} else {
			b.WriteString("<")
		}
		b.WriteString(" ")
	} else {
		b.WriteString("  ") // padding for alignment
	}

	// Render tabs on current page
	pageEntries := pages[currentPage]
	for i, entryIdx := range pageEntries {
		entry := h.entries[entryIdx]

		// Add separator before (except first)
		if i > 0 {
			b.WriteString(separatorStyle.Render(" │ "))
		}

		// Bullet for selected tab
		if entry.selected {
			b.WriteString(bulletStyle.Render("▸ "))
			b.WriteString(activeStyle.Render(entry.render()))
		} else {
			b.WriteString("  ") // spacing to align with bullet
			b.WriteString(entry.render())
		}
	}

	// Right arrow (if not on last page)
	if showNext {
		b.WriteString(" ")
		// Check for notifications on next pages
		lastOnPage := pages[currentPage][len(pages[currentPage])-1]
		hasNotification := h.hasNotificationInRange(lastOnPage+1, len(h.entries))
		if hasNotification {
			b.WriteString(notificationStyle.Render(">"))
		} else {
			b.WriteString(">")
		}
	}

	return b.String()
}

// buildHitSpans mirrors renderTabContent's layout and records the column range
// of every clickable tab, plus the previous/next page arrows.
func (h *horizontalTabHeader) buildHitSpans(pages [][]int, currentPage, totalPages int) []tabHitSpan {
	if len(pages) == 0 || currentPage >= len(pages) {
		return nil
	}

	showPrev := currentPage > 0
	showNext := currentPage < totalPages-1

	spans := make([]tabHitSpan, 0, len(pages[currentPage])+2)

	// x = left border (1) + left arrow/padding (2)
	x := 1 + 2

	if showPrev {
		if prevPage := pages[currentPage-1]; len(prevPage) > 0 {
			spans = append(spans, tabHitSpan{start: x - 2, end: x, id: h.entries[prevPage[0]].id})
		}
	}

	for i, entryIdx := range pages[currentPage] {
		if i > 0 {
			x += 3 // separator " │ "
		}

		start := x
		x += 2 + lipgloss.Width(h.entries[entryIdx].render()) // bullet + label
		spans = append(spans, tabHitSpan{start: start, end: x, id: h.entries[entryIdx].id})
	}

	if showNext {
		if nextPage := pages[currentPage+1]; len(nextPage) > 0 {
			spans = append(spans, tabHitSpan{start: x, end: x + 2, id: h.entries[nextPage[0]].id})
		}
	}

	return spans
}

// TabAt returns the tab at the given cell. All three header rows are clickable
// so users do not have to hit the label row exactly.
func (h *horizontalTabHeader) TabAt(x, y int) (string, bool) {
	if y < 0 || y > 2 {
		return "", false
	}

	if h.hitSpans == nil {
		pages := h.calculatePages()
		currentPage, totalPages := h.findCurrentPage(pages)
		h.hitSpans = h.buildHitSpans(pages, currentPage, totalPages)
	}

	for _, span := range h.hitSpans {
		if x >= span.start && x < span.end {
			return span.id, true
		}
	}

	return "", false
}

// hasNotificationInRange checks if any entry in [start, end) has a notification
func (h *horizontalTabHeader) hasNotificationInRange(start, end int) bool {
	for i := start; i < end && i < len(h.entries); i++ {
		if h.entries[i].hasNotification {
			return true
		}
	}
	return false
}

func (h *horizontalTabHeader) AddTab(channel, identity string) (string, tea.Cmd) {
	entry := tabHeaderEntry{
		id:       uuid.New().String(),
		name:     channel,
		identity: identity,
	}

	h.entries = append(h.entries, entry)
	h.hitSpans = nil

	return entry.id, nil
}

func (h *horizontalTabHeader) SelectTab(id string) {
	for i := range h.entries {
		h.entries[i].selected = h.entries[i].id == id
		if h.entries[i].selected {
			h.entries[i].hasNotification = false
		}
	}
}

func (h *horizontalTabHeader) RemoveTab(id string) {
	h.entries = slices.DeleteFunc(h.entries, func(e tabHeaderEntry) bool {
		return e.id == id
	})
	h.hitSpans = nil
}

func (h *horizontalTabHeader) Resize(width, _ int) {
	h.width = width
	h.hitSpans = nil
}

func (h *horizontalTabHeader) MinWidth() int {
	return 0
}
