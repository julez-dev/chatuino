package mainui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
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

type KeyMap struct {
	Down key.Binding
	Up   key.Binding
}

// DefaultKeyMap returns a set of pager-like default keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
	}
}

var (
	indicator         = lipgloss.NewStyle().Foreground(lipgloss.Color("135")).Background(lipgloss.Color("135")).Render(" ")
	indicatorWidth, _ = lipgloss.Size(indicator)
)

type chatEntry struct {
	Position position
	Selected bool
	Message  twitch.IRCer
}

type position struct {
	CursorStart int
	CursorEnd   int
}

type chatWindow struct {
	parentTabID string
	channel     string
	channelID   string

	logger        zerolog.Logger
	m             KeyMap
	width, height int
	emoteStore    EmoteStore
	focused       bool

	cursor             int
	lineStart, lineEnd int

	// Entries keep track which actual original message is behind a single row.
	// A single message can span multiple line so this is needed to resolve a message based on a line
	entries []*chatEntry

	// Every single row, multiple rows may be part of a single message
	lines []string
}

func newChatWindow(logger zerolog.Logger, tabID string, width, height int, channel string, channelID string, emoteStore EmoteStore) *chatWindow {
	c := chatWindow{
		m:           DefaultKeyMap(),
		logger:      logger,
		parentTabID: tabID,
		channel:     channel,
		width:       width,
		height:      height,
		channelID:   channelID,
		emoteStore:  emoteStore,
	}

	return &c
}

func (c *chatWindow) Init() tea.Cmd {
	return nil
}

func (c *chatWindow) Update(msg tea.Msg) (*chatWindow, tea.Cmd) {

	switch msg := msg.(type) {
	case recvTwitchMessage:
		if msg.targetID == c.parentTabID {
			c.handleMessage(msg.message)
			c.updatePort()
			return c, nil
		}
	case tea.KeyMsg:
		if c.focused {
			switch {
			case key.Matches(msg, c.m.Down):
				c.messageDown(1)
			case key.Matches(msg, c.m.Up):
				c.messageUp(1)
			}
			switch msg.String() {
			case "d":
				c.logger.Info().Int("cursor", c.cursor).Int("start", c.lineStart).Int("end", c.lineEnd).Send()
				c.logger.Info().Strs("lines", c.lines).Send()
				c.logger.Info().Any("entries", c.entries).Send()
			case "b":
				c.moveToBottom()
			case "t":
				c.moveToTop()
			case "f11":
				// chat
				type state struct {
					Lines              []string
					Cursor             int
					LineStart, LineEnd int
					View               string
					Entries            []*chatEntry
				}

				dump := state{
					Lines:     c.lines,
					Cursor:    c.cursor,
					LineEnd:   c.lineEnd,
					LineStart: c.lineStart,
					View:      c.View(),
					Entries:   c.entries,
				}

				f, err := os.Create("chat_dump.json")

				if err != nil {
					panic(err)
				}

				defer f.Close()

				bytes, err := json.Marshal(dump)

				if err != nil {
					panic(err)
				}

				f.Write(bytes)
			}
		}
	}

	c.updatePort()

	return c, nil
}

func (c *chatWindow) View() string {
	lines := make([]string, c.height)

	copy(lines, c.lines[c.lineStart:c.lineEnd])

	c.logger.Info().Int("len", len(lines)).Int("start", c.lineStart).Int("end", c.lineEnd).Send()

	return strings.Join(lines, "\n")
}

func (c *chatWindow) Focus() {
	c.focused = true
}

func (c *chatWindow) Blur() {
	c.focused = false
}

func (c *chatWindow) entryForCurrentCursor() (int, *chatEntry) {
	if len(c.entries) < 1 {
		return -1, nil
	}

	for i, e := range c.entries {
		if c.cursor >= e.Position.CursorStart && c.cursor <= e.Position.CursorEnd {
			return i, e
		}
	}

	return -1, nil
}

func (c *chatWindow) messageDown(n int) {
	if len(c.entries) < 1 {
		return
	}

	i, e := c.entryForCurrentCursor()

	if i == -1 {
		return
	}

	e.Selected = false

	i = clamp(i+n, 0, len(c.entries)-1)

	c.entries[i].Selected = true
	c.cursor = c.entries[i].Position.CursorEnd

	c.markSelectedMessages()
}

func (c *chatWindow) messageUp(n int) {
	if len(c.entries) < 1 {
		return
	}

	i, e := c.entryForCurrentCursor()

	if i == -1 {
		return
	}

	e.Selected = false

	i = clamp(i-n, 0, len(c.entries)-1)

	c.entries[i].Selected = true
	c.cursor = c.entries[i].Position.CursorStart

	c.markSelectedMessages()
}

func (c *chatWindow) moveToBottom() {
	i, currentEntry := c.entryForCurrentCursor()

	if currentEntry == nil {
		return
	}

	c.messageDown(len(c.entries) - i)
}

func (c *chatWindow) moveToTop() {
	i, currentEntry := c.entryForCurrentCursor()

	if currentEntry == nil {
		return
	}

	c.messageUp(i)
}

func (c *chatWindow) getNewestEntry() *chatEntry {
	if len(c.entries) > 0 {
		return c.entries[len(c.entries)-1]
	}

	return nil
}

func (c *chatWindow) markSelectedMessages() {
	for i, s := range c.lines {
		s = strings.TrimSuffix(s, indicator)
		c.lines[i] = strings.TrimRight(s, " ")
	}

	for _, e := range c.entries {
		if !e.Selected {
			continue
		}

		lines := c.lines[e.Position.CursorStart : e.Position.CursorEnd+1]

		for i, s := range lines {
			strWidth, _ := lipgloss.Size(s)
			spacerLen := c.width - strWidth - indicatorWidth

			if spacerLen < 0 {
				c.logger.Warn().
					Int("spacer-len", spacerLen).
					Int("viewport-width", c.width).
					Int("str-width", strWidth).
					Int("indicator-width", indicatorWidth).
					Str("line", s).Msg("prevented negative count panic")
				spacerLen = 0
			}

			s = s + strings.Repeat(" ", spacerLen) + indicator
			lines[i] = s
		}
	}
}

func (c *chatWindow) handleMessage(msg twitch.IRCer) {
	if msg, ok := msg.(*twitch.PrivateMessage); ok {
		if len(c.entries) > cleanupThreshold {
			_, currentEntry := c.entryForCurrentCursor()

			if currentEntry == nil || currentEntry.Position.CursorStart > cleanupThreshold {
				c.logger.Info().Int("amount", cleanupThreshold-int(cleanupAfterMessage)).Msg("clean up messages now")
				c.entries = c.entries[cleanupThreshold-int(cleanupAfterMessage):]
				c.recalculateLines()
			}
		}

		lines := c.messageToText(msg)

		// create new message - append to entries list
		var (
			positionStart    = -1
			wasLatestMessage = true
		)

		if newestEntry := c.getNewestEntry(); newestEntry != nil {
			positionStart = newestEntry.Position.CursorEnd
			wasLatestMessage = newestEntry.Selected
			newestEntry.Selected = false
		}

		entry := &chatEntry{
			Position: position{
				CursorStart: positionStart + 1,
				CursorEnd:   positionStart + len(lines),
			},
			Selected: wasLatestMessage,
			Message:  msg,
		}

		c.entries = append(c.entries, entry)
		c.lines = append(c.lines, lines...)

		if wasLatestMessage {
			c.messageDown(1)
		}
	}
}

func (c *chatWindow) messageToText(msg twitch.IRCer) []string {
	switch msg := msg.(type) {
	case *twitch.PrivateMessage:
		// filter non-printable characters
		message := strings.Map(func(r rune) rune {
			// There are a coupe of emojis that cause issues when displaying them
			// They will always overflow the message width
			if r == '🫰' {
				return -1
			}

			if unicode.IsPrint(r) {
				return r
			}

			return -1
		}, msg.Message)

		message = c.colorMessageEmotes(message)
		var lines []string

		userColor := lipgloss.NewStyle().Foreground(lipgloss.Color(msg.UserColor))

		dateUserStr := fmt.Sprintf("%s %s: ", msg.SentAt.Local().Format("15:04:05"), userColor.Render(msg.From)) // start of the message (sent date + username)
		widthDateUserStr, _ := lipgloss.Size(dateUserStr)

		textLimit := c.width - widthDateUserStr - indicatorWidth

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
		if e, ok := c.emoteStore.GetByText(c.channelID, split); ok {
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

func (c *chatWindow) updatePort() {
	// validate cursors position
	c.cursor = clamp(c.cursor, 0, len(c.lines))

	switch {
	case c.cursor <= c.lineStart: // cursor is before the selection
		c.lineStart = c.cursor
		c.lineEnd = clamp(c.lineStart+len(c.lines), c.lineStart, c.lineStart+c.height)
	case c.cursor >= c.lineEnd: // cursor is after the selection
		c.lineEnd = c.cursor + 1
		c.lineStart = clamp(c.lineEnd-c.height, 0, c.lineEnd)
	case c.cursor > c.lineStart && c.cursor < c.lineEnd:
		c.lineEnd = clamp(c.lineStart+len(c.lines), c.lineStart, c.lineStart+c.height)
	}
}

func (c *chatWindow) recalculateLines() {
	if len(c.entries) < 1 && len(c.lines) < 1 {
		return
	}

	// get the currently selected entry, to reset the cursor to the new position once calculated
	_, selected := c.entryForCurrentCursor()

	c.lines = make([]string, 0, len(c.lines))

	var prevEntry *chatEntry

	for _, e := range c.entries {
		lastCursorEnd := -1

		if prevEntry != nil {
			lastCursorEnd = prevEntry.Position.CursorEnd
		}

		lines := c.messageToText(e.Message)
		c.lines = append(c.lines, lines...)

		e.Position.CursorStart = lastCursorEnd + 1
		e.Position.CursorEnd = lastCursorEnd + len(lines)
		prevEntry = e
	}

	if selected != nil {
		c.cursor = selected.Position.CursorEnd
		c.lineEnd = c.cursor + 1
		c.lineStart = clamp(c.lineEnd-c.height, 0, c.lineEnd)
	}

	c.markSelectedMessages()
}
