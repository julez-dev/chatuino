package mainui

import (
	"strings"

	"github.com/julez-dev/chatuino/save"

	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/ui/nav"
)

// tabHitSpan is a clickable column range on the tab header content row.
type tabHitSpan struct {
	start, end int // [start, end) columns, including the leading border column
	id         string
}

type horizontalTabHeader struct {
	width int

	// pre-created styles to avoid allocations in View()/renderTabContent() (called every frame)
	borderStyle       lipgloss.Style
	bulletStyle       lipgloss.Style
	separatorStyle    lipgloss.Style
	activeStyle       lipgloss.Style
	notificationStyle lipgloss.Style
}

func newHorizontalTabHeader(width int, theme save.Theme) *horizontalTabHeader {
	return &horizontalTabHeader{
		width:             width,
		borderStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color(theme.InputPromptColor)),
		bulletStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color(theme.InputPromptColor)),
		separatorStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(theme.DimmedTextColor)),
		activeStyle:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.InputPromptColor)),
		notificationStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ChatNoticeAlertColor)),
	}
}

func (h *horizontalTabHeader) View(rows []nav.Row) string {
	if len(rows) == 0 || h.width <= 0 {
		return ""
	}

	borderStyle := h.borderStyle

	// Calculate pages with variable width
	pages := h.calculatePages(rows)
	currentPage, totalPages := h.findCurrentPage(rows, pages)

	// Top label with page and workspace context.
	topLabel := tabHeaderLabel("Tabs", rows, currentPage+1, totalPages)

	innerWidth := max(0, h.width-2) // -2 for left/right │

	// Build borders - total width should be h.width
	// Top: ┌─ + label + fill + ─┐ = 2 + label + fill + 2 = innerWidth + 2 = h.width
	topFill := innerWidth - lipgloss.Width(topLabel) - 2 // -2 for "─" before and after label
	if topFill < 0 {
		topFill = 0
	}
	topBorder := "┌─" + topLabel + strings.Repeat("─", topFill) + "─┐"

	// Bottom: └ + fill + ┘ = 1 + fill + 1, so fill = innerWidth
	bottomBorder := "└" + strings.Repeat("─", innerWidth) + "┘"

	// Build content with tabs, separators, arrows
	content := h.renderTabContent(rows, pages, currentPage, totalPages)

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

// calculatePages groups rows into pages based on variable width
func (h *horizontalTabHeader) calculatePages(rows []nav.Row) [][]int {
	if len(rows) == 0 {
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
	showBadges := showWorkspaceBadges(rows)

	for i, row := range rows {
		// Width = bullet(2 "▸ ") + optional badge + content + separator(3 " │ ")
		// For first item on page, no leading separator needed
		rowWidth := 2 + lipgloss.Width(workspaceRowLabel(row, showBadges))
		if len(currentPage) > 0 {
			rowWidth += 3 // add separator width
		}

		if currentWidth+rowWidth > availableWidth && len(currentPage) > 0 {
			// Start new page
			pages = append(pages, currentPage)
			currentPage = []int{i}
			currentWidth = 2 + lipgloss.Width(workspaceRowLabel(row, showBadges)) // reset without separator
		} else {
			currentPage = append(currentPage, i)
			currentWidth += rowWidth
		}
	}

	if len(currentPage) > 0 {
		pages = append(pages, currentPage)
	}

	return pages
}

// findCurrentPage returns the page index containing the active row
func (h *horizontalTabHeader) findCurrentPage(rows []nav.Row, pages [][]int) (pageIndex, totalPages int) {
	totalPages = len(pages)
	if totalPages == 0 {
		return 0, 0
	}

	activeIndex := -1
	for i, row := range rows {
		if row.Active {
			activeIndex = i
			break
		}
	}

	if activeIndex == -1 {
		return 0, totalPages
	}

	// Find which page contains the active row
	for pi, page := range pages {
		for _, idx := range page {
			if idx == activeIndex {
				return pi, totalPages
			}
		}
	}

	return 0, totalPages
}

// renderTabContent renders the tab row with bullets, separators, and arrows
func (h *horizontalTabHeader) renderTabContent(rows []nav.Row, pages [][]int, currentPage, totalPages int) string {
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
		hasNotification := hasNotificationInRange(rows, 0, pages[currentPage][0])
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
	pageRows := pages[currentPage]
	showBadges := showWorkspaceBadges(rows)

	for i, rowIdx := range pageRows {
		row := rows[rowIdx]
		label := workspaceRowLabel(row, showBadges)

		// Add separator before (except first)
		if i > 0 {
			b.WriteString(separatorStyle.Render(" │ "))
		}

		// Bullet for selected tab
		if row.Active {
			b.WriteString(bulletStyle.Render("▸ "))
			b.WriteString(activeStyle.Render(label))
		} else {
			b.WriteString("  ") // spacing to align with bullet
			b.WriteString(label)
		}
	}

	// Right arrow (if not on last page)
	if showNext {
		b.WriteString(" ")
		// Check for notifications on next pages
		lastOnPage := pages[currentPage][len(pages[currentPage])-1]
		hasNotification := hasNotificationInRange(rows, lastOnPage+1, len(rows))
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
func (h *horizontalTabHeader) buildHitSpans(rows []nav.Row, pages [][]int, currentPage, totalPages int) []tabHitSpan {
	if len(pages) == 0 || currentPage >= len(pages) {
		return nil
	}

	showPrev := currentPage > 0
	showNext := currentPage < totalPages-1

	spans := make([]tabHitSpan, 0, len(pages[currentPage])+2)
	showBadges := showWorkspaceBadges(rows)

	// x = left border (1) + left arrow/padding (2)
	x := 1 + 2

	if showPrev {
		if prevPage := pages[currentPage-1]; len(prevPage) > 0 {
			spans = append(spans, tabHitSpan{start: x - 2, end: x, id: rows[prevPage[0]].ID})
		}
	}

	for i, rowIdx := range pages[currentPage] {
		if i > 0 {
			x += 3 // separator " │ "
		}

		start := x
		x += 2 + lipgloss.Width(workspaceRowLabel(rows[rowIdx], showBadges)) // bullet + badge + label
		spans = append(spans, tabHitSpan{start: start, end: x, id: rows[rowIdx].ID})
	}

	if showNext {
		if nextPage := pages[currentPage+1]; len(nextPage) > 0 {
			spans = append(spans, tabHitSpan{start: x, end: x + 2, id: rows[nextPage[0]].ID})
		}
	}

	return spans
}

// TabAt returns the tab at the given cell. All three header rows are clickable
// so users do not have to hit the label row exactly.
func (h *horizontalTabHeader) TabAt(rows []nav.Row, x, y int) (string, bool) {
	if y < 0 || y > 2 || len(rows) == 0 {
		return "", false
	}

	pages := h.calculatePages(rows)
	currentPage, totalPages := h.findCurrentPage(rows, pages)

	for _, span := range h.buildHitSpans(rows, pages, currentPage, totalPages) {
		if x >= span.start && x < span.end {
			return span.id, true
		}
	}

	return "", false
}

// hasNotificationInRange checks if any row in [start, end) has a notification
func hasNotificationInRange(rows []nav.Row, start, end int) bool {
	for i := start; i < end && i < len(rows); i++ {
		if rows[i].HasNotification {
			return true
		}
	}

	return false
}

func (h *horizontalTabHeader) Resize(width, _ int) {
	h.width = width
}

func (h *horizontalTabHeader) MinWidth([]nav.Row) int {
	return 0
}
