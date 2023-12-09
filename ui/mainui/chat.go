package mainui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/twitch/command"
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/reflow/wrap"
	"github.com/rs/zerolog"
)

const (
	cleanupAfterMessage float64 = 400.0
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

var badgeMap = map[string]string{
	"broadcaster": lipgloss.NewStyle().Foreground(lipgloss.Color("#E91916")).Render("Streamer"),
	"no_audio":    "No Audio",
	"vip":         lipgloss.NewStyle().Foreground(lipgloss.Color("#E005B9")).Render("VIP"),
	"subscriber":  lipgloss.NewStyle().Foreground(lipgloss.Color("#8B54F0")).Render("Sub"),
	"admin":       "Admin",
	"staff":       "Staff",
	"Turbo":       lipgloss.NewStyle().Foreground(lipgloss.Color("135")).Render("Turbo"),
	"moderator":   lipgloss.NewStyle().Foreground(lipgloss.Color("#00AD03")).Render("Mod"),
}

var (
	indicator      = lipgloss.NewStyle().Foreground(lipgloss.Color("135")).Background(lipgloss.Color("135")).Render("@")
	indicatorWidth = lipgloss.Width(indicator) + 1 // for empty space
)

var (
	stvStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#0aa6ec"))
	ttvStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#a35df2"))
)

type chatEntry struct {
	Position  position
	Selected  bool
	IsDeleted bool
	Message   twitch.IRCer
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
	// A single message can span multiple lines so this is needed to resolve a message based on a line
	entries []*chatEntry

	// Every single row, multiple rows may be part of a single message
	lines []string

	// optimize color rendering by caching render functions
	// so we don't need to recreate a new lipgloss.Style for every message
	userColorCache map[string]func(...string) string
}

func newChatWindow(logger zerolog.Logger, tabID string, width, height int, channel string, channelID string, emoteStore EmoteStore) *chatWindow {
	c := chatWindow{
		m:              DefaultKeyMap(),
		logger:         logger,
		parentTabID:    tabID,
		channel:        channel,
		width:          width,
		height:         height,
		channelID:      channelID,
		emoteStore:     emoteStore,
		userColorCache: map[string]func(...string) string{},
	}

	return &c
}

func (c *chatWindow) Init() tea.Cmd {
	return nil
}

func (c *chatWindow) Update(msg tea.Msg) (*chatWindow, tea.Cmd) {
	switch msg := msg.(type) {
	case chatEventMessage:
		if msg.channel == c.channel {
			c.handleMessage(msg.message)
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

				f.Write([]byte(stripAnsi(string(bytes))))
			}
		}
	}

	c.updatePort()

	return c, nil
}

func (c *chatWindow) View() string {
	lines := append(c.lines[c.lineStart:c.lineEnd], make([]string, c.height-len(c.lines[c.lineStart:c.lineEnd]))...)

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

	c.updatePort()
	c.markSelectedMessage()
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

	c.updatePort()
	c.markSelectedMessage()
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

func (c *chatWindow) markSelectedMessage() {
	linesInView := c.lines[c.lineStart:c.lineEnd]
	for i, s := range linesInView {
		if strings.HasPrefix(s, indicator+" ") {
			s = strings.TrimPrefix(s, indicator+" ")
			linesInView[i] = "  " + s
		}
	}

	for _, e := range c.entries {
		if !e.Selected {
			continue
		}

		lines := c.lines[e.Position.CursorStart : e.Position.CursorEnd+1]

		for i, s := range lines {
			if strings.HasPrefix(s, indicator) {
				continue
			}

			s = strings.TrimPrefix(s, "  ")
			lines[i] = indicator + " " + s
		}
	}
}

func (c *chatWindow) handleMessage(msg twitch.IRCer) {
	switch msg.(type) {
	case error, *command.PrivateMessage, *command.Notice, *command.ClearChat: // supported Message types
	default: // exit only on other types
		return
	}

	// cleanup messages if we have more messages than cleanupThreshold
	if len(c.entries) > cleanupThreshold {
		_, currentEntry := c.entryForCurrentCursor()

		if currentEntry == nil || currentEntry.Position.CursorStart > cleanupThreshold {
			c.logger.Info().Int("amount", cleanupThreshold-int(cleanupAfterMessage)).Msg("clean up messages now")
			c.entries = c.entries[cleanupThreshold-int(cleanupAfterMessage):]
			c.recalculateLines()
		}
	}

	// if timeout message, rewrite all messages from user
	if timeoutMsg, ok := msg.(*command.ClearChat); ok {
		var hasDeleted bool
		for _, e := range c.entries {
			privMsg, ok := e.Message.(*command.PrivateMessage)

			if !ok {
				continue
			}

			if strings.EqualFold(privMsg.DisplayName, timeoutMsg.UserName) && !e.IsDeleted && !strings.HasPrefix(privMsg.Message, "[deleted by moderator]") {
				hasDeleted = true
				e.IsDeleted = true
				privMsg.Message = fmt.Sprintf("[deleted by moderator] %s", privMsg.Message)
			}
		}

		if hasDeleted {
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
	c.updatePort()

	if wasLatestMessage {
		c.moveToBottom()
	}
}

func (c *chatWindow) messageToText(msg twitch.IRCer) []string {
	switch msg := msg.(type) {
	case error:
		availableWidth := c.width - indicatorWidth

		wrappedText := wrap.String(wordwrap.String(
			time.Now().Format("15:04:05")+" [System]: "+strings.ReplaceAll(msg.Error(), "\n", ""),
			availableWidth,
		), availableWidth)

		splits := strings.Split(wrappedText, "\n")
		return splits
	case *command.PrivateMessage:
		// filter non-printable characters
		message := strings.Map(func(r rune) rune {
			// There are a coupe of emojis that cause issues when displaying them
			// They will always overflow the message width
			if unicode.IsControl(r) {
				return -1
			}

			if unicode.IsPrint(r) {
				return r
			}

			return -1
		}, msg.Message)

		var (
			startMsgStr string
			badges      = make([]string, 0, len(msg.Badges)) // Acts like all badges will be mappable
		)

		for _, badge := range msg.Badges {
			if b, ok := badgeMap[badge.Name]; ok {
				badges = append(badges, b)
			}
		}

		// if render function not in cache yet, compute now
		userRenderFunc, ok := c.userColorCache[msg.Color]

		if !ok {
			userRenderFunc = lipgloss.NewStyle().Foreground(lipgloss.Color(msg.Color)).Render
			c.userColorCache[msg.Color] = userRenderFunc
		}

		if len(badges) == 0 {
			// start of the message (sent date + username)
			startMsgStr = fmt.Sprintf("  %s %s: ",
				msg.TMISentTS.Local().Format("15:04:05"),
				userRenderFunc(msg.DisplayName),
			)
		} else {
			// start of the message (sent date + badges + username)
			startMsgStr = fmt.Sprintf("  %s [%s] %s: ",
				msg.TMISentTS.Local().Format("15:04:05"),
				strings.Join(badges, ", "),
				userRenderFunc(msg.DisplayName),
			)
		}

		startMsgStrWidth := lipgloss.Width(startMsgStr)

		const startMsgPadding = 35
		if startMsgStrWidth < startMsgPadding {
			startMsgStr = startMsgStr + strings.Repeat(" ", startMsgPadding-startMsgStrWidth)
			startMsgStrWidth = lipgloss.Width(startMsgStr)
		}

		// calculate the maximum text length which the message content is allowed to have
		textLimit := c.width - startMsgStrWidth - indicatorWidth

		message = c.colorMessageEmotes(message)

		// wrap text to textLimit, if soft wrapping fails (for example in links) force break
		wrappedText := wrap.String(wordwrap.String(message, textLimit), textLimit)
		splits := strings.Split(wrappedText, "\n")

		lines := make([]string, 0, len(splits))
		lines = append(lines, startMsgStr+splits[0])

		if len(splits) > 1 {
			for _, line := range splits[1:] {
				lines = append(lines, strings.Repeat(" ", startMsgStrWidth)+line)
			}
		}

		return lines
	case *command.Notice:
		textLimit := c.width - indicatorWidth

		styled := lipgloss.NewStyle().Italic(true).Render(time.Now().Format("15:04:05") + "[System] " + msg.Message)
		wrappedText := wrap.String(wordwrap.String(styled, textLimit), textLimit)
		splits := strings.Split(wrappedText, "\n")

		return splits
	case *command.ClearChat:

		prefix := "  " + msg.TMISentTS.Format("15:04:05")
		textLimit := c.width - indicatorWidth - lipgloss.Width(prefix)

		text := " [System] " + msg.UserName
		if msg.BanDuration == 0 {
			text += " was permanently banned."
		} else {
			dur := time.Duration(msg.BanDuration * 1e9)
			text += " was timed out for " + dur.String()
		}

		wrappedText := wrap.String(wordwrap.String(text, textLimit), textLimit)
		splits := strings.Split(wrappedText, "\n")
		splits[0] = prefix + splits[0]

		return splits
	}

	return []string{}
}

func (c *chatWindow) colorMessageEmotes(message string) string {
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

	c.markSelectedMessage()
}
