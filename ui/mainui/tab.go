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

type tab struct {
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
	chatWindow   *chatWindow
	userInspect  *userInspect
	messageInput *component.SuggestionTextInput
	statusInfo   *status
	unbanWindow  *unbanrequest.UnbanWindow

	err error
}

func newTab(
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
) *tab {
	return &tab{
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

func (t *tab) Init() tea.Cmd {
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

			beforeHeight := lipgloss.Height(t.streamInfo.View())

			t.streamInfo, cmd = t.streamInfo.Update(msg)
			afterHeight := lipgloss.Height(t.streamInfo.View())

			// only do expensive resize if view height has changed
			if beforeHeight != afterHeight {
				t.handleResize()
			}

			return t, cmd
		}
	case setChannelDataMessage:
		if msg.targetID != t.id {
			return t, nil
		}

		t.channelDataLoaded = true

		t.channelID = msg.channelID
		t.streamInfo = newStreamInfo(msg.channelID, t.ttvAPI, t.width)
		t.chatWindow = newChatWindow(t.logger, t.width, t.height, t.channel, msg.channelID, t.emoteStore, t.keymap)
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
				ChannelUserName: t.channel,
				MsgID:           command.MsgID(uuid.NewString()),
				Message:         fmt.Sprintf("-- Loaded %d recent messages; powered by https://recent-messages.robotty.de --", len(msg.initialMessages)),
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

		t.handleResize()

		return t, tea.Batch(t.streamInfo.Init(), t.statusInfo.Init(), tea.Sequence(ircCmds...))

	case chatEventMessage: // delegate message event to chat window
		if msg.channel != "" && msg.channel != t.channel {
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
				if key.Matches(msg, t.keymap.ChatPopUp) && (t.state == inChatWindow || t.state == userInspectMode) {
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
	// Chat Window
	// User Inspect Window (if in user inspect mode)
	// Message Input
	// Status Info

	si := t.streamInfo.View()
	if si != "" {
		builder.WriteString(si)
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

func (t *tab) Close() error {
	if !t.channelDataLoaded {
		return nil
	}

	t.streamInfo.done <- struct{}{}
	close(t.streamInfo.done)
	return nil
}

func (t *tab) handleEscapePressed() {
	if t.state == userInspectMode {
		t.state = inChatWindow
		t.userInspect = nil
		t.chatWindow.Focus()
		t.handleResize()
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

func (t *tab) handleOpenBrowser(msg tea.KeyMsg) tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("%s/%s", twitchBaseURL, t.channel) // open channel in browser

		// open popout chat if modifier is pressed
		if msg.String() == "p" {
			url = fmt.Sprintf(popupFmt, t.channel)
		}

		if err := browser.OpenURL(url); err != nil {
			t.logger.Error().Err(err).Msg("error while opening twitch channel in browser")
		}

		return nil
	}
}

func (t *tab) handleStartInsertMode() tea.Cmd {
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

func (t *tab) handleOpenBanRequest() tea.Cmd {
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

	t.handleResize()

	return t.unbanWindow.Init()
}

func (t *tab) handleMessageSent() tea.Cmd {
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
		channelID := t.chatWindow.channelID
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

func (t *tab) handleCopyMessage() {
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

func (t *tab) handleOpenUserInspect() tea.Cmd {
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
	t.userInspect = newUserInspect(t.logger, t.ttvAPI, t.id, t.width, t.height, username, t.channel, t.chatWindow.channelID, t.emoteStore, t.keymap)
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

	t.handleResize()
	t.chatWindow.Blur()
	t.userInspect.chatWindow.userColorCache = t.chatWindow.userColorCache
	t.userInspect.chatWindow.Focus()

	// t.chatWindow, cmd = t.chatWindow.Update(msg)
	// cmds = append(cmds, cmd)

	return tea.Batch(cmds...)
}

func (t *tab) handleTimeoutShortcut() {
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

func (t *tab) renderMessageInput() string {
	if t.account.IsAnonymous {
		return ""
	}

	return t.messageInput.View()
}

func (t *tab) handleResize() {
	if t.channelDataLoaded {
		t.statusInfo.width = t.width
		t.streamInfo.width = t.width

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

		if t.state == userInspectMode || t.state == userInspectInsertMode {
			t.chatWindow.height = (t.height - heightStreamInfo - heightStatusInfo) / 2
			t.chatWindow.width = t.width

			t.userInspect.height = t.height - heightStreamInfo - t.chatWindow.height - heightStatusInfo - heightMessageInput
			t.userInspect.width = t.width
			t.userInspect.handleResize()
			t.chatWindow.recalculateLines()
		} else {
			t.chatWindow.height = t.height - heightMessageInput - heightStreamInfo - heightStatusInfo

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

func (t *tab) focus() {
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

func (t *tab) blur() {
	t.focused = false

	if t.channelDataLoaded {
		t.chatWindow.Blur()
		t.messageInput.Blur()

		if t.userInspect != nil {
			t.userInspect.chatWindow.Blur()
		}
	}
}
