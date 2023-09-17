package accountui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/save"
)

type state int

const (
	inTable state = iota
	inCreate
)

type setAccountListMessage struct {
	err         error
	accountList save.AccountList
}

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

type keyMap struct {
	Up         key.Binding
	Down       key.Binding
	Create     key.Binding
	Remove     key.Binding
	MarkLeader key.Binding
	Help       key.Binding
	Quit       key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Create, k.Remove, k.MarkLeader}, // first column
		{k.Help, k.Quit}, // second column
	}
}

type List struct {
	key           keyMap
	accountList   save.AccountList
	table         table.Model
	create        createModel
	tableHelp     help.Model
	state         state
	width, height int
	err           error
}

func NewList() List {
	columns := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Is main", Width: 10},
		{Title: "User", Width: 20},
		{Title: "Created", Width: 20},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("135")).
		Bold(false)
	t.SetStyles(s)

	return List{
		key: keyMap{
			Up:   t.KeyMap.LineUp,
			Down: t.KeyMap.LineDown,
			Create: key.NewBinding(
				key.WithKeys("f1"),
				key.WithHelp("f1", "register new account"),
			),
			Remove: key.NewBinding(
				key.WithKeys("r"),
				key.WithHelp("r", "remove selected account"),
			),
			MarkLeader: key.NewBinding(
				key.WithKeys("m"),
				key.WithHelp("m", "mark selected account as main"),
			),
			Help: key.NewBinding(
				key.WithKeys("?"),
				key.WithHelp("?", "toggle help"),
			),
			Quit: key.NewBinding(
				key.WithKeys("esc", "ctrl+c"),
				key.WithHelp("esc", "quit"),
			),
		},
		table:     t,
		tableHelp: help.New(),
	}
}

func (l List) Init() tea.Cmd {
	return func() tea.Msg {
		accountList, err := save.AccountListFromDisk()
		return setAccountListMessage{
			err:         err,
			accountList: accountList,
		}
	}
}

func (l List) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	if l.state == inTable {
		l.table, cmd = l.table.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		l.create, cmd = l.create.Update(msg)
		cmds = append(cmds, cmd)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		l.tableHelp.Width = msg.Width
		l.width = msg.Width
		l.height = msg.Height
		l.create.width = msg.Width
		l.create.height = msg.Height
		l.table.SetWidth(msg.Width)
	case setAccountMessage:
		l.err = msg.err
		l.state = inTable
		cmds = append(cmds, l.addNewAccountRefresh(msg.account))
		return l, tea.Batch(cmds...)

	case setAccountListMessage:
		l.err = msg.err
		l.accountList = msg.accountList
		rows := make([]table.Row, 0, len(msg.accountList.Accounts))

		for _, acc := range msg.accountList.GetAll() {
			rows = append(rows, table.Row{
				acc.ID, fmt.Sprintf("%v", acc.IsMain), acc.DisplayName, acc.CreatedAt.Local().Format("02.01.2006 15:04"),
			})
		}

		l.table.SetRows(rows)
		l.table.SetCursor(0)
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, l.key.Create):
			if l.state == inTable {
				l.state = inCreate
				l.create = newCreateModel(l.width, l.height)
			} else {
				l.state = inTable
			}
		case key.Matches(msg, l.key.Remove):
			if l.state == inTable {
				curr := l.table.SelectedRow()
				if curr != nil {
					cmds = append(cmds, l.removeAccountRefresh(curr[0]))
					return l, tea.Batch(cmds...)
				}
			}
		case key.Matches(msg, l.key.MarkLeader):
			if l.state == inTable {
				curr := l.table.SelectedRow()
				if curr != nil {
					cmds = append(cmds, l.markAccountMain(curr[0]))
					return l, tea.Batch(cmds...)
				}
			}
		case key.Matches(msg, l.key.Help):
			l.tableHelp.ShowAll = !l.tableHelp.ShowAll
		case key.Matches(msg, l.key.Quit):
			return l, tea.Quit
		}
	}

	return l, tea.Batch(cmds...)
}

func (l List) View() string {
	if l.state == inTable {
		display := ""
		if l.err != nil {
			display = fmt.Sprintf("got error: %s\n\n", l.err)
		}

		display = display + l.table.View() + "\n" + l.tableHelp.View(l.key)

		return lipgloss.NewStyle().
			AlignHorizontal(lipgloss.Center).
			AlignVertical(lipgloss.Center).
			Width(l.width - 2).
			Height(l.height - 2).
			Border(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color("135")).
			Render(display)
	} else {
		return l.create.View()
	}
}

func (l List) markAccountMain(id string) tea.Cmd {
	return func() tea.Msg {
		list, err := save.AccountListFromDisk()
		if err != nil {
			return setAccountListMessage{
				err: err,
			}
		}

		list.MarkAsMain(id)

		err = list.Save()
		if err != nil {
			return setAccountListMessage{
				err: err,
			}
		}

		return setAccountListMessage{
			accountList: list,
		}
	}
}

func (l List) removeAccountRefresh(id string) tea.Cmd {
	return func() tea.Msg {
		list, err := save.AccountListFromDisk()
		if err != nil {
			return setAccountListMessage{
				err: err,
			}
		}

		list.Remove(id)

		err = list.Save()
		if err != nil {
			return setAccountListMessage{
				err: err,
			}
		}

		return setAccountListMessage{
			accountList: list,
		}
	}
}

func (l List) addNewAccountRefresh(account save.Account) tea.Cmd {
	return func() tea.Msg {
		list, err := save.AccountListFromDisk()
		if err != nil {
			return setAccountListMessage{
				err: err,
			}
		}

		// If this is the first account, add as admin account
		if len(list.Accounts) < 1 {
			account.IsMain = true
		}

		list.Upsert(account)

		err = list.Save()
		if err != nil {
			return setAccountListMessage{
				err: err,
			}
		}

		return setAccountListMessage{
			accountList: list,
		}
	}
}
