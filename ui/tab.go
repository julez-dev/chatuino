package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/rs/zerolog"
)

type setChatInstanceMessage struct {
	chat         *twitch.Chat
	messagesRecv <-chan twitch.IRCer
}

type recvTwitchMessage struct {
	message twitch.IRCer
}

type tab struct {
	ctx    context.Context
	cancel context.CancelFunc

	channel    string
	logger     zerolog.Logger
	chatWindow *chatWindow

	ready        bool
	chat         *twitch.Chat
	messagesRecv <-chan twitch.IRCer
	messageLog   []string
}

func newTab(ctx context.Context, logger zerolog.Logger, channel string, width, height int) *tab {
	ctx, cancel := context.WithCancel(ctx)
	return &tab{
		ctx:     ctx,
		cancel:  cancel,
		logger:  logger,
		channel: channel,
		chatWindow: &chatWindow{
			logger: logger,
			width:  width,
			height: height,
		},
	}
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

		return setChatInstanceMessage{
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
		t.chat = msg.chat
		t.messagesRecv = msg.messagesRecv
		t.ready = true
		cmds = append(cmds, waitMessage(*t))
	case recvTwitchMessage:
		cmds = append(cmds, waitMessage(*t))
	}

	t.chatWindow, cmd = t.chatWindow.Update(msg)
	cmds = append(cmds, cmd)

	return t, tea.Batch(cmds...)
}

func (t *tab) View() string {
	return t.chatWindow.View()
}

func waitMessage(t tab) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-t.messagesRecv:
			return recvTwitchMessage{
				message: msg,
			}
		case <-t.ctx.Done():
			return nil
		}
	}
}
