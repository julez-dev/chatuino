package accountui

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/server"
)

type AccountProvider interface {
	GetAllAccounts() ([]save.Account, error)
	GetAccountBy(id string) (save.Account, error)
	UpdateTokensFor(id, accessToken, refreshToken string) error
	MarkAccountAsMain(id string) error
	Remove(id string) error
	Add(account save.Account) error
}

type state int

const (
	inTable state = iota
	inCreate
)

type setAccountsMessage struct {
	err      error
	accounts []save.Account
}

// var baseStyle = lipgloss.NewStyle().
//	BorderStyle(lipgloss.NormalBorder()).
//	BorderForeground(lipgloss.Color("240"))

type keyMapWithHelp struct {
	save.KeyMap
}

func (k keyMapWithHelp) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

func (k keyMapWithHelp) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Create, k.Remove, k.MarkLeader}, // first column
		{k.Help, k.Quit}, // second column
	}
}

type List struct {
	key             keyMapWithHelp
	accountProvider AccountProvider
	table           table.Model
	create          createModel
	tableHelp       help.Model
	state           state
	width, height   int
	err             error
	theme           save.Theme

	clientID, apiHost string
}

func NewList(clientID, apiHost string, accountProvider AccountProvider, keymap save.KeyMap, theme save.Theme) List {
	columns := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Main Account", Width: 15},
		{Title: "Login", Width: 15},
		{Title: "Date added", Width: 20},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color(theme.ListFontColor)).
		Background(lipgloss.Color(theme.ListBackgroundColor)).
		Bold(false)
	t.SetStyles(s)

	return List{
		apiHost:         apiHost,
		clientID:        clientID,
		accountProvider: accountProvider,
		key: keyMapWithHelp{
			KeyMap: keymap,
		},
		table:     t,
		tableHelp: help.New(),
		theme:     theme,
	}
}

func (l List) Init() tea.Cmd {
	return func() tea.Msg {
		accounts, err := fetchAccountsNonAnonymous(l.accountProvider)
		return setAccountsMessage{
			err:      err,
			accounts: accounts,
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

		if msg.err == nil {
			cmds = append(cmds, l.addNewAccountRefresh(msg.account))
		}

		return l, tea.Batch(cmds...)

	case setAccountsMessage:
		l.err = msg.err

		if l.err != nil {
			return l, nil
		}

		rows := make([]table.Row, 0, len(msg.accounts))

		for _, acc := range msg.accounts {
			rows = append(rows, table.Row{
				acc.ID, fmt.Sprintf("%v", acc.IsMain), acc.DisplayName, acc.CreatedAt.Local().Format("02.01.2006 15:04"),
			})
		}

		l.table.SetRows(rows)
		l.table.SetCursor(0)
	case tea.KeyMsg:
		l.err = nil
		switch {
		case key.Matches(msg, l.key.Create):
			if l.state == inTable {
				l.state = inCreate
				l.create = newCreateModel(l.width, l.height, l.clientID, l.apiHost, l.key.KeyMap, l.theme)
				return l, l.create.Init()
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
			display = fmt.Sprintf("failed: %s\n\n", l.err)
		}

		display = display + "\n" + l.table.View() + "\n" + l.tableHelp.View(l.key)

		return lipgloss.NewStyle().
			AlignHorizontal(lipgloss.Center).
			AlignVertical(lipgloss.Center).
			Width(l.width - 2).
			Height(l.height - 2).
			Render(display)
	} else {
		return l.create.View()
	}
}

func (l List) markAccountMain(id string) tea.Cmd {
	return func() tea.Msg {
		err := l.accountProvider.MarkAccountAsMain(id)
		if err != nil {
			return setAccountsMessage{
				err: err,
			}
		}

		accounts, err := fetchAccountsNonAnonymous(l.accountProvider)
		if err != nil {
			return setAccountsMessage{
				err: err,
			}
		}

		return setAccountsMessage{
			accounts: accounts,
		}
	}
}

func (l List) removeAccountRefresh(id string) tea.Cmd {
	return func() tea.Msg {
		acc, err := l.accountProvider.GetAccountBy(id)
		if err != nil {
			return setAccountsMessage{
				err: err,
			}
		}

		if acc.IsAnonymous {
			return nil
		}

		srv := server.NewClient(l.apiHost, nil)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		if err := srv.RevokeToken(ctx, acc.AccessToken); err != nil {
			return setAccountsMessage{
				err: err,
			}
		}

		if err := l.accountProvider.Remove(acc.ID); err != nil {
			return setAccountsMessage{
				err: err,
			}
		}

		accounts, err := fetchAccountsNonAnonymous(l.accountProvider)
		if err != nil {
			return setAccountsMessage{
				err: err,
			}
		}

		return setAccountsMessage{
			accounts: accounts,
		}
	}
}

func (l List) addNewAccountRefresh(account save.Account) tea.Cmd {
	return func() tea.Msg {
		var shouldSetMain bool

		if accounts, err := l.accountProvider.GetAllAccounts(); err == nil {
			accounts = slices.DeleteFunc(accounts, func(a save.Account) bool {
				return a.IsAnonymous
			})

			shouldSetMain = len(accounts) == 0
		}

		account.IsMain = shouldSetMain

		if err := l.accountProvider.Add(account); err != nil {
			return setAccountsMessage{
				err: err,
			}
		}

		accounts, err := fetchAccountsNonAnonymous(l.accountProvider)
		if err != nil {
			return setAccountsMessage{
				err: err,
			}
		}

		return setAccountsMessage{
			accounts: accounts,
		}
	}
}

func fetchAccountsNonAnonymous(provider AccountProvider) ([]save.Account, error) {
	accounts, err := provider.GetAllAccounts()
	if err != nil {
		return nil, err
	}

	accounts = slices.DeleteFunc(accounts, func(a save.Account) bool {
		return a.IsAnonymous
	})

	return accounts, nil
}
