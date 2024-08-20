package mainui

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/command"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/reflow/wrap"
	"github.com/rs/zerolog"
)

const (
	cleanupAfterMessage float64 = 800.0
	cleanupThreshold            = int(cleanupAfterMessage * 1.5)
	// prefixPadding               = 41
	prefixPadding = 0
)

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
	subAlertStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#a35df2")).Bold(true)
	noticeAlertStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#a35df2")).Bold(true)
	clearChatAlertStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#a35df2")).Bold(true)
	errorAlertStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6347")).Bold(true)
)

type chatEntry struct {
	Position                  position
	Selected                  bool
	IsDeleted                 bool
	OverwrittenMessageContent string
	Event                     chatEventMessage
	IsIgnored                 bool
}

type position struct {
	CursorStart int
	CursorEnd   int
}

type chatWindowState int

const (
	viewChatWindowState chatWindowState = iota
	searchChatWindowState
)

type chatWindow struct {
	logger        zerolog.Logger
	keymap        save.KeyMap
	width, height int
	emoteStore    EmoteStore

	focused bool
	state   chatWindowState

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
	searchInput    textinput.Model
}

func newChatWindow(logger zerolog.Logger, width, height int, emoteStore EmoteStore, keymap save.KeyMap) *chatWindow {
	input := textinput.New()
	input.CharLimit = 25
	input.Prompt = "  /"
	input.Placeholder = "search"
	input.Cursor.BlinkSpeed = time.Millisecond * 750
	input.Width = width

	c := chatWindow{
		keymap:         keymap,
		logger:         logger,
		width:          width,
		height:         height,
		emoteStore:     emoteStore,
		userColorCache: map[string]func(...string) string{},
		searchInput:    input,
	}

	return &c
}

func (c *chatWindow) Init() tea.Cmd {
	return nil
}

func (c *chatWindow) Update(msg tea.Msg) (*chatWindow, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case chatEventMessage:
		c.handleMessage(msg)
		return c, nil
	case tea.KeyMsg:
		if c.focused {
			switch {
			// start search
			case key.Matches(msg, c.keymap.SearchMode):
				return c, c.handleStartSearchMode()
			// stop search
			case key.Matches(msg, c.keymap.Escape) && c.state == searchChatWindowState:
				c.handleStopSearchMode()
				return c, nil
			// update search, allow up and down arrow keys for navigation in result
			case c.state == searchChatWindowState && msg.String() != "up" && msg.String() != "down":
				c.searchInput, cmd = c.searchInput.Update(msg)
				c.applySearch()
				cmds = append(cmds, cmd)
				return c, tea.Batch(cmds...)
			case key.Matches(msg, c.keymap.Down):
				c.messageDown(1)
			case key.Matches(msg, c.keymap.Up):
				c.messageUp(1)
				return c, nil
			case key.Matches(msg, c.keymap.GoToBottom):
				c.moveToBottom()
			case key.Matches(msg, c.keymap.GoToTop):
				c.moveToTop()
			case key.Matches(msg, c.keymap.DumpChat):
				c.debugDumpChat()

			}
		}
	}

	if c.state == searchChatWindowState && c.focused {
		c.searchInput, cmd = c.searchInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	c.updatePort()

	return c, tea.Batch(cmds...)
}

func (c *chatWindow) handleStartSearchMode() tea.Cmd {
	c.state = searchChatWindowState
	c.searchInput.Focus()
	c.recalculateLines()
	return c.searchInput.Focus()
}

func (c *chatWindow) handleStopSearchMode() {
	c.state = viewChatWindowState
	var last *chatEntry
	for e := range slices.Values(c.entries) {
		e.IsIgnored = false
		last = e
		e.Selected = false
	}

	if last != nil {
		last.Selected = true
	}
	c.searchInput.SetValue("")
	c.recalculateLines()
	c.moveToBottom()
}

func (c *chatWindow) View() string {
	height := c.height

	if c.state == searchChatWindowState {
		height--
	}

	if height < 1 {
		return ""
	}

	spaces := make([]string, height-len(c.lines[c.lineStart:c.lineEnd]))
	lines := append(c.lines[c.lineStart:c.lineEnd], spaces...)

	if c.state == searchChatWindowState {
		return c.searchInput.View() + "\n" + strings.Join(lines, "\n")
	}

	return strings.Join(lines, "\n")
}

func (c *chatWindow) Focus() {
	c.focused = true
}

func (c *chatWindow) Blur() {
	c.focused = false
}

func (c *chatWindow) debugDumpChat() {
	// chat
	type state struct {
		Lines              []string
		Cursor             int
		LineStart, LineEnd int
		View               string
		Entries            []*chatEntry
		UserCache          []string
	}

	dump := state{
		Lines:     c.lines,
		Cursor:    c.cursor,
		LineEnd:   c.lineEnd,
		LineStart: c.lineStart,
		View:      c.View(),
		Entries:   c.entries,
	}

	dump.UserCache = make([]string, 0, len(c.userColorCache))

	for k := range c.userColorCache {
		dump.UserCache = append(dump.UserCache, k)
	}

	f, err := os.Create("chat_dump.json")
	if err != nil {
		panic(err)
	}

	defer func() {
		_ = f.Close()
	}()

	bytes, err := json.Marshal(dump)
	if err != nil {
		panic(err)
	}

	_, _ = f.Write([]byte(stripAnsi(string(bytes))))
}

func (c *chatWindow) entryForCurrentCursor() (int, *chatEntry) {
	active := c.activeEntries()
	if len(active) < 1 {
		return -1, nil
	}

	for i, e := range active {
		if c.cursor >= e.Position.CursorStart && c.cursor <= e.Position.CursorEnd {
			return i, e
		}
	}

	return -1, nil
}

func (c *chatWindow) messageDown(n int) {
	active := c.activeEntries()
	if len(active) < 1 {
		return
	}

	i, e := c.entryForCurrentCursor()

	if i == -1 {
		return
	}

	e.Selected = false

	i = clamp(i+n, 0, len(active)-1)

	active[i].Selected = true
	c.cursor = active[i].Position.CursorEnd

	c.updatePort()
	c.markSelectedMessage()
}

func (c *chatWindow) messageUp(n int) {
	active := c.activeEntries()
	if len(active) < 1 {
		return
	}

	i, e := c.entryForCurrentCursor()

	if i == -1 {
		return
	}

	e.Selected = false

	i = clamp(i-n, 0, len(active)-1)

	active[i].Selected = true
	c.cursor = active[i].Position.CursorStart

	c.updatePort()
	c.markSelectedMessage()
}

func (c *chatWindow) moveToBottom() {
	i, currentEntry := c.entryForCurrentCursor()

	if currentEntry == nil {
		return
	}

	active := c.activeEntries()
	c.messageDown(len(active) - i)
}

func (c *chatWindow) moveToTop() {
	i, currentEntry := c.entryForCurrentCursor()

	if currentEntry == nil {
		return
	}

	c.messageUp(i)
}

func (c *chatWindow) getNewestEntry() *chatEntry {
	active := c.activeEntries()
	if len(active) > 0 {
		return active[len(active)-1]
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

	active := c.activeEntries()

	for e := range slices.Values(active) {
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

func (c *chatWindow) cleanup() {
	// todo: make this smarter, so we can delete more often
	// c.logger.Info().Msgf("(%d/%d)", len(c.entries), cleanupThreshold)
	if len(c.entries) < cleanupThreshold {
		return
	}

	if c.state == searchChatWindowState {
		return
	}

	if e := c.getNewestEntry(); e == nil || !e.Selected {
		c.logger.Info().Msg("skip cleanup because not on newest message")
		return
	}

	c.logger.Info().Int("cleanup-after", int(cleanupAfterMessage)).Int("len", len(c.entries)).Msg("cleanup")
	c.entries = c.entries[int(cleanupAfterMessage):]
	c.recalculateLines()

	// for i, e := range c.entries {
	// 	if e.IsIgnored {
	// 		continue
	// 	}

	// 	if c.lineStart >= e.Position.CursorStart && c.lineStart <= e.Position.CursorEnd {
	// 		c.logger.Info().Int("index", i).Int("threshold", cleanupThreshold).Msg("cleanup")

	// 		if i > cleanupThreshold {
	// 			c.entries = c.entries[i:]
	// 			c.recalculateLines()
	// 		}

	// 		return
	// 	}
	// }
}

func (c *chatWindow) handleMessage(msg chatEventMessage) {
	switch msg.message.(type) {
	case error, *command.PrivateMessage, *command.Notice, *command.ClearChat, *command.SubMessage, *command.SubGiftMessage, *command.AnnouncementMessage: // supported Message types
	default: // exit only on other types
		return
	}

	// cleanup messages if we have more messages than cleanupThreshold
	// if len(c.entries) > cleanupThreshold {
	// 	_, currentEntry := c.entryForCurrentCursor()

	// 	remainingStart := len(c.entries) - int(cleanupAfterMessage)
	// 	c.logger.Info().Int("c.lineStart", c.lineStart).Int("remaining-start", remainingStart).Int("new-len", len(c.entries[remainingStart:])).Msg("cleanup")
	// 	if currentEntry == nil || c.lineStart > remainingStart {
	// 		c.logger.Info().Int("c.lineStart", c.lineStart).Int("remaining-start", remainingStart).Int("new-len", len(c.entries[remainingStart:])).Msg("done cleanup")

	// 		c.entries = c.entries[remainingStart:]
	// 		c.recalculateLines()
	// 	}
	// }

	// 1st: number of entries before line start
	c.cleanup()

	// if timeout message, rewrite all messages from user
	if timeoutMsg, ok := msg.message.(*command.ClearChat); ok {
		var hasDeleted bool
		for _, e := range c.entries {
			privMsg, ok := e.Event.message.(*command.PrivateMessage)

			if !ok {
				continue
			}

			if strings.EqualFold(privMsg.DisplayName, timeoutMsg.UserName) && !e.IsDeleted && !strings.HasPrefix(privMsg.Message, "[deleted by moderator]") {
				hasDeleted = true
				e.IsDeleted = true
				privMsg.Message = fmt.Sprintf("[deleted by moderator]\n%s", privMsg.Message)
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
		Selected:                  wasLatestMessage,
		OverwrittenMessageContent: msg.messageContentEmoteOverride,
		Event:                     msg,
	}

	// we are currently searching and the new entry does not match the search, then ignore new entry
	if c.state == searchChatWindowState && !c.entryMatchesSearch(entry) {
		entry.IsIgnored = true
		c.entries = append(c.entries, entry)
	} else {
		c.entries = append(c.entries, entry)
		c.lines = append(c.lines, lines...)
	}

	c.updatePort()

	if wasLatestMessage {
		c.moveToBottom()
	}
}

func (c *chatWindow) messageToText(event chatEventMessage) []string {
	switch msg := event.message.(type) {
	case error:
		prefix := "  " + strings.Repeat(" ", len(time.Now().Format("15:04:05"))) + " [" + errorAlertStyle.Render("Error") + "]: "
		text := strings.ReplaceAll(msg.Error(), "\n", "")
		return c.wordwrapMessage(prefix, c.colorMessage(text))
	case *command.PrivateMessage:
		badges := make([]string, 0, len(msg.Badges)) // Acts like all badges will be mappable

		// format users badges
		for _, badge := range msg.Badges {
			if b, ok := badgeMap[badge.Name]; ok {
				badges = append(badges, b)
			}
		}

		// if render function not in cache yet, compute now
		userRenderFunc, ok := c.userColorCache[strings.ToLower(msg.DisplayName)]

		if !ok {
			userRenderFunc = lipgloss.NewStyle().Foreground(lipgloss.Color(msg.Color)).Render
			c.userColorCache[strings.ToLower(msg.DisplayName)] = userRenderFunc
		}

		var prefix string
		if len(badges) == 0 {
			// start of the message (sent date + username)
			prefix = fmt.Sprintf("  %s %s: ",
				msg.TMISentTS.Local().Format("15:04:05"),
				userRenderFunc(msg.DisplayName),
			)
		} else {
			// start of the message (sent date + badges + username)
			prefix = fmt.Sprintf("  %s [%s] %s: ",
				msg.TMISentTS.Local().Format("15:04:05"),
				strings.Join(badges, ", "),
				userRenderFunc(msg.DisplayName),
			)
		}

		return c.wordwrapMessage(prefix, c.colorMessage(event.messageContentEmoteOverride))
	case *command.Notice:
		prefix := "  " + msg.FakeTimestamp.Local().Format("15:04:05") + " [" + noticeAlertStyle.Render("Notice") + "]: "
		styled := lipgloss.NewStyle().Italic(true).Render(msg.Message)

		return c.wordwrapMessage(prefix, c.colorMessage(styled))
	case *command.ClearChat:
		prefix := "  " + msg.TMISentTS.Local().Format("15:04:05") + " [" + clearChatAlertStyle.Render("Clear Chat") + "]: "

		userRenderFunc, ok := c.userColorCache[msg.UserName]

		var text string

		if !ok {
			text += msg.UserName
		} else {
			text += userRenderFunc(msg.UserName)
		}

		if msg.BanDuration == 0 {
			text += " was permanently banned."
		} else {
			dur := time.Duration(msg.BanDuration * 1e9)
			text += " was timed out for " + dur.String()
		}

		return c.wordwrapMessage(prefix, c.colorMessage(text))
	case *command.SubMessage:
		prefix := "  " + msg.TMISentTS.Local().Format("15:04:05") + " [" + subAlertStyle.Render("Sub Alert") + "]: "

		subResubText := "subscribed"
		if msg.MsgID == "resub" {
			subResubText = "resubscribed"
		}

		// if render function not in cache yet, compute now
		userRenderFunc, ok := c.userColorCache[msg.Login]

		if !ok {
			userRenderFunc = lipgloss.NewStyle().Foreground(lipgloss.Color(msg.Color)).Render
			c.userColorCache[msg.Login] = userRenderFunc
		}

		text := fmt.Sprintf("%s just %s with a %s subscription. (%d Months, %d Month Streak)",
			userRenderFunc(msg.DisplayName),
			subResubText,
			msg.SubPlan.String(),
			msg.CumulativeMonths,
			msg.StreakMonths,
		)

		if event.messageContentEmoteOverride != "" {
			text += ": " + event.messageContentEmoteOverride
		}

		return c.wordwrapMessage(prefix, c.colorMessage(text))
	case *command.SubGiftMessage:
		prefix := "  " + msg.TMISentTS.Local().Format("15:04:05") + " [" + subAlertStyle.Render("Sub Gift Alert") + "]: "

		gifterRenderFunc, ok := c.userColorCache[msg.Login]

		if !ok {
			gifterRenderFunc = lipgloss.NewStyle().Foreground(lipgloss.Color(msg.Color)).Render
			c.userColorCache[msg.Login] = gifterRenderFunc
		}

		receiptRenderFunc, ok := c.userColorCache[msg.RecipientUserName]

		if !ok {
			receiptRenderFunc = lipgloss.NewStyle().Render
		}

		text := fmt.Sprintf("%s gifted a %s sub to %s. (%d Months)",
			gifterRenderFunc(msg.DisplayName),
			msg.SubPlan.String(),
			receiptRenderFunc(msg.ReceiptDisplayName),
			msg.Months,
		)

		return c.wordwrapMessage(prefix, c.colorMessage(text))
	case *command.AnnouncementMessage:
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(msg.ParamColor.RGBHex())).Bold(true)

		prefix := "  " + msg.TMISentTS.Local().Format("15:04:05") + " [" + style.Render("Announcement") + "] "

		userRenderFn, ok := c.userColorCache[msg.Login]

		if !ok {
			userRenderFn = lipgloss.NewStyle().Foreground(lipgloss.Color(msg.Color)).Render
			c.userColorCache[msg.Login] = userRenderFn
		}

		text := fmt.Sprintf("%s: %s",
			userRenderFn(msg.DisplayName),
			event.messageContentEmoteOverride,
		)

		return c.wordwrapMessage(prefix, c.colorMessage(text))
	}

	return []string{}
}

func (c *chatWindow) colorMessage(content string) string {
	content = c.colorMessageMentions(content)
	return content
}

func (c *chatWindow) wordwrapMessage(prefix, content string) []string {
	content = strings.Map(func(r rune) rune {
		// this rune is commonly used to bypass the twitch spam detection
		if r == duplicateBypass {
			return -1
		}

		return r
	}, content)

	prefixWidth := lipgloss.Width(prefix)

	// Assure that the prefix is at least prefixPadding wide
	if prefixWidth < prefixPadding {
		prefix = prefix + strings.Repeat(" ", prefixPadding-prefixWidth)
		prefixWidth = lipgloss.Width(prefix)
	}

	contentWidthLimit := c.width - indicatorWidth - prefixWidth

	// softwrap text to contentWidthLimit, if soft wrapping fails (for example in links) force break
	wrappedText := wrap.String(wordwrap.String(content, contentWidthLimit), contentWidthLimit)
	splits := strings.Split(wrappedText, "\n")

	lines := make([]string, 0, len(splits))
	lines = append(lines, prefix+splits[0]) // first line is prefix + content at index 0

	// if there are more lines, add prefixPadding spaces to the beginning of the line
	for _, line := range splits[1:] {
		lines = append(lines, strings.Repeat(" ", prefixWidth)+line)
	}

	return lines
}

func (c *chatWindow) colorMessageMentions(message string) string {
	words := strings.Split(message, " ")
	for i, word := range words {
		cleaned := strings.ToLower(stripDisplayNameEdges(word))

		renderFn, ok := c.userColorCache[cleaned]

		if !ok {
			continue
		}

		words[i] = renderFn(word)
	}

	return strings.Join(words, " ")
}

func (c *chatWindow) updatePort() {
	// validate cursors position
	c.cursor = clamp(c.cursor, 0, len(c.lines)-1)

	height := c.height
	if c.state == searchChatWindowState {
		height--
	}

	if height < 0 {
		c.lineStart = 0
		c.lineEnd = 0
		return
	}

	switch {
	case len(c.lines) < height: // all lines fit in the height
		c.lineStart = 0
		c.lineEnd = len(c.lines)
	case c.cursor <= c.lineStart: // cursor is before the selection
		c.lineStart = c.cursor
		c.lineEnd = clamp(c.lineStart+len(c.lines), c.lineStart, c.lineStart+height)
	case c.cursor >= c.lineEnd: // cursor is after the selection
		c.lineEnd = c.cursor + 1
		c.lineStart = clamp(c.lineEnd-height, 0, c.lineEnd)
	case c.cursor > c.lineStart && c.cursor < c.lineEnd:
		c.lineEnd = clamp(c.lineStart+len(c.lines), c.lineStart, c.lineStart+height)
	}
}

func (c *chatWindow) recalculateLines() {
	c.searchInput.Width = c.width

	entries := c.activeEntries()

	if len(entries) < 1 && len(c.lines) < 1 {
		return
	}

	// get the currently selected entry, to reset the cursor to the new position once calculated
	_, selected := c.entryForCurrentCursor()

	c.lines = make([]string, 0, len(c.lines))

	var prevEntry *chatEntry

	for e := range slices.Values(entries) {
		lastCursorEnd := -1

		if prevEntry != nil {
			lastCursorEnd = prevEntry.Position.CursorEnd
		}

		lines := c.messageToText(e.Event)
		c.lines = append(c.lines, lines...)

		e.Position.CursorStart = lastCursorEnd + 1
		e.Position.CursorEnd = lastCursorEnd + len(lines)
		prevEntry = e
	}

	if selected != nil {
		height := c.height
		if c.state == searchChatWindowState {
			height--
		}

		c.cursor = selected.Position.CursorEnd
	}

	c.updatePort()
	c.markSelectedMessage()
}

func (c *chatWindow) activeEntries() []*chatEntry {
	activeEntries := []*chatEntry{}
	for e := range slices.Values(c.entries) {
		if !e.IsIgnored {
			activeEntries = append(activeEntries, e)
		}
	}

	return activeEntries
}

func (c *chatWindow) applySearch() {
	var last *chatEntry
	for e := range slices.Values(c.entries) {
		e.Selected = false
		if c.entryMatchesSearch(e) {
			e.IsIgnored = false
			last = e
			continue
		}

		e.IsIgnored = true
	}

	if last != nil {
		last.Selected = true
	}

	c.recalculateLines()
	c.updatePort()
	c.moveToBottom()
}

func (c *chatWindow) entryMatchesSearch(e *chatEntry) bool {
	cast, ok := e.Event.message.(*command.PrivateMessage)

	if !ok {
		return false
	}

	search := c.searchInput.Value()
	if fuzzy.MatchFold(search, cast.DisplayName) || fuzzy.MatchFold(search, cast.Message) {
		return true
	}

	return false
}
