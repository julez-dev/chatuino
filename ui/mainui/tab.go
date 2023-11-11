package mainui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/emote/autocomplete"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/rs/zerolog"
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

type recvTwitchLocalMessage struct {
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

type tabState int

const (
	inChatWindow tabState = iota
	insertMode
)

type apiClient interface {
	GetUsers(ctx context.Context, logins []string, ids []string) (twitch.UserResponse, error)
	GetStreamInfo(ctx context.Context, broadcastID []string) (twitch.GetStreamsResponse, error)
}

type tab struct {
	id    string
	state tabState

	account save.Account
	focused bool

	channelDataLoaded bool
	initialMessages   []*twitch.PrivateMessage

	channel    string
	emoteStore EmoteStore

	width, height int

	// twitch chat connection
	chat         *twitch.Chat
	messagesOut  chan<- twitch.IRCer
	messagesRecv <-chan twitch.IRCer

	ttvAPI apiClient

	// internal cancellation
	ctx        context.Context
	cancelFunc context.CancelFunc

	// components
	chatWindow   *chatWindow
	streamInfo   *streamInfo
	messageInput textinput.Model

	eAutocomplete *autocomplete.Completer

	err error
}

func newTab(
	id,
	clientID string,
	serverAPI *server.Client,
	channel string,
	width, height int,
	emoteStore EmoteStore,
	account save.Account,
	initialMessages []*twitch.PrivateMessage,
) (tab, error) {
	var ttvAPI apiClient

	if account.IsAnonymous {
		ttvAPI = serverAPI
	} else {
		api, err := twitch.NewAPI(
			clientID,
			twitch.WithUserAuthentication(account.AccessToken, account.RefreshToken, serverAPI),
		)
		if err != nil {
			return tab{}, fmt.Errorf("error while creating twitch api client: %w", err)
		}

		ttvAPI = api
	}

	ctx, cancel := context.WithCancel(context.Background())

	input := textinput.New()

	input.Validate = func(s string) error {
		if strings.ContainsRune(s, '\U0001FAF0') || strings.ContainsRune(s, '\n') {
			return fmt.Errorf("disallowed input")
		}

		return nil
	}

	input.PromptStyle = input.PromptStyle.Foreground(lipgloss.Color("135"))

	return tab{
		id:              id,
		width:           width,
		height:          height,
		account:         account,
		ctx:             ctx,
		cancelFunc:      cancel,
		channel:         channel,
		emoteStore:      emoteStore,
		ttvAPI:          ttvAPI,
		messageInput:    input,
		eAutocomplete:   &autocomplete.Completer{},
		initialMessages: initialMessages,
	}, nil
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
				err:      fmt.Errorf("error while fetching ttv user %s: %w", t.channel, err),
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
				err:      fmt.Errorf("error while refreshing emote cache for %s (%s): %w", t.channel, userData.Data[0].ID, err),
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

		if t.channelDataLoaded {
			t.chatWindow, cmd = t.chatWindow.Update(msg)
		}

		return t, tea.Batch(t.waitTwitchMessage(), cmd)
	case recvTwitchLocalMessage:
		if msg.targetID != t.id {
			return t, nil
		}

		if t.channelDataLoaded {
			t.chatWindow, cmd = t.chatWindow.Update(msg)
		}

		return t, cmd
	case setChannelDataMessage:
		if msg.targetID != t.id {
			return t, nil
		}

		completer := autocomplete.NewCompleter(t.emoteStore.GetAllForUser(msg.channelID))
		completer.Reset()

		t.eAutocomplete = &completer
		t.channelDataLoaded = true
		t.chatWindow = newChatWindow(zerolog.New(io.Discard), t.id, t.width, t.height, t.channel, msg.channelID, t.emoteStore)

		for _, m := range t.initialMessages {
			t.chatWindow.handleMessage(m)
		}

		t.initialMessages = nil

		if t.focused {
			t.chatWindow.Focus()
		}

		t.streamInfo = newStreamInfo(t.ctx, msg.channelID, t.ttvAPI, t.width)
		t.handleResize()

		return t, t.streamInfo.Init()
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

	if t.channelDataLoaded && t.focused {
		if t.state == insertMode {
			t.messageInput, cmd = t.messageInput.Update(msg)
			cmds = append(cmds, cmd)

			if pos := t.messageInput.Position(); pos > 0 && t.eAutocomplete.HasSearch() {
				currChar := t.messageInput.Value()[pos-1]
				if currChar == ' ' {
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

					if !t.eAutocomplete.HasSearch() {
						t.eAutocomplete.SetSearch(currWord)
					}

					t.eAutocomplete.Next()

					wordStartIndex, wordEndIndex := indexWordAtIndex(input, t.messageInput.Position())

					newInput := input[:wordStartIndex] + t.eAutocomplete.Current().Text + input[wordEndIndex:]
					t.messageInput.SetValue(newInput)
					t.messageInput.SetCursor(wordStartIndex + len(t.eAutocomplete.Current().Text))
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
			}
		}
	}

	if t.channelDataLoaded {
		t.chatWindow, cmd = t.chatWindow.Update(msg)
		cmds = append(cmds, cmd)

		info, cmd := t.streamInfo.Update(msg)
		t.streamInfo = &info
		cmds = append(cmds, cmd)
	}

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

	return lipgloss.JoinVertical(
		lipgloss.Left,
		t.streamInfo.View(),
		t.chatWindow.View(),
		t.renderMessageInput(),
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

func (t *tab) renderMessageInput() string {
	return lipgloss.NewStyle().
		Width(t.width - 2). // width of the chat window minus the border
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("135")).
		Render(t.messageInput.View())
}

func (t *tab) handleResize() {
	if t.channelDataLoaded {
		heightMessageInput := lipgloss.Height(t.renderMessageInput())

		t.streamInfo.width = t.width
		heightInfo := lipgloss.Height(t.streamInfo.View())

		t.chatWindow.height = t.height - heightMessageInput - heightInfo
		t.chatWindow.width = t.width
		t.chatWindow.recalculateLines()
	}
}

func (t *tab) focus() {
	t.focused = true

	if t.channelDataLoaded {
		t.chatWindow.Focus()
	}
}

func (t *tab) blur() {
	t.focused = false

	if t.channelDataLoaded {
		t.chatWindow.Blur()
	}
}
