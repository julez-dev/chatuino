package chatui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/reflow/wrap"
	"github.com/rs/zerolog"
)

const (
	cleanupAfterMessage float64 = 250.0
	cleanupThreshold            = int(cleanupAfterMessage * 1.5)
)

var (
	indicator         = lipgloss.NewStyle().Foreground(lipgloss.Color("135")).Background(lipgloss.Color("135")).Render(" ")
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
	case resizeTabContainerMessage:
		if len(c.entries) > 0 {
			c.redrawMessages()
		}
	case recvTwitchMessage:
		if msg.target == c.parentTab.id {
			c.handleRecvTwitchMessage(msg.message)
		}
	}

	if c.focused {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "k":
				c.MoveUp(1)
			case "j":
				c.MoveDown(1)
			case "b":
				c.MoveToBottom()
			}
		}
	}

	return c, tea.Batch(cmds...)
}

func (c *chatWindow) redrawMessages() {
	_, preRedrawEntry := c.findEntryForCursor()

	c.cursor = 0
	c.start = 0
	c.end = 0
	c.lines = make([]string, 0, len(c.entries)) // the number of lines will always be at least the number of entries
	c.viewport.SetContent("")

	var prevEntry *chatEntry

	for i, e := range c.entries {
		lastCursorEnd := -1

		if prevEntry != nil {
			lastCursorEnd = prevEntry.Position.CursorEnd
		}

		lines := c.messageToText(e.Message)

		if len(lines) > 0 {
			e.Position.CursorStart = lastCursorEnd + 1
			e.Position.CursorEnd = lastCursorEnd + len(lines)
			c.lines = append(c.lines, lines...)
			c.entries[i] = e
		}

		// If the cursor is not set to an entry (no entries found) or if the entry was set the cursor before the redraw
		// -> set the cursor to the current entry
		if preRedrawEntry == nil || e == preRedrawEntry {
			c.cursor = e.Position.CursorStart
		}

		prevEntry = e
	}

	c.markCurrentMessage()
	c.UpdateViewport()
}

func (c *chatWindow) handleRecvTwitchMessage(msg twitch.IRCer) {
	// remove messages after threshold is reached to clean up some memory
	if len(c.entries) > cleanupThreshold {
		_, currentEntry := c.findEntryForCursor()

		if currentEntry == nil || currentEntry.Position.CursorStart > cleanupThreshold {
			c.logger.Info().Int("amount", cleanupThreshold-int(cleanupAfterMessage)).Msg("clean up messages now")
			c.entries = c.entries[cleanupThreshold-int(cleanupAfterMessage):]
			c.redrawMessages()
		}
	}

	lastCursorEnd := -1
	if len(c.entries) > 0 {
		newest := c.entries[len(c.entries)-1]
		lastCursorEnd = newest.Position.CursorEnd
	}

	lines := c.messageToText(msg)

	if len(lines) > 0 {
		entry := &chatEntry{
			Position: position{
				CursorStart: lastCursorEnd + 1,
				CursorEnd:   lastCursorEnd + len(lines),
			},
			Message: msg,
		}

		wasLatestLine := c.isLatestEntry()

		c.lines = append(c.lines, lines...)
		c.entries = append(c.entries, entry)
		if wasLatestLine {
			c.MoveDown(1)
		} else {
			c.UpdateViewport()
		}
	} else {
		c.logger.Info().Msg("got zero line message, ignoring")
	}
}

func (c *chatWindow) findEntryForCursor() (int, *chatEntry) {
	for i, entry := range c.entries {
		if c.cursor >= entry.Position.CursorStart && c.cursor <= entry.Position.CursorEnd {
			return i, entry
		}
	}

	return -1, nil
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
	cIndex, _ := c.findEntryForCursor()
	if cIndex == -1 {
		return
	}

	c.removeMarkCurrentMessage()
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

func (c *chatWindow) MoveToBottom() {
	if len(c.entries) < 1 {
		return
	}

	cIndex, _ := c.findEntryForCursor()
	c.MoveDown((len(c.entries) - cIndex))
}

// Move down n number of messages
func (c *chatWindow) MoveDown(n int) {
	cIndex, _ := c.findEntryForCursor()

	if cIndex == -1 {
		return
	}

	c.removeMarkCurrentMessage()
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
	if _, entry := c.findEntryForCursor(); entry != nil {
		lines := c.lines[entry.Position.CursorStart : entry.Position.CursorEnd+1]

		for i, s := range lines {
			s = strings.TrimSuffix(s, indicator)
			lines[i] = strings.TrimRight(s, " ")
		}
	}
}

func (c *chatWindow) markCurrentMessage() {
	_, entry := c.findEntryForCursor()

	lines := c.lines[entry.Position.CursorStart : entry.Position.CursorEnd+1]

	for i, s := range lines {
		strWidth, _ := lipgloss.Size(s)
		spacerLen := c.viewport.Width - strWidth - indicatorWidth

		if spacerLen < 0 {
			c.logger.Warn().
				Int("spacer-len", spacerLen).
				Int("viewport-width", c.viewport.Width).
				Int("str-width", strWidth).
				Int("indicator-width", indicatorWidth).
				Str("line", s).Msg("prevented negative count panic")
			spacerLen = 0
		}

		s = s + strings.Repeat(" ", spacerLen) + indicator
		lines[i] = s
	}
}

func (c *chatWindow) messageToText(msg twitch.IRCer) []string {
	switch msg := msg.(type) {
	case *twitch.PrivateMessage:
		// filter non printable characters
		message := strings.Map(func(r rune) rune {
			if unicode.IsPrint(r) {
				return r
			}

			return -1
		}, msg.Message)

		message = c.colorMessageEmotes(message)
		lines := []string{}

		userColor := lipgloss.NewStyle().Foreground(lipgloss.Color(msg.UserColor))

		dateUserStr := fmt.Sprintf("%s %s: ", msg.SentAt.Local().Format("15:04:05"), userColor.Render(msg.From)) // start of the message (sent date + user name)
		widthDateUserStr, _ := lipgloss.Size(dateUserStr)

		textLimit := c.viewport.Width - widthDateUserStr - indicatorWidth

		// wrap text to textLimit, if soft wrapping fails (for example in links) force break
		wrappedText := wrap.String(wordwrap.String(message, textLimit), textLimit)
		splits := strings.Split(wrappedText, "\n")

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

func (c *chatWindow) colorMessageEmotes(message string) string {
	var (
		stvStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#0aa6ec"))
		ttvStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#a35df2"))
	)

	splits := strings.Fields(message)
	for i, split := range splits {
		if e, ok := c.parentTab.emoteStore.GetByText(c.parentTab.channelID, split); ok {
			switch e.Platform {
			case emote.Twitch:
				splits[i] = ttvStyle.Render(split)
			case emote.SevenTV:
				splits[i] = stvStyle.Render(split)
			}

			continue
		}

		splits[i] = split
	}

	return strings.Join(splits, " ")
}
