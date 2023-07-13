package chatui

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/rs/zerolog"
)

type setErrorMessage struct {
	target uuid.UUID
	err    error
}

type setChatInstanceMessage struct {
	target       uuid.UUID
	chat         *twitch.Chat
	messagesOut  chan<- twitch.IRCer
	messagesRecv <-chan twitch.IRCer
	errRecv      <-chan error
}

type setChannelIDMessage struct {
	target    uuid.UUID
	channelID string
}

type recvTwitchMessage struct {
	target  uuid.UUID
	message twitch.IRCer
}

type removeTabMessage struct {
	id uuid.UUID
}

type tabState int

const (
	inChatWindow tabState = iota
	insertMode
)

type tab struct {
	id      uuid.UUID
	ctx     context.Context
	cancel  context.CancelFunc
	focused bool
	err     error

	width, height int

	channel   string
	channelID string

	logger zerolog.Logger

	chat         *twitch.Chat
	messagesOut  chan<- twitch.IRCer
	messagesRecv <-chan twitch.IRCer
	messageLog   []string

	state tabState

	channelInfo  *channelInfo
	chatWindow   *chatWindow
	messageInput textinput.Model

	emoteStore emoteStore
	ttvAPI     twitchAPI
	account    save.Account
}

func newTab(ctx context.Context, logger zerolog.Logger, channel string, emoteStore emoteStore, account save.Account) *tab {
	ctx, cancel := context.WithCancel(ctx)

	input := textinput.New()
	input.PromptStyle = input.PromptStyle.Foreground(lipgloss.Color("135"))

	ttvAPI := twitch.NewAPI(nil, account.AccessToken, os.Getenv("TWITCH_CLIENT_ID"))
	tab := &tab{
		id:           uuid.New(),
		ctx:          ctx,
		cancel:       cancel,
		logger:       logger,
		channel:      channel,
		messageInput: input,
		emoteStore:   emoteStore,
		ttvAPI:       ttvAPI,
		account:      account,
	}

	tab.channelInfo = newChannelInfo(ctx, logger, ttvAPI, channel)

	tab.chatWindow = &chatWindow{
		parentTab: tab,
		logger:    logger,
	}

	return tab
}

func (t *tab) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, 2)

	cmds = append(cmds, func() tea.Msg {
		in := make(chan twitch.IRCer)

		go func() {
			<-t.ctx.Done()
			close(in)
		}()

		chat := twitch.NewChat()

		out, errChan, err := chat.Connect(t.ctx, in, t.account.DisplayName, t.account.AccessToken)
		if err != nil {
			t.logger.Err(err).Send()
			return nil
		}

		in <- twitch.JoinMessage{
			Channel: t.channel,
		}

		return setChatInstanceMessage{
			messagesOut:  in,
			errRecv:      errChan,
			target:       t.id,
			chat:         chat,
			messagesRecv: out,
		}
	})

	cmds = append(cmds, func() tea.Msg {
		userData, err := t.ttvAPI.GetUsers(t.ctx, []string{t.channel}, nil)
		if err != nil {
			t.logger.Err(err).Send()
			return nil
		}

		// refresh emote set for joined channel
		if err := t.emoteStore.RefreshLocal(t.ctx, userData.Data[0].ID); err != nil {
			t.logger.Err(err).Send()
			return nil
		}

		return setChannelIDMessage{
			target:    t.id,
			channelID: userData.Data[0].ID,
		}
	})

	cmds = append(cmds, t.channelInfo.Init())

	return tea.Batch(cmds...)
}

func (t *tab) Update(msg tea.Msg) (*tab, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case resizeTabContainerMessage:
		t.chatWindow.viewport.Height = msg.Height
		t.chatWindow.viewport.Width = msg.Width
		t.channelInfo.width = msg.Width

		t.width = msg.Width
		t.height = msg.Height
	case setChatInstanceMessage:
		if msg.target == t.id {
			t.chat = msg.chat
			t.messagesRecv = msg.messagesRecv
			t.messagesOut = msg.messagesOut
			cmds = append(cmds, waitMessage(*t))

			cmds = append(cmds, func() tea.Msg {
				select {
				case err := <-msg.errRecv:
					return setErrorMessage{
						target: t.id,
						err:    err,
					}
				case <-t.ctx.Done():
					return setErrorMessage{
						target: t.id,
						err:    t.ctx.Err(),
					}
				}
			})
		}
	case recvTwitchMessage:
		if msg.target == t.id {
			cmds = append(cmds, waitMessage(*t))
		}
	case setErrorMessage:
		if msg.target == t.id {
			t.err = msg.err
		}
	}

	if t.focused {
		if t.state == insertMode {
			t.messageInput, cmd = t.messageInput.Update(msg)
			cmds = append(cmds, cmd)
		}

		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "i":
				t.state = insertMode
				t.messageInput.Focus()
				t.chatWindow.Blur()
			case "esc":
				t.state = inChatWindow
				t.chatWindow.Focus()
				t.messageInput.Blur()
			case "enter":
				if t.state == insertMode && len(t.messageInput.Value()) > 0 {
					msg := &twitch.PrivateMessage{
						In:      t.channel,
						Message: t.messageInput.Value(),
						From:    t.account.DisplayName,
						SentAt:  time.Now(),
					}
					t.messagesOut <- msg
					t.chatWindow.handleRecvTwitchMessage(msg)
					t.messageInput.SetValue("")
				}
			case "q":
				if t.state == inChatWindow {
					t.cancel()
					cmds = append(cmds, func() tea.Msg {
						return removeTabMessage{
							id: t.id,
						}
					})
				}
			}
		}
	}

	t.messageInput.Width = t.chatWindow.viewport.Width - 5

	t.channelInfo, cmd = t.channelInfo.Update(msg)
	cmds = append(cmds, cmd)

	// calculate chatWindow height with channel info height
	infoHeight := 0
	info := t.channelInfo.View()
	if info != "" {
		infoHeight = lipgloss.Height(info)
	}

	t.chatWindow.viewport.Height = t.height - 3 - infoHeight

	t.chatWindow, cmd = t.chatWindow.Update(msg)
	cmds = append(cmds, cmd)

	return t, tea.Batch(cmds...)
}

func (t *tab) View() string {
	if t.err != nil {
		style := lipgloss.NewStyle().
			Width(t.width).
			Height(t.height).
			Align(lipgloss.Center)

		return style.Render(fmt.Sprintf("Got error while fetching messages: %s", t.err))
	}

	inputView := lipgloss.NewStyle().
		Width(t.chatWindow.viewport.Width - 2). // width of the chat window minus the border
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("135")).
		Render(t.messageInput.View())

	if t.channelInfo.hasData {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			t.channelInfo.View(),
			t.chatWindow.View(),
			inputView,
		)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		t.chatWindow.View(),
		inputView,
	)
}

func (t *tab) Focus() {
	t.focused = true

	if t.state == inChatWindow {
		t.chatWindow.Focus()
	} else {
		t.messageInput.Focus()
	}
}

func (t *tab) Blur() {
	t.focused = false

	t.chatWindow.Blur()
	t.messageInput.Blur()
}

func waitMessage(t tab) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg, ok := <-t.messagesRecv:
			if !ok {
				return nil
			}

			return recvTwitchMessage{
				target:  t.id,
				message: msg,
			}
		case <-t.ctx.Done():
			return nil
		}
	}
}
