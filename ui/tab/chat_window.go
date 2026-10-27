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
	*chat.Buffer

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
		Buffer:      chat.New(width, height, settings, theme),
		settings:    settings,
		keymap:      keymap,
		width:       width,
		height:      height,
		searchInput: input,
	}
}

func (c *chatWindow) Init() tea.Cmd {
	return nil
}

func (c *chatWindow) Update(msg tea.Msg) (*chatWindow, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	if cmd, handled := c.Buffer.Update(msg); handled {
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
			c.MessageDown(1)
			c.SnapScroll()
			return c, nil
		case key.Matches(keyMsg, c.keymap.Up):
			c.MessageUp(1)
			c.SnapScroll()
			return c, nil
		case key.Matches(keyMsg, c.keymap.PageUp):
			c.PageUp()
			c.SnapScroll()
			return c, nil
		case key.Matches(keyMsg, c.keymap.PageDown):
			c.PageDown()
			c.SnapScroll()
			return c, nil
		case key.Matches(keyMsg, c.keymap.GoToBottom):
			c.MoveToBottom()
			c.SnapScroll()
			return c, nil
		case key.Matches(keyMsg, c.keymap.GoToTop):
			c.MoveToTop()
			c.SnapScroll()
			return c, nil
		case key.Matches(keyMsg, c.keymap.CopyToClipboard):
			entry, ok := c.Selected()
			if !ok {
				return c, nil
			}

			if text := c.ClipboardText(entry.ID); text != "" {
				return c, tea.SetClipboard(text)
			}

			return c, nil
		case key.Matches(keyMsg, c.keymap.DumpChat):
			c.DebugDump()
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
	c.SetSearchActive(true)
	c.ClearFilter()
	c.SnapScroll()
	return c.searchInput.Focus()
}

func (c *chatWindow) handleStopSearchModeKeepSelected() {
	entry, ok := c.Selected()

	c.handleStopSearchMode()

	if ok {
		c.Select(entry.ID)
		c.SnapScroll()
	}
}

func (c *chatWindow) handleStopSearchMode() {
	c.state = viewChatWindowState
	c.searchInput.SetValue("")
	c.SetSearchActive(false)
	c.ClearFilter()
	c.MoveToBottom()
	c.SnapScroll()
}

func (c *chatWindow) View() string {
	height := c.height

	if c.state == searchChatWindowState {
		height--
	}

	rows, _ := c.Rows(height)

	if c.state == searchChatWindowState {
		searchLine := c.searchInput.View()
		status := c.FilterStatus()

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

// Resize updates dimensions.
func (c *chatWindow) Resize(width, height int) {
	c.width = width
	c.height = height
	c.searchInput.SetWidth(width)
	c.Buffer.Resize(width, height)
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

func (c *chatWindow) applySearch() {
	c.SetFilter(c.searchInput.Value())
}
