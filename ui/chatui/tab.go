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
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/emote/autocomplete"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/rs/zerolog"
)

type setErrorMessage struct {
	target uuid.UUID
	err    error
}

type setEmoteSet struct {
	target uuid.UUID
	emotes emote.EmoteSet
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

	state tabState

	channelInfo  *channelInfo
	chatWindow   *chatWindow
	messageInput textinput.Model

	eAutocomplete *autocomplete.Completer
	emoteStore    emoteStore
	ttvAPI        twitchAPI
	account       save.Account
}

func newTab(ctx context.Context, logger zerolog.Logger, channel string, emoteStore emoteStore, account save.Account) *tab {
	ctx, cancel := context.WithCancel(ctx)

	input := textinput.New()
	input.PromptStyle = input.PromptStyle.Foreground(lipgloss.Color("135"))

	ttvAPI := twitch.NewAPI(nil, account.AccessToken, os.Getenv("TWITCH_CLIENT_ID"))
	tab := &tab{
		id:            uuid.New(),
		ctx:           ctx,
		cancel:        cancel,
		logger:        logger,
		channel:       channel,
		messageInput:  input,
		emoteStore:    emoteStore,
		ttvAPI:        ttvAPI,
		eAutocomplete: &autocomplete.Completer{},
		account:       account,
	}

	tab.channelInfo = newChannelInfo(ctx, logger, ttvAPI, channel)

	tab.chatWindow = newChatWindow(logger, tab, emoteStore)

	return tab
}

func (t *tab) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, 3)

	cmds = append(cmds, t.channelInfo.Init())

	cmds = append(cmds, func() tea.Msg {
		in := make(chan twitch.IRCer)

		chat := twitch.NewChat()

		out, errChan, err := chat.Connect(t.ctx, in, t.account.DisplayName, t.account.AccessToken)
		if err != nil {
			t.logger.Err(err).Send()
			return nil
		}

		in <- twitch.JoinMessage{
			Channel: t.channel,
		}

		go func() {
			<-t.ctx.Done()
			close(in)
		}()

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

		return setEmoteSet{
			target: t.id,
			emotes: t.emoteStore.GetAllForUser(userData.Data[0].ID),
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
	case setEmoteSet:
		if msg.target == t.id {
			comp := autocomplete.NewCompleter(msg.emotes)
			t.eAutocomplete = &comp
		}

	// case resizeTabContainerMessage:
	// 	t.width = msg.Width
	// 	t.height = msg.Height
	// 	t.setWidthAndHeight()
	case channelInfoSetMessage:
		t.setWidthAndHeight()
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

			if pos := t.messageInput.Position(); pos > 0 && t.eAutocomplete.HasSearch() {
				currChar := t.messageInput.Value()[pos-1]
				if currChar == ' ' {
					t.logger.Info().Msg("reset eAutocomplete")
					t.eAutocomplete.Reset()
				}
			}

		}

		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "tab":
				if t.state == insertMode {
					input := t.messageInput.Value()
					currWord := selectWordAtIndex(input, t.messageInput.Position())
					t.logger.Info().Str("word", currWord).Msg("current word")

					if !t.eAutocomplete.HasSearch() {
						t.logger.Info().Str("term", currWord).Msg("setting search")
						t.eAutocomplete.SetSearch(currWord)
					}

					t.eAutocomplete.Next()

					wordStartIndex, wordEndIndex := indexWordAtIndex(input, t.messageInput.Position())

					newInput := input[:wordStartIndex] + t.eAutocomplete.Current().Text + input[wordEndIndex:]
					t.messageInput.SetValue(newInput)
					t.messageInput.SetCursor(wordStartIndex + len(t.eAutocomplete.Current().Text))
					t.logger.Info().Str("word", t.eAutocomplete.Current().Text).Str("new", newInput).Send()
				}
			case "ctrl+w", " ":
				if t.state == insertMode {
					t.eAutocomplete.Reset()
				}
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
					t.chatWindow.handleMessage(msg)
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

	t.channelInfo, cmd = t.channelInfo.Update(msg)
	cmds = append(cmds, cmd)

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
		Width(t.chatWindow.width - 2). // width of the chat window minus the border
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

func (t *tab) setWidthAndHeight() {
	t.channelInfo.width = clamp(t.width, 0, t.width)
	t.messageInput.Width = t.width - 5

	// calculate chatWindow height with channel info height
	var infoHeight int
	channelInfoView := t.channelInfo.View()

	if channelInfoView != "" {
		infoHeight = lipgloss.Height(channelInfoView)
	}

	t.chatWindow.height = clamp(t.height-infoHeight, 0, t.height)
	t.chatWindow.width = clamp(t.width, 0, t.width)
	t.chatWindow.recalculateLines()
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
