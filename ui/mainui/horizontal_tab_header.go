package mainui

import (
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

func (h *horizontalTabHeader) Update(msg tea.Msg) (*horizontalTabHeader, tea.Cmd) {
	return h, nil
}

func (h *horizontalTabHeader) View() string {
	if len(h.entries) == 0 {
		return ""
	}

	entries := h.entriesForPage()
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
			rendered = h.tabHeaderActiveStyle.Render(e.render())
		} else {
			rendered = h.tabHeaderStyle.Render(e.render())
		}

		renderedVisible = append(renderedVisible, rendered)
	}

	var out string

	if showPrevious {
		out += "< "
	}

	out += lipgloss.JoinHorizontal(lipgloss.Left, renderedVisible...)

	if showNext {
		spaces := h.width - lipgloss.Width(out) - 4
		out += strings.Repeat(" ", spaces) + " >"
	}

	return out
}

func (v *horizontalTabHeader) addTab(channel, identity string) (string, tea.Cmd) {
	entry := tabHeaderEntry{
		id:       uuid.New().String(),
		name:     channel,
		identity: identity,
	}

	v.entries = append(v.entries, entry)
	v.resize(v.width, 0) // new entry might be longer than others

	return entry.id, nil
}

func (v *horizontalTabHeader) selectTab(id string) {
	// reset selected
	for i := range v.entries {
		v.entries[i].selected = false
	}

	i := v.indexForEntry(id)
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
	return len(v.entries) / v.perPage
}

func (v *horizontalTabHeader) entriesForPage() []tabHeaderEntry {
	start := v.page * v.perPage
	end := start + v.perPage

	if end > len(v.entries) {
		end = len(v.entries)
	}

	return v.entries[start:end]
}

func (v *horizontalTabHeader) removeTab(id string) {
	v.entries = slices.DeleteFunc(v.entries, func(e tabHeaderEntry) bool {
		return e.id == id
	})
	v.resize(v.width, 0)
}

func (h *horizontalTabHeader) resize(width, _ int) {
	h.width = width

	entryWidth := h.maxEntryWidth()
	h.perPage = h.width / entryWidth

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
		w := lipgloss.Width(entry.render()) + 1 // extra margin
		if w > max {
			max = w
		}
	}

	return max
}

func (h *horizontalTabHeader) minWidth() int {
	return 0
}
