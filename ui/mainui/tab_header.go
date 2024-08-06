package mainui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const bellEmojiPrefix = string(rune(128276)) + " "

var (
	tabHeaderStyle       = lipgloss.NewStyle().Background(lipgloss.Color("#556")).MarginBottom(1).MarginLeft(1).UnsetMarginTop()
	tabHeaderActiveStyle = tabHeaderStyle.Background(lipgloss.Color("135"))
)

type requestNotificationIconMessage struct {
	tabID string
}

type tabHeaderEntry struct {
	id       string
	name     string
	identity string
	selected bool
}

type tabHeader struct {
	width   int
	entries []tabHeaderEntry
}

func newTabHeader() *tabHeader {
	return &tabHeader{
		width:   10,
		entries: make([]tabHeaderEntry, 0),
	}
}

func (t *tabHeader) Init() tea.Cmd {
	return nil
}

func (t *tabHeader) Update(msg tea.Msg) (*tabHeader, tea.Cmd) {
	if req, ok := msg.(requestNotificationIconMessage); ok {
		log.Logger.Info().Str("id", req.tabID).Msg("got noti request")
		for i := range t.entries {
			// add bell prefix if tab id matched, and tab is not already active
			if t.entries[i].id == req.tabID && !t.entries[i].selected {
				if !strings.HasPrefix(t.entries[i].name, bellEmojiPrefix) {
					t.entries[i].name = bellEmojiPrefix + t.entries[i].name
					return t, nil
				}
			}
		}
	}

	return t, nil
}

func (t *tabHeader) View() string {
	var rowIndex int
	var displayRows [][]string

	for _, e := range t.entries {

		style := tabHeaderStyle

		if e.selected {
			style = tabHeaderActiveStyle
		}

		displayEntry := style.Render(fmt.Sprintf("%s [%s]", e.name, e.identity))

		widthEntry := lipgloss.Width(displayEntry)

		// create the first row if not exists
		var widthCurrentRow int
		if len(displayRows) > 0 {
			widthCurrentRow = lipgloss.Width(lipgloss.JoinHorizontal(lipgloss.Left, displayRows[rowIndex]...))
		} else {
			displayRows = append(displayRows, []string{})
			widthCurrentRow = 0
		}

		// if new entry would overflow => create new row
		if widthEntry+widthCurrentRow > t.width {
			rowIndex++
			displayRows = append(displayRows, []string{})
			displayRows[rowIndex] = append(displayRows[rowIndex], displayEntry)
		} else {
			// does not overflow, add to existing
			displayRows[rowIndex] = append(displayRows[rowIndex], displayEntry)
		}
	}

	var flattenedDisplayRows []string

	for _, row := range displayRows {
		flattenedDisplayRows = append(flattenedDisplayRows, lipgloss.JoinHorizontal(lipgloss.Left, row...))
	}

	return lipgloss.JoinVertical(lipgloss.Left, flattenedDisplayRows...)
}

func (t *tabHeader) selectTab(id string) {
	for i, e := range t.entries {
		if e.id == id {
			t.entries[i].name = strings.TrimPrefix(t.entries[i].name, bellEmojiPrefix)
			t.entries[i].selected = true
		} else {
			t.entries[i].selected = false
		}
	}
}

func (t *tabHeader) removeTab(id string) {
	t.entries = slices.DeleteFunc(t.entries, func(entry tabHeaderEntry) bool {
		return entry.id == id
	})
}

func (t *tabHeader) addTab(channel, identity string) string {
	entry := tabHeaderEntry{
		id:       uuid.New().String(),
		name:     channel,
		identity: identity,
	}

	t.entries = append(t.entries, entry)

	return entry.id
}
