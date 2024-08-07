package mainui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/julez-dev/chatuino/ui/mainui/unbanrequest"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cli/browser"
	"github.com/julez-dev/chatuino/multiplex"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/twitch/command"
	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/julez-dev/chatuino/ui/component"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	twitchBaseURL = "https://www.twitch.tv"
	popupFmt      = "https://www.twitch.tv/popout/%s/chat?popout=1"
)

type setErrorMessage struct {
	targetID string
	err      error
}

type setChannelDataMessage struct {
	targetID        string
	channel         string
	channelID       string
	initialMessages []twitch.IRCer
}

type tabState int

func (t tabState) String() string {
	switch t {
	case 1:
		return "Insert"
	case 2:
		return "Inspect"
	case 3:
		return "Inspect / Insert"
	case 4:
		return "Unban"
	}

	return "View"
}

const (
	inChatWindow tabState = iota
	insertMode
	userInspectMode
	userInspectInsertMode
	unbanRequestMode
)

type moderationAPIClient interface {
	APIClient
	BanUser(ctx context.Context, broadcasterID string, moderatorID string, data twitch.BanUserData) error
	UnbanUser(ctx context.Context, broadcasterID string, moderatorID string, userID string) error
	FetchUnbanRequests(ctx context.Context, broadcasterID, moderatorID string) ([]twitch.UnbanRequest, error)
	ResolveBanRequest(ctx context.Context, broadcasterID, moderatorID, requestID, status string) (twitch.UnbanRequest, error)
}

type broadcastTab struct {
	id     string
	logger zerolog.Logger
	keymap save.KeyMap

	state tabState

	provider AccountProvider
	account  save.Account // the account for this tab, should not rely on access token & refresh token, should be fetched each time used
	focused  bool

	channelDataLoaded bool
	lastMessageSent   string

	channel    string
	channelID  string
	emoteStore EmoteStore

	width, height int

	ttvAPI               APIClient
	recentMessageService RecentMessageService

	// components
	streamInfo   *streamInfo
	poll         *poll
	chatWindow   *chatWindow
	userInspect  *userInspect
	messageInput *component.SuggestionTextInput
	statusInfo   *status
	unbanWindow  *unbanrequest.UnbanWindow

	err error
}

func newBroadcastTab(
	id string,
	logger zerolog.Logger,
	ttvAPI APIClient,
	channel string,
	width, height int,
	emoteStore EmoteStore,
	account save.Account,
	accountProvider AccountProvider,
	recentMessageService RecentMessageService,
	keymap save.KeyMap,
) *broadcastTab {
	return &broadcastTab{
		id:                   id,
		logger:               logger,
		keymap:               keymap,
		width:                width,
		height:               height,
		account:              account,
		provider:             accountProvider,
		channel:              channel,
		emoteStore:           emoteStore,
		ttvAPI:               ttvAPI,
		recentMessageService: recentMessageService,
	}
}

func (t *broadcastTab) Init() tea.Cmd {
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()

		userData, err := t.ttvAPI.GetUsers(ctx, []string{t.channel}, nil)
		if err != nil {
			return setErrorMessage{
				targetID: t.id,
				err:      fmt.Errorf("could not fetch ttv user %s: %w", t.channel, err),
			}
		}

		if len(userData.Data) < 1 {
			return setErrorMessage{
				targetID: t.id,
				err:      fmt.Errorf("could not find channel: %s", t.channel),
			}
		}

		// refresh emote set for joined channel
		if err := t.emoteStore.RefreshLocal(ctx, userData.Data[0].ID); err != nil {
			return setErrorMessage{
				targetID: t.id,
				err:      fmt.Errorf("could not refresh emote cache for %s (%s): %w", t.channel, userData.Data[0].ID, err),
			}
		}

		if err := t.emoteStore.RefreshGlobal(ctx); err != nil {
			return setErrorMessage{
				targetID: t.id,
				err:      fmt.Errorf("could not refresh global emote cache for %s (%s): %w", t.channel, userData.Data[0].ID, err),
			}
		}

		// fetch recent messages
		recentMessages, err := t.recentMessageService.GetRecentMessagesFor(ctx, t.channel)
		if err != nil {
			return setErrorMessage{
				targetID: t.id,
				err:      err,
			}
		}

		return setChannelDataMessage{
			targetID:        t.id,
			channelID:       userData.Data[0].ID,
			channel:         userData.Data[0].DisplayName,
			initialMessages: recentMessages,
		}
	}

	return cmd
}

func (t *broadcastTab) InitWithUserData(userData twitch.UserData) tea.Cmd {
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()

		// refresh emote set for joined channel
		if err := t.emoteStore.RefreshLocal(ctx, userData.ID); err != nil {
			return setErrorMessage{
				targetID: t.id,
				err:      fmt.Errorf("could not refresh emote cache for %s (%s): %w", t.channel, userData.ID, err),
			}
		}

		if err := t.emoteStore.RefreshGlobal(ctx); err != nil {
			return setErrorMessage{
				targetID: t.id,
				err:      fmt.Errorf("could not refresh global emote cache for %s (%s): %w", t.channel, userData.ID, err),
			}
		}

		// fetch recent messages
		recentMessages, err := t.recentMessageService.GetRecentMessagesFor(ctx, t.channel)
		if err != nil {
			return setErrorMessage{
				targetID: t.id,
				err:      err,
			}
		}

		return setChannelDataMessage{
			targetID:        t.id,
			channelID:       userData.ID,
			channel:         userData.DisplayName,
			initialMessages: recentMessages,
		}
	}

	return cmd
}

func (t *broadcastTab) Update(msg tea.Msg) (tab, tea.Cmd) {
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
			if msg.target != t.channelID {
				return t, nil
			}

			t.streamInfo, cmd = t.streamInfo.Update(msg)
			t.HandleResize()
			return t, cmd
		}
	case setChannelDataMessage:
		if msg.targetID != t.id {
			return t, nil
		}

		t.channelDataLoaded = true

		t.channelID = msg.channelID
		t.streamInfo = newStreamInfo(msg.channelID, t.ttvAPI, t.width)
		t.poll = newPoll(t.width)
		t.chatWindow = newChatWindow(t.logger, t.width, t.height, t.emoteStore, t.keymap)
		t.messageInput = component.NewSuggestionTextInput(t.chatWindow.userColorCache)
		t.statusInfo = newStatus(t.logger, t.ttvAPI, t, t.width, t.height, t.account.ID, msg.channelID)

		// set chat suggestions if non-anonymous user
		if !t.account.IsAnonymous {
			// TODO: This blocks in update function, should be moved to CMD
			emoteSet := t.emoteStore.GetAllForUser(msg.channelID)
			suggestions := make([]string, 0, len(emoteSet))

			for _, emote := range emoteSet {
				suggestions = append(suggestions, emote.Text)
			}

			t.messageInput.SetSuggestions(suggestions)
		}

		if t.focused {
			t.chatWindow.Focus()
		}

		// pass recent messages, recorded before the application was started, to chat window
		for _, ircMessage := range msg.initialMessages {
			t.chatWindow.handleMessage(ircMessage)
		}

		// notify user about loaded messages
		if len(msg.initialMessages) > 0 {
			t.chatWindow.handleMessage(&command.Notice{
				FakeTimestamp:   time.Now(),
				ChannelUserName: t.channel,
				MsgID:           command.MsgID(uuid.NewString()),
				Message:         fmt.Sprintf("Loaded %d recent messages; powered by https://recent-messages.robotty.de", len(msg.initialMessages)),
			})
		}

		ircCmds := make([]tea.Cmd, 0, 2)

		ircCmds = append(ircCmds, func() tea.Msg {
			return forwardChatMessage{
				msg: multiplex.InboundMessage{
					AccountID: t.account.ID,
					Msg:       multiplex.IncrementTabCounter{},
				},
			}
		})

		ircCmds = append(ircCmds, func() tea.Msg {
			return forwardChatMessage{
				msg: multiplex.InboundMessage{
					AccountID: t.account.ID,
					Msg: command.JoinMessage{
						Channel: msg.channel,
					},
				},
			}
		})

		// subscribe to channel events
		//  - if authenticated user
		//  - if channel belongs to user
		// sadly due to cost limits, we only allow this events users channel not other channels
		if eventSubAPI, ok := t.ttvAPI.(eventsub.EventSubService); ok && t.account.ID == msg.channelID {
			for _, subType := range [...]string{"channel.poll.begin", "channel.poll.progress", "channel.poll.end", "channel.ad_break.begin"} {
				cmds = append(cmds, func() tea.Msg {
					return forwardEventSubMessage{
						accountID: t.account.ID,
						msg: eventsub.InboundMessage{
							Service: eventSubAPI,
							Req: twitch.CreateEventSubSubscriptionRequest{
								Type:    subType,
								Version: "1",
								Condition: map[string]string{
									"broadcaster_user_id": msg.channelID,
								},
							},
						},
					}
				})
			}

			cmds = append(cmds, func() tea.Msg {
				return forwardEventSubMessage{
					accountID: t.account.ID,
					msg: eventsub.InboundMessage{
						Service: eventSubAPI,
						Req: twitch.CreateEventSubSubscriptionRequest{
							Type:    "channel.raid",
							Version: "1",
							Condition: map[string]string{
								"to_broadcaster_user_id": msg.channelID, // broadcaster gets raided
							},
						},
					},
				}
			})

			cmds = append(cmds, func() tea.Msg {
				return forwardEventSubMessage{
					accountID: t.account.ID,
					msg: eventsub.InboundMessage{
						Service: eventSubAPI,
						Req: twitch.CreateEventSubSubscriptionRequest{
							Type:    "channel.raid",
							Version: "1",
							Condition: map[string]string{
								"from_broadcaster_user_id": msg.channelID, // another channel gets raided from broadcaster
							},
						},
					},
				}
			})
		}

		t.HandleResize()
		cmds = append(cmds, t.streamInfo.Init(), t.statusInfo.Init(), tea.Sequence(ircCmds...))
		return t, tea.Batch(cmds...)

	case EventSubMessage:
		t.handleEventSubMessage(msg.Payload)
		return t, nil
	case chatEventMessage: // delegate message event to chat window
		// ignore all messages that don't target this account and channel
		if !(t.AccountID() == msg.accountID && (t.Channel() == msg.channel || msg.channel == "")) {
			return t, nil
		}

		if t.channelDataLoaded {
			t.chatWindow, cmd = t.chatWindow.Update(msg)
			cmds = append(cmds, cmd)

			// if room state update, update status info
			if _, ok := msg.message.(*command.RoomState); ok {
				cmds = append(cmds, t.statusInfo.Init()) // resend init command
			}

			if t.state == userInspectMode {
				t.userInspect, cmd = t.userInspect.Update(msg)
				cmds = append(cmds, cmd)
			}
		}

		if err, ok := msg.message.(error); ok {
			// if is error returned from final retry, don't wait again and return early
			var matchErr twitch.RetryReachedError

			if errors.As(err, &matchErr) {
				t.logger.Info().Err(err).Msg("retry limit reached error matched, don't wait for next message")
				return t, tea.Batch(cmds...)
			}
		}

		return t, tea.Batch(cmds...)
	}

	if t.channelDataLoaded {
		if t.focused {
			switch msg := msg.(type) {
			case tea.KeyMsg:
				// Focus message input
				if key.Matches(msg, t.keymap.InsertMode) {
					cmd := t.handleStartInsertMode()
					cmds = append(cmds, cmd)
					return t, tea.Batch(cmds...)
				}

				// Overlay unban request window
				if key.Matches(msg, t.keymap.UnbanRequestMode) && t.state == inChatWindow && !t.account.IsAnonymous {
					cmd := t.handleOpenBanRequest()
					cmds = append(cmds, cmd)
					return t, tea.Batch(cmds...)
				}

				// Open user inspect mode, where only messages from a specific user are shown
				if key.Matches(msg, t.keymap.InspectMode) && (t.state == inChatWindow || t.state == userInspectMode) {
					cmd := t.handleOpenUserInspect()
					cmds = append(cmds, cmd)
					return t, tea.Batch(cmds...)
				}

				// Open chat in browser
				if key.Matches(msg, t.keymap.ChatPopUp, t.keymap.ChannelPopUp) && (t.state == inChatWindow || t.state == userInspectMode) {
					return t, t.handleOpenBrowser(msg)
				}

				// Send message
				if key.Matches(msg, t.keymap.Confirm) && len(t.messageInput.Value()) > 0 && (t.state == insertMode || t.state == userInspectInsertMode) {
					return t, t.handleMessageSent()
				}

				// Set quick time out message to message input
				if key.Matches(msg, t.keymap.QuickTimeout) {
					t.handleTimeoutShortcut()
					return t, nil
				}

				// Copy selected message to message input
				if key.Matches(msg, t.keymap.CopyMessage) {
					t.handleCopyMessage()
					return t, nil
				}

				// Close overlay windows
				if key.Matches(msg, t.keymap.Escape) {
					t.handleEscapePressed()
					return t, nil
				}
			}

			if t.state == insertMode || t.state == userInspectInsertMode {
				t.messageInput, cmd = t.messageInput.Update(msg)
				cmds = append(cmds, cmd)
			}
		}

		t.chatWindow, cmd = t.chatWindow.Update(msg)
		cmds = append(cmds, cmd)

		t.streamInfo, cmd = t.streamInfo.Update(msg)
		cmds = append(cmds, cmd)

		t.statusInfo, cmd = t.statusInfo.Update(msg)
		cmds = append(cmds, cmd)

		if t.state == unbanRequestMode {
			t.unbanWindow, cmd = t.unbanWindow.Update(msg)
			cmds = append(cmds, cmd)
		}

		if t.state == userInspectMode {
			t.userInspect, cmd = t.userInspect.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return t, tea.Batch(cmds...)
}

func (t *broadcastTab) View() string {
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

	// In Unban Request Mode only render unban request window + status info
	if t.state == unbanRequestMode {
		builder.WriteString(t.unbanWindow.View())
		statusInfo := t.statusInfo.View()
		if statusInfo != "" {
			builder.WriteString("\n")
			builder.WriteString(statusInfo)
		}

		return builder.String()
	}

	// Render Order:
	// Stream Info
	// Poll
	// Chat Window
	// User Inspect Window (if in user inspect mode)
	// Message Input
	// Status Info

	si := t.streamInfo.View()
	if si != "" {
		builder.WriteString(si)
		builder.WriteString("\n")
	}

	pollView := t.poll.View()
	if pollView != "" {
		builder.WriteString(pollView)
		builder.WriteString("\n")
	}

	cw := t.chatWindow.View()
	builder.WriteString(cw)

	if t.state == userInspectMode || t.state == userInspectInsertMode {
		uiView := t.userInspect.View()
		builder.WriteString("\n")
		builder.WriteString(uiView)
	}

	mi := t.renderMessageInput()
	if mi != "" {
		builder.WriteString("\n ")
		builder.WriteString(mi)
	}

	statusInfo := t.statusInfo.View()
	if statusInfo != "" {
		builder.WriteString("\n")
		builder.WriteString(statusInfo)
	}

	return builder.String()
}

func (t *broadcastTab) Focused() bool {
	return t.focused
}

func (t *broadcastTab) AccountID() string {
	return t.account.ID
}

func (t *broadcastTab) Channel() string {
	return t.channel
}

func (t *broadcastTab) ChannelID() string {
	return t.channelID
}

func (t *broadcastTab) State() tabState {
	return t.state
}

func (t *broadcastTab) IsDataLoaded() bool {
	return t.channelDataLoaded
}

func (t *broadcastTab) ID() string {
	return t.id
}

func (t *broadcastTab) Kind() tabKind {
	return broadcastTabKind
}

func (t *broadcastTab) SetSize(width, height int) {
	t.width = width
	t.height = height
}

func (t *broadcastTab) handleEscapePressed() {
	if t.state == userInspectMode {
		t.state = inChatWindow
		t.userInspect = nil
		t.chatWindow.Focus()
		t.HandleResize()
		t.chatWindow.updatePort()
		return
	}

	if t.state == userInspectInsertMode {
		t.state = userInspectMode
		t.userInspect.chatWindow.Focus()
		t.messageInput.Blur()
		return
	}

	if !t.account.IsAnonymous {
		t.state = inChatWindow
		t.chatWindow.Focus()
		t.messageInput.Blur()
	}

	t.unbanWindow = nil
}

func (t *broadcastTab) handleOpenBrowser(msg tea.KeyMsg) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("%s/%s", twitchBaseURL, t.channel) // open channel in browser

		// open popout chat if modifier is pressed
		if key.Matches(msg, t.keymap.ChatPopUp) {
			url = fmt.Sprintf(popupFmt, t.channel)
		}

		if err := browser.OpenURL(url); err != nil {
			t.logger.Error().Err(err).Msg("error while opening twitch channel in browser")
		}

		return nil
	}
}

func (t *broadcastTab) handleStartInsertMode() tea.Cmd {
	if !t.account.IsAnonymous && (t.state == inChatWindow || t.state == userInspectMode) {
		if t.state == inChatWindow {
			t.state = insertMode
		} else {
			t.state = userInspectInsertMode
			t.userInspect.chatWindow.Blur()
		}

		t.messageInput.Focus()
		t.chatWindow.Blur()

		return t.messageInput.InputModel.Cursor.BlinkCmd()
	}

	return nil
}

func (t *broadcastTab) handleOpenBanRequest() tea.Cmd {
	t.state = unbanRequestMode
	t.unbanWindow = unbanrequest.New(
		t.ttvAPI.(moderationAPIClient),
		t.keymap,
		t.channel,
		t.channelID,
		t.account.ID,
		t.height,
		t.width,
	)

	t.HandleResize()

	return t.unbanWindow.Init()
}

func (t *broadcastTab) handleMessageSent() tea.Cmd {
	input := t.messageInput.Value()

	// reset state
	if t.state == userInspectInsertMode {
		t.state = userInspectMode
		t.userInspect.chatWindow.Focus()
	} else {
		t.state = inChatWindow
		t.chatWindow.Focus()
	}

	t.messageInput.Blur()
	t.messageInput.SetValue("")

	// Check if input is a command
	if strings.HasPrefix(input, "/") {
		// Message input is only allowed for authenticated users
		// so ttvAPI is guaranteed to be a moderationAPIClient
		// we sadly can't know if the user is actually a moderator in the channel
		// so operations that require moderation privileges will fail
		client := t.ttvAPI.(moderationAPIClient)

		// Get command name
		end := strings.Index(input, " ")
		if end == -1 {
			end = len(input)
		}

		commandName := input[1:end]

		argStr := strings.TrimSpace(input[end:])
		args := strings.SplitN(argStr, " ", 3)
		channelID := t.channelID
		channel := t.channel
		accountID := t.account.ID

		return handleCommand(commandName, args, channelID, channel, accountID, client)
	}

	// Check if message is the same as the last message sent
	// If so, append special character to bypass twitch duplicate message filter
	if strings.EqualFold(input, t.lastMessageSent) {
		input = input + string(duplicateBypass)
	}

	t.lastMessageSent = input

	msg := &command.PrivateMessage{
		ID:              uuid.Must(uuid.NewUUID()).String(),
		ChannelUserName: t.channel,
		Message:         input,
		DisplayName:     t.account.DisplayName,
		TMISentTS:       time.Now(),
	}

	t.chatWindow.handleMessage(msg)

	if t.state == userInspectMode {
		t.userInspect.chatWindow.handleMessage(msg)
	}

	return func() tea.Msg {
		return forwardChatMessage{
			msg: multiplex.InboundMessage{
				AccountID: t.account.ID,
				Msg:       msg,
			},
		}
	}
}

func (t *broadcastTab) handleCopyMessage() {
	if t.account.IsAnonymous {
		return
	}

	var entry *chatEntry

	if t.state == inChatWindow {
		_, entry = t.chatWindow.entryForCurrentCursor()
	} else {
		_, entry = t.userInspect.chatWindow.entryForCurrentCursor()
	}

	if entry == nil || entry.IsDeleted {
		return
	}

	msg, ok := entry.Message.(*command.PrivateMessage)

	if !ok {
		return
	}

	if t.state == userInspectMode {
		t.state = userInspectInsertMode
	} else {
		t.state = insertMode
	}
	t.messageInput.Focus()
	t.messageInput.SetValue(msg.Message)
}

func (t *broadcastTab) handleOpenUserInspect() tea.Cmd {
	var (
		cmds []tea.Cmd
		cmd  tea.Cmd
		e    *chatEntry
	)

	if t.state == inChatWindow {
		_, e = t.chatWindow.entryForCurrentCursor()
	} else {
		_, e = t.userInspect.chatWindow.entryForCurrentCursor()
	}

	if e == nil {
		return nil
	}

	var username string
	switch msg := e.Message.(type) {
	case *command.PrivateMessage:
		username = msg.DisplayName
	case *command.ClearChat:
		username = msg.UserName
	default:
		return nil
	}

	t.state = userInspectMode
	t.userInspect = newUserInspect(t.logger, t.ttvAPI, t.id, t.width, t.height, username, t.channel, t.emoteStore, t.keymap)
	cmds = append(cmds, t.userInspect.Init())

	for _, e := range t.chatWindow.entries {
		t.userInspect, cmd = t.userInspect.Update(chatEventMessage{
			accountID: t.account.ID,
			channel:   t.channel,
			message:   e.Message,
		})
		cmds = append(cmds, cmd)
	}

	t.userInspect.chatWindow.moveToBottom()

	t.HandleResize()
	t.chatWindow.Blur()
	t.userInspect.chatWindow.userColorCache = t.chatWindow.userColorCache
	t.userInspect.chatWindow.Focus()

	// t.chatWindow, cmd = t.chatWindow.Update(msg)
	// cmds = append(cmds, cmd)

	return tea.Batch(cmds...)
}

func (t *broadcastTab) handleTimeoutShortcut() {
	if t.account.IsAnonymous {
		return
	}

	var entry *chatEntry

	if t.state == inChatWindow {
		_, entry = t.chatWindow.entryForCurrentCursor()
	} else if t.state == userInspectMode {
		_, entry = t.userInspect.chatWindow.entryForCurrentCursor()
	}

	if entry == nil {
		return
	}

	msg, ok := entry.Message.(*command.PrivateMessage)

	if !ok {
		return
	}

	if t.state == userInspectMode {
		t.state = userInspectInsertMode
	} else {
		t.state = insertMode
	}

	t.messageInput.Focus()
	t.messageInput.SetValue("/timeout " + msg.DisplayName + " 600")
}

func (t *broadcastTab) renderMessageInput() string {
	if t.account.IsAnonymous {
		return ""
	}

	return t.messageInput.View()
}

func (t *broadcastTab) HandleResize() {
	if t.channelDataLoaded {
		t.statusInfo.width = t.width
		t.streamInfo.width = t.width
		t.poll.setWidth(t.width)

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

		pollView := t.poll.View()
		pollHeight := lipgloss.Height(pollView)
		if pollView == "" {
			pollHeight = 0
		}

		if t.state == userInspectMode || t.state == userInspectInsertMode {
			t.chatWindow.height = (t.height - heightStreamInfo - pollHeight - heightStatusInfo) / 2
			t.chatWindow.width = t.width

			t.userInspect.height = t.height - heightStreamInfo - pollHeight - t.chatWindow.height - heightStatusInfo - heightMessageInput
			t.userInspect.width = t.width
			t.userInspect.handleResize()
			t.chatWindow.recalculateLines()
		} else {
			t.chatWindow.height = t.height - pollHeight - heightMessageInput - heightStreamInfo - heightStatusInfo

			if t.chatWindow.height < 0 {
				t.chatWindow.height = 0
			}

			log.Logger.Info().Int("t.chatWindow.height", t.chatWindow.height).Int("height", t.height).Int("heightStreamInfo", heightStreamInfo).Int("heightStatusInfo", heightStatusInfo).Msg("handleResize")

			t.chatWindow.width = t.width
			t.chatWindow.recalculateLines()
		}

		t.messageInput.SetWidth(t.width)

		if t.state == unbanRequestMode {
			t.unbanWindow.SetWidth(t.width)
			t.unbanWindow.SetHeight(t.height - heightStatusInfo)
		}
	}
}

func (t *broadcastTab) handleEventSubMessage(msg eventsub.Message[eventsub.NotificationPayload]) {
	if msg.Payload.Subscription.Condition["broadcaster_user_id"] != t.channelID &&
		msg.Payload.Subscription.Condition["from_broadcaster_user_id"] != t.channelID &&
		msg.Payload.Subscription.Condition["to_broadcaster_user_id"] != t.channelID {
		return
	}

	switch msg.Payload.Subscription.Type {
	case "channel.poll.begin":
		t.chatWindow.handleMessage(&command.Notice{
			FakeTimestamp:   time.Now(),
			ChannelUserName: t.channel,
			MsgID:           command.MsgID(uuid.NewString()),
			Message:         fmt.Sprintf("Poll %q has started!", msg.Payload.Event.Title),
		})
		t.poll.setPollData(msg)
		t.poll.enabled = true
		t.HandleResize()
	case "channel.poll.progress":
		heightBefore := lipgloss.Height(t.poll.View())
		t.poll.setPollData(msg)
		t.poll.enabled = true
		heightAfter := lipgloss.Height(t.poll.View())

		if heightAfter != heightBefore {
			t.HandleResize()
		}
	case "channel.poll.end":
		winner := msg.Payload.Event.Choices[0]

		for _, choice := range msg.Payload.Event.Choices {
			if choice.Votes > winner.Votes {
				winner = choice
			}
		}

		t.chatWindow.handleMessage(&command.Notice{
			FakeTimestamp:   time.Now(),
			ChannelUserName: t.channel,
			MsgID:           command.MsgID(uuid.NewString()),
			Message:         fmt.Sprintf("Poll %q has ended, %q has won with %d votes!", msg.Payload.Event.Title, winner.Title, winner.Votes),
		})

		t.poll.enabled = false
		t.HandleResize()
	case "channel.raid":
		// broadcaster raided another channel
		if msg.Payload.Event.FromBroadcasterUserID == t.channelID {
			t.chatWindow.handleMessage(&command.Notice{
				FakeTimestamp:   time.Now(),
				ChannelUserName: t.channel,
				MsgID:           command.MsgID(uuid.NewString()),
				Message:         fmt.Sprintf("Raiding %s with %d Viewers!", msg.Payload.Event.ToBroadcasterUserName, msg.Payload.Event.Viewers),
			})

			return
		}

		// broadcaster gets raided
		t.chatWindow.handleMessage(&command.Notice{
			FakeTimestamp:   time.Now(),
			ChannelUserName: t.channel,
			MsgID:           command.MsgID(uuid.NewString()),
			Message:         fmt.Sprintf("You are getting raided by %s with %d Viewers!", msg.Payload.Event.FromBroadcasterUserName, msg.Payload.Event.Viewers),
		})
	case "channel.ad_break.begin":
		var chatMsg string

		if msg.Payload.Event.IsAutomatic {
			chatMsg = fmt.Sprintf("A automatic %d second ad just started!", msg.Payload.Event.DurationInSeconds)
		} else {
			chatMsg = fmt.Sprintf("A %d second ad, requested by %s, just started!", msg.Payload.Event.DurationInSeconds, msg.Payload.Event.RequesterUserName)
		}

		t.chatWindow.handleMessage(&command.Notice{
			FakeTimestamp:   time.Now(),
			ChannelUserName: t.channel,
			MsgID:           command.MsgID(uuid.NewString()),
			Message:         chatMsg,
		})
	}
}

func (t *broadcastTab) Focus() {
	t.focused = true

	if t.channelDataLoaded {
		switch t.state {
		case inChatWindow:
			t.chatWindow.Focus()
		case userInspectMode:
			t.userInspect.chatWindow.Focus()
		case userInspectInsertMode, insertMode:
			t.messageInput.Focus()
		}
	}
}

func (t *broadcastTab) Blur() {
	t.focused = false

	if t.channelDataLoaded {
		t.chatWindow.Blur()
		t.messageInput.Blur()

		if t.userInspect != nil {
			t.userInspect.chatWindow.Blur()
		}
	}
}
