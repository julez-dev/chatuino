package mainui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/jellydator/ttlcache/v3"
	"github.com/lithammer/fuzzysearch/fuzzy"

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
	"github.com/julez-dev/chatuino/twitch/ivr"
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
	isUserMod       bool
}

type setUserIdentityData struct {
	targetID  string
	colorData twitch.UserChatColor
}

type broadcastTabState int

func (t broadcastTabState) String() string {
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
	inChatWindow broadcastTabState = iota
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
	SendChatAnnouncement(ctx context.Context, broadcasterID string, moderatorID string, req twitch.CreateChatAnnouncementRequest) error
	CreateStreamMarker(ctx context.Context, req twitch.CreateStreamMarkerRequest) (twitch.StreamMarker, error)
}

type userAuthenticatedAPIClient interface {
	CreateClip(ctx context.Context, broadcastID string, hasDelay bool) (twitch.CreatedClip, error)
	GetUserChatColor(ctx context.Context, userIDs []string) ([]twitch.UserChatColor, error)
}

type ModStatusFetcher interface {
	GetModVIPList(ctx context.Context, channel string) (ivr.ModVIPResponse, error)
}

type broadcastTab struct {
	id     string
	logger zerolog.Logger
	keymap save.KeyMap

	state            broadcastTabState
	isLocalSub       bool
	isUniqueOnlyChat bool
	lastMessages     *ttlcache.Cache[string, struct{}]

	isUserMod bool
	provider  AccountProvider
	account   save.Account // the account for this tab, should not rely on access token & refresh token, should be fetched each time used
	focused   bool

	channelDataLoaded bool
	lastMessageSent   string
	lastMessageSentAt time.Time

	channel    string
	channelID  string
	emoteStore EmoteStore

	colorData twitch.UserChatColor

	width, height     int
	userConfiguration UserConfiguration

	ttvAPI               APIClient
	modFetcher           ModStatusFetcher
	recentMessageService RecentMessageService
	emoteReplacer        EmoteReplacer
	messageLogger        MessageLogger

	// components
	streamInfo   *streamInfo
	poll         *poll
	chatWindow   *chatWindow
	userInspect  *userInspect
	messageInput *component.SuggestionTextInput
	statusInfo   *streamStatus
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
	emoteReplacer EmoteReplacer,
	messageLogger MessageLogger,
	userConfiguration UserConfiguration,
) *broadcastTab {
	cache := ttlcache.New(
		ttlcache.WithTTL[string, struct{}](time.Second * 10),
	)
	go cache.Start()

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
		emoteReplacer:        emoteReplacer,
		messageLogger:        messageLogger,
		lastMessages:         cache,
		userConfiguration:    userConfiguration,
		modFetcher:           ivr.NewAPI(http.DefaultClient),
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

		modVips, err := t.modFetcher.GetModVIPList(ctx, t.channel)
		if err != nil {
			return setErrorMessage{
				targetID: t.id,
				err:      fmt.Errorf("could not fetch mod/vip list for %s: %w", t.channel, err),
			}
		}

		var isUserMod bool
		for _, mod := range modVips.Mods {
			if mod.ID == t.account.ID {
				isUserMod = true
				break
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
			isUserMod:       isUserMod,
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

		modVips, err := t.modFetcher.GetModVIPList(ctx, t.channel)
		if err != nil {
			return setErrorMessage{
				targetID: t.id,
				err:      fmt.Errorf("could not fetch mod/vip list for %s: %w", t.channel, err),
			}
		}

		var isUserMod bool
		for _, mod := range modVips.Mods {
			if mod.ID == t.account.ID {
				isUserMod = true
				break
			}
		}

		return setChannelDataMessage{
			targetID:        t.id,
			channelID:       userData.ID,
			channel:         userData.DisplayName,
			initialMessages: recentMessages,
			isUserMod:       isUserMod,
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
		t.chatWindow = newChatWindow(t.logger, t.width, t.height, t.emoteStore, t.keymap, t.userConfiguration)
		t.messageInput = component.NewSuggestionTextInput(t.chatWindow.userColorCache, t.userConfiguration.Settings.BuildCustomSuggestionMap())
		t.messageInput.InputModel.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.userConfiguration.Theme.InputPromptColor))
		t.statusInfo = newStreamStatus(t.logger, t.ttvAPI, t, t.width, t.height, t.account.ID, msg.channelID, t.userConfiguration)

		// set chat suggestions if non-anonymous user
		if !t.account.IsAnonymous {
			t.isUserMod = msg.isUserMod

			// if user is broadcaster, allow mod commands
			if t.account.ID == msg.channelID {
				t.isUserMod = true
			}

			userEmoteSet := t.emoteStore.AllEmotesUsableByUser(t.account.ID)
			channelEmoteSet := t.emoteStore.GetAllForUser(msg.channelID) // includes bttv, 7tv

			unique := make(map[string]struct{}, len(userEmoteSet)+len(channelEmoteSet))

			for _, emote := range userEmoteSet {
				unique[emote.Text] = struct{}{}
			}

			for _, emote := range channelEmoteSet {
				unique[emote.Text] = struct{}{}
			}

			suggestions := slices.Collect(maps.Keys(unique))
			t.messageInput.SetSuggestions(suggestions)

			// user is mod or broadcaster, include mod commands
			if t.isUserMod {
				t.messageInput.IncludeModeratorCommands = true
			}
		}

		if t.focused {
			t.chatWindow.Focus()
		}

		ircCmds := make([]tea.Cmd, 0, 4)

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

		// notify user about loaded messages
		msg.initialMessages = append(msg.initialMessages, &command.Notice{
			FakeTimestamp:   time.Now(),
			ChannelUserName: t.channel,
			MsgID:           command.MsgID(uuid.NewString()),
			Message:         fmt.Sprintf("Loaded %d recent messages; powered by https://recent-messages.robotty.de", len(msg.initialMessages)),
		})

		// pass recent messages, recorded before the application was started, to chat window
		ircCmds = append(ircCmds, func() tea.Msg {
			return requestLocalMessageHandleMessageBatch{
				messages:  msg.initialMessages,
				tabID:     t.id,
				accountID: t.account.ID,
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

		if !t.account.IsAnonymous {
			cmds = append(cmds, func() tea.Msg {
				api, ok := t.ttvAPI.(userAuthenticatedAPIClient)
				if !ok {
					return nil
				}

				ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
				defer cancel()

				colorData, err := api.GetUserChatColor(ctx, []string{t.account.ID})
				if err != nil {
					t.logger.Error().Err(err).Str("account-id", t.account.ID).Msg("failed to fetch user chat color")
					return nil
				}

				resp := setUserIdentityData{}
				if len(colorData) > 0 {
					resp.colorData = colorData[0]
				}

				resp.targetID = t.id
				return resp
			})
		}

		t.HandleResize()
		cmds = append(cmds, t.streamInfo.Init(), t.statusInfo.Init(), tea.Sequence(ircCmds...))
		return t, tea.Batch(cmds...)
	case setUserIdentityData:
		if msg.targetID != t.id {
			return t, nil
		}

		t.colorData = msg.colorData
		return t, nil
	case EventSubMessage:
		cmd = t.handleEventSubMessage(msg.Payload)
		return t, cmd
	case chatEventMessage: // delegate message event to chat window
		// ignore all messages that don't target this account and channel
		if t.AccountID() != msg.accountID || t.Channel() != msg.channel && msg.channel != "" {
			return t, nil
		}

		if t.channelDataLoaded {
			if t.shouldIgnoreMessage(msg.message) {
				return t, nil
			}

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

			// add message content to cache
			if cast, ok := msg.message.(*command.PrivateMessage); ok {
				t.lastMessages.Set(cast.Message, struct{}{}, ttlcache.DefaultTTL)
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
				// Focus message input, when not in insert mode and not in search mode inside chat window, depending on the current active chat window
				if key.Matches(msg, t.keymap.InsertMode) &&
					(t.state == inChatWindow && t.chatWindow.state != searchChatWindowState || t.state == userInspectMode && t.userInspect.chatWindow.state != searchChatWindowState) {
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
					cmd := t.handleOpenUserInspectFromMessage()
					cmds = append(cmds, cmd)
					return t, tea.Batch(cmds...)
				}

				// Open chat in browser
				if key.Matches(msg, t.keymap.ChatPopUp, t.keymap.ChannelPopUp) && (t.state == inChatWindow || t.state == userInspectMode) {
					return t, t.handleOpenBrowser(msg)
				}

				// Send message
				if key.Matches(msg, t.keymap.Confirm) && len(t.messageInput.Value()) > 0 && (t.state == insertMode || t.state == userInspectInsertMode) {
					t.messageInput, _ = t.messageInput.Update(tea.KeyMsg{Type: tea.KeyEnter})
					return t, t.handleMessageSent(false)
				}

				// Send message - quick send
				if key.Matches(msg, t.keymap.QuickSent) && len(t.messageInput.Value()) > 0 && (t.state == insertMode || t.state == userInspectInsertMode) {
					t.messageInput, _ = t.messageInput.Update(tea.KeyMsg{Type: tea.KeyEnter})
					return t, t.handleMessageSent(true)
				}

				// Message Accept Suggestion Template Replace
				// always allow accept suggestion key so even new texts can be templated
				if key.Matches(msg, t.messageInput.KeyMap.AcceptSuggestion) && len(t.messageInput.Value()) > 0 && (t.state == insertMode || t.state == userInspectInsertMode) {
					t.messageInput, _ = t.messageInput.Update(msg)
					cmds = append(cmds, t.replaceInputTemplate())
					return t, tea.Batch(cmds...)
				}

				// Set quick time out message to message input
				if key.Matches(msg, t.keymap.QuickTimeout) && (t.state == inChatWindow || t.state == userInspectMode) {
					t.handleTimeoutShortcut()
					return t, nil
				}

				// Copy selected message to message input
				if key.Matches(msg, t.keymap.CopyMessage) && (t.state == inChatWindow || t.state == userInspectMode) {
					t.handleCopyMessage()
					return t, nil
				}

				// Close overlay windows
				if key.Matches(msg, t.keymap.Escape) {
					// first end search in user inspect sub window
					if t.userInspect != nil && t.userInspect.chatWindow.state == searchChatWindowState {
						t.userInspect.chatWindow, cmd = t.userInspect.chatWindow.Update(msg)
						cmds = append(cmds, cmd)
						return t, tea.Batch(cmds...)
					}

					// second case, end inspect mode or end insert mode in inspect window
					if t.state == userInspectMode || t.state == userInspectInsertMode {
						t.handleEscapePressed()
						return t, nil
					}

					// third case, end search in 'main' chat window
					if t.chatWindow.state == searchChatWindowState {
						t.chatWindow, cmd = t.chatWindow.Update(msg)
						cmds = append(cmds, cmd)
						return t, tea.Batch(cmds...)
					}

					t.handleEscapePressed()
					return t, nil
				}
			}

			if t.state == insertMode || t.state == userInspectInsertMode {
				t.messageInput, cmd = t.messageInput.Update(msg)
				cmds = append(cmds, cmd)
			}
		}

		// don't update any components when key message but not focused
		if _, ok := msg.(tea.KeyMsg); ok && !t.focused {
			return t, nil
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

func (t *broadcastTab) State() broadcastTabState {
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
		// open popup chat if modifier is pressed
		if key.Matches(msg, t.keymap.ChatPopUp) {
			t.handleOpenBrowserChatPopUp()()
			return nil
		}

		t.handleOpenBrowserChannel()()
		return nil
	}
}

func (t *broadcastTab) handleOpenBrowserChatPopUp() tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf(popupFmt, t.channel)

		if err := browser.OpenURL(url); err != nil {
			t.logger.Error().Err(err).Msg("error while opening twitch channel in browser")
		}
		return nil
	}
}

func (t *broadcastTab) handleOpenBrowserChannel() tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf("%s/%s", twitchBaseURL, t.channel)

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

// handlePyramidMessagesCommand build a message pyramid with the given word and count
// like this:
// word
// word word
// word word word
// word word
// word
func (t *broadcastTab) handlePyramidMessagesCommand(args []string) tea.Cmd {
	accountIsStreamer := t.account.ID == t.channelID

	if !accountIsStreamer && t.statusInfo != nil && t.statusInfo.settings.SlowMode {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: t.account.ID,
				channel:   t.channel,
				tabID:     t.id,
				message: &command.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Pyramid command is disabled in slow mode",
				},
			}
		}
	}

	if len(args) < 2 {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: t.account.ID,
				channel:   t.channel,
				tabID:     t.id,
				message: &command.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Expected Usage: /pyramid <word> <count>",
				},
			}
		}
	}

	word := args[0]
	count, err := strconv.Atoi(args[1])
	if err != nil {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: t.account.ID,
				channel:   t.channel,
				tabID:     t.id,
				message: &command.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Failed to convert count to integer",
				},
			}
		}
	}

	var msgs []twitch.IRCer
	for i := 1; i <= count; i++ {
		msgs = append(msgs, &command.PrivateMessage{
			ID:              uuid.Must(uuid.NewUUID()).String(),
			ChannelUserName: t.channel,
			Message:         strings.Repeat(word+" ", i),
			DisplayName:     t.account.DisplayName,
			TMISentTS:       time.Now(),
		})
	}

	for i := count - 1; i > 0; i-- {
		msgs = append(msgs, &command.PrivateMessage{
			ID:              uuid.Must(uuid.NewUUID()).String(),
			ChannelUserName: t.channel,
			Message:         strings.Repeat(word+" ", i),
			DisplayName:     t.account.DisplayName,
			TMISentTS:       time.Now(),
		})
	}

	var delay time.Duration
	if accountIsStreamer {
		delay = time.Millisecond * 500
	} else {
		delay = time.Millisecond * 1050
	}

	var cmds []tea.Cmd
	for i, msg := range msgs {
		cmds = append(cmds, func() tea.Msg {
			time.Sleep(delay)
			if i%2 == 0 {
				msg.(*command.PrivateMessage).Message += string(duplicateBypass)
			}
			return forwardChatMessage{
				msg: multiplex.InboundMessage{
					AccountID: t.account.ID,
					Msg:       msg,
				},
			}
		})

		cmds = append(cmds, func() tea.Msg {
			return requestLocalMessageHandleMessage{
				accountID: t.AccountID(),
				message:   msg,
			}
		})

	}

	return tea.Sequence(cmds...)
}

func (t *broadcastTab) handleLocalSubCommand(enable bool) tea.Cmd {
	if enable && t.isLocalSub {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: t.account.ID,
				channel:   t.channel,
				tabID:     t.id,
				message: &command.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Already in local submode",
				},
			}
		}
	}

	if !enable && !t.isLocalSub {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: t.account.ID,
				channel:   t.channel,
				tabID:     t.id,
				message: &command.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Already out of local submode",
				},
			}
		}
	}

	t.isLocalSub = enable

	return nil
}

func (t *broadcastTab) handleUniqueOnlyChatCommand(enable bool) tea.Cmd {
	if enable && t.isUniqueOnlyChat {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: t.account.ID,
				channel:   t.channel,
				tabID:     t.id,
				message: &command.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Already in unique only chat",
				},
			}
		}
	}

	if !enable && !t.isUniqueOnlyChat {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: t.account.ID,
				channel:   t.channel,
				tabID:     t.id,
				message: &command.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Already out of unique only chat",
				},
			}
		}
	}

	t.isUniqueOnlyChat = enable

	return nil
}

func (t *broadcastTab) shouldIgnoreMessage(msg twitch.IRCer) bool {
	cast, ok := msg.(*command.PrivateMessage)

	// all non private messages are okay
	if !ok {
		return false
	}

	// TODO: shared chat; ignore for now
	if cast.SourceRoomID != "" && cast.SourceRoomID != t.channelID {
		return true
	}

	// never ignore messages from the user,broadcaster,subs,mods,vips,paid messages,staff,bits or message mentions user
	if cast.UserID == t.account.ID || cast.UserID == t.channelID || cast.Mod || cast.PaidAmount != 0 || cast.VIP ||
		messageContainsCaseInsensitive(cast, t.account.DisplayName) || cast.Bits != 0 ||
		cast.UserType == command.Admin || cast.UserType == command.GlobalMod || cast.UserType == command.Staff {
		return false
	}

	// is sub and sender is not sub
	if t.isLocalSub && !cast.Subscriber {
		return true
	}

	if t.isUniqueOnlyChat {
		messagesInStore := t.lastMessages.Keys()
		wordsSrc := strings.Fields(cast.Message)
		lenWords := len(wordsSrc)

		// ignore if message is only one word and the message is in the last messages
		if lenWords == 1 && slices.ContainsFunc(messagesInStore, func(e string) bool { return strings.EqualFold(e, cast.Message) }) {
			return true
		} else if lenWords == 1 {
			return false
		}

		uniqueWordsSrc := map[string]struct{}{}
		for word := range slices.Values(wordsSrc) {
			uniqueWordsSrc[word] = struct{}{}
		}

		// uniqueWordsTarget := map[string]struct{}{}
		for stored := range slices.Values(messagesInStore) {
			distance := fuzzy.LevenshteinDistance(cast.Message, stored)
			if distance < 3 {
				return true
			}

			// for word := range slices.Values(strings.Fields(stored)) {
			// 	uniqueWordsTarget[strings.ToLower(word)] = struct{}{}
			// }

			// wordListSrc := slices.Collect(maps.Keys(uniqueWordsSrc))
			// var matches int
			// for word := range slices.Values(wordListSrc) {
			// 	word = strings.ToLower(word)
			// 	if _, ok := uniqueWordsTarget[word]; ok {
			// 		matches++
			// 	}
			// }

			// // if more than 70% of the words are the same, ignore the message
			// if float64(matches)/float64(lenWords) > 0.7 {
			// 	return true
			// }

			// clear(uniqueWordsTarget)
		}

	}

	return false
}

func (t *broadcastTab) handleMessageSent(quickSend bool) tea.Cmd {
	input := t.messageInput.Value()

	if !quickSend {
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
	}

	t.chatWindow.moveToBottom()

	// Check if input is a command
	if strings.HasPrefix(input, "/") {
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

		switch commandName {
		case "inspect":
			return t.handleOpenUserInspect(args[0])
		case "popupchat":
			return t.handleOpenBrowserChatPopUp()
		case "channel":
			return t.handleOpenBrowserChannel()
		case "banrequests":
			return t.handleOpenBanRequest()
		case "pyramid":
			return t.handlePyramidMessagesCommand(args)
		case "localsubscribers":
			return t.handleLocalSubCommand(true)
		case "localsubscribersoff":
			return t.handleLocalSubCommand(false)
		case "uniqueonly":
			return t.handleUniqueOnlyChatCommand(true)
		case "uniqueonlyoff":
			return t.handleUniqueOnlyChatCommand(false)
		case "createclip":
			return t.handleCreateClipMessage()
		}

		if !t.isUserMod {
			return func() tea.Msg {
				respMsg := chatEventMessage{
					isFakeEvent: true,
					accountID:   t.account.ID,
					tabID:       t.id,
					message: &command.Notice{
						FakeTimestamp: time.Now(),
						Message:       "Moderator commands are not available since you are not a moderator",
					},
				}
				return respMsg
			}
		}

		// Message input is only allowed for authenticated users
		// so ttvAPI is guaranteed to be a moderationAPIClient
		// we sadly can't know if the user is actually a moderator in the channel
		// so operations that require moderation privileges will fail
		client := t.ttvAPI.(moderationAPIClient)

		return handleCommand(commandName, args, channelID, channel, accountID, client)
	}

	// Check if message is the same as the last message sent
	// If so, append special character to bypass twitch duplicate message filter
	if strings.EqualFold(input, t.lastMessageSent) {
		input = input + " " + string(duplicateBypass)
	}

	msg := &command.PrivateMessage{
		ID:              uuid.Must(uuid.NewUUID()).String(),
		ChannelUserName: t.channel,
		Message:         input,
		DisplayName:     t.account.DisplayName,
		TMISentTS:       time.Now(),
		Color:           t.colorData.Color,
	}

	lastSent := t.lastMessageSentAt
	cmds := []tea.Cmd{}
	cmds = append(cmds, func() tea.Msg {
		const delay = time.Second
		diff := time.Since(lastSent)
		if diff < delay {
			time.Sleep(delay - diff)
		}

		return forwardChatMessage{
			msg: multiplex.InboundMessage{
				AccountID: t.account.ID,
				Msg:       msg,
			},
		}
	})

	cmds = append(cmds, func() tea.Msg {
		return requestLocalMessageHandleMessage{
			accountID: t.AccountID(),
			message:   msg,
		}
	})

	t.lastMessageSent = input
	t.lastMessageSentAt = time.Now()

	return tea.Sequence(cmds...)
}

func (t *broadcastTab) handleCreateClipMessage() tea.Cmd {
	return func() tea.Msg {
		api, ok := t.ttvAPI.(userAuthenticatedAPIClient)
		if !ok {
			t.logger.Warn().Str("broadcast", t.channel).Str("account", t.account.DisplayName).Msg("provided API does not support user authenticated API")
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
		defer cancel()

		clip, err := api.CreateClip(ctx, t.channelID, false)

		notice := &command.Notice{
			FakeTimestamp: time.Now(),
		}

		resp := chatEventMessage{
			isFakeEvent: true,
			accountID:   t.account.ID,
			channel:     t.channel,
			tabID:       t.id,
			message:     notice,
		}

		if err != nil {
			apiErr := twitch.APIError{}
			if errors.As(err, &apiErr) {
				switch apiErr.Status {
				case http.StatusUnauthorized:
					notice.Message = fmt.Sprintf("@%s Failed to create clip because you are unauthenticated or missing a auth scope; please authenticate again", t.account.DisplayName)
					return resp
				case http.StatusForbidden:
					notice.Message = fmt.Sprintf("@%s Failed to create clip because broadcaster restricted the ability to capture clips", t.account.DisplayName)
					return resp
				case http.StatusNotFound:
					notice.Message = fmt.Sprintf("@%s Failed to create clip because broadcaster is not live", t.account.DisplayName)
					return resp
				}
			}

			notice.Message = fmt.Sprintf("@%s Failed to create clip: %s", t.account.DisplayName, err)
			return resp
		}

		notice.Message = fmt.Sprintf("@%s Created clip can be edited here: %s", t.account.DisplayName, clip.EditURL)
		return resp
	}
}

func (t *broadcastTab) handleCopyMessage() {
	if t.account.IsAnonymous {
		return
	}

	var entry *chatEntry

	if t.state == inChatWindow {
		_, entry = t.chatWindow.entryForCurrentCursor()
		if t.chatWindow.state == searchChatWindowState {
			t.chatWindow.handleStopSearchMode()
		}
		t.chatWindow.Blur()
	} else {
		_, entry = t.userInspect.chatWindow.entryForCurrentCursor()
		if t.userInspect.chatWindow.state == searchChatWindowState {
			t.userInspect.chatWindow.handleStopSearchMode()
		}
		t.userInspect.chatWindow.Blur()
	}

	if entry == nil || entry.IsDeleted {
		return
	}

	msg, ok := entry.Event.message.(*command.PrivateMessage)

	if !ok {
		return
	}

	if t.state == userInspectMode {
		t.state = userInspectInsertMode
	} else {
		t.state = insertMode
	}

	t.messageInput.Focus()
	t.messageInput.SetValue(strings.ReplaceAll(msg.Message, string(duplicateBypass), ""))
}

func (t *broadcastTab) handleOpenUserInspect(username string) tea.Cmd {
	var cmds []tea.Cmd

	t.state = userInspectMode
	t.userInspect = newUserInspect(t.logger, t.ttvAPI, t.id, t.width, t.height, username, t.channel, t.emoteStore, t.keymap, t.emoteReplacer, t.messageLogger, t.userConfiguration)

	initialEvents := make([]chatEventMessage, 0, len(t.chatWindow.entries))
	for e := range slices.Values(t.chatWindow.entries) {
		initialEvents = append(initialEvents, chatEventMessage{
			isFakeEvent:                 true,
			accountID:                   t.account.ID,
			channel:                     t.channel,
			messageContentEmoteOverride: e.OverwrittenMessageContent,
			message:                     e.Event.message,
		})
	}

	cmds = append(cmds, t.userInspect.init(initialEvents))

	t.HandleResize()
	t.chatWindow.Blur()
	t.userInspect.chatWindow.userColorCache = t.chatWindow.userColorCache
	t.userInspect.chatWindow.Focus()

	return tea.Batch(cmds...)
}

func (t *broadcastTab) handleOpenUserInspectFromMessage() tea.Cmd {
	var e *chatEntry

	if t.state == inChatWindow {
		_, e = t.chatWindow.entryForCurrentCursor()
	} else {
		_, e = t.userInspect.chatWindow.entryForCurrentCursor()
	}

	if e == nil {
		return nil
	}

	var username string
	switch msg := e.Event.message.(type) {
	case *command.PrivateMessage:
		username = msg.DisplayName
	case *command.ClearChat:
		username = msg.UserName
	default:
		return nil
	}

	return t.handleOpenUserInspect(username)
}

func (t *broadcastTab) handleTimeoutShortcut() {
	if t.account.IsAnonymous {
		return
	}

	var entry *chatEntry

	switch t.state {
	case inChatWindow:
		_, entry = t.chatWindow.entryForCurrentCursor()
	case userInspectMode:
		_, entry = t.userInspect.chatWindow.entryForCurrentCursor()
	}

	if entry == nil {
		return
	}

	msg, ok := entry.Event.message.(*command.PrivateMessage)

	if !ok {
		return
	}

	if t.state == userInspectMode {
		t.state = userInspectInsertMode
		t.userInspect.chatWindow.handleStopSearchMode()
		t.userInspect.chatWindow.Blur()
	} else {
		t.state = insertMode
		t.chatWindow.handleStopSearchMode()
		t.chatWindow.Blur()
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

func (t *broadcastTab) handleEventSubMessage(msg eventsub.Message[eventsub.NotificationPayload]) tea.Cmd {
	if msg.Payload.Subscription.Condition["broadcaster_user_id"] != t.channelID &&
		msg.Payload.Subscription.Condition["from_broadcaster_user_id"] != t.channelID &&
		msg.Payload.Subscription.Condition["to_broadcaster_user_id"] != t.channelID {
		return nil
	}

	createCMDFunc := func(ircer twitch.IRCer) tea.Cmd {
		return func() tea.Msg {
			return requestLocalMessageHandleMessage{
				message:   ircer,
				accountID: t.AccountID(),
			}
		}
	}

	switch msg.Payload.Subscription.Type {
	case "channel.poll.begin":
		t.poll.setPollData(msg)
		t.poll.enabled = true
		t.HandleResize()
		return createCMDFunc(
			&command.Notice{
				FakeTimestamp:   time.Now(),
				ChannelUserName: t.channel,
				MsgID:           command.MsgID(uuid.NewString()),
				Message:         fmt.Sprintf("Poll %q has started!", msg.Payload.Event.Title),
			},
		)
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

		t.poll.enabled = false
		t.HandleResize()

		return createCMDFunc(
			&command.Notice{
				FakeTimestamp:   time.Now(),
				ChannelUserName: t.channel,
				MsgID:           command.MsgID(uuid.NewString()),
				Message:         fmt.Sprintf("Poll %q has ended, %q has won with %d votes!", msg.Payload.Event.Title, winner.Title, winner.Votes),
			},
		)
	case "channel.raid":
		// broadcaster raided another channel
		if msg.Payload.Event.FromBroadcasterUserID == t.channelID {
			return createCMDFunc(
				&command.Notice{
					FakeTimestamp:   time.Now(),
					ChannelUserName: t.channel,
					MsgID:           command.MsgID(uuid.NewString()),
					Message:         fmt.Sprintf("Raiding %s with %d Viewers!", msg.Payload.Event.ToBroadcasterUserName, msg.Payload.Event.Viewers),
				},
			)
		}

		// broadcaster gets raided
		return createCMDFunc(
			&command.Notice{
				FakeTimestamp:   time.Now(),
				ChannelUserName: t.channel,
				MsgID:           command.MsgID(uuid.NewString()),
				Message:         fmt.Sprintf("You are getting raided by %s with %d Viewers!", msg.Payload.Event.FromBroadcasterUserName, msg.Payload.Event.Viewers),
			},
		)
	case "channel.ad_break.begin":
		var chatMsg string

		if msg.Payload.Event.IsAutomatic {
			chatMsg = fmt.Sprintf("A automatic %d second ad just started!", msg.Payload.Event.DurationInSeconds)
		} else {
			chatMsg = fmt.Sprintf("A %d second ad, requested by %s, just started!", msg.Payload.Event.DurationInSeconds, msg.Payload.Event.RequesterUserName)
		}

		return createCMDFunc(
			&command.Notice{
				FakeTimestamp:   time.Now(),
				ChannelUserName: t.channel,
				MsgID:           command.MsgID(uuid.NewString()),
				Message:         chatMsg,
			},
		)
	}

	return nil
}

func (t *broadcastTab) replaceInputTemplate() tea.Cmd {
	input := t.messageInput.Value()

	notice := &command.Notice{
		FakeTimestamp: time.Now(),
	}

	resp := chatEventMessage{
		isFakeEvent: true,
		accountID:   t.account.ID,
		channel:     t.channel,
		tabID:       t.id,
		message:     notice,
	}

	tmpl, err := template.New("").Parse(input)
	if err != nil {
		notice.Message = fmt.Sprintf("Error while parsing template: %s", err)

		return func() tea.Msg {
			return resp
		}
	}

	data := map[string]any{
		"CurrentTime":     time.Now().Local().Format("15:04:05"),
		"CurrentDateTime": time.Now().Local().Format("2006-01-02 15:04:05"),
		"BroadcastID":     t.channelID,
		"BroadcastName":   t.channel,
	}

	// if a row is currently selected
	if _, e := t.chatWindow.entryForCurrentCursor(); e != nil {
		switch msg := e.Event.message.(type) {
		case *command.PrivateMessage:
			data["SelectedDisplayName"] = msg.DisplayName
			data["SelectedMessageContent"] = msg.Message
			data["SelectedUserID"] = msg.UserID
			data["RawMessage"] = msg
			data["MessageType"] = "PrivateMessage"
		case *command.SubMessage:
			data["SelectedDisplayName"] = msg.DisplayName
			data["SelectedMessageContent"] = msg.Message
			data["SelectedUserID"] = msg.UserID

			data["SubMessageCumulativeMonths"] = msg.CumulativeMonths
			data["SubMessageStreakMonths"] = msg.StreakMonths
			data["SubMessageSubPlan"] = msg.SubPlan.String()

			data["RawMessage"] = msg
			data["MessageType"] = "SubMessage"
		case *command.SubGiftMessage:
			data["SelectedDisplayName"] = msg.DisplayName
			data["SelectedMessageContent"] = msg
			data["SelectedUserID"] = msg.UserID

			data["SubGiftReceiptDisplayName"] = msg.ReceiptDisplayName
			data["SubGiftRecipientID"] = msg.RecipientID
			data["SubGiftMonths"] = msg.Months
			data["SubGiftSubPlan"] = msg.SubPlan.String()
			data["SubGiftGiftMonths"] = msg.GiftMonths

			data["RawMessage"] = msg
			data["MessageType"] = "SubGiftMessage"
		}
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		notice.Message = fmt.Sprintf("Error while executing template: %s", err)
		return func() tea.Msg {
			return resp
		}
	}

	t.messageInput.SetValue(out.String())
	return nil
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

func (t *broadcastTab) close() {
	t.lastMessages.DeleteAll()
	t.lastMessages.Stop()
	t.lastMessages = nil
}
