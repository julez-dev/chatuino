package chatui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/save"
)

type accountProvider interface {
	GetAll() []save.Account
}

type joinChannelCmd struct {
	channel string
}

type listItem struct {
	title string
}

func (i listItem) Title() string       { return i.title }
func (i listItem) Description() string { return "" }
func (i listItem) FilterValue() string { return i.title }

type channelInputScreen struct {
	focused       bool
	width, height int
	input         textinput.Model
	list          list.Model

	accounts []save.Account
}

func newChannelInputScreen(width, height int, accountProvider accountProvider) *channelInputScreen {
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

	accounts := accountProvider.GetAll()
	listItems := make([]list.Item, 0, len(accounts))

	for _, a := range accounts {
		listItems = append(listItems, listItem{title: a.DisplayName})
	}
	list := list.New(listItems, list.NewDefaultDelegate(), width, 10)

	list.SetShowHelp(false)
	list.SetShowPagination(false)
	list.SetShowTitle(false)
	list.SetStatusBarItemName("account", "accounts")

	return &channelInputScreen{
		width:    width,
		height:   height,
		input:    input,
		accounts: accounts,
		list:     list,
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

	c.list, cmd = c.list.Update(msg)
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
	labelIdentity := lipgloss.NewStyle().MarginBottom(2).MarginTop(2).Foreground(lipgloss.Color("135")).Render("Choose a identity")

	b.WriteString(
		screenStyle.Render(
			lipgloss.JoinVertical(lipgloss.Left, label, c.input.View(), labelIdentity, c.list.View()),
		),
	)

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
