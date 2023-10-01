package mainui

import (
	"context"
	"errors"
	"fmt"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/rs/zerolog"
	"io"
	"os"
)

type chatConnectionInitiatedMessage struct {
	targetID string
	err      error

	chat         *twitch.Chat
	messagesOut  chan<- twitch.IRCer
	messagesRecv <-chan twitch.IRCer
	errRecv      <-chan error
}

type recvTwitchMessage struct {
	targetID string
	message  twitch.IRCer
}

type setErrorMessage struct {
	targetID string
	err      error
}

type setChannelDataMessage struct {
	targetID  string
	channel   string
	channelID string
}

type tab struct {
	id      string
	account save.Account
	focused bool

	channelDataLoaded bool
	channel           string
	emoteStore        EmoteStore

	width, height int

	// twitch chat connection
	chat         *twitch.Chat
	messagesOut  chan<- twitch.IRCer
	messagesRecv <-chan twitch.IRCer

	ttvAPI *twitch.API

	// internal cancelation
	ctx        context.Context
	cancelFunc context.CancelFunc

	// components
	chatWindow   chatWindow
	messageInput textinput.Model

	err error
}

func newTab(id string, channel string, width, height int, emoteStore EmoteStore, account save.Account) tab {
	ttvAPI := twitch.NewAPI(nil, account.AccessToken, os.Getenv("TWITCH_CLIENT_ID"))

	ctx, cancel := context.WithCancel(context.Background())

	input := textinput.New()
	input.PromptStyle = input.PromptStyle.Foreground(lipgloss.Color("135"))

	return tab{
		id:           id,
		width:        width,
		height:       height,
		account:      account,
		ctx:          ctx,
		cancelFunc:   cancel,
		channel:      channel,
		emoteStore:   emoteStore,
		ttvAPI:       ttvAPI,
		messageInput: input,
	}
}

func (t tab) Init() tea.Cmd {
	var cmds []tea.Cmd

	cmds = append(cmds, func() tea.Msg {
		in := make(chan twitch.IRCer)
		chat := twitch.NewChat()

		out, errChan, err := chat.Connect(t.ctx, in, t.account.DisplayName, t.account.AccessToken)
		if err != nil {
			close(in)
			return chatConnectionInitiatedMessage{
				targetID: t.id,
				err:      err,
			}
		}

		in <- twitch.JoinMessage{Channel: t.channel}

		go func() {
			<-t.ctx.Done()
			close(in)
		}()

		return chatConnectionInitiatedMessage{
			targetID:     t.id,
			errRecv:      errChan,
			messagesOut:  in,
			messagesRecv: out,
			chat:         chat,
		}
	})

	cmds = append(cmds, func() tea.Msg {

		userData, err := t.ttvAPI.GetUsers(t.ctx, []string{t.channel}, nil)
		if err != nil {
			return setErrorMessage{
				targetID: t.id,
				err:      err,
			}
		}

		if len(userData.Data) < 1 {
			return setErrorMessage{
				targetID: t.id,
				err:      fmt.Errorf("could not find channel: %s", t.channel),
			}
		}

		// refresh emote set for joined channel
		if err := t.emoteStore.RefreshLocal(t.ctx, userData.Data[0].ID); err != nil {
			return setErrorMessage{
				targetID: t.id,
				err:      err,
			}
		}

		return setChannelDataMessage{
			targetID:  t.id,
			channelID: userData.Data[0].ID,
			channel:   userData.Data[0].DisplayName,
		}
	})

	return tea.Batch(cmds...)
}

func (t tab) Update(msg tea.Msg) (tab, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case setErrorMessage:
		if msg.targetID != t.id {
			return t, nil
		}

		t.err = errors.Join(t.err, msg.err)
		return t, nil
	case recvTwitchMessage:
		if msg.targetID != t.id {
			return t, nil
		}

		t.chatWindow, cmd = t.chatWindow.Update(msg)

		return t, tea.Batch(t.waitTwitchMessage(), cmd)
	case setChannelDataMessage:
		if msg.targetID != t.id {
			return t, nil
		}

		t.channelDataLoaded = true

		t.chatWindow = newChatWindow(zerolog.New(io.Discard), t.id, t.width, t.height, t.channel, msg.channelID, t.emoteStore)
		return t, nil
	case chatConnectionInitiatedMessage:
		if msg.targetID != t.id {
			return t, nil
		}

		if msg.err != nil {
			t.err = msg.err
			return t, nil
		}

		cmds := make([]tea.Cmd, 0, 2)
		t.chat = msg.chat
		t.messagesRecv = msg.messagesRecv
		t.messagesOut = msg.messagesOut

		cmds = append(cmds, t.waitTwitchMessage())
		cmds = append(cmds, func() tea.Msg {
			select {
			case err := <-msg.errRecv:
				return setErrorMessage{
					targetID: t.id,
					err:      err,
				}
			case <-t.ctx.Done():
				return nil
			}
		})

		return t, tea.Batch(cmds...)
	}

	t.chatWindow, cmd = t.chatWindow.Update(msg)
	cmds = append(cmds, cmd)

	return t, tea.Batch(cmds...)
}

func (t tab) View() string {
	if t.err != nil {
		return lipgloss.NewStyle().
			Width(t.width).
			Height(t.height).
			MaxWidth(t.width).
			MaxHeight(t.height).
			AlignHorizontal(lipgloss.Center).
			AlignVertical(lipgloss.Center).
			Render(t.err.Error())
	}

	if !t.channelDataLoaded {
		return lipgloss.NewStyle().
			Width(t.width).
			Height(t.height).
			MaxWidth(t.width).
			MaxHeight(t.height).
			AlignHorizontal(lipgloss.Center).
			AlignVertical(lipgloss.Center).
			Render("Fetching channel data...")
	}

	inputView := lipgloss.NewStyle().
		Width(t.chatWindow.width - 2). // width of the chat window minus the border
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("135")).
		Render(t.messageInput.View())

	_ = inputView

	return lipgloss.JoinVertical(
		lipgloss.Left,
		t.chatWindow.View(),
		//inputView,
	)
}

func (r *tab) Close() error {
	r.cancelFunc()
	return r.ctx.Err()
}

func (t tab) waitTwitchMessage() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg, ok := <-t.messagesRecv:
			if !ok {
				return nil
			}

			return recvTwitchMessage{
				targetID: t.id,
				message:  msg,
			}
		case <-t.ctx.Done():
			return nil
		}
	}
}

func (t *tab) handleResize() {
	heightMessageInput := lipgloss.Height(t.messageInput.View())

	t.chatWindow.height = t.height - heightMessageInput - 50
	t.chatWindow.width = t.width
	t.chatWindow.recalculateLines()
}

func (t *tab) focus() {
	t.focused = true
	t.chatWindow.Focus()
}

func (t *tab) blur() {
	t.focused = false
	t.chatWindow.Blur()
}
