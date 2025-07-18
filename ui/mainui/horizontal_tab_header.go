package mainui

import (
	"math"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
)

type horizontalTabHeader struct {
	width   int
	entries []tabHeaderEntry

	userConfig           UserConfiguration
	tabHeaderStyle       lipgloss.Style
	tabHeaderActiveStyle lipgloss.Style

	perPage int // items per page
	page    int // current page
}

func newHorizontalTabHeader(width int, userConfiguration UserConfiguration) *horizontalTabHeader {
	return &horizontalTabHeader{
		width:                width,
		userConfig:           userConfiguration,
		tabHeaderStyle:       lipgloss.NewStyle().Background(lipgloss.Color(userConfiguration.Theme.TabHeaderBackgroundColor)).MarginRight(1),
		tabHeaderActiveStyle: lipgloss.NewStyle().Background(lipgloss.Color(userConfiguration.Theme.TabHeaderActiveBackgroundColor)).MarginRight(1),
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

	start, end := h.currentPageBounds()
	entries := h.entries[start:end]
	var renderedVisible []string

	var (
		showPrevious bool
		showNext     bool
	)

	if h.page > 0 {
		showPrevious = true
	}

	if h.page < h.maxPage() {
		showNext = true
	}

	for _, e := range entries {
		var rendered string
		if e.selected {
			rendered = h.tabHeaderActiveStyle.Width(h.maxEntryWidth()).AlignHorizontal(lipgloss.Center).Render(e.render())
		} else {
			rendered = h.tabHeaderStyle.Width(h.maxEntryWidth()).AlignHorizontal(lipgloss.Center).Render(e.render())
		}

		renderedVisible = append(renderedVisible, rendered)
	}

	var out string

	if showPrevious {
		// check if any previous entries have a notification
		var hasNotification bool
		before := h.entries[:start]
		for _, e := range before {
			if e.hasNotification {
				hasNotification = true
				break
			}
		}

		if hasNotification {
			out += lipgloss.NewStyle().Foreground(lipgloss.Color(h.userConfig.Theme.ChatNoticeAlertColor)).Render("< ")
		} else {
			out += "< "
		}
	}

	out += lipgloss.JoinHorizontal(lipgloss.Left, renderedVisible...)

	if showNext {
		// cehck if any next entries have a notification
		var hasNotification bool
		after := h.entries[end:]
		for _, e := range after {
			if e.hasNotification {
				hasNotification = true
				break
			}
		}

		spaces := h.width - lipgloss.Width(out) - 4
		if spaces < 0 {
			spaces = 0
		}
		out += strings.Repeat(" ", spaces)

		if hasNotification {
			out += lipgloss.NewStyle().Foreground(lipgloss.Color(h.userConfig.Theme.ChatNoticeAlertColor)).Render(" >")
		} else {
			out += " >"
		}
	}

	return out
}

func (v *horizontalTabHeader) AddTab(channel, identity string) (string, tea.Cmd) {
	entry := tabHeaderEntry{
		id:       uuid.New().String(),
		name:     channel,
		identity: identity,
	}

	v.entries = append(v.entries, entry)
	v.Resize(v.width, 0) // new entry might be longer than others

	return entry.id, nil
}

func (v *horizontalTabHeader) SelectTab(id string) {
	// reset selected
	for i := range v.entries {
		v.entries[i].selected = false
	}

	i := v.indexForEntry(id)
	v.entries[i].hasNotification = false
	v.entries[i].selected = true
	v.calculatePage(i)
}

func (v *horizontalTabHeader) indexForEntry(id string) int {
	for i, e := range v.entries {
		if e.id == id {
			return i
		}
	}

	return 0
}

func (v *horizontalTabHeader) calculatePage(entryIndex int) {
	if v.perPage == 0 {
		return
	}

	page := entryIndex / v.perPage
	v.page = page
}
func (v *horizontalTabHeader) maxPage() int {
	if v.perPage == 0 {
		return 0
	}
	return (len(v.entries)+v.perPage-1)/v.perPage - 1
}

func (v *horizontalTabHeader) currentPageBounds() (int, int) {
	start := v.page * v.perPage
	end := start + v.perPage

	if end > len(v.entries) {
		end = len(v.entries)
	}

	return start, end
}

func (v *horizontalTabHeader) RemoveTab(id string) {
	v.entries = slices.DeleteFunc(v.entries, func(e tabHeaderEntry) bool {
		return e.id == id
	})

	v.Resize(v.width, 0)
}

func (h *horizontalTabHeader) Resize(width, _ int) {
	h.width = width

	entryWidth := h.maxEntryWidth()
	h.perPage = int(math.Floor(float64(h.width-8) / float64(entryWidth))) // total width - 8 (4 for earch arrow display on side)

	// in case of new page balance select to page with currently selected item
	for i, e := range h.entries {
		if e.selected {
			h.calculatePage(i)
			return
		}
	}
}

func (h *horizontalTabHeader) maxEntryWidth() int {
	max := 5
	for _, entry := range h.entries {
		entry.hasNotification = true
		w := lipgloss.Width(entry.render()) + 1
		if w > max {
			max = w
		}
	}

	return max
}

func (h *horizontalTabHeader) MinWidth() int {
	return 0
}
