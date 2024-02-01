package mainui

import (
	"fmt"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/save"
)

type currentJoinInput int

const (
	channelInput currentJoinInput = iota
	accountSelect
)

type listItem struct {
	title string
}

func (i listItem) Title() string       { return i.title }
func (i listItem) Description() string { return "" }
func (i listItem) FilterValue() string { return i.title }

type joinChannelMessage struct {
	channel string
	account save.Account
}

type setAccountsMessage struct {
	accounts []save.Account
}
type join struct {
	focused       bool
	width, height int
	input         textinput.Model
	list          list.Model
	selectedInput currentJoinInput
	accounts      []save.Account
	keymap        save.KeyMap
	provider      AccountProvider
}

func newJoin(provider AccountProvider, width, height int, keymap save.KeyMap) join {
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
	input.Prompt = ""

	list := list.New(nil, list.NewDefaultDelegate(), width, height/2)

	list.Select(0)
	list.SetShowHelp(false)
	list.SetShowPagination(false)
	list.SetShowTitle(false)
	list.DisableQuitKeybindings()
	list.SetStatusBarItemName("account", "accounts")

	return join{
		width:    width,
		height:   height,
		input:    input,
		provider: provider,
		list:     list,
		keymap:   keymap,
	}
}

func (j join) Init() tea.Cmd {
	return func() tea.Msg {
		accounts, err := j.provider.GetAllAccounts()
		if err != nil {
			return nil
		}

		for i, a := range accounts {
			if a.IsAnonymous {
				accounts[i].DisplayName = "Anonymous"
			}
		}

		return setAccountsMessage{accounts: accounts}
	}
}

func (j join) Update(msg tea.Msg) (join, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	if j.focused {
		switch msg := msg.(type) {
		case setAccountsMessage:
			j.accounts = msg.accounts
			listItems := make([]list.Item, 0, len(j.accounts))

			var index int
			for i, a := range j.accounts {
				listItems = append(listItems, listItem{title: a.DisplayName})

				if a.IsMain {
					index = i
				}
			}

			j.list.SetItems(listItems)
			j.list.Select(index)
			return j, nil
		case tea.KeyMsg:
			if key.Matches(msg, j.keymap.Next) {
				if j.selectedInput == channelInput {
					j.selectedInput = accountSelect
				} else {
					j.selectedInput = channelInput
				}
			}

			if key.Matches(msg, j.keymap.Confirm) {
				return j, func() tea.Msg {
					return joinChannelMessage{
						channel: j.input.Value(),
						account: j.accounts[j.list.Cursor()],
					}
				}
			}
		}
	}

	if j.selectedInput == channelInput {
		j.input, cmd = j.input.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		j.list, cmd = j.list.Update(msg)
		cmds = append(cmds, cmd)
	}

	return j, tea.Batch(cmds...)
}

func (j join) View() string {
	style := lipgloss.NewStyle().
		Width(j.width - 2). // - border width
		MaxWidth(j.width).
		Height(j.height - 2). // - border height
		MaxHeight(j.height).
		Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("135")).
		AlignHorizontal(lipgloss.Center)
	// AlignVertical(lipgloss.Center)

	labelStyle := lipgloss.NewStyle().MarginBottom(2).MarginTop(2).Foreground(lipgloss.Color("135")).Render
	labelIdentityStyle := lipgloss.NewStyle().MarginBottom(2).MarginTop(2).Foreground(lipgloss.Color("135")).Render

	var (
		label         string
		labelIdentity string
	)

	if j.selectedInput == channelInput {
		label = labelStyle("> Enter a channel to join")
		labelIdentity = labelIdentityStyle("Choose an identity")
	} else {
		label = labelStyle("Enter a channel to join")
		labelIdentity = labelIdentityStyle("> Choose an identity")
	}

	return style.Render(label + "\n" + j.input.View() + "\n" + labelIdentity + "\n" + j.list.View())
}

func (c *join) focus() {
	c.focused = true
	c.input.Focus()
}

func (c *join) blur() {
	c.focused = false
	c.input.Blur()
}
