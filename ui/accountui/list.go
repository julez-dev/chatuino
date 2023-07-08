package accountui

import (
	"time"

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

type setAccountMessage struct {
	err         error
	accountList save.AccountList
}

var baseStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.NormalBorder()).
	BorderForeground(lipgloss.Color("240"))

type List struct {
	accountList save.AccountList
	table       table.Model
	state       state
}

func NewList() List {
	columns := []table.Column{
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
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	return List{
		table: t,
	}
}

func (l List) Init() tea.Cmd {
	return func() tea.Msg {
		accountList, err := save.AccountListFromDisk()
		return setAccountMessage{
			err:         err,
			accountList: accountList,
		}
	}
}

func (l List) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case setAccountMessage:
		l.accountList = msg.accountList
		rows := make([]table.Row, 0, len(msg.accountList.Accounts))

		for _, acc := range msg.accountList.Accounts {
			rows = append(rows, table.Row{
				acc.DisplayName, time.Now().Local().Format("02.01.2006 15:04"),
			})
		}

		l.table.SetRows(rows)
	case tea.KeyMsg:
		switch msg.String() {
		case "f1":
			if l.state == inTable {
				l.state = inCreate
			} else {
				l.state = inTable
			}
		case "esc":
			if l.table.Focused() {
				l.table.Blur()
			} else {
				l.table.Focus()
			}
		case "q", "ctrl+c":
			return l, tea.Quit
		}
	}

	if l.state == inTable {
		l.table, cmd = l.table.Update(msg)
	}

	return l, cmd
}

func (l List) View() string {
	if l.state == inTable {
		return l.table.View()
	} else {
		return ""
	}
}
