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

type currentTabInput int

const (
	channelInput currentTabInput = iota
	accountSelect
)

type accountProvider interface {
	GetAllWithAnonymous() []save.Account
}

type joinChannelCmd struct {
	channel string
	account save.Account
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
	selectedInput currentTabInput

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
	input.Prompt = " "

	accounts := accountProvider.GetAllWithAnonymous()
	listItems := make([]list.Item, 0, len(accounts))
	var mainAccountIndex int

	for i, a := range accounts {
		name := a.DisplayName
		if a.IsAnonymous {
			name = "Anonymous"
		}

		listItems = append(listItems, listItem{title: name})
		if a.IsMain {
			mainAccountIndex = i
		}
	}

	list := list.New(listItems, list.NewDefaultDelegate(), width, height-20)

	list.Select(mainAccountIndex)
	list.SetShowHelp(false)
	list.SetShowPagination(false)
	list.SetShowTitle(false)
	list.DisableQuitKeybindings()
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
			case "tab":
				if c.selectedInput == channelInput {
					c.selectedInput = accountSelect
				} else {
					c.selectedInput = channelInput
				}
			case "enter":
				return c, func() tea.Msg {
					return joinChannelCmd{
						channel: c.input.Value(),
						account: c.accounts[c.list.Cursor()],
					}
				}
			}
		}
	}

	if c.selectedInput == channelInput {
		c.input, cmd = c.input.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		c.list, cmd = c.list.Update(msg)
		cmds = append(cmds, cmd)
	}

	return c, tea.Batch(cmds...)
}

func (c *channelInputScreen) View() string {
	b := strings.Builder{}

	screenStyle := lipgloss.NewStyle().
		Width(c.width - 2).
		Height(c.height - 2).
		AlignHorizontal(lipgloss.Center).
		Border(lipgloss.ThickBorder()).
		BorderForeground(lipgloss.Color("135"))

	labelStyle := lipgloss.NewStyle().MarginBottom(2).MarginTop(2).Foreground(lipgloss.Color("135")).Render
	labelIdentityStyle := lipgloss.NewStyle().MarginBottom(2).MarginTop(2).Foreground(lipgloss.Color("135")).Render

	var (
		label         string
		labelIdentity string
	)

	if c.selectedInput == channelInput {
		label = labelStyle("> Enter a channel to join")
		labelIdentity = labelIdentityStyle("Choose a identity")
	} else {
		label = labelStyle("Enter a channel to join")
		labelIdentity = labelIdentityStyle("> Choose a identity")
	}

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
