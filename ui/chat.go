package ui

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rs/zerolog"
)

type chatEntry struct {
	Position position
	Lines    []string
}

type position struct {
	CursorStart int
	CursorEnd   int
}

func (p position) isInRange(i int) bool {
	return i >= p.CursorStart && i <= p.CursorEnd
}

type ChatWindow struct {
	logger zerolog.Logger

	cursor int // overall message cursor, a single message can span multiple lines
	start  int
	end    int

	entries            []*chatEntry
	totalNumberOfLines int

	yPosition int
	width     int
	height    int

	viewport viewport.Model
}

func (c *ChatWindow) Init() tea.Cmd {
	return nil
}

func (c *ChatWindow) Update(msg tea.Msg) (*ChatWindow, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)
	_ = cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		c.logger.Info().Msg("key event")

		switch msg.String() {
		case "u":
			c.MoveUp()
		case "d":
			c.MoveDown(1)
		}
	case resizeChatContainerMessage:
		c.viewport.Height = msg.Height
		c.viewport.Width = msg.Width
		c.viewport.YPosition = msg.YPosition

	case recvTwitchMessage:

		lastCursorEnd := -1

		if len(c.entries) > 0 {
			newest := c.entries[len(c.entries)-1]
			lastCursorEnd = newest.Position.CursorEnd
		}

		lines := []string{msg.message}
		position := position{
			CursorStart: lastCursorEnd + 1,
			CursorEnd:   lastCursorEnd + 1 + len(lines) - 1,
		}

		entry := &chatEntry{
			Position: position,
			Lines:    lines,
		}

		c.totalNumberOfLines += len(lines)

		c.entries = append(c.entries, entry)
		// c.logger.Info().Any("entries", c.entries).Send()

		c.UpdateViewport()
	}

	return c, tea.Batch(cmds...)
}

func (c *ChatWindow) UpdateViewport() {
	if c.cursor >= 0 {
		c.start = clamp(c.cursor-c.viewport.Height, 0, c.cursor)
	} else {
		c.start = 0
	}

	c.end = clamp(c.cursor+c.viewport.Height, c.cursor, c.totalNumberOfLines)

	flattened := c.flattenMessages()

	renderedRows := []string{}
	for i := c.start; i < c.end; i++ {
		renderedRows = append(renderedRows, flattened[i])
	}

	c.viewport.SetContent(
		lipgloss.JoinVertical(lipgloss.Left, renderedRows...),
	)
}

func (c *ChatWindow) MoveUp() {
	c.cursor = clamp(c.cursor-1, 0, len(c.entries)-1)
	switch {
	case c.start == 0:
		c.viewport.SetYOffset(clamp(c.viewport.YOffset, 0, c.cursor))
	case c.start < c.viewport.Height:
		c.viewport.SetYOffset(clamp(c.viewport.YOffset+1, 0, c.cursor))
	case c.viewport.YOffset >= 1:
		c.viewport.YOffset = clamp(c.viewport.YOffset+1, 1, c.viewport.Height)
	}

	c.UpdateViewport()
}

func (c *ChatWindow) MoveDown(n int) {
	c.cursor = clamp(c.cursor+n, 0, len(c.entries)-1)
	c.UpdateViewport()

	switch {
	case c.end == len(c.entries):
		c.viewport.SetYOffset(clamp(c.viewport.YOffset-n, 1, c.viewport.Height))
	case c.cursor > (c.end-c.start)/2:
		c.viewport.SetYOffset(clamp(c.viewport.YOffset-n, 1, c.cursor))
	case c.viewport.YOffset > 1:
	case c.cursor > c.viewport.YOffset+c.viewport.Height-1:
		c.viewport.SetYOffset(clamp(c.viewport.YOffset+1, 0, 1))
	}
}

func (c *ChatWindow) flattenMessages() []string {
	messages := make([]string, 0, len(c.entries))
	for _, e := range c.entries {
		messages = append(messages, e.Lines...)
	}
	return messages
}

func (c *ChatWindow) View() string {
	return c.viewport.View()
}

func (c *ChatWindow) getSelectedEntry() *chatEntry {
	for _, e := range c.entries {
		if e.Position.isInRange(c.cursor) {
			return e
		}
	}

	return nil
}

// func chunks(s string, chunkSize int) []string {
// 	if len(s) == 0 {
// 		return nil
// 	}
// 	if chunkSize >= len(s) {
// 		return []string{s}
// 	}
// 	var chunks []string = make([]string, 0, (len(s)-1)/chunkSize+1)
// 	currentLen := 0
// 	currentStart := 0
// 	for i := range s {
// 		if currentLen == chunkSize {
// 			chunks = append(chunks, s[currentStart:i])
// 			currentLen = 0
// 			currentStart = i
// 		}
// 		currentLen++
// 	}
// 	chunks = append(chunks, s[currentStart:])
// 	return chunks
// }
