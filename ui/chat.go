package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/muesli/reflow/wordwrap"
	"github.com/rs/zerolog"
)

var (
	indicator         = lipgloss.NewStyle().Foreground(lipgloss.Color("135")).Render("\u2588")
	indicatorWidth, _ = lipgloss.Size(indicator)
)

type chatEntry struct {
	Position position
	Message  twitch.IRCer
}

type position struct {
	CursorStart int
	CursorEnd   int
}

type chatWindow struct {
	parentTab *tab
	logger    zerolog.Logger
	focused   bool // if window should consume keyboard messages

	cursor int // Overall message cursor, a single message can span multiple lines
	start  int
	end    int

	// Entries keep track which actual original message is behind a single row.
	// A single message can span multiple line so this is needed to resolve a message based on a line
	entries []*chatEntry

	// Every single row, multiple rows may be part of a single message
	lines []string

	// Position of the wrapped viewport
	yPosition int
	width     int
	height    int

	viewport viewport.Model
}

func (c *chatWindow) Init() tea.Cmd {
	return nil
}

func (c *chatWindow) Update(msg tea.Msg) (*chatWindow, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)
	_ = cmd

	switch msg := msg.(type) {
	case resizeChatContainerMessage:
		c.height = msg.Height
		c.width = msg.Width
		c.viewport.Height = msg.Height
		c.viewport.Width = msg.Width
	case recvTwitchMessage:
		if msg.target == c.parentTab.id {

			lastCursorEnd := -1

			if len(c.entries) > 0 {
				newest := c.entries[len(c.entries)-1]
				lastCursorEnd = newest.Position.CursorEnd
			}

			lines := messageToText(msg.message, c.width-indicatorWidth)

			entry := &chatEntry{
				Position: position{
					CursorStart: lastCursorEnd + 1,
					CursorEnd:   lastCursorEnd + len(lines),
				},
				Message: msg.message,
			}

			wasLatestLine := c.isLatestEntry()

			c.lines = append(c.lines, lines...)
			c.entries = append(c.entries, entry)
			if wasLatestLine {
				c.MoveDown(1)
			} else {
				c.UpdateViewport()
			}
		}
	}

	if c.focused {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "u":
				c.MoveUp(1)
			case "d":
				c.MoveDown(1)
			}
		}
	}

	return c, tea.Batch(cmds...)
}

func (c *chatWindow) findEntryForCursor() (int, *chatEntry) {
	for i, entry := range c.entries {
		if c.cursor >= entry.Position.CursorStart && c.cursor <= entry.Position.CursorEnd {
			return i, entry
		}
	}

	return 0, nil
}

func (c *chatWindow) UpdateViewport() {
	renderedRows := make([]string, 0, len(c.lines))

	if c.cursor >= 0 {
		c.start = clamp(c.cursor-c.viewport.Height, 0, c.cursor)
	} else {
		c.start = 0
	}
	c.end = clamp(c.cursor+c.viewport.Height, c.cursor, len(c.lines))
	for i := c.start; i < c.end; i++ {
		renderedRows = append(renderedRows, c.lines[i])
	}

	c.viewport.SetContent(
		lipgloss.JoinVertical(lipgloss.Left, renderedRows...),
	)
}

func (c *chatWindow) isLatestEntry() bool {
	if len(c.entries)-1 < 0 {
		return true
	}

	index, _ := c.findEntryForCursor()
	return index == len(c.entries)-1
}

func (c *chatWindow) Focus() {
	c.focused = true
}

func (c *chatWindow) Blur() {
	c.focused = false
}

// Move up n number of messages
func (c *chatWindow) MoveUp(n int) {
	c.removeMarkCurrentMessage()
	cIndex, _ := c.findEntryForCursor()
	nIndex := clamp(cIndex-n, 0, len(c.entries)-1)
	c.cursor = c.entries[nIndex].Position.CursorStart

	switch {
	case c.start == 0:
		c.viewport.SetYOffset(clamp(c.viewport.YOffset, 0, c.cursor))
	case c.start < c.viewport.Height:
		c.viewport.SetYOffset(clamp(c.viewport.YOffset+n, 0, c.cursor))
	case c.viewport.YOffset >= 1:
		c.viewport.YOffset = clamp(c.viewport.YOffset+n, 1, c.viewport.Height)
	}
	c.markCurrentMessage()
	c.UpdateViewport()
}

// Move down n number of messages
func (c *chatWindow) MoveDown(n int) {
	c.removeMarkCurrentMessage()
	cIndex, _ := c.findEntryForCursor()
	nIndex := clamp(cIndex+n, 0, len(c.entries)-1) // the index of the n message after current message
	c.cursor = c.entries[nIndex].Position.CursorEnd
	c.markCurrentMessage()
	c.UpdateViewport()

	switch {
	case c.end == len(c.lines):
		c.viewport.SetYOffset(clamp(c.viewport.YOffset-n, 1, c.viewport.Height))
	case c.cursor > (c.end-c.start)/2:
		c.viewport.SetYOffset(clamp(c.viewport.YOffset-n, 1, c.cursor))
	case c.viewport.YOffset > 1:
	case c.cursor > c.viewport.YOffset+c.viewport.Height-1:
		c.viewport.SetYOffset(clamp(c.viewport.YOffset+1, 0, 1))
	}
}

func (c *chatWindow) View() string {
	return c.viewport.View()
}

func (c *chatWindow) removeMarkCurrentMessage() {
	_, entry := c.findEntryForCursor()

	lines := c.lines[entry.Position.CursorStart : entry.Position.CursorEnd+1]

	for i, s := range lines {
		s = strings.TrimSuffix(s, indicator)
		lines[i] = strings.TrimRight(s, " ")
	}
}

func (c *chatWindow) markCurrentMessage() {
	_, entry := c.findEntryForCursor()

	lines := c.lines[entry.Position.CursorStart : entry.Position.CursorEnd+1]

	for i, s := range lines {
		strWidth, _ := lipgloss.Size(s)
		spacerLen := c.width - strWidth - indicatorWidth
		s = s + strings.Repeat(" ", spacerLen) + indicator
		lines[i] = s
	}
}

func messageToText(msg twitch.IRCer, maxWidth int) []string {
	switch msg := msg.(type) {
	case *twitch.PrivateMessage:
		lines := []string{}

		dateUserStr := fmt.Sprintf("%s %s: ", msg.SentAt.Local().Format("15:04:05"), msg.From)
		widthDateUserStr, _ := lipgloss.Size(dateUserStr)

		textLimit := maxWidth - widthDateUserStr - indicatorWidth
		splits := strings.Split(wordwrap.String(msg.Message, textLimit), "\n")

		lines = append(lines, dateUserStr+splits[0])

		if len(splits) > 1 {
			for _, line := range splits[1:] {
				lines = append(lines, strings.Repeat(" ", widthDateUserStr)+line)
			}
		}

		return lines
	}

	return []string{}
}
