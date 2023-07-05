package ui

import (
	"context"
	"time"

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

type setChannelIDMessage struct {
	target    uuid.UUID
	channelID string
}

type recvTwitchMessage struct {
	target  uuid.UUID
	message twitch.IRCer
}

type setChannelInfoMessage struct {
	target uuid.UUID
	viewer int
	title  string
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

	channel   string
	channelID string
	title     string
	viewer    int

	logger zerolog.Logger

	chat         *twitch.Chat
	messagesRecv <-chan twitch.IRCer
	messageLog   []string

	state        tabState
	chatWindow   *chatWindow
	messageInput textinput.Model

	emoteStore emoteStore
	ttvAPI     twitchAPI
}

func newTab(ctx context.Context, logger zerolog.Logger, channel string, emoteStore emoteStore, ttvAPI twitchAPI) *tab {
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
		emoteStore:   emoteStore,
		ttvAPI:       ttvAPI,
	}

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

	return tea.Batch(cmds...)
}

func (t *tab) Update(msg tea.Msg) (*tab, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case setChannelIDMessage:
		if msg.target == t.id {
			t.channelID = msg.channelID
			cmds = append(cmds, func() tea.Msg {
				return fetchStreamData(*t)
			})
		}
	case setChannelInfoMessage:
		if msg.target == t.id {
			t.title = msg.title
			t.viewer = msg.viewer
			t.logger.Info().Str("title", msg.title).Int("viewer", msg.viewer).Send()
			cmds = append(cmds, doTick(*t))
		}
	case resizeTabContainerMessage:
		t.chatWindow.viewport.Height = msg.Height - 4 // Space for input box
		t.chatWindow.viewport.Width = msg.Width
	case setChatInstanceMessage:
		if msg.target == t.id {
			t.chat = msg.chat
			t.messagesRecv = msg.messagesRecv
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

	t.chatWindow, cmd = t.chatWindow.Update(msg)
	cmds = append(cmds, cmd)

	return t, tea.Batch(cmds...)
}

func (t *tab) View() string {
	t.messageInput.Width = t.chatWindow.viewport.Width - 5
	inputView := lipgloss.NewStyle().
		Width(t.chatWindow.viewport.Width - 2). // width of the chat window minus the border
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

func fetchStreamData(t tab) tea.Msg {
	resp, err := t.ttvAPI.GetStreamInfo(t.ctx, []string{
		t.channelID,
	})

	if err != nil {
		t.logger.Err(err).Msg("failed to get channel info")
		return nil
	}

	if len(resp.Data) < 1 {
		return nil
	}

	return setChannelInfoMessage{
		target: t.id,
		title:  resp.Data[0].Title,
		viewer: resp.Data[0].ViewerCount,
	}
}

func doTick(t tab) tea.Cmd {
	return tea.Tick(time.Second*45, func(_ time.Time) tea.Msg {
		return fetchStreamData(t)
	})
}
