package ui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/rs/zerolog"
)

type setChatInstanceMessage struct {
	chat         *twitch.Chat
	messagesRecv <-chan twitch.IRCer
}

type recvTwitchMessage struct {
	message string
}

type Tab struct {
	ctx    context.Context
	cancel context.CancelFunc

	Channel    string
	Logger     zerolog.Logger
	chatWindow *ChatWindow

	ready        bool
	chat         *twitch.Chat
	messagesRecv <-chan twitch.IRCer
	messageLog   []string
}

func NewTab(ctx context.Context, logger zerolog.Logger, channel string, width, height int) *Tab {
	ctx, cancel := context.WithCancel(ctx)
	return &Tab{
		ctx:     ctx,
		cancel:  cancel,
		Logger:  logger,
		Channel: channel,
		chatWindow: &ChatWindow{
			logger: logger,
			width:  width,
			height: height,
		},
	}
}

func (t *Tab) Init() tea.Cmd {
	return func() tea.Msg {
		in := make(chan twitch.IRCer)

		go func() {
			<-t.ctx.Done()
			close(in)
		}()

		chat := twitch.NewChat()

		out, err := chat.Connect(t.ctx, in, twitch.AnonymousUser, twitch.AnonymousOAuth)
		if err != nil {
			t.Logger.Err(err).Send()
			return nil
		}

		return setChatInstanceMessage{
			chat:         chat,
			messagesRecv: out,
		}
	}
}

func (t *Tab) Update(msg tea.Msg) (*Tab, tea.Cmd) {
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
		// 	// t.messageLog = append(t.messageLog, msg.message)

		// 	// wasAtBottomBefore := t.chatWindow.AtBottom()
		// 	// t.chatWindow.SetContent(strings.Join(t.messageLog, "\n"))

		// 	// if wasAtBottomBefore {
		// 	// 	t.chatWindow.GotoBottom()
		// }
		cmds = append(cmds, waitMessage(*t))
	}

	t.chatWindow, cmd = t.chatWindow.Update(msg)
	cmds = append(cmds, cmd)

	return t, tea.Batch(cmds...)
}

func (t *Tab) View() string {
	return t.chatWindow.View()
}

func waitMessage(t Tab) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-t.messagesRecv:
			return recvTwitchMessage{
				message: messageToText(msg),
			}
		case <-t.ctx.Done():
			return nil
		}
	}
}

func messageToText(msg twitch.IRCer) string {
	switch msg := msg.(type) {
	case *twitch.PrivateMessage:
		return fmt.Sprintf("%s %s: %s", msg.SentAt.Local().Format("15:04:05"), msg.From, msg.Message)
	}

	return ""
}
