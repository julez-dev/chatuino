package mainui

import (
	"fmt"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/save"
	"unicode"
)

type JoinKeyMap struct {
	FieldSelect key.Binding
	Enter       key.Binding
}

func buildDefaultJoinKeyMap() JoinKeyMap {
	return JoinKeyMap{
		FieldSelect: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "Select next field"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "Confirm select"),
		),
	}
}

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

type join struct {
	focused       bool
	width, height int
	input         textinput.Model
	list          list.Model
	selectedInput currentJoinInput

	keymap   JoinKeyMap
	accounts []save.Account
}

func newJoin(accounts []save.Account, width, height int) join {
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

	list := list.New(listItems, list.NewDefaultDelegate(), width, height/2)

	list.Select(mainAccountIndex)
	list.SetShowHelp(false)
	list.SetShowPagination(false)
	list.SetShowTitle(false)
	list.DisableQuitKeybindings()
	list.SetStatusBarItemName("account", "accounts")

	return join{
		width:    width,
		height:   height,
		input:    input,
		accounts: accounts,
		list:     list,
		keymap:   buildDefaultJoinKeyMap(),
	}
}

func (j join) Init() tea.Cmd {
	return nil
}

func (j join) Update(msg tea.Msg) (join, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	if j.focused {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if key.Matches(msg, j.keymap.FieldSelect) {
				if j.selectedInput == channelInput {
					j.selectedInput = accountSelect
				} else {
					j.selectedInput = channelInput
				}
			}

			if key.Matches(msg, j.keymap.Enter) {
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
	//AlignVertical(lipgloss.Center)

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
