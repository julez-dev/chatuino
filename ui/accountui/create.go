package accountui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/twitch"
)

type createState int

const (
	input createState = iota
	loading
	finished
)

type setAccountMessage struct {
	account save.Account
	err     error
}

type createModel struct {
	state     createState
	textinput textinput.Model
	spinner   spinner.Model

	width, height     int
	clientID, apiHost string

	err     error
	account save.Account
}

func newCreateModel(width, height int, clientID, apiHost string) createModel {
	ti := textinput.New()
	ti.Placeholder = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx%xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	ti.Focus()

	s := spinner.New()

	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return createModel{
		clientID:  clientID,
		apiHost:   apiHost,
		textinput: ti,
		spinner:   s,
		width:     width,
		height:    height,
	}
}

func (c createModel) Init() tea.Cmd {
	return nil
}

func (c createModel) Update(msg tea.Msg) (createModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case setAccountMessage:
		if msg.err != nil {
			c.err = msg.err
			c.state = finished
			return c, nil
		}

		c.account = msg.account
		c.state = finished
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if c.state == input {
				cmd = c.handleSent(c.textinput.Value())
				c.state = loading
				return c, tea.Batch(cmd, c.spinner.Tick)
			}
		}
	}

	if c.state == loading {
		c.spinner, cmd = c.spinner.Update(msg)
		return c, cmd
	}

	if c.state == input {
		c.textinput, cmd = c.textinput.Update(msg)
	}

	return c, cmd
}

func (c createModel) View() string {
	view := ""
	switch c.state {
	case input:
		view = fmt.Sprintf(
			"Please enter the Access Token + Refresh Token combination.\n\n%s",
			c.textinput.View(),
		) + "\n"
	case loading:
		view = c.spinner.View() + " Loading user information"
	case finished:
		if c.err != nil {
			view = "Got error while creating account " + c.err.Error()
			break
		}

		view = "Successfully got account data for " + c.account.DisplayName
	}

	return lipgloss.NewStyle().
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Center).
		Width(c.width - 2).
		Height(c.height - 2).
		Border(lipgloss.ThickBorder()).
		BorderForeground(lipgloss.Color("135")).
		Render(view)
}

func (c createModel) handleSent(input string) tea.Cmd {
	return func() tea.Msg {
		split := strings.SplitN(input, "%", 2)

		if len(split) != 2 {
			return setAccountMessage{
				err: fmt.Errorf("got invalid input"),
			}
		}

		tmpAccount := &save.Account{
			ID:           "temp-static-account",
			AccessToken:  split[0],
			RefreshToken: split[1],
		}

		api, err := twitch.NewAPI(
			c.clientID,
			twitch.WithUserAuthentication(newStaticAccountProvider(tmpAccount),
				server.NewClient(c.apiHost, nil),
				tmpAccount.ID,
			),
		)
		if err != nil {
			return setAccountMessage{
				err: err,
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		resp, err := api.GetUsers(ctx, nil, nil)
		if err != nil {
			return setAccountMessage{
				err: err,
			}
		}

		if len(resp.Data) != 1 {
			return setAccountMessage{
				err: fmt.Errorf("got invalid accounts"),
			}
		}

		return setAccountMessage{
			account: save.Account{
				ID:           resp.Data[0].ID,
				DisplayName:  resp.Data[0].DisplayName,
				AccessToken:  tmpAccount.AccessToken,
				RefreshToken: tmpAccount.RefreshToken,
				CreatedAt:    time.Now(),
			},
		}
	}
}
