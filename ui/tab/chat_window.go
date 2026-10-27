package tab

import (
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/chat"
)

type chatWindowState int

const (
	viewChatWindowState chatWindowState = iota
	searchChatWindowState
)

// chatWindow is the tab shell around a chat transcript. It owns the search
// widget, key handling and search mode; the transcript owns entries, rows,
// selection, viewport, filtering and the renderer.
type chatWindow struct {
	buffer *chat.Buffer

	settings save.Settings
	keymap   save.KeyMap

	width, height int
	focused       bool
	state         chatWindowState

	searchInput textinput.Model
}

func newChatWindow(width, height int, settings save.Settings, theme save.Theme, keymap save.KeyMap) *chatWindow {
	input := textinput.New()
	input.CharLimit = 128
	input.Prompt = "  /"
	input.Placeholder = "search — content: user: /regex/ badge: is:mod|sub|vip|first"
	styles := input.Styles()
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.InputPromptColor))
	styles.Cursor.BlinkSpeed = time.Millisecond * 750
	input.SetStyles(styles)
	input.SetWidth(width)

	return &chatWindow{
		buffer:      chat.New(width, height, settings, theme),
		settings:    settings,
		keymap:      keymap,
		width:       width,
		height:      height,
		searchInput: input,
	}
}

// UsePalette shares another transcript's user colors.
func (c *chatWindow) UsePalette(palette *chat.Palette) {
	c.buffer.UsePalette(palette)
}

// Palette returns the shared user-color palette.
func (c *chatWindow) Palette() *chat.Palette {
	return c.buffer.Palette()
}

// SetTimeFormat changes the timestamp format of rendered messages.
func (c *chatWindow) SetTimeFormat(format string) {
	c.buffer.SetTimeFormat(format)
}

func (c *chatWindow) Init() tea.Cmd {
	return nil
}

func (c *chatWindow) Update(msg tea.Msg) (*chatWindow, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	if cmd, handled := c.buffer.Update(msg); handled {
		return c, cmd
	}

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && c.focused {
		switch {
		case key.Matches(keyMsg, c.keymap.SearchMode) && c.state != searchChatWindowState:
			return c, c.handleStartSearchMode()
		case key.Matches(keyMsg, c.keymap.Escape) && c.state == searchChatWindowState:
			c.handleStopSearchMode()
			return c, nil
		case key.Matches(keyMsg, c.keymap.Confirm) && c.state == searchChatWindowState:
			c.handleStopSearchModeKeepSelected()
			return c, nil
		case c.state == searchChatWindowState && keyMsg.String() != "up" && keyMsg.String() != "down":
			c.searchInput, cmd = c.searchInput.Update(keyMsg)
			c.applySearch()
			cmds = append(cmds, cmd)
			return c, tea.Batch(cmds...)
		case key.Matches(keyMsg, c.keymap.Down):
			c.buffer.MessageDown(1)
			c.buffer.SnapScroll()
			return c, nil
		case key.Matches(keyMsg, c.keymap.Up):
			c.buffer.MessageUp(1)
			c.buffer.SnapScroll()
			return c, nil
		case key.Matches(keyMsg, c.keymap.PageUp):
			c.buffer.PageUp()
			c.buffer.SnapScroll()
			return c, nil
		case key.Matches(keyMsg, c.keymap.PageDown):
			c.buffer.PageDown()
			c.buffer.SnapScroll()
			return c, nil
		case key.Matches(keyMsg, c.keymap.GoToBottom):
			c.buffer.MoveToBottom()
			c.buffer.SnapScroll()
			return c, nil
		case key.Matches(keyMsg, c.keymap.GoToTop):
			c.buffer.MoveToTop()
			c.buffer.SnapScroll()
			return c, nil
		case key.Matches(keyMsg, c.keymap.CopyToClipboard):
			entry, ok := c.buffer.Selected()
			if !ok {
				return c, nil
			}

			if text := c.buffer.ClipboardText(entry.ID); text != "" {
				return c, tea.SetClipboard(text)
			}

			return c, nil
		case key.Matches(keyMsg, c.keymap.DumpChat):
			c.buffer.DebugDump()
		}
	}

	if c.state == searchChatWindowState && c.focused {
		c.searchInput, cmd = c.searchInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return c, tea.Batch(cmds...)
}

func (c *chatWindow) handleStartSearchMode() tea.Cmd {
	c.state = searchChatWindowState
	c.searchInput.Focus()
	c.buffer.SetSearchActive(true)
	c.buffer.ClearFilter()
	c.buffer.SnapScroll()
	return c.searchInput.Focus()
}

func (c *chatWindow) handleStopSearchModeKeepSelected() {
	entry, ok := c.buffer.Selected()

	c.handleStopSearchMode()

	if ok {
		c.buffer.Select(entry.ID)
		c.buffer.SnapScroll()
	}
}

func (c *chatWindow) handleStopSearchMode() {
	c.state = viewChatWindowState
	c.searchInput.SetValue("")
	c.buffer.SetSearchActive(false)
	c.buffer.ClearFilter()
	c.buffer.MoveToBottom()
	c.buffer.SnapScroll()
}

func (c *chatWindow) View() string {
	height := c.height

	if c.state == searchChatWindowState {
		height--
	}

	rows, _ := c.buffer.Rows(height)

	if c.state == searchChatWindowState {
		searchLine := c.searchInput.View()
		status := c.buffer.FilterStatus()

		switch {
		case status.Err != "":
			searchLine += "  [!] " + status.Err
		case status.Hint != "":
			searchLine += "  " + status.Hint
		case status.Matches > 0:
			searchLine += "  " + strconv.Itoa(status.Matches) + " matches"
		}

		return searchLine + "\n" + strings.Join(rows, "\n")
	}

	return strings.Join(rows, "\n")
}

// handleChatClick resolves controls on top of the chat rows.
func (c *chatWindow) handleChatClick(row, col int) bool {
	return c.buffer.HandleChatClick(row, col)
}

// Resize updates dimensions.
func (c *chatWindow) Resize(width, height int) {
	c.width = width
	c.height = height
	c.searchInput.SetWidth(width)
	c.buffer.Resize(width, height)
}

func (c *chatWindow) Focus() {
	c.focused = true
}

func (c *chatWindow) Blur() {
	c.focused = false
}

// Focused reports whether this window currently owns the selection.
func (c *chatWindow) Focused() bool {
	return c.focused
}

// entryForCurrentCursor returns the selected entry.
func (c *chatWindow) entryForCurrentCursor() (int, *chat.Entry) {
	return c.buffer.Current()
}

// entryAtLine returns the entry containing an absolute rendered line.
func (c *chatWindow) entryAtLine(line int) (int, *chat.Entry) {
	return c.buffer.EntryAtLine(line)
}

// selectEntry selects one entry.
func (c *chatWindow) selectEntry(entry *chat.Entry) {
	if entry == nil {
		return
	}

	c.buffer.Select(entry.ID)
}

// moveSelectionByLines moves the selection by n rendered lines.
func (c *chatWindow) moveSelectionByLines(n int) {
	c.buffer.Move(n)
}

func (c *chatWindow) pageUp() {
	c.buffer.PageUp()
}

func (c *chatWindow) pageDown() {
	c.buffer.PageDown()
}

func (c *chatWindow) scrollByLines(n int) {
	c.buffer.ScrollByLines(n)
}

func (c *chatWindow) viewportRenderStart() int {
	return c.buffer.ViewportRenderStart()
}

func (c *chatWindow) emoteNameAt(row, col int) (string, bool) {
	return c.buffer.EmoteNameAt(row, col)
}

func (c *chatWindow) selectLine(line int) {
	c.buffer.SelectLine(line)
}

func (c *chatWindow) selectAtViewportRow(row int) {
	c.buffer.SelectAtViewportRow(row)
}

func (c *chatWindow) goToEntry(entry *chat.Entry) {
	if entry == nil {
		return
	}

	c.buffer.Select(entry.ID)
	c.buffer.SnapScroll()
}

func (c *chatWindow) messageDown(n int) {
	c.buffer.MessageDown(n)
}

func (c *chatWindow) messageUp(n int) {
	c.buffer.MessageUp(n)
}

func (c *chatWindow) moveToBottom() {
	c.buffer.MoveToBottom()
}

func (c *chatWindow) moveToTop() {
	c.buffer.MoveToTop()
}

func (c *chatWindow) getNewestEntry() *chat.Entry {
	return c.buffer.NewestPtr()
}

func (c *chatWindow) handleMessage(msg chat.ChatEventMessage) tea.Cmd {
	return c.buffer.Append(msg)
}

func (c *chatWindow) updateNotice(id, message string) bool {
	return c.buffer.ReplaceNotice(id, message)
}

func (c *chatWindow) clipboardText(entry *chat.Entry) string {
	if entry == nil {
		return ""
	}

	return c.buffer.ClipboardText(entry.ID)
}

func (c *chatWindow) atTail() bool {
	return c.buffer.CaughtUp()
}

func (c *chatWindow) caughtUp() bool {
	return c.buffer.CaughtUp()
}

func (c *chatWindow) Entries() []chat.Entry {
	return c.buffer.Entries()
}

func (c *chatWindow) LineRange(entry *chat.Entry) (int, int, bool) {
	if entry == nil {
		return 0, 0, false
	}

	return c.buffer.LineRange(entry.ID)
}

func (c *chatWindow) debugDumpChat() {
	c.buffer.DebugDump()
}

func (c *chatWindow) applySearch() {
	c.buffer.SetFilter(c.searchInput.Value())
}

func (c *chatWindow) UpdatePort() {
	c.buffer.UpdatePort()
}

func (c *chatWindow) SnapScroll() {
	c.buffer.SnapScroll()
}

func (c *chatWindow) Selected() (chat.Entry, bool) {
	return c.buffer.Selected()
}

func (c *chatWindow) RenderedRows() []string {
	rows, _ := c.buffer.Rows(c.height)
	return rows
}

func (c *chatWindow) RawRows() []string {
	return c.buffer.RawRows()
}

func (c *chatWindow) NewMessages() int {
	return c.buffer.NewMessages()
}

func (c *chatWindow) StartOf(entry *chat.Entry) int {
	start, _, _ := c.buffer.LineRange(entryID(entry))
	return start
}

func (c *chatWindow) EndOf(entry *chat.Entry) int {
	_, end, _ := c.buffer.LineRange(entryID(entry))
	return end
}

func entryID(entry *chat.Entry) chat.EntryID {
	if entry == nil {
		return 0
	}

	return entry.ID
}

func (c *chatWindow) TimePaddingWidth() int {
	return c.buffer.TimePaddingWidth()
}

func (c *chatWindow) CursorLine() int {
	return c.buffer.CursorLine()
}

func (c *chatWindow) newMessageBadge() (string, int) {
	return c.buffer.Badge()
}

func (c *chatWindow) newMessageBadgeSpan(renderStart, height, width int) (int, int, bool) {
	return c.buffer.BadgeSpan(renderStart, height, width)
}

func (c *chatWindow) newMessageBadgeHit(row, col int) bool {
	return c.buffer.BadgeHit(row, col)
}
