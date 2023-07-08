package chatui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type joinChannelCmd struct {
	channel string
}

type channelInputScreen struct {
	focused       bool
	width, height int
	input         textinput.Model
}

func newChannelInputScreen(width, height int) *channelInputScreen {
	input := textinput.New()
	input.Placeholder = "Channel"
	input.CharLimit = 25
	input.Focus()
	input.Validate = func(s string) error {
		for _, r := range s {
			if unicode.IsSpace(r) {
				return fmt.Errorf("white space not allowed")
			}
		}
		return nil
	}

	return &channelInputScreen{
		width:  width,
		height: height,
		input:  input,
	}
}

func (c *channelInputScreen) Init() tea.Cmd {
	return nil
}

func (c *channelInputScreen) Update(msg tea.Msg) (*channelInputScreen, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.height = msg.Height
		c.width = msg.Width
	}

	if c.focused {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				return c, func() tea.Msg {
					return joinChannelCmd{
						channel: c.input.Value(),
					}
				}
			}
		}
	}

	c.input, cmd = c.input.Update(msg)
	cmds = append(cmds, cmd)

	return c, tea.Batch(cmds...)
}

func (c *channelInputScreen) View() string {
	b := strings.Builder{}

	screenStyle := lipgloss.NewStyle().
		Width(c.width - 2).
		Height(c.height - 2).
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Center).
		Border(lipgloss.ThickBorder()).
		BorderForeground(lipgloss.Color("135"))

	label := lipgloss.NewStyle().MarginBottom(2).Foreground(lipgloss.Color("135")).Render("Enter a channel to join")

	b.WriteString(screenStyle.Render(lipgloss.JoinVertical(lipgloss.Left, label, c.input.View())))

	return b.String()
}

func (c *channelInputScreen) Focus() {
	c.focused = true
	c.input.Focus()
}

func (c *channelInputScreen) Blur() {
	c.focused = false
	c.input.Blur()
}
