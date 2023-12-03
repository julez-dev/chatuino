package mainui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/multiplexer"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/twitch/command"
	"github.com/julez-dev/chatuino/ui/component"
	"github.com/pkg/browser"
	"github.com/rs/zerolog"
)

const (
	twitchBaseURL = "https://twitch.tv"
	popupFmt      = "https://www.twitch.tv/popout/%s/chat?popout="
)

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

func (t tabState) String() string {
	switch t {
	case 1:
		return "Insert"
	case 2:
		return "Inspect"
	}

	return "View"
}

const (
	inChatWindow tabState = iota
	insertMode
	userInspectMode
)

type apiClient interface {
	GetUsers(ctx context.Context, logins []string, ids []string) (twitch.UserResponse, error)
	GetStreamInfo(ctx context.Context, broadcastID []string) (twitch.GetStreamsResponse, error)
	GetChatSettings(ctx context.Context, broadcasterID string, moderatorID string) (twitch.GetChatSettingsResponse, error)
}

type tab struct {
	id     string
	logger zerolog.Logger

	state tabState

	provider AccountProvider
	account  save.Account // the account for this tab, should not rely on access token & refresh token, should be fetched each time used
	focused  bool

	channelDataLoaded bool
	initialMessages   []*command.PrivateMessage

	channel    string
	emoteStore EmoteStore

	width, height int

	ttvAPI apiClient

	// internal cancellation
	ctx        context.Context
	cancelFunc context.CancelFunc

	// components
	streamInfo   *streamInfo
	chatWindow   *chatWindow
	userInspect  *userInspect
	messageInput *component.SuggestionTextInput
	statusInfo   *status

	err error
}

func newTab(
	id string,
	logger zerolog.Logger,
	clientID string,
	serverAPI *server.Client,
	channel string,
	width, height int,
	emoteStore EmoteStore,
	account save.Account,
	accountProvider AccountProvider,
	initialMessages []*command.PrivateMessage,
) (*tab, error) {
	var ttvAPI apiClient

	if account.IsAnonymous {
		ttvAPI = serverAPI
	} else {
		api, err := twitch.NewAPI(
			clientID,
			twitch.WithUserAuthentication(accountProvider, serverAPI, account.ID),
		)
		if err != nil {
			return nil, fmt.Errorf("error while creating twitch api client: %w", err)
		}

		ttvAPI = api
	}

	ctx, cancel := context.WithCancel(context.Background())

	input := component.NewSuggestionTextInput()
	input.SetWidth(width)

	return &tab{
		id:              id,
		logger:          logger,
		width:           width,
		height:          height,
		account:         account,
		provider:        accountProvider,
		ctx:             ctx,
		cancelFunc:      cancel,
		channel:         channel,
		emoteStore:      emoteStore,
		ttvAPI:          ttvAPI,
		messageInput:    input,
		initialMessages: initialMessages,
	}, nil
}

func (t *tab) Init() tea.Cmd {
	cmd := func() tea.Msg {
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

		if err := t.emoteStore.RefreshGlobal(t.ctx); err != nil {
			return setErrorMessage{
				targetID: t.id,
				err:      fmt.Errorf("error while refreshing global emote cache for %s (%s): %w", t.channel, userData.Data[0].ID, err),
			}
		}

		return setChannelDataMessage{
			targetID:  t.id,
			channelID: userData.Data[0].ID,
			channel:   userData.Data[0].DisplayName,
		}
	}

	return cmd
}

func (t *tab) Update(msg tea.Msg) (*tab, tea.Cmd) {
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
	case setStreamInfo:
		if t.channelDataLoaded {
			if msg.target != t.streamInfo.id {
				return t, nil
			}

			beforeView := t.streamInfo.View()

			t.streamInfo, cmd = t.streamInfo.Update(msg)

			// only do expensive resize if view has changed
			if beforeView != t.streamInfo.View() {
				t.handleResize()
			}

			return t, cmd
		}
	case setChannelDataMessage:
		if msg.targetID != t.id {
			return t, nil
		}

		t.channelDataLoaded = true

		// set chat suggestions if non anonymous user
		if !t.account.IsAnonymous {
			emoteSet := t.emoteStore.GetAllForUser(msg.channelID)
			suggestions := make([]string, 0, len(emoteSet))

			for _, emote := range emoteSet {
				suggestions = append(suggestions, emote.Text)
			}

			t.messageInput.SetSuggestions(suggestions)
		}

		t.streamInfo = newStreamInfo(t.ctx, msg.channelID, t.ttvAPI, t.width)
		t.chatWindow = newChatWindow(t.logger, t.id, t.width, t.height, t.channel, msg.channelID, t.emoteStore)
		t.statusInfo = newStatus(t.logger, t.ttvAPI, t, t.width, t.height, t.account.ID, msg.channelID)

		for _, m := range t.initialMessages {
			t.chatWindow.handleMessage(m)
		}

		t.initialMessages = nil

		if t.focused {
			t.chatWindow.Focus()
		}

		ircCmds := make([]tea.Cmd, 0, 2)

		ircCmds = append(ircCmds, func() tea.Msg {
			return forwardChatMessage{
				msg: multiplexer.InboundMessage{
					AccountID: t.account.ID,
					Msg:       multiplexer.IncrementTabCounter{},
				},
			}
		})

		ircCmds = append(ircCmds, func() tea.Msg {
			return forwardChatMessage{
				msg: multiplexer.InboundMessage{
					AccountID: t.account.ID,
					Msg: command.JoinMessage{
						Channel: msg.channel,
					},
				},
			}
		})

		t.handleResize()

		return t, tea.Batch(t.streamInfo.Init(), t.statusInfo.Init(), tea.Sequence(ircCmds...))

	case chatEventMessage: // delegate message event to chat window
		if msg.channel != t.channel {
			return t, nil
		}

		if t.channelDataLoaded {
			t.chatWindow, cmd = t.chatWindow.Update(msg)
			cmds = append(cmds, cmd)

			// if room state update, update status info
			if _, ok := msg.message.(*command.RoomState); ok {
				cmds = append(cmds, t.statusInfo.Init()) // resend init command
			}

			// if in inspect mode, update user inspect window only on private messages
			if t.state == userInspectMode {
				irc, ok := msg.message.(*command.PrivateMessage)

				if ok {
					if irc.DisplayName == t.userInspect.user {
						t.userInspect, cmd = t.userInspect.Update(msg)
						cmds = append(cmds, cmd)
					}
				}
			}
		}

		if err, ok := msg.message.(ircConnectionError); ok {
			// if is error returned from final retry, don't wait again and return early
			var matchErr twitch.RetryReachedError

			if errors.As(err, &matchErr) {
				t.logger.Info().Err(err).Msg("retry limit reached error matched, don't wait for next message")
				return t, nil
			}
		}

		return t, tea.Batch(cmds...)
	}

	if t.channelDataLoaded && t.focused {
		if t.state == insertMode {
			t.messageInput, cmd = t.messageInput.Update(msg)
			cmds = append(cmds, cmd)
		}

		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "i":
				if !t.account.IsAnonymous && t.state == inChatWindow {
					t.state = insertMode
					t.messageInput.Focus()
					t.chatWindow.Blur()
				}
			case "u":
				// open up user inspect mode
				switch t.state {
				case inChatWindow, userInspectMode:
					_, e := t.chatWindow.entryForCurrentCursor()

					if e == nil {
						return t, nil
					}

					msg, ok := e.Message.(*command.PrivateMessage)

					if !ok {
						return t, nil
					}

					t.state = userInspectMode
					t.userInspect = newUserInspect(t.logger, t.ttvAPI, t.id, t.width, t.height, msg.DisplayName, t.channel, t.chatWindow.channelID, t.emoteStore)
					cmds = append(cmds, t.userInspect.Init())

					for _, e := range t.chatWindow.entries {
						msg, ok := e.Message.(*command.PrivateMessage)

						if !ok {
							continue
						}

						if msg.DisplayName == t.userInspect.user {
							t.userInspect.chatWindow.handleMessage(msg)
						}
					}

					t.userInspect.chatWindow.moveToBottom()

					t.handleResize()
					t.chatWindow.Blur()
					t.userInspect.chatWindow.Focus()

					t.chatWindow, cmd = t.chatWindow.Update(msg)
					cmds = append(cmds, cmd)

					return t, tea.Batch(cmds...)
				}
			case "p", "c":
				switch t.state {
				case inChatWindow, userInspectMode:
					return t, func() tea.Msg {
						url := fmt.Sprintf("%s/%s", twitchBaseURL, t.channel)

						if msg.String() == "p" {
							url = fmt.Sprintf(popupFmt, t.channel)
						}

						if err := browser.OpenURL(url); err != nil {
							t.logger.Error().Err(err).Msg("error while opening twitch channel in browser")
						}

						return nil
					}
				}
			case "esc":
				if t.state == userInspectMode {
					t.state = inChatWindow
					t.chatWindow.Focus()
					t.handleResize()
					t.chatWindow.updatePort()
					return t, nil
				}

				if !t.account.IsAnonymous {
					t.state = inChatWindow
					t.chatWindow.Focus()
					t.messageInput.Blur()
				}

				return t, nil
			case "enter":
				if t.state == insertMode && len(t.messageInput.Value()) > 0 {
					msg := &command.PrivateMessage{
						ChannelUserName: t.channel,
						Message:         t.messageInput.Value(),
						DisplayName:     t.account.DisplayName,
						TMISentTS:       time.Now(),
					}
					t.chatWindow.handleMessage(msg)
					t.messageInput.SetValue("")
					return t, func() tea.Msg {
						return forwardChatMessage{
							msg: multiplexer.InboundMessage{
								AccountID: t.account.ID,
								Msg:       msg,
							},
						}
					}
				}
			}
		}
	}

	if t.channelDataLoaded {
		t.chatWindow, cmd = t.chatWindow.Update(msg)
		cmds = append(cmds, cmd)

		t.streamInfo, cmd = t.streamInfo.Update(msg)
		cmds = append(cmds, cmd)

		t.statusInfo, cmd = t.statusInfo.Update(msg)
		cmds = append(cmds, cmd)

		if t.state == userInspectMode {
			t.userInspect, cmd = t.userInspect.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return t, tea.Batch(cmds...)
}

func (t *tab) View() string {
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

	builder := strings.Builder{}

	si := t.streamInfo.View()
	if si != "" {
		builder.WriteString(si)
		builder.WriteString("\n")
	}

	cw := t.chatWindow.View()
	builder.WriteString(cw)

	if t.state == userInspectMode {
		uiView := t.userInspect.View()
		builder.WriteString("\n")
		builder.WriteString(uiView)
	}

	mi := t.renderMessageInput()
	if mi != "" {
		builder.WriteString("\n")
		builder.WriteString(mi)
	}

	statusInfo := t.statusInfo.View()
	if statusInfo != "" {
		builder.WriteString("\n")
		builder.WriteString(statusInfo)
	}

	return builder.String()
}

func (r *tab) Close() error {
	r.cancelFunc()
	return r.ctx.Err()
}

func (t *tab) renderMessageInput() string {
	if t.account.IsAnonymous || t.state == userInspectMode {
		return ""
	}

	return t.messageInput.View()
}

func (t *tab) handleResize() {
	if t.channelDataLoaded {
		messageInput := t.renderMessageInput()
		heightMessageInput := lipgloss.Height(messageInput)

		if messageInput == "" {
			heightMessageInput = 0
		}

		statusInfo := t.statusInfo.View()
		heightStatusInfo := lipgloss.Height(statusInfo)

		if statusInfo == "" {
			heightStatusInfo = 0
		}

		streamInfo := t.streamInfo.View()
		heightStreamInfo := lipgloss.Height(streamInfo)
		if streamInfo == "" {
			heightStreamInfo = 0
		}

		t.statusInfo.width = t.width
		t.streamInfo.width = t.width
		if t.state == userInspectMode {
			t.chatWindow.height = (t.height - heightStreamInfo - heightStatusInfo) / 2
			t.chatWindow.width = t.width

			t.userInspect.height = t.height - heightStreamInfo - t.chatWindow.height - heightStatusInfo
			t.userInspect.width = t.width
			t.userInspect.handleResize()
		} else {
			t.chatWindow.height = t.height - heightMessageInput - heightStreamInfo - heightStatusInfo
			t.chatWindow.width = t.width
			t.chatWindow.recalculateLines()
		}

		t.messageInput.SetWidth(t.width)
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
