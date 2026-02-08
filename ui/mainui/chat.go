package mainui

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/reflow/wordwrap"
	"github.com/julez-dev/reflow/wrap"
	"github.com/rs/zerolog/log"
)

const (
	cleanupAfterMessage float64 = 800.0
	cleanupThreshold            = int(cleanupAfterMessage * 1.5)
	// prefixPadding               = 41
	prefixPadding = 0
)

type chatEntry struct {
	Position   position
	Selected   bool
	IsDeleted  bool
	Event      chatEventMessage
	IsFiltered bool // message is filtered out by search
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
	deps          *DependencyContainer
	width, height int

	timeFormatFunc func(time.Time) string

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

	// styles
	indicator      string
	indicatorWidth int

	subAlertStyle       lipgloss.Style
	noticeAlertStyle    lipgloss.Style
	clearChatAlertStyle lipgloss.Style
	errorAlertStyle     lipgloss.Style
	dimmedStyle         lipgloss.Style
}

func newChatWindow(width, height int, deps *DependencyContainer) *chatWindow {
	input := textinput.New()
	input.CharLimit = 25
	input.Prompt = "  /"
	input.Placeholder = "search"
	input.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.InputPromptColor))
	input.Cursor.BlinkSpeed = time.Millisecond * 750
	input.Width = width

	indicator := lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.ChatIndicatorColor)).Background(lipgloss.Color(deps.UserConfig.Theme.ChatIndicatorColor)).Render(">")

	timeFormat := deps.UserConfig.Settings.Chat.TimeFormat

	c := chatWindow{
		deps:           deps,
		width:          width,
		height:         height,
		userColorCache: map[string]func(...string) string{},
		timeFormatFunc: func(t time.Time) string {
			return t.Local().Format(timeFormat)
		},
		searchInput: input,

		indicator:           indicator,
		indicatorWidth:      lipgloss.Width(indicator),
		subAlertStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.ChatSubAlertColor)).Bold(true),
		noticeAlertStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.ChatNoticeAlertColor)).Bold(true),
		clearChatAlertStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.ChatClearChatColor)).Bold(true),
		errorAlertStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.ChatErrorColor)).Bold(true),
		dimmedStyle:         lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.DimmedTextColor)),
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
			case key.Matches(msg, c.deps.Keymap.SearchMode):
				return c, c.handleStartSearchMode()
			// stop search
			case key.Matches(msg, c.deps.Keymap.Escape) && c.state == searchChatWindowState:
				c.handleStopSearchMode()
				return c, nil
			case key.Matches(msg, c.deps.Keymap.Confirm) && c.state == searchChatWindowState:
				c.handleStopSearchModeKeepSelected()
				return c, nil
			// update search, allow up and down arrow keys for navigation in result
			case c.state == searchChatWindowState && msg.String() != "up" && msg.String() != "down":
				c.searchInput, cmd = c.searchInput.Update(msg)
				c.applySearch()
				cmds = append(cmds, cmd)
				return c, tea.Batch(cmds...)
			case key.Matches(msg, c.deps.Keymap.Down):
				c.messageDown(1)
			case key.Matches(msg, c.deps.Keymap.Up):
				c.messageUp(1)
				return c, nil
			case key.Matches(msg, c.deps.Keymap.GoToBottom):
				c.moveToBottom()
			case key.Matches(msg, c.deps.Keymap.GoToTop):
				c.moveToTop()
			case key.Matches(msg, c.deps.Keymap.DumpChat):
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

func (c *chatWindow) handleStopSearchModeKeepSelected() {
	_, e := c.entryForCurrentCursor()

	if e == nil {
		c.handleStopSearchMode()
		return
	}

	c.handleStopSearchMode()
	c.goToEntry(e)
}

func (c *chatWindow) handleStopSearchMode() {
	c.state = viewChatWindowState
	var last *chatEntry
	for e := range slices.Values(c.entries) {
		e.IsFiltered = false
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
		//Lines:     c.lines,
		Cursor:    c.cursor,
		LineEnd:   c.lineEnd,
		LineStart: c.lineStart,
		//View:      c.View(),
		Entries: c.entries,
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

func (c *chatWindow) goToEntry(entry *chatEntry) {
	active := c.activeEntries()
	if len(active) < 1 {
		return
	}

	for i := range c.entries {
		c.entries[i].Selected = false
	}

	for _, e := range active {
		if e == entry {
			e.Selected = true
			c.cursor = e.Position.CursorEnd
			c.updatePort()
			c.markSelectedMessage()
			return
		}
	}
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
		s, found := strings.CutPrefix(s, c.indicator+" ")
		if found {
			linesInView[i] = "  " + s
		}
	}

	active := c.activeEntries()

	for e := range slices.Values(active) {
		if !e.Selected {
			continue
		}

		m := min(len(c.lines), e.Position.CursorEnd+1)
		lines := c.lines[e.Position.CursorStart:m]

		for i, s := range lines {
			if strings.HasPrefix(s, c.indicator) {
				continue
			}

			s = strings.TrimPrefix(s, "  ")
			lines[i] = c.indicator + " " + s
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
		log.Logger.Info().Msg("skip cleanup because not on newest message")
		return
	}

	log.Logger.Info().Int("cleanup-after", int(cleanupAfterMessage)).Int("len", len(c.entries)).Msg("cleanup")
	c.entries = c.entries[int(cleanupAfterMessage):]
	c.recalculateLines()

	// users that should not be removed from the color cache
	usersLeft := make(map[string]struct{}, len(c.entries))
	for _, e := range c.entries {
		if privMsg, ok := e.Event.message.(*twitchirc.PrivateMessage); ok {
			usersLeft[strings.ToLower(privMsg.LoginName)] = struct{}{}
		}
	}

	// remove no longer needed user color cache entries
	for user := range c.userColorCache {
		// user no longer exists in chat
		if _, ok := usersLeft[user]; !ok {
			log.Logger.Info().Str("user", user).Msg("delete user from cache")
			delete(c.userColorCache, user)
		}
	}

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
	case error, *twitchirc.PrivateMessage, *twitchirc.Notice, *twitchirc.ClearChat, *twitchirc.SubMessage, *twitchirc.SubGiftMessage, *twitchirc.AnnouncementMessage, *twitchirc.ClearMessage: // supported Message types
	default: // exit only on other types
		return
	}

	c.cleanup()
	c.handleTimeoutMessage(msg)
	c.handleMessageDeletion(msg)

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
		Event:    msg,
	}

	// we are currently searching and the new entry does not match the search, then ignore new entry
	if c.state == searchChatWindowState && !c.entryMatchesSearch(entry) {
		entry.IsFiltered = true
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

func (c *chatWindow) handleTimeoutMessage(msg chatEventMessage) {
	if timeoutMsg, ok := msg.message.(*twitchirc.ClearChat); ok && timeoutMsg.UserName != nil {
		var hasDeleted bool
		for _, e := range c.entries {
			privMsg, ok := e.Event.message.(*twitchirc.PrivateMessage)

			if !ok {
				continue
			}

			if strings.EqualFold(privMsg.LoginName, *timeoutMsg.UserName) && !e.IsDeleted {
				hasDeleted = true
				e.IsDeleted = true
				e.Event.displayModifier.strikethrough = true
			}
		}

		if hasDeleted {
			c.recalculateLines()
		}
	}
}

func (c *chatWindow) handleMessageDeletion(msg chatEventMessage) {
	if clearMsg, ok := msg.message.(*twitchirc.ClearMessage); ok {
		var hasDeleted bool
		for _, e := range c.entries {
			privMsg, ok := e.Event.message.(*twitchirc.PrivateMessage)

			if !ok {
				continue
			}

			if strings.EqualFold(privMsg.ID, clearMsg.TargetMsgID) && !e.IsDeleted {
				hasDeleted = true
				e.IsDeleted = true
				e.Event.displayModifier.strikethrough = true
			}
		}

		if hasDeleted {
			c.recalculateLines()
		}
	}
}

// buildAlertPrefix creates a standardized prefix with timestamp and styled alert label.
// Example output: "  15:04:05 [Notice]: "
func (c *chatWindow) buildAlertPrefix(timestamp time.Time, label string, style lipgloss.Style) string {
	return "  " + c.dimmedStyle.Render(c.timeFormatFunc(timestamp)) + " [" + style.Render(label) + "]: "
}

// formatMessageText applies word replacements and color processing to message content.
func (c *chatWindow) formatMessageText(content string, modifier messageContentModifier) string {
	if modifier.strikethrough || modifier.italic {
		s := lipgloss.NewStyle()

		if modifier.strikethrough {
			s = s.Strikethrough(true).StrikethroughSpaces(false)
		}

		if modifier.italic {
			s = s.Italic(true)
		}

		return s.Render(content)
	}

	content = c.applyWordReplacements(content, modifier.wordReplacements)

	if modifier.messageSuffix != "" {
		content += modifier.messageSuffix
	}
	return content
}

// applyWordReplacements applies word replacements from the display modifier to the given content.
// It replaces each key in the wordReplacements map with its corresponding value.
func (c *chatWindow) applyWordReplacements(content string, replacements wordReplacement) string {
	if replacements == nil {
		return content
	}

	// Sort keys by length (longest first) to prevent partial matches
	keys := make([]string, 0, len(replacements))
	for k := range replacements {
		keys = append(keys, k)
	}

	slices.SortFunc(keys, func(a, b string) int {
		// Sort by length descending, then lexicographically
		if len(a) != len(b) {
			return len(b) - len(a)
		}
		return strings.Compare(a, b)
	})

	for _, original := range keys {
		content = strings.ReplaceAll(content, original, replacements[original])
	}
	return content
}

func (c *chatWindow) setUserColorModifier(content string, modifier *messageContentModifier) {
	words := strings.Split(content, " ")

	if modifier.wordReplacements == nil {
		modifier.wordReplacements = make(wordReplacement)
	}

	for _, word := range words {
		cleaned := strings.ToLower(stripDisplayNameEdges(word))
		renderFn, ok := c.userColorCache[cleaned]

		if !ok {
			// fallback try if empty
			if f, ok := c.userColorCache[word]; ok {
				renderFn = f
			} else {
				continue
			}
		}

		stripped := stripDisplayNameEdges(word)
		modifier.wordReplacements[stripped] = renderFn(stripped)
		modifier.wordReplacements[word] = renderFn(word)
	}
}

func (c *chatWindow) messageToText(event chatEventMessage) []string {
	switch msg := event.message.(type) {
	case error:
		prefix := "  " + strings.Repeat(" ", len(c.timeFormatFunc(time.Now()))) + " [" + c.errorAlertStyle.Render("Error") + "]: "
		text := strings.ReplaceAll(msg.Error(), "\n", "")
		return c.wordwrapMessage(prefix, c.formatMessageText(text, event.displayModifier))
	case *twitchirc.PrivateMessage:
		userRenderFunc := c.getSetUserColorFunc(msg.LoginName, msg.Color)

		// Build prefix components: time, [guest channel], [badges], username
		parts := []string{"  " + c.dimmedStyle.Render(c.timeFormatFunc(msg.TMISentTS))}

		if event.channelGuestDisplayName != "" {
			parts = append(parts, "|"+event.channelGuestDisplayName+"|")
		}

		if len(event.displayModifier.badgeReplacement) > 0 && !c.deps.UserConfig.Settings.Chat.DisableBadges {
			badges := formatBadgeReplacement(c.deps.UserConfig.Settings, event.displayModifier.badgeReplacement)
			if c.deps.UserConfig.Settings.Chat.GraphicBadges {
				// Hair space (U+200A) - narrower gap since badges have pixel padding
				parts = append(parts, badges+" "+userRenderFunc(msg.DisplayName)+": ")
			} else {
				parts = append(parts, badges)
				parts = append(parts, userRenderFunc(msg.DisplayName)+": ")
			}
		} else {
			parts = append(parts, userRenderFunc(msg.DisplayName)+": ")
		}
		prefix := strings.Join(parts, " ")

		c.setUserColorModifier(msg.Message, &event.displayModifier)
		return c.wordwrapMessage(prefix, c.formatMessageText(msg.Message, event.displayModifier))
	case *twitchirc.Notice:
		title := "Notice"
		if event.isFakeEvent {
			title = "Fake-Notice"
		}

		prefix := c.buildAlertPrefix(msg.FakeTimestamp, title, c.noticeAlertStyle)

		event.displayModifier.italic = true
		c.setUserColorModifier(msg.Message, &event.displayModifier)

		return c.wordwrapMessage(prefix, c.formatMessageText(msg.Message, event.displayModifier))
	case *twitchirc.ClearMessage:
		prefix := c.buildAlertPrefix(msg.TMISentTS, "Clear Message", c.clearChatAlertStyle)
		prefix += "A message from "
		text := msg.Login + " was removed."

		c.setUserColorModifier(text, &event.displayModifier)

		return c.wordwrapMessage(prefix, c.formatMessageText(text, event.displayModifier))
	case *twitchirc.ClearChat:
		prefix := c.buildAlertPrefix(msg.TMISentTS, "Clear Chat", c.clearChatAlertStyle)

		if msg.TargetUserID == nil {
			return c.wordwrapMessage(prefix, c.formatMessageText("Clear chat prevented by Chatuino. Chat restored.", event.displayModifier))
		}

		text := *msg.UserName

		if msg.BanDuration != nil && *msg.BanDuration > 0 {
			text += " was timed out for " + humanizeDuration(time.Duration(*msg.BanDuration)*time.Second)
		} else {
			text += " was permanently banned."
		}

		c.setUserColorModifier(text, &event.displayModifier)

		return c.wordwrapMessage(prefix, c.formatMessageText(text, event.displayModifier))
	case *twitchirc.SubMessage:
		prefix := c.buildAlertPrefix(msg.TMISentTS, "Sub Alert", c.subAlertStyle)

		subResubText := "subscribed"
		if msg.MsgID == "resub" {
			subResubText = "resubscribed"
		}

		_ = c.getSetUserColorFunc(msg.Login, msg.Color)
		text := fmt.Sprintf("%s just %s with a %s subscription. (%d Months, %d Month Streak)",
			msg.DisplayName,
			subResubText,
			msg.SubPlan.String(),
			msg.CumulativeMonths,
			msg.StreakMonths,
		)

		// Append user message if present
		if msg.Message != "" {
			text += ": " + msg.Message
		}

		c.setUserColorModifier(text, &event.displayModifier)

		return c.wordwrapMessage(prefix, c.formatMessageText(text, event.displayModifier))
	case *twitchirc.SubGiftMessage:
		prefix := c.buildAlertPrefix(msg.TMISentTS, "Sub Gift Alert", c.subAlertStyle)

		_ = c.getSetUserColorFunc(msg.Login, msg.Color)

		text := fmt.Sprintf("%s gifted a %s sub to %s. (%d Months)",
			msg.DisplayName,
			msg.SubPlan.String(),
			msg.ReceiptDisplayName,
			msg.Months,
		)

		c.setUserColorModifier(text, &event.displayModifier)

		return c.wordwrapMessage(prefix, c.formatMessageText(text, event.displayModifier))
	case *twitchirc.AnnouncementMessage:
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(msg.ParamColor.RGBHex())).Bold(true)
		prefix := "  " + c.timeFormatFunc(msg.TMISentTS) + " [" + style.Render("Announcement") + "] "

		_ = c.getSetUserColorFunc(msg.Login, msg.Color)
		text := fmt.Sprintf("%s: %s",
			msg.DisplayName,
			c.applyWordReplacements(msg.Message, event.displayModifier.wordReplacements),
		)

		c.setUserColorModifier(text, &event.displayModifier)

		return c.wordwrapMessage(prefix, c.formatMessageText(text, event.displayModifier))
	}

	return []string{}
}

func (c *chatWindow) getSetUserColorFunc(name string, colorHex string) func(strs ...string) string {
	_, ok := c.userColorCache[name]

	if !ok {
		if colorHex == "" {
			colorHex = randomHexColor()
		}

		style := lipgloss.NewStyle().Foreground(lipgloss.Color(colorHex))
		c.userColorCache[name] = style.Render
	}

	return c.userColorCache[name]
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

	contentWidthLimit := c.width - c.indicatorWidth - prefixWidth

	// softwrap text to contentWidthLimit, if soft wrapping fails (for example in links) force break
	wrappedText := wrap.String(wordwrap.String(content, contentWidthLimit), contentWidthLimit)
	splits := strings.Split(wrappedText, "\n")

	lines := make([]string, 0, len(splits))
	lines = append(lines, prefix+splits[0]) // first line is prefix + content at index 0

	// if there are more lines, add prefixPadding spaces to the beginning of the line
	for _, line := range splits[1:] {
		if c.deps.UserConfig.Settings.Chat.DisablePaddingWrappedLines {
			lines = append(lines, strings.Repeat(" ", len(c.timeFormatFunc(time.Now()))+3)+line)
		} else {
			lines = append(lines, strings.Repeat(" ", prefixWidth)+line)
		}
	}

	return lines
}

func (c *chatWindow) updatePort() {
	height := c.height
	if c.state == searchChatWindowState {
		height--
	}

	if height <= 0 {
		c.lineStart = 0
		c.lineEnd = 0
		return
	}

	// Clamp cursor to valid range
	if len(c.lines) > 0 {
		c.cursor = clamp(c.cursor, 0, len(c.lines)-1)
	} else {
		c.cursor = 0
		c.lineStart = 0
		c.lineEnd = 0
		return
	}

	// If viewport is larger than content, show everything
	if height >= len(c.lines) {
		c.lineStart = 0
		c.lineEnd = len(c.lines)
		return
	}

	// Adjust viewport if cursor is outside
	if c.cursor < c.lineStart {
		// Cursor is above the viewport, so move viewport up to show cursor at the top
		c.lineStart = c.cursor
	} else if c.cursor >= c.lineStart+height {
		// Cursor is below the viewport, so move viewport down to show cursor at the bottom
		c.lineStart = c.cursor - height + 1
	}

	// The viewport is defined by lineStart and height
	c.lineEnd = c.lineStart + height

	// Final check to ensure lineEnd does not exceed the number of lines.
	if c.lineEnd > len(c.lines) {
		c.lineEnd = len(c.lines)
		c.lineStart = c.lineEnd - height
		if c.lineStart < 0 {
			c.lineStart = 0
		}
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
		c.cursor = selected.Position.CursorEnd
	}

	c.updatePort()
	c.markSelectedMessage()
}

func (c *chatWindow) activeEntries() []*chatEntry {
	if c.searchInput.Value() == "" {
		return c.entries
	}

	activeEntries := make([]*chatEntry, 0, c.height)
	for e := range slices.Values(c.entries) {
		if !e.IsFiltered {
			activeEntries = append(activeEntries, e)
		}
	}

	return activeEntries
}

func (c *chatWindow) applySearch() {
	var last *chatEntry
	for e := range slices.Values(c.entries) {
		e.Selected = false
		if len(c.searchInput.Value()) > 2 && c.entryMatchesSearch(e) {
			e.IsFiltered = false
			last = e
			continue
		}

		e.IsFiltered = true
	}

	if last != nil {
		last.Selected = true
	}

	c.recalculateLines()
	c.moveToBottom()
}

func (c *chatWindow) entryMatchesSearch(e *chatEntry) bool {
	cast, ok := e.Event.message.(*twitchirc.PrivateMessage)

	if !ok {
		return false
	}

	// search := c.searchInput.Value()
	// if fuzzy.RankMatchFold(search, cast.DisplayName) > 5 || fuzzy.RankMatchFold(search, cast.Message) > 6 {
	// 	return true
	// }

	search := strings.ToLower(c.searchInput.Value())
	if strings.Contains(strings.ToLower(cast.DisplayName), search) || strings.Contains(strings.ToLower(cast.Message), search) {
		return true
	}

	return false
}

func formatBadgeReplacement(settings save.Settings, replacements map[string]string) string {
	// Sort keys for deterministic badge order
	keys := slices.Sorted(maps.Keys(replacements))
	values := make([]string, 0, len(keys))
	for _, k := range keys {
		values = append(values, replacements[k])
	}

	if !settings.Chat.GraphicBadges {
		return fmt.Sprintf("[%s]", strings.Join(values, ","))
	}

	return strings.Join(values, "")
}
