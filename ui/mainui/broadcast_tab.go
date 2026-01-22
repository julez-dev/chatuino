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
	"golang.org/x/sync/errgroup"

	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/save"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cli/browser"
	"github.com/julez-dev/chatuino/multiplex"
	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/julez-dev/chatuino/twitch/ivr"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/component"
	"github.com/rs/zerolog/log"
)

const (
	streamWebFmt       = "https://player.twitch.tv/?channel=%s&enableExtensions=false&muted=false&parent=chatuino.net&player=popout&quality=chunked&volume=0.2"
	streamChatPopUpFmt = "https://www.twitch.tv/popout/%s/chat?popout=1"
)

var modCommandAlternativeMapping = map[string]string{
	"/ban_selected":     "/ban",
	"/unban_selected":   "/unban",
	"/timeout_selected": "/timeout",
}

type setErrorMessage struct {
	targetID string
	err      error
}

type setChannelDataMessage struct {
	targetID        string
	channelLogin    string
	channel         string
	channelID       string
	initialMessages []twitchirc.IRCer
	isUserMod       bool
}

type emoteSetRefreshedMessage struct {
	targetID string
	err      error
	manually bool
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
		return "Emote Overview"
	}

	return "View"
}

const (
	inChatWindow broadcastTabState = iota
	insertMode
	userInspectMode
	userInspectInsertMode
	emoteOverviewMode
)

type moderationAPIClient interface {
	APIClient
	BanUser(ctx context.Context, broadcasterID string, moderatorID string, data twitchapi.BanUserData) error
	UnbanUser(ctx context.Context, broadcasterID string, moderatorID string, userID string) error
	DeleteMessage(ctx context.Context, broadcasterID string, moderatorID string, messageID string) error
	SendChatAnnouncement(ctx context.Context, broadcasterID string, moderatorID string, req twitchapi.CreateChatAnnouncementRequest) error
	CreateStreamMarker(ctx context.Context, req twitchapi.CreateStreamMarkerRequest) (twitchapi.StreamMarker, error)
}

type userAuthenticatedAPIClient interface {
	CreateClip(ctx context.Context, broadcastID string, hasDelay bool) (twitchapi.CreatedClip, error)
	GetUserChatColor(ctx context.Context, userIDs []string) ([]twitchapi.UserChatColor, error)
	SendChatMessage(ctx context.Context, data twitchapi.SendChatMessageRequest) (twitchapi.SendChatMessageResponse, error)
}

type ModStatusFetcher interface {
	GetModVIPList(ctx context.Context, channel string) (ivr.ModVIPResponse, error)
}

type broadcastTab struct {
	id      string
	account save.Account

	state            broadcastTabState
	isLocalSub       bool
	isUniqueOnlyChat bool
	lastMessages     *ttlcache.Cache[string, struct{}]

	isUserMod bool
	focused   bool

	channelDataLoaded bool
	lastMessageSent   string
	lastMessageSentAt time.Time

	channel      string
	channelID    string
	channelLogin string

	width, height int

	deps       *DependencyContainer
	modFetcher ModStatusFetcher

	// components
	streamInfo    *streamInfo
	poll          *poll
	chatWindow    *chatWindow
	userInspect   *userInspect
	messageInput  *component.SuggestionTextInput
	statusInfo    *streamStatus
	emoteOverview *emoteOverview
	spinner       spinner.Model

	err error
}

func newBroadcastTab(
	tabID string,
	width, height int,
	account save.Account,
	channel string,
	deps *DependencyContainer,
) *broadcastTab {
	cache := ttlcache.New(
		ttlcache.WithTTL[string, struct{}](time.Second * 10),
	)
	go cache.Start()

	return &broadcastTab{
		id:           tabID,
		width:        width,
		height:       height,
		account:      account,
		channel:      channel,
		lastMessages: cache,
		deps:         deps,
		modFetcher:   ivr.NewAPI(http.DefaultClient),
		spinner:      spinner.New(spinner.WithSpinner(customEllipsisSpinner)),
	}
}

func (t *broadcastTab) Init() tea.Cmd {
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()

		userData, err := t.deps.APIUserClients[t.account.ID].GetUsers(ctx, []string{t.channel}, nil)
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

		msg := t.InitWithUserData(userData.Data[0])()

		return msg
	}

	return tea.Batch(cmd, t.spinner.Tick)
}

func (t *broadcastTab) InitWithUserData(userData twitchapi.UserData) tea.Cmd {
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()

		group, ctx := errgroup.WithContext(ctx)

		var recentMessages []twitchirc.IRCer
		group.Go(func() error {
			// fetch recent messages
			msgs, err := t.deps.RecentMessageService.GetRecentMessagesFor(ctx, userData.Login)

			// call sometimes timeouts, but recent message are not really that important to crash the tab, so ignore the error
			if err != nil {
				return nil
			}

			recentMessages = msgs

			return nil
		})

		var isUserMod bool
		group.Go(func() error {
			modVips, err := t.modFetcher.GetModVIPList(ctx, userData.Login)
			if err != nil {
				return fmt.Errorf("could not fetch mods for %s (%s): %w", userData.Login, userData.ID, err)
			}

			for _, mod := range modVips.Mods {
				if mod.ID == t.account.ID {
					isUserMod = true
					break
				}
			}

			return nil
		})

		if err := group.Wait(); err != nil {
			return setErrorMessage{
				targetID: t.id,
				err:      fmt.Errorf("could not do initial fetching for %s: %w", userData.Login, err),
			}
		}

		return setChannelDataMessage{
			targetID:        t.id,
			channelID:       userData.ID,
			channel:         userData.DisplayName,
			channelLogin:    userData.Login,
			initialMessages: recentMessages,
			isUserMod:       isUserMod,
		}
	}

	return cmd
}

func (t *broadcastTab) refreshEmotes(login, channelID string, manually bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		group, ctx := errgroup.WithContext(ctx)

		group.Go(func() error {
			if err := t.deps.EmoteCache.RefreshLocal(ctx, channelID); err != nil {
				return fmt.Errorf("could not refresh emote cache for %s (%s): %w", login, channelID, err)
			}

			return nil
		})

		group.Go(func() error {
			if err := t.deps.BadgeCache.RefreshChannel(ctx, channelID); err != nil {
				return fmt.Errorf("could not refresh badge cache for %s (%s): %w", login, channelID, err)
			}

			return nil
		})

		err := group.Wait()
		if err != nil {
			return emoteSetRefreshedMessage{
				targetID: t.id,
				err:      err,
				manually: manually,
			}
		}

		return emoteSetRefreshedMessage{
			targetID: t.id,
			manually: manually,
		}
	}
}

func (t *broadcastTab) Update(msg tea.Msg) (tab, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case error:
		if !t.channelDataLoaded {
			return t, nil
		}

		return t, func() tea.Msg {
			return requestLocalMessageHandleMessage{
				tabID:     t.id,
				accountID: t.AccountID(),
				message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       msg.Error(),
				},
			}
		}
	case setErrorMessage:
		if msg.targetID != t.id {
			return t, nil
		}

		t.err = errors.Join(t.err, msg.err)
		return t, nil
	case setStreamInfoMessage:
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

		t.channelLogin = msg.channelLogin
		t.channelID = msg.channelID
		t.streamInfo = newStreamInfo(msg.channelID, t.deps.APIUserClients[t.account.ID], t.width)
		t.poll = newPoll(t.width)
		t.chatWindow = newChatWindow(t.width, t.height, t.deps)

		t.messageInput = component.NewSuggestionTextInput(t.chatWindow.userColorCache, t.deps.UserConfig.Settings.BuildCustomSuggestionMap())
		t.messageInput.EmoteReplacer = t.deps.EmoteReplacer // enable emote replacement
		t.messageInput.InputModel.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.deps.UserConfig.Theme.InputPromptColor))
		t.messageInput.SetMaxVisibleLines(3) // allow input to grow up to 3 lines

		t.statusInfo = newStreamStatus(t.width, t.height, t, t.account.ID, msg.channelID, t.deps)

		// set chat suggestions if non-anonymous user
		if !t.account.IsAnonymous {
			t.isUserMod = msg.isUserMod

			// if user is broadcaster, allow mod commands
			if t.account.ID == msg.channelID {
				t.isUserMod = true
			}

			// user is mod or broadcaster, include mod commands
			if t.isUserMod {
				t.messageInput.IncludeModeratorCommands = true
			}
		}

		if t.focused {
			t.chatWindow.Focus()
		}

		ircCmds := make([]tea.Cmd, 0, 3)

		// notify user about loaded messages
		msg.initialMessages = append(msg.initialMessages, &twitchirc.Notice{
			FakeTimestamp:   time.Now(),
			ChannelUserName: t.channelLogin,
			MsgID:           twitchirc.MsgID(uuid.NewString()),
			Message:         fmt.Sprintf("Loaded %d recent messages; powered by https://recent-messages.robotty.de", len(msg.initialMessages)),
		})

		// Pass recent messages, recorded before the application was started, to chat window
		// all irc commands will be processed as a sequence. This means all remote messages should be handled before the join irc command
		// is sent. This should keep the message order consistent.
		ircCmds = append(ircCmds, func() tea.Msg {
			return requestLocalMessageHandleBatchMessage{
				messages:  msg.initialMessages,
				tabID:     t.id,
				accountID: t.account.ID,
			}
		})

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
					Msg: twitchirc.JoinMessage{
						Channel: msg.channelLogin,
					},
				},
			}
		})

		cmds = append(cmds, t.refreshEmotes(msg.channelLogin, msg.channelID, false))

		// subscribe to channel events
		//  - if authenticated user
		//  - if channel belongs to user
		// sadly due to cost limits, we only allow this events users channel not other channels
		if eventSubAPI, ok := t.deps.APIUserClients[t.account.ID].(eventsub.EventSubService); ok && t.account.ID == msg.channelID {
			for _, subType := range [...]string{"channel.poll.begin", "channel.poll.progress", "channel.poll.end", "channel.ad_break.begin"} {
				cmds = append(cmds, func() tea.Msg {
					return forwardEventSubMessage{
						accountID: t.account.ID,
						msg: eventsub.InboundMessage{
							Service: eventSubAPI,
							Req: twitchapi.CreateEventSubSubscriptionRequest{
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
						Req: twitchapi.CreateEventSubSubscriptionRequest{
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
						Req: twitchapi.CreateEventSubSubscriptionRequest{
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
	case emoteSetRefreshedMessage:
		if !t.account.IsAnonymous && msg.targetID == t.id {
			if msg.err != nil && !errors.Is(msg.err, emote.ErrPartialFetch) {
				t.err = errors.Join(t.err, msg.err)
				return t, nil
			}

			userEmoteSet := t.deps.EmoteCache.AllEmotesUsableByUser(t.account.ID)

			log.Info().Str("user-id", t.account.ID).Int("len", len(userEmoteSet)).Msg("fetched emotes for user")

			channelEmoteSet := t.deps.EmoteCache.GetAllForChannel(t.channelID) // includes bttv, 7tv

			unique := make(map[string]struct{}, len(userEmoteSet)+len(channelEmoteSet))

			for _, emote := range userEmoteSet {
				unique[emote.Text] = struct{}{}
			}

			for _, e := range channelEmoteSet {
				// We want to set all emotes available to the user. This means we shouldn't set twitch sub emotes and others like follower and bits that
				// the user doesn't have access to.
				// All twitch emotes for the current channel to which the users has access to should be in the userEmoteSet, since they are returned from the API.
				// So we only include all 3rd pary emotes here
				if e.Platform == emote.Twitch {
					continue
				}

				unique[e.Text] = struct{}{}
			}

			suggestions := slices.Collect(maps.Keys(unique))
			t.messageInput.SetSuggestions(suggestions)

			// notify user if not all emotes could be fetched
			if errors.Is(msg.err, emote.ErrPartialFetch) {
				return t, func() tea.Msg {
					return requestLocalMessageHandleMessage{
						tabID:     t.id,
						accountID: t.AccountID(),
						message: &twitchirc.Notice{
							FakeTimestamp: time.Now(),
							Message:       msg.err.Error(),
						},
					}
				}
			}

			if msg.manually {
				return t, func() tea.Msg {
					return requestLocalMessageHandleMessage{
						tabID:     t.id,
						accountID: t.AccountID(),
						message: &twitchirc.Notice{
							FakeTimestamp: time.Now(),
							Message:       "Emotes refreshed manually",
						},
					}
				}
			}
		}

		return t, nil
	case EventSubMessage:
		cmd = t.handleEventSubMessage(msg.Payload)
		return t, cmd
	case chatEventMessage: // delegate message event to chat window
		// ignore all messages that don't target this account and channel

		if t.AccountID() != msg.accountID || t.channelLogin != msg.channel && msg.channel != "" {
			return t, nil
		}

		if t.channelDataLoaded {
			if t.shouldIgnoreMessage(msg.message) {
				return t, nil
			}

			if msg, ok := msg.message.(*twitchirc.PrivateMessage); ok {
				if messageContainsCaseInsensitive(msg, t.account.DisplayName) {
					cmds = append(cmds, func() tea.Msg {
						return requestNotificationIconMessage{
							tabID: t.id,
						}
					})
				}
			}

			t.chatWindow, cmd = t.chatWindow.Update(msg)
			cmds = append(cmds, cmd)

			// if room state update, update status info
			if _, ok := msg.message.(*twitchirc.RoomState); ok {
				cmds = append(cmds, t.statusInfo.Init()) // resend init command
			}

			if t.state == userInspectMode {
				t.userInspect, cmd = t.userInspect.Update(msg)
				cmds = append(cmds, cmd)
			}

			// add message content to cache
			if cast, ok := msg.message.(*twitchirc.PrivateMessage); ok {
				t.lastMessages.Set(cast.Message, struct{}{}, ttlcache.DefaultTTL)
			}

		}

		if err, ok := msg.message.(error); ok {
			// if is error returned from final retry, don't wait again and return early
			var matchErr twitchirc.RetryReachedError

			if errors.As(err, &matchErr) {
				log.Logger.Info().Err(err).Msg("retry limit reached error matched, don't wait for next message")
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
				if key.Matches(msg, t.deps.Keymap.InsertMode) &&
					(t.state == inChatWindow && t.chatWindow.state != searchChatWindowState || t.state == userInspectMode && t.userInspect.chatWindow.state != searchChatWindowState) {
					cmd := t.handleStartInsertMode()
					cmds = append(cmds, cmd)
					return t, tea.Batch(cmds...)
				}

				// Open user inspect mode, where only messages from a specific user are shown
				if key.Matches(msg, t.deps.Keymap.InspectMode) && (t.state == inChatWindow || t.state == userInspectMode) {
					cmd := t.handleOpenUserInspectFromMessage()
					cmds = append(cmds, cmd)
					return t, tea.Batch(cmds...)
				}

				// Open chat in browser
				if key.Matches(msg, t.deps.Keymap.ChatPopUp, t.deps.Keymap.ChannelPopUp) && (t.state == inChatWindow || t.state == userInspectMode) {
					return t, t.handleOpenBrowser(msg)
				}

				// Send message
				if key.Matches(msg, t.deps.Keymap.Confirm) && len(t.messageInput.Value()) > 0 && (t.state == insertMode || t.state == userInspectInsertMode) {
					t.messageInput, _ = t.messageInput.Update(tea.KeyMsg{Type: tea.KeyEnter})
					return t, t.handleMessageSent(false)
				}

				// Send message - quick send
				if key.Matches(msg, t.deps.Keymap.QuickSent) && len(t.messageInput.Value()) > 0 && (t.state == insertMode || t.state == userInspectInsertMode) {
					t.messageInput, _ = t.messageInput.Update(tea.KeyMsg{Type: tea.KeyEnter})
					return t, t.handleMessageSent(true)
				}

				// Message Accept Suggestion Template Replace
				// always allow accept suggestion key so even new texts can be templated
				if key.Matches(msg, t.messageInput.KeyMap.AcceptSuggestion) && len(t.messageInput.Value()) > 0 && (t.state == insertMode || t.state == userInspectInsertMode) {
					lineCountBefore := t.messageInput.LineCount()
					t.messageInput, _ = t.messageInput.Update(msg)
					cmds = append(cmds, t.replaceInputTemplate())
					if t.messageInput.LineCount() != lineCountBefore {
						t.HandleResize()
					}
					return t, tea.Batch(cmds...)
				}

				// Set quick time out message to message input
				if key.Matches(msg, t.deps.Keymap.QuickTimeout) && (t.state == inChatWindow || t.state == userInspectMode) {
					t.handleTimeoutShortcut()
					return t, nil
				}

				// Copy selected message to message input
				if key.Matches(msg, t.deps.Keymap.CopyMessage) && (t.state == inChatWindow || t.state == userInspectMode) {
					t.handleCopyMessage()
					return t, nil
				}

				// Close overlay windows
				if key.Matches(msg, t.deps.Keymap.Escape) {
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
				// Track line count before update to detect changes
				lineCountBefore := t.messageInput.LineCount()

				t.messageInput, cmd = t.messageInput.Update(msg)
				cmds = append(cmds, cmd)

				// Recalculate layout if input line count changed (text wrapped/unwrapped)
				if t.messageInput.LineCount() != lineCountBefore {
					t.HandleResize()
				}
			}
		}

		// don't update any components when key message but not focused
		if _, ok := msg.(tea.KeyMsg); ok && !t.focused {
			return t, nil
		}

		if t.state != emoteOverviewMode {
			t.chatWindow, cmd = t.chatWindow.Update(msg)
			cmds = append(cmds, cmd)
		}

		t.streamInfo, cmd = t.streamInfo.Update(msg)
		cmds = append(cmds, cmd)

		t.statusInfo, cmd = t.statusInfo.Update(msg)
		cmds = append(cmds, cmd)

		if t.emoteOverview != nil {
			_, ok := msg.(emoteOverviewSetDataMessage)

			// allow emoteOverviewSetDataMessage even when no longer in state
			if ok || t.state == emoteOverviewMode {
				t.emoteOverview, cmd = t.emoteOverview.Update(msg)
				cmds = append(cmds, cmd)
			}
		}

		if t.state == userInspectMode {
			t.userInspect, cmd = t.userInspect.Update(msg)
			cmds = append(cmds, cmd)
		}
	} else {
		t.spinner, cmd = t.spinner.Update(msg)
		cmds = append(cmds, cmd)
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
			Render(t.spinner.View() + " Loading")
	}

	builder := strings.Builder{}

	// In Emote Overview Mode only render emote overview + status info
	if t.state == emoteOverviewMode {
		builder.WriteString(t.emoteOverview.View())
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
	} else {
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

func (t *broadcastTab) Focused() bool {
	return t.focused
}

func (t *broadcastTab) AccountID() string {
	return t.account.ID
}

func (t *broadcastTab) Channel() string {
	return t.channelLogin
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
	if t.state == userInspectMode || t.state == emoteOverviewMode {
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
}

func (t *broadcastTab) handleOpenBrowser(msg tea.KeyMsg) tea.Cmd {
	return func() tea.Msg {
		// open popup chat if modifier is pressed
		if key.Matches(msg, t.deps.Keymap.ChatPopUp) {
			t.handleOpenBrowserChatPopUp()()
			return nil
		}

		t.handleOpenBrowserChannel()()
		return nil
	}
}

func (t *broadcastTab) handleOpenBrowserChatPopUp() tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf(streamChatPopUpFmt, t.channelLogin)

		if err := browser.OpenURL(url); err != nil {
			log.Logger.Error().Err(err).Msg("error while opening twitch channel in browser")
		}
		return nil
	}
}

func (t *broadcastTab) handleOpenBrowserChannel() tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf(streamWebFmt, t.channelLogin)

		if err := browser.OpenURL(url); err != nil {
			log.Logger.Error().Err(err).Msg("error while opening twitch channel in browser")
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
				channel:   t.channelLogin,
				channelID: t.channelID,
				tabID:     t.id,
				message: &twitchirc.Notice{
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
				channel:   t.channelLogin,
				channelID: t.channelID,
				tabID:     t.id,
				message: &twitchirc.Notice{
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
				channel:   t.channelLogin,
				channelID: t.channelID,
				tabID:     t.id,
				message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Failed to convert count to integer",
				},
			}
		}
	}

	client := t.deps.APIUserClients[t.account.ID].(userAuthenticatedAPIClient)
	broadcasterID := t.channelID
	userID := t.account.ID

	return func() tea.Msg {
		var msgs []string

		for i := 1; i <= count; i++ {
			msgs = append(msgs, strings.Repeat(word+" ", i))
		}

		for i := count - 1; i > 0; i-- {
			msgs = append(msgs, strings.Repeat(word+" ", i))
		}

		var delay time.Duration
		if accountIsStreamer {
			delay = time.Millisecond * 500
		} else {
			delay = time.Millisecond * 1050
		}

		notice := &twitchirc.Notice{
			FakeTimestamp: time.Now(),
		}

		resp := chatEventMessage{
			isFakeEvent: true,
			accountID:   userID,
			channel:     t.channelLogin,
			channelID:   t.channelID,
			tabID:       t.id,
			message:     notice,
		}

		for i, msg := range msgs {
			if i > 0 {
				time.Sleep(delay)
			}

			if i%2 == 0 {
				msg += string(duplicateBypass)
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()

			r, err := client.SendChatMessage(ctx, twitchapi.SendChatMessageRequest{
				BroadcasterID: broadcasterID,
				SenderID:      userID,
				Message:       msg,
			})
			if err != nil {
				notice.Message = fmt.Sprintf("Could not send message: %s", err.Error())
				return resp
			}

			if len(r.Data) > 0 && !r.Data[0].IsSent {
				notice.Message = fmt.Sprintf("Could not send message: %s", r.Data[0].DropReason.Message)
				return resp
			}
		}

		return nil
	}
}

func (t *broadcastTab) handleLocalSubCommand(enable bool) tea.Cmd {
	if enable && t.isLocalSub {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: t.account.ID,
				channel:   t.channelLogin,
				channelID: t.channelID,
				tabID:     t.id,
				message: &twitchirc.Notice{
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
				channel:   t.channelLogin,
				channelID: t.channelID,
				tabID:     t.id,
				message: &twitchirc.Notice{
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
				channel:   t.channelLogin,
				channelID: t.channelID,
				tabID:     t.id,
				message: &twitchirc.Notice{
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
				channel:   t.channelLogin,
				channelID: t.channelID,
				tabID:     t.id,
				message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Already out of unique only chat",
				},
			}
		}
	}

	t.isUniqueOnlyChat = enable

	return nil
}

func (t *broadcastTab) shouldIgnoreMessage(msg twitchirc.IRCer) bool {
	if messageMatchesBlocked(msg, t.deps.UserConfig.Settings.BlockSettings) {
		return true
	}

	cast, ok := msg.(*twitchirc.PrivateMessage)

	// all non private messages are okay
	if !ok {
		return false
	}

	// never ignore messages from the user,broadcaster,subs,mods,vips,paid messages,staff,bits or message mentions user
	if cast.UserID == t.account.ID || cast.UserID == t.channelID || cast.Mod || cast.PaidAmount != 0 || cast.VIP ||
		messageContainsCaseInsensitive(cast, t.account.DisplayName) || cast.Bits != 0 ||
		cast.UserType == twitchirc.Admin || cast.UserType == twitchirc.GlobalMod || cast.UserType == twitchirc.Staff {
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
		t.HandleResize() // Recalculate layout after clearing input
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
		args := strings.Split(argStr, " ")
		channelID := t.channelID
		channel := t.channelLogin
		accountID := t.account.ID

		switch commandName {
		case "inspect":
			return t.handleOpenUserInspect(args)
		case "popupchat":
			return t.handleOpenBrowserChatPopUp()
		case "channel":
			return t.handleOpenBrowserChannel()
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
		case "emotes":
			return t.handleOpenEmoteOverview()
		case "refreshemotes":
			return t.handleManualRefreshEmotes()
		}

		if !t.isUserMod {
			return func() tea.Msg {
				respMsg := chatEventMessage{
					isFakeEvent: true,
					accountID:   t.account.ID,
					tabID:       t.id,
					message: &twitchirc.Notice{
						FakeTimestamp: time.Now(),
						Message:       "Moderator commands are not available since you are not a moderator",
					},
				}
				return respMsg
			}
		}

		// Message input is only allowed for authenticated users
		// so ttvAPI is guaranteed to be a moderationAPIClient
		client := t.deps.APIUserClients[t.account.ID].(moderationAPIClient)

		return handleCommand(commandName, args, channelID, channel, accountID, client)
	}

	// Check if message is the same as the last message sent
	// If so, append special character to bypass twitch duplicate message filter
	if strings.EqualFold(input, t.lastMessageSent) {
		input = input + " " + string(duplicateBypass)
	}

	lastSent := t.lastMessageSentAt
	client := t.deps.APIUserClients[t.account.ID].(userAuthenticatedAPIClient)
	broadcasterID := t.channelID
	userID := t.account.ID

	cmd := func() tea.Msg {
		const delay = time.Second
		diff := time.Since(lastSent)
		if diff < delay {
			time.Sleep(delay - diff)
		}

		notice := &twitchirc.Notice{
			FakeTimestamp: time.Now(),
		}

		resp := chatEventMessage{
			isFakeEvent: true,
			accountID:   userID,
			channel:     t.channelLogin,
			channelID:   t.channelID,
			tabID:       t.id,
			message:     notice,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		r, err := client.SendChatMessage(ctx, twitchapi.SendChatMessageRequest{
			BroadcasterID: broadcasterID,
			SenderID:      userID,
			Message:       input,
		})
		if err != nil {
			notice.Message = fmt.Sprintf("Could not send message: %s", err.Error())
			return resp
		}

		if len(r.Data) > 0 && !r.Data[0].IsSent {
			notice.Message = fmt.Sprintf("Could not send message: %s", r.Data[0].DropReason.Message)
			return resp
		}

		return nil
	}

	t.lastMessageSent = input
	t.lastMessageSentAt = time.Now()

	return cmd
}

func (t *broadcastTab) handleCreateClipMessage() tea.Cmd {
	return func() tea.Msg {
		api, ok := t.deps.APIUserClients[t.account.ID].(userAuthenticatedAPIClient)
		if !ok {
			log.Logger.Warn().Str("broadcast", t.channelLogin).Str("account", t.account.DisplayName).Msg("provided API does not support user authenticated API")
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
		defer cancel()

		clip, err := api.CreateClip(ctx, t.channelID, false)

		notice := &twitchirc.Notice{
			FakeTimestamp: time.Now(),
		}

		resp := chatEventMessage{
			isFakeEvent: true,
			accountID:   t.account.ID,
			channel:     t.channelLogin,
			channelID:   t.channelID,
			tabID:       t.id,
			message:     notice,
		}

		if err != nil {
			apiErr := twitchapi.APIError{}
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

		if entry == nil {
			return
		}

		if t.chatWindow.state == searchChatWindowState {
			t.chatWindow.handleStopSearchMode()
		}
		t.chatWindow.Blur()
	} else {
		_, entry = t.userInspect.chatWindow.entryForCurrentCursor()

		if entry == nil {
			return
		}

		if t.userInspect.chatWindow.state == searchChatWindowState {
			t.userInspect.chatWindow.handleStopSearchMode()
		}
		t.userInspect.chatWindow.Blur()
	}

	msg, ok := entry.Event.message.(*twitchirc.PrivateMessage)

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
	t.HandleResize() // Recalculate layout after copying message
}

func (t *broadcastTab) handleOpenUserInspect(args []string) tea.Cmd {
	var cmds []tea.Cmd

	if len(args) < 1 {
		return nil
	}

	username := args[0]

	t.state = userInspectMode
	t.userInspect = newUserInspect(t.id, t.width, t.height, username, t.channelLogin, t.account.ID, t.deps)

	initialEvents := make([]chatEventMessage, 0, 15)
	for e := range slices.Values(t.chatWindow.entries) {
		initialEvents = append(initialEvents, chatEventMessage{
			isFakeEvent:             true,
			accountID:               t.account.ID,
			channel:                 t.channelLogin,
			channelID:               t.channelID,
			message:                 e.Event.message,
			displayModifier:         e.Event.displayModifier,
			channelGuestID:          e.Event.channelGuestID,
			channelGuestDisplayName: e.Event.channelGuestDisplayName,
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
	case *twitchirc.PrivateMessage:
		username = msg.LoginName
	case *twitchirc.ClearChat:
		if msg.UserName == nil {
			return nil
		}

		username = *msg.UserName
	default:
		return nil
	}

	return t.handleOpenUserInspect([]string{username})
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

	msg, ok := entry.Event.message.(*twitchirc.PrivateMessage)

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
	t.HandleResize() // Recalculate layout after setting timeout command
}

func (t *broadcastTab) renderMessageInput() string {
	if t.account.IsAnonymous {
		return ""
	}

	inputView := t.messageInput.View()
	borderColor := lipgloss.Color(t.deps.UserConfig.Theme.InputPromptColor)
	borderStyle := lipgloss.NewStyle().Foreground(borderColor)

	// Labels
	topLabel := "[ Chat ]"
	charCount := fmt.Sprintf("[ %d / %d ]", len([]rune(t.messageInput.Value())), t.messageInput.InputModel.CharLimit)

	innerWidth := t.width - 2 // -2 for left/right border chars

	// Top border: ┌─[ Chat ]─────...─┐
	topFill := innerWidth - len(topLabel) - 2
	topBorder := "┌─" + topLabel + strings.Repeat("─", topFill) + "─┐"

	// Bottom border: └─────...─[ 7 / 500 ]─┘ (counter on RIGHT)
	bottomFill := innerWidth - len(charCount) - 2
	bottomBorder := "└─" + strings.Repeat("─", bottomFill) + charCount + "─┘"

	// Wrap input lines with │ borders
	inputLines := strings.Split(inputView, "\n")
	var borderedLines []string
	for _, line := range inputLines {
		padNeeded := max(0, innerWidth-lipgloss.Width(line))
		borderedLines = append(borderedLines, "│"+line+strings.Repeat(" ", padNeeded)+"│")
	}

	// Combine
	result := borderStyle.Render(topBorder) + "\n"
	result += borderStyle.Render(strings.Join(borderedLines, "\n")) + "\n"
	result += borderStyle.Render(bottomBorder)

	return result
}

func (t *broadcastTab) HandleResize() {
	if t.channelDataLoaded {
		t.statusInfo.width = t.width
		t.streamInfo.width = t.width
		t.poll.setWidth(t.width)

		// Set messageInput width BEFORE rendering to ensure correct wrapping
		t.messageInput.SetWidth(t.width)

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
			heightStreamInfo = 1
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

		if t.state == emoteOverviewMode {
			log.Logger.Info().Int("width", t.width).Int("height", t.height-heightStatusInfo).Msg("resize emoteOverview")
			t.emoteOverview.resize(t.width, t.height-heightStatusInfo)
		}
	}
}

func (t *broadcastTab) handleEventSubMessage(msg eventsub.Message[eventsub.NotificationPayload]) tea.Cmd {
	if msg.Payload.Subscription.Condition["broadcaster_user_id"] != t.channelID &&
		msg.Payload.Subscription.Condition["from_broadcaster_user_id"] != t.channelID &&
		msg.Payload.Subscription.Condition["to_broadcaster_user_id"] != t.channelID {
		return nil
	}

	createCMDFunc := func(ircer twitchirc.IRCer) tea.Cmd {
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
			&twitchirc.Notice{
				FakeTimestamp:   time.Now(),
				ChannelUserName: t.channelLogin,
				MsgID:           twitchirc.MsgID(uuid.NewString()),
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
			&twitchirc.Notice{
				FakeTimestamp:   time.Now(),
				ChannelUserName: t.channelLogin,
				MsgID:           twitchirc.MsgID(uuid.NewString()),
				Message:         fmt.Sprintf("Poll %q has ended, %q has won with %d votes!", msg.Payload.Event.Title, winner.Title, winner.Votes),
			},
		)
	case "channel.raid":
		// broadcaster raided another channel
		if msg.Payload.Event.FromBroadcasterUserID == t.channelID {
			return createCMDFunc(
				&twitchirc.Notice{
					FakeTimestamp:   time.Now(),
					ChannelUserName: t.channelLogin,
					MsgID:           twitchirc.MsgID(uuid.NewString()),
					Message:         fmt.Sprintf("Raiding %s with %d Viewers!", msg.Payload.Event.ToBroadcasterUserName, msg.Payload.Event.Viewers),
				},
			)
		}

		// broadcaster gets raided
		return createCMDFunc(
			&twitchirc.Notice{
				FakeTimestamp:   time.Now(),
				ChannelUserName: t.channelLogin,
				MsgID:           twitchirc.MsgID(uuid.NewString()),
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
			&twitchirc.Notice{
				FakeTimestamp:   time.Now(),
				ChannelUserName: t.channelLogin,
				MsgID:           twitchirc.MsgID(uuid.NewString()),
				Message:         chatMsg,
			},
		)
	}

	return nil
}

func (t *broadcastTab) replaceInputTemplate() tea.Cmd {
	input := t.messageInput.InputModel.Value()

	notice := &twitchirc.Notice{
		FakeTimestamp: time.Now(),
	}

	resp := chatEventMessage{
		isFakeEvent: true,
		accountID:   t.account.ID,
		channel:     t.channelLogin,
		channelID:   t.channelID,
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
		"BroadcastName":   t.channelLogin,
	}

	// if a row is currently selected
	if _, e := t.chatWindow.entryForCurrentCursor(); e != nil {
		switch msg := e.Event.message.(type) {
		case *twitchirc.PrivateMessage:
			data["SelectedDisplayName"] = msg.DisplayName
			data["SelectedMessageContent"] = msg.Message
			data["SelectedUserID"] = msg.UserID
			data["MessageID"] = msg.ID

			data["RawMessage"] = msg
			data["MessageType"] = "PrivateMessage"
		case *twitchirc.SubMessage:
			data["SelectedDisplayName"] = msg.DisplayName
			data["SelectedMessageContent"] = msg.Message
			data["SelectedUserID"] = msg.UserID
			data["MessageID"] = msg.ID

			data["SubMessageCumulativeMonths"] = msg.CumulativeMonths
			data["SubMessageStreakMonths"] = msg.StreakMonths
			data["SubMessageSubPlan"] = msg.SubPlan.String()

			data["RawMessage"] = msg
			data["MessageType"] = "SubMessage"
		case *twitchirc.SubGiftMessage:
			data["SelectedDisplayName"] = msg.DisplayName
			data["SelectedMessageContent"] = msg
			data["SelectedUserID"] = msg.UserID
			data["MessageID"] = msg.ID

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

	inputText := out.String()

	// replace alternative mod commands
	if t.isUserMod && strings.HasPrefix(inputText, "/") {
		for from, to := range maps.All(modCommandAlternativeMapping) {
			if strings.HasPrefix(inputText, from) {
				inputText = strings.Replace(inputText, from, to, 1)
				break
			}
		}
	}

	t.messageInput.SetValue(inputText)
	t.HandleResize() // Recalculate layout after template replacement
	return nil
}

func (t *broadcastTab) handleOpenEmoteOverview() tea.Cmd {
	if t.account.IsAnonymous {
		return nil
	}

	t.state = emoteOverviewMode

	if t.emoteOverview != nil {
		t.HandleResize()
		return nil
	}

	t.emoteOverview = NewEmoteOverview(t.channelID, t.deps.EmoteCache, t.deps.EmoteReplacer, t.width, t.height)
	t.HandleResize()
	return t.emoteOverview.Init()
}

func (t *broadcastTab) handleManualRefreshEmotes() tea.Cmd {
	if t.account.IsAnonymous {
		return nil
	}

	t.deps.EmoteCache.RemoveEmoteSetForChannel(t.channelID)

	return t.refreshEmotes(t.channelLogin, t.channelID, true)
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

	if t.emoteOverview != nil {
		t.emoteOverview.close()
		t.emoteOverview = nil
	}
}
