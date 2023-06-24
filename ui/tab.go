package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/rs/zerolog"
)

type setChatInstanceMessage struct {
	target       uuid.UUID
	chat         *twitch.Chat
	messagesRecv <-chan twitch.IRCer
}

type recvTwitchMessage struct {
	target  uuid.UUID
	message twitch.IRCer
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

	channel string
	logger  zerolog.Logger

	ready        bool
	chat         *twitch.Chat
	messagesRecv <-chan twitch.IRCer
	messageLog   []string

	state        tabState
	chatWindow   *chatWindow
	messageInput textinput.Model
}

func newTab(ctx context.Context, logger zerolog.Logger, channel string, width, height int) *tab {
	ctx, cancel := context.WithCancel(ctx)

	input := textinput.New()
	input.PromptStyle = input.PromptStyle.Foreground(lipgloss.Color("135"))

	tab := &tab{
		id:           uuid.New(),
		ctx:          ctx,
		cancel:       cancel,
		logger:       logger,
		channel:      channel,
		messageInput: input,
	}

	tab.chatWindow = &chatWindow{
		parentTab: tab,
		logger:    logger,
		width:     width,
		height:    height,
	}

	return tab
}

func (t *tab) Init() tea.Cmd {
	return func() tea.Msg {
		in := make(chan twitch.IRCer)

		go func() {
			<-t.ctx.Done()
			close(in)
		}()

		chat := twitch.NewChat()

		out, err := chat.Connect(t.ctx, in, twitch.AnonymousUser, twitch.AnonymousOAuth)
		if err != nil {
			t.logger.Err(err).Send()
			return nil
		}

		in <- twitch.JoinMessage{
			Channel: t.channel,
		}

		return setChatInstanceMessage{
			target:       t.id,
			chat:         chat,
			messagesRecv: out,
		}
	}
}

func (t *tab) Update(msg tea.Msg) (*tab, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case setChatInstanceMessage:
		if msg.target == t.id {
			t.chat = msg.chat
			t.messagesRecv = msg.messagesRecv
			t.ready = true
			cmds = append(cmds, waitMessage(*t))
		}
	case recvTwitchMessage:
		if msg.target == t.id {
			cmds = append(cmds, waitMessage(*t))
		}
	}

	if t.focused {
		if t.state == inChatWindow {
			t.messageInput, cmd = t.messageInput.Update(msg)
			cmds = append(cmds, cmd)
		}

		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "i":
				t.state = inChatWindow
				t.messageInput.Focus()
				t.chatWindow.Blur()
			case "esc":
				t.state = inChatWindow
				t.chatWindow.Focus()
				t.messageInput.Blur()
			}
		}
	}

	t.chatWindow, cmd = t.chatWindow.Update(msg)
	cmds = append(cmds, cmd)

	return t, tea.Batch(cmds...)
}

func (t *tab) View() string {
	t.messageInput.Width = t.chatWindow.width - 5
	inputView := lipgloss.NewStyle().
		Width(t.chatWindow.width - 2). // width of the chat window minus the border
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("135")).
		Render(t.messageInput.View())

	return lipgloss.JoinVertical(lipgloss.Left, t.chatWindow.View(), inputView)
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
		case msg := <-t.messagesRecv:
			return recvTwitchMessage{
				target:  t.id,
				message: msg,
			}
		case <-t.ctx.Done():
			return nil
		}
	}
}
