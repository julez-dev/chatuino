package tab

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

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/cli/browser"
	"github.com/google/uuid"
	"github.com/jellydator/ttlcache/v3"
	"github.com/julez-dev/chatuino/badge"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/julez-dev/chatuino/twitch/ivr"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/component"
	"github.com/julez-dev/chatuino/ui/nav"
	"github.com/julez-dev/chatuino/wspool"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
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
	err error
}

type setChannelDataMessage struct {
	channelLogin    string
	channel         string
	channelID       string
	initialMessages []twitchirc.IRCer
	isUserMod       bool
	modFetchErr     error
}

type emoteSetRefreshedMessage struct {
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

type ModStatusFetcher interface {
	GetModVIPList(ctx context.Context, channel string) (ivr.ModVIPResponse, error)
}

type Broadcast struct {
	id      string
	account save.Account

	state            broadcastTabState
	isLocalSub       bool
	isUniqueOnlyChat bool
	lastMessages     *ttlcache.Cache[string, struct{}]

	isUserMod          bool
	isModStatusAssumed bool // true when mod fetch failed; don't show "Mod" in status
	focused            bool
	updateAvailable    string // newer version to advertise, empty when up to date

	channelDataLoaded         bool
	pendingChannelSuggestions []string
	lastMessageSent           string
	lastMessageSentAt         time.Time

	// double-click tracking for chat messages
	lastClickEntry *chatEntry
	lastClickAt    time.Time

	channel      string
	channelID    string
	channelLogin string

	width, height int
	fullWidth     int // nonzero when Root renders a shared full-width status line

	// layout is the row allocation recorded by HandleResize. The view and the
	// mouse handlers read it instead of recomputing the arrangement.
	layout broadcastLayout

	settings       save.Settings
	theme          save.Theme
	keymap         save.KeyMap
	client         ClientCapabilities
	pool           ConnectionPool
	recentMessages RecentMessageService
	messageLogger  MessageLogger
	emoteCache     EmoteCache
	emoteReplacer  EmoteReplacer
	badgeCache     *badge.Cache
	badgeReplacer  BadgeReplacer

	modFetcher ModStatusFetcher

	// pre-created style to avoid allocations in View()
	centeredStyle lipgloss.Style // for loading/error states; Width/Height set at render time

	// components
	streamInfo    *streamInfo
	poll          *poll
	chatWindow    *chatWindow
	userInspect   *userInspect
	input         *chatInput
	statusInfo    *streamStatus
	emoteOverview *emoteOverview
	spinner       spinner.Model

	err error
}

func NewBroadcast(
	tabID string,
	width, height int,
	account save.Account,
	channel string,
	settings save.Settings,
	theme save.Theme,
	keymap save.KeyMap,
	client ClientCapabilities,
	pool ConnectionPool,
	recentMessages RecentMessageService,
	messageLogger MessageLogger,
	emoteCache EmoteCache,
	emoteReplacer EmoteReplacer,
	badgeCache *badge.Cache,
	badgeReplacer BadgeReplacer,
) *Broadcast {
	cache := ttlcache.New(
		ttlcache.WithTTL[string, struct{}](time.Second * 10),
	)
	go cache.Start()

	return &Broadcast{
		id:             tabID,
		width:          width,
		height:         height,
		account:        account,
		channel:        channel,
		channelLogin:   channel, // Initialize from param; updated to canonical value after init
		lastMessages:   cache,
		settings:       settings,
		theme:          theme,
		keymap:         keymap,
		client:         client,
		pool:           pool,
		recentMessages: recentMessages,
		messageLogger:  messageLogger,
		emoteCache:     emoteCache,
		emoteReplacer:  emoteReplacer,
		badgeCache:     badgeCache,
		badgeReplacer:  badgeReplacer,
		modFetcher:     ivr.NewAPI(http.DefaultClient),
		spinner:        spinner.New(spinner.WithSpinner(LoadingSpinner)),

		centeredStyle: lipgloss.NewStyle().AlignHorizontal(lipgloss.Center).AlignVertical(lipgloss.Center),
	}
}

// localNoticeCmd returns a command that shows an informational notice only in this
// tab, without sending anything to Twitch.
func (t *Broadcast) localNoticeCmd(message string) tea.Cmd {
	return func() tea.Msg {
		return RequestLocalMessageHandleMessage{
			TabID:     t.id,
			AccountID: t.AccountID(),
			Message: &twitchirc.Notice{
				FakeTimestamp: time.Now(),
				Message:       message,
			},
		}
	}
}

func (t *Broadcast) Init() tea.Cmd {
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()

		userData, err := t.client.API.GetUsers(ctx, []string{t.channel}, nil)
		if err != nil {
			return nav.Address(t.id, setErrorMessage{
				err: fmt.Errorf("could not fetch ttv user %s: %w", t.channel, err),
			})
		}

		if len(userData.Data) < 1 {
			return nav.Address(t.id, setErrorMessage{
				err: fmt.Errorf("could not find channel: %s", t.channel),
			})
		}

		msg := t.InitWithUserData(userData.Data[0])()

		return msg
	}

	return tea.Batch(cmd, t.spinner.Tick)
}

func (t *Broadcast) InitWithUserData(userData twitchapi.UserData) tea.Cmd {
	cmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()

		group, ctx := errgroup.WithContext(ctx)

		var recentMessages []twitchirc.IRCer
		group.Go(func() error {
			// fetch recent messages
			msgs, err := t.recentMessages.GetRecentMessagesFor(ctx, userData.Login)
			// call sometimes timeouts, but recent message are not really that important to crash the tab, so ignore the error
			if err != nil {
				return nil
			}

			recentMessages = msgs

			return nil
		})

		var isUserMod bool
		var modFetchErr error
		group.Go(func() error {
			modVips, err := t.modFetcher.GetModVIPList(ctx, userData.Login)
			if err != nil {
				// Don't fail the tab — assume mod so commands remain available.
				// The actual mod status will be corrected by USERSTATE from IRC.
				isUserMod = true
				modFetchErr = err
				return nil
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
			return nav.Address(t.id, setErrorMessage{
				err: fmt.Errorf("could not do initial fetching for %s: %w", userData.Login, err),
			})
		}

		return nav.Address(t.id, setChannelDataMessage{
			channelID:       userData.ID,
			channel:         userData.DisplayName,
			channelLogin:    userData.Login,
			initialMessages: recentMessages,
			isUserMod:       isUserMod,
			modFetchErr:     modFetchErr,
		})
	}

	return cmd
}

func (t *Broadcast) refreshEmotes(login, channelID string, manually bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		group, ctx := errgroup.WithContext(ctx)

		group.Go(func() error {
			if err := t.emoteCache.RefreshLocal(ctx, channelID); err != nil {
				return fmt.Errorf("could not refresh emote cache for %s (%s): %w", login, channelID, err)
			}

			return nil
		})

		group.Go(func() error {
			if err := t.badgeCache.RefreshChannel(ctx, channelID); err != nil {
				return fmt.Errorf("could not refresh badge cache for %s (%s): %w", login, channelID, err)
			}

			return nil
		})

		err := group.Wait()
		if err != nil {
			return nav.Address(t.id, emoteSetRefreshedMessage{
				err:      err,
				manually: manually,
			})
		}

		return nav.Address(t.id, emoteSetRefreshedMessage{
			manually: manually,
		})
	}
}

func (t *Broadcast) Focused() bool {
	return t.focused
}

func (t *Broadcast) AccountID() string {
	return t.account.ID
}

func (t *Broadcast) Channel() string {
	return t.channelLogin
}

// Title is the header label: the channel as the user entered it.
func (t *Broadcast) Title() string {
	title, _ := Caption(BroadcastKind, t.account, t.channel)
	return title
}

// Identity is the account label shown next to the title.
func (t *Broadcast) Identity() string {
	_, identity := Caption(BroadcastKind, t.account, t.channel)
	return identity
}

func (t *Broadcast) ChannelID() string {
	return t.channelID
}

// SetUpdateAvailable makes the status bar advertise a newer version. An empty
// version clears the notice.
func (t *Broadcast) SetUpdateAvailable(latest string) {
	t.updateAvailable = latest
}

// LocalUniqueOnly reports whether the tab drops repeated messages locally.
func (t *Broadcast) LocalUniqueOnly() bool { return t.isUniqueOnlyChat }

// SetLocalUniqueOnly restores that filter when a session is reloaded.
func (t *Broadcast) SetLocalUniqueOnly(v bool) { t.isUniqueOnlyChat = v }

// LocalSubOnly reports whether the tab drops non-subscriber messages locally.
func (t *Broadcast) LocalSubOnly() bool { return t.isLocalSub }

// SetLocalSubOnly restores that filter when a session is reloaded.
func (t *Broadcast) SetLocalSubOnly(v bool) { t.isLocalSub = v }

// IsComposing reports whether a text input inside the tab is capturing printable
// keys. The shell asks this to decide whether a key press is text or a command.
func (t *Broadcast) IsComposing() bool {
	return t.state.isComposing()
}

func (t *Broadcast) IsSearching() bool {
	// Root asks this while deciding what a key press means, which also happens for a
	// tab that is still loading its channel data and therefore has no chat window.
	if t.chatWindow != nil && t.chatWindow.state == searchChatWindowState {
		return true
	}

	return t.userInspect != nil && t.userInspect.chatWindow.state == searchChatWindowState
}

func (t *Broadcast) IsDataLoaded() bool {
	return t.channelDataLoaded
}

func (t *Broadcast) ID() string {
	return t.id
}

func (t *Broadcast) Kind() TabKind {
	return BroadcastKind
}

func (t *Broadcast) SetSize(width, height int) {
	t.width = width
	t.height = height
}

func (t *Broadcast) SetFullWidth(width int) {
	t.fullWidth = width
}

func (t *Broadcast) Focus() {
	t.focused = true

	if !t.channelDataLoaded {
		return
	}

	// The mode decides which child owns the keyboard.
	switch {
	case t.state == emoteOverviewMode:
		// The overview consumes tab input itself; do not focus hidden widgets.
	case t.state.isComposing():
		t.input.Focus()
	case t.state.isInspecting():
		if t.userInspect != nil {
			t.userInspect.chatWindow.Focus()
		}
	default:
		t.chatWindow.Focus()
	}
}

func (t *Broadcast) Blur() {
	t.focused = false
	t.lastClickEntry = nil
	t.lastClickAt = time.Time{}

	// Everything below talks to children, which only exist once the channel data
	// loaded. A tab that is still loading (for example a restored tab whose data is
	// fetched after it became active) has none of them yet.
	if t.channelDataLoaded {
		// Draft/reply/mode state belongs to this tab, not its pane's focus.
		// Only the focused tab receives input, so another pane cannot send it.
		t.chatWindow.Blur()
		t.input.Blur()

		if t.userInspect != nil {
			t.userInspect.chatWindow.Blur()
		}
	}
}

func (t *Broadcast) Close() {
	t.lastMessages.DeleteAll()
	t.lastMessages.Stop()
	t.lastMessages = nil

	if t.emoteOverview != nil {
		t.emoteOverview.close()
		t.emoteOverview = nil
	}
}

// Updates and channel setup.

func (t *Broadcast) Update(msg tea.Msg) (Tab, tea.Cmd) {
	switch msg := msg.(type) {
	case error:
		if !t.channelDataLoaded {
			return t, nil
		}

		return t, t.localNoticeCmd(msg.Error())
	case setErrorMessage:
		t.err = errors.Join(t.err, msg.err)
		return t, nil
	case SetStreamInfoMessage:
		if t.channelDataLoaded {
			if msg.Target != t.channelID {
				return t, nil
			}

			var cmd tea.Cmd
			t.streamInfo, cmd = t.streamInfo.Update(msg)
			t.HandleResize()
			return t, cmd
		}
	case setChannelDataMessage:
		return t, t.handleChannelDataLoaded(msg)
	case emoteSetRefreshedMessage:
		return t, t.handleEmoteSetRefreshed(msg)
	case ChannelSuggestionsLoadedMessage:
		if t.input != nil {
			t.input.SetChannelSuggestions(msg.Channels)
		} else {
			t.pendingChannelSuggestions = msg.Channels
		}
		return t, nil
	case wspool.EventSubEvent:
		if msg.Error != nil {
			log.Logger.Err(msg.Error).Msg("EventSub error")
			return t, nil
		}
		return t, t.handleEventSubMessage(msg.Message)
	case ChatEventMessage: // delegate message event to chat window
		return t, t.handleChatEvent(msg)
	}

	// while the channel data is loading, only the spinner is animated
	if !t.channelDataLoaded {
		var cmd tea.Cmd
		t.spinner, cmd = t.spinner.Update(msg)
		return t, cmd
	}

	var cmds []tea.Cmd

	if t.focused {
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
			if handled, cmd := t.handleKeyPress(keyMsg); handled {
				return t, cmd
			}
		}

		if t.state.isComposing() {
			cmd, heightChanged := t.input.Update(msg)
			cmds = append(cmds, cmd)

			// Recalculate layout if input height changed (text wrapped/unwrapped)
			if heightChanged {
				t.HandleResize()
			}
		}
	}

	// don't update any components when key message but not focused
	if _, ok := msg.(tea.KeyPressMsg); ok && !t.focused {
		return t, nil
	}

	cmds = append(cmds, t.updateChildren(msg))

	return t, tea.Batch(cmds...)
}

// handleChannelDataLoaded finishes the tab setup once the initial channel data has
// been fetched: it creates all child components, replays recent messages, joins the
// IRC channel and subscribes to EventSub events.
func (t *Broadcast) handleChannelDataLoaded(msg setChannelDataMessage) tea.Cmd {
	var cmds []tea.Cmd

	t.channelDataLoaded = true

	t.channelLogin = msg.channelLogin
	t.channelID = msg.channelID
	t.streamInfo = newStreamInfo(msg.channelID, t.client.API, t.width)
	t.poll = newPoll(t.width)
	t.chatWindow = newChatWindow(t.width, t.height, t.settings, t.theme, t.keymap)

	// enable inline emote replacement; the single line input can only display single row images
	var emoteReplacer component.Replacer
	if graphicRows(t.settings) <= 1 {
		emoteReplacer = t.emoteReplacer
	}
	t.input = newChatInput(t.chatWindow.palette, t.settings.BuildCustomSuggestionMap(), t.theme, emoteReplacer)
	t.input.SetSuggestionKeyMap(t.keymap)

	if len(t.pendingChannelSuggestions) > 0 {
		t.input.SetChannelSuggestions(t.pendingChannelSuggestions)
		t.pendingChannelSuggestions = nil
	}

	t.statusInfo = newStreamStatus(t.width, t.height, t, t.account.ID, msg.channelID, t.theme, t.client.API)

	// set chat suggestions if non-anonymous user
	if !t.account.IsAnonymous {
		t.isUserMod = msg.isUserMod
		t.isModStatusAssumed = msg.modFetchErr != nil

		// if user is broadcaster, allow mod commands
		if t.account.ID == msg.channelID {
			t.isUserMod = true
			t.isModStatusAssumed = false
		}

		// user is mod or broadcaster, include mod commands
		if t.isUserMod {
			t.input.SetIncludeModeratorCommands(true)
		}
	}

	if t.focused {
		t.chatWindow.Focus()
	}

	ircCmds := make([]tea.Cmd, 0, 3)

	if msg.modFetchErr != nil {
		msg.initialMessages = append(msg.initialMessages, &twitchirc.Notice{
			FakeTimestamp:   time.Now(),
			ChannelUserName: t.channelLogin,
			MsgID:           twitchirc.MsgID(uuid.NewString()),
			Message:         fmt.Sprintf("Could not fetch mod status, assuming mod. (%s)", msg.modFetchErr),
		})
	}

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
		return RequestLocalMessageHandleBatchMessage{
			Messages:  msg.initialMessages,
			TabID:     t.id,
			AccountID: t.account.ID,
		}
	})

	// Connect to IRC and join channel via Pool
	accountID := t.account.ID
	channelLogin := msg.channelLogin
	ircCmds = append(ircCmds, func() tea.Msg {
		t.pool.ConnectIRC(accountID)
		t.pool.JoinChannel(accountID, channelLogin)
		return nil
	})

	cmds = append(cmds, t.refreshEmotes(msg.channelLogin, msg.channelID, false))
	cmds = append(cmds, t.subscribeChannelEventCmds(msg.channelID)...)

	t.HandleResize()
	cmds = append(cmds, t.streamInfo.Init(), t.statusInfo.Init(), tea.Sequence(ircCmds...))
	return tea.Batch(cmds...)
}

// subscribeChannelEventCmds returns the commands that subscribe to the channel's
// EventSub events (polls, raids, ad breaks).
//
// Subscriptions are only created
//   - if authenticated user
//   - if channel belongs to user
//
// sadly due to cost limits, we only allow this events users channel not other channels
func (t *Broadcast) subscribeChannelEventCmds(channelID string) []tea.Cmd {
	eventSubAPI := t.client.EventSub
	if eventSubAPI == nil || t.account.ID != channelID {
		return nil
	}

	var cmds []tea.Cmd
	accountID := t.account.ID

	for _, subType := range [...]string{"channel.poll.begin", "channel.poll.progress", "channel.poll.end", "channel.ad_break.begin"} {
		subType := subType // capture for closure
		cmds = append(cmds, func() tea.Msg {
			t.pool.SubscribeEventSub(accountID, twitchapi.CreateEventSubSubscriptionRequest{
				Type:    subType,
				Version: "1",
				Condition: map[string]string{
					"broadcaster_user_id": channelID,
				},
			}, eventSubAPI)
			return nil
		})
	}

	cmds = append(cmds, func() tea.Msg {
		t.pool.SubscribeEventSub(accountID, twitchapi.CreateEventSubSubscriptionRequest{
			Type:    "channel.raid",
			Version: "1",
			Condition: map[string]string{
				"to_broadcaster_user_id": channelID, // broadcaster gets raided
			},
		}, eventSubAPI)
		return nil
	})

	cmds = append(cmds, func() tea.Msg {
		t.pool.SubscribeEventSub(accountID, twitchapi.CreateEventSubSubscriptionRequest{
			Type:    "channel.raid",
			Version: "1",
			Condition: map[string]string{
				"from_broadcaster_user_id": channelID, // another channel gets raided from broadcaster
			},
		}, eventSubAPI)
		return nil
	})

	return cmds
}

// handleEmoteSetRefreshed reacts to a finished emote refresh by recomputing the
// input suggestions from the user's and the channel's emote sets.
func (t *Broadcast) handleEmoteSetRefreshed(msg emoteSetRefreshedMessage) tea.Cmd {
	if t.account.IsAnonymous {
		return nil
	}

	if msg.err != nil && !errors.Is(msg.err, emote.ErrPartialFetch) {
		t.err = errors.Join(t.err, msg.err)
		return nil
	}

	userEmoteSet := t.emoteCache.AllEmotesUsableByUser(t.account.ID)

	log.Info().Str("user-id", t.account.ID).Int("len", len(userEmoteSet)).Msg("fetched emotes for user")

	channelEmoteSet := t.emoteCache.GetAllForChannel(t.channelID) // includes bttv, 7tv

	t.input.SetSuggestions(emoteSuggestions(userEmoteSet, channelEmoteSet))

	// notify user if not all emotes could be fetched
	if errors.Is(msg.err, emote.ErrPartialFetch) {
		return t.localNoticeCmd(msg.err.Error())
	}

	if msg.manually {
		return t.localNoticeCmd("Emotes refreshed manually")
	}

	return nil
}

// emoteSuggestions computes the input suggestion list from the emotes the user can
// use and the channel's third party emotes.
//
// We want to suggest all emotes available to the user. This means we shouldn't
// include twitch sub emotes and others like follower and bits that the user
// doesn't have access to. All twitch emotes for the current channel to which the
// user has access to should be in userEmoteSet, since they are returned from the
// API. So only third party emotes are taken from the channel set.
func emoteSuggestions(userEmoteSet []emote.Emote, channelEmoteSet emote.EmoteSet) []string {
	unique := make(map[string]struct{}, len(userEmoteSet)+len(channelEmoteSet))

	for _, e := range userEmoteSet {
		unique[e.Text] = struct{}{}
	}

	for _, e := range channelEmoteSet {
		if e.Platform == emote.Twitch {
			continue
		}

		unique[e.Text] = struct{}{}
	}

	return slices.Collect(maps.Keys(unique))
}

// handleChatEvent forwards a chat event to the chat window and dependent
// components, unless the message is filtered by the tab's ignore rules.
func (t *Broadcast) handleChatEvent(msg ChatEventMessage) tea.Cmd {
	// ignore all messages that don't target this account and channel
	if t.AccountID() != msg.AccountID || t.channelLogin != msg.Channel && msg.Channel != "" {
		return nil
	}

	if !t.channelDataLoaded {
		return nil
	}

	if t.shouldIgnoreMessage(msg.Message) {
		return nil
	}

	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	if msg, ok := msg.Message.(*twitchirc.PrivateMessage); ok {
		if messageContainsCaseInsensitive(msg, t.account.DisplayName) {
			cmds = append(cmds, func() tea.Msg {
				return RequestNotificationIconMessage{
					TabID: t.id,
				}
			})
		}
	}

	t.chatWindow, cmd = t.chatWindow.Update(msg)
	cmds = append(cmds, cmd)

	// if room state update, update status info
	if _, ok := msg.Message.(*twitchirc.RoomState); ok {
		cmds = append(cmds, t.statusInfo.Init()) // resend init command
	}

	if t.state == userInspectMode {
		t.userInspect, cmd = t.userInspect.Update(msg)
		cmds = append(cmds, cmd)
	}

	// add message content to cache
	if cast, ok := msg.Message.(*twitchirc.PrivateMessage); ok {
		t.lastMessages.Set(cast.Message, struct{}{}, ttlcache.DefaultTTL)
	}

	return tea.Batch(cmds...)
}

// updateChildren fans a message out to all child components that are active in the
// tab's current state.
func (t *Broadcast) updateChildren(msg tea.Msg) tea.Cmd {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

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

	return tea.Batch(cmds...)
}

// Rendering and resizing.

// View renders the tab including the status bar.
func (t *Broadcast) View() string {
	return t.render(true)
}

// ViewWithoutStatusBar returns the body for a pane or vertical tab-list view;
// Root renders the focused tab's status bar at full terminal width.
func (t *Broadcast) ViewWithoutStatusBar() string {
	return t.render(false)
}

// render is the single rendering path of a broadcast tab. Region placement comes
// from the layout computed by HandleResize, so rendering, child sizing and
// hit-testing cannot drift apart.
func (t *Broadcast) render(includeStatus bool) string {
	if t.err != nil {
		return t.centeredStyle.
			Width(t.width).Height(t.height).MaxWidth(t.width).MaxHeight(t.height).
			Render(t.err.Error())
	}

	if !t.channelDataLoaded {
		return t.centeredStyle.
			Width(t.width).Height(t.height).MaxWidth(t.width).MaxHeight(t.height).
			Render(t.spinner.View() + " Loading channel")
	}

	// In emote overview mode the overview replaces the whole content area and
	// only the status bar is appended.
	if t.state == emoteOverviewMode {
		var blocks []string
		if t.layout.emoteOverview.visible() {
			blocks = append(blocks, t.emoteOverview.View())
		}
		if includeStatus && t.layout.status.visible() {
			blocks = append(blocks, t.statusInfo.View())
		}
		return strings.Join(blocks, "\n")
	}

	blocks := make([]string, 0, 6)

	// The stream info slot keeps its row even when the channel is offline, so
	// the chat does not jump when the info appears.
	blocks = append(blocks, t.streamInfo.View())

	if t.layout.poll.visible() {
		blocks = append(blocks, t.poll.View())
	}

	if t.layout.chat.visible() {
		blocks = append(blocks, t.chatWindow.View())
	}

	if t.layout.userInspect.visible() && t.userInspect != nil {
		blocks = append(blocks, t.userInspect.View())
	}

	if t.layout.composer.visible() {
		blocks = append(blocks, t.input.View())
	}

	if includeStatus && t.layout.status.visible() {
		blocks = append(blocks, t.statusInfo.View())
	}

	return strings.Join(blocks, "\n")
}

// StatusBarView returns the status bar for Root's shared status line.
func (t *Broadcast) StatusBarView() string {
	if !t.channelDataLoaded || t.statusInfo == nil {
		return ""
	}
	return t.statusInfo.View()
}

// renderMessageInput renders the message composer. Anonymous accounts cannot
// send messages, so nothing is rendered for them.
func (t *Broadcast) renderMessageInput() string {
	if t.account.IsAnonymous {
		return ""
	}

	return t.input.View()
}

// HandleResize sizes the children and records the layout the view and the mouse
// handlers use. Widths are applied first because the composer and the stream
// info wrap, and their height depends on the width they are rendered at.
func (t *Broadcast) HandleResize() {
	if !t.channelDataLoaded {
		return
	}

	t.applyWidths()

	var statusView string
	if t.fullWidth == 0 {
		// Without an external shared status line, reserve the status row inside
		// this tab. Tiled/vertical views let Root allocate that row separately.
		statusView = t.statusInfo.View()
	}

	in := layoutInputs{
		height:        t.height,
		streamInfo:    lipgloss.Height(t.streamInfo.View()),
		poll:          viewHeight(t.poll.View()),
		status:        viewHeight(statusView),
		inspecting:    t.state.isInspecting(),
		emoteOverview: t.state == emoteOverviewMode,
	}
	if !t.account.IsAnonymous {
		in.composer = t.input.Height()
	}

	t.layout = allocateBroadcastLayout(in)

	t.chatWindow.Resize(t.width, t.layout.chat.height)

	if t.layout.userInspect.visible() {
		t.userInspect.width = t.width
		t.userInspect.height = t.layout.userInspect.height
		t.userInspect.handleResize()
	}

	if t.state == emoteOverviewMode && t.emoteOverview != nil {
		t.emoteOverview.resize(t.width, t.layout.emoteOverview.height)
	}
}

// applyWidths propagates the tab width to the children that wrap their content.
func (t *Broadcast) applyWidths() {
	if t.fullWidth > 0 {
		t.statusInfo.width = t.fullWidth
	} else {
		t.statusInfo.width = t.width
	}

	t.streamInfo.width = t.width
	t.streamInfo.cachedDirty = true
	t.poll.setWidth(t.width)

	// Set the input width before measuring, so its height reflects wrapping at
	// the tab width.
	t.input.SetWidth(t.width)
}

// Layout allocation shared by rendering and mouse hit-testing.

// layoutSpan is the vertical extent a region occupies in the tab view. Rows are
// zero-based and relative to the top of the tab.
type layoutSpan struct {
	top    int
	height int
}

// visible reports whether the region occupies any rows.
func (s layoutSpan) visible() bool { return s.height > 0 }

// contains reports whether y falls inside the region.
func (s layoutSpan) contains(y int) bool { return y >= s.top && y < s.top+s.height }

// broadcastLayout is the row allocation of a broadcast tab. It is the single
// source of truth for three consumers:
//
//   - HandleResize sizes the children from the spans,
//   - the view renders the blocks in span order,
//   - the mouse handlers map a tab-local row back to the region under the cursor.
//
// Blocks are stacked without separator rows: the newline that joins two blocks
// terminates the upper block's last row, so the next block starts immediately
// below it. The stream info slot is the exception: it always occupies at least
// one row, even when the channel is offline, so the chat does not jump when the
// stream info appears.
type broadcastLayout struct {
	height int

	streamInfo    layoutSpan
	poll          layoutSpan
	chat          layoutSpan
	userInspect   layoutSpan
	composer      layoutSpan
	status        layoutSpan
	emoteOverview layoutSpan // replaces the content area in emote overview mode
}

// layoutInputs are the measured row counts of the fixed-size regions around the
// flexible chat area. Passing measurements instead of child models keeps the
// allocation a pure function that can be tested on its own.
type layoutInputs struct {
	height        int
	streamInfo    int // rows the stream info slot occupies; at least 1
	poll          int // rows the poll occupies; 0 when absent
	composer      int // rows the composer occupies; 0 when absent
	status        int // rows the status bar occupies inside the tab; 0 when absent
	inspecting    bool
	emoteOverview bool
}

// allocateBroadcastLayout distributes the available rows. The chat window and
// the user inspect window share the leftover space; in inspect mode the chat
// keeps half of the space above the composer, matching the pre-refactor
// allocation.
func allocateBroadcastLayout(in layoutInputs) broadcastLayout {
	l := broadcastLayout{height: in.height}

	if in.emoteOverview {
		overviewHeight := max(0, in.height-in.status)
		l.emoteOverview = layoutSpan{top: 0, height: overviewHeight}
		l.status = layoutSpan{top: overviewHeight, height: in.status}

		return l
	}

	l.streamInfo = layoutSpan{top: 0, height: max(1, in.streamInfo)}
	top := l.streamInfo.height

	l.poll = layoutSpan{top: top, height: in.poll}
	top += in.poll

	var chatHeight int
	if in.inspecting {
		chatHeight = (in.height - l.streamInfo.height - in.poll - in.status) / 2
	} else {
		chatHeight = in.height - l.streamInfo.height - in.poll - in.composer - in.status
	}

	chatHeight = max(0, chatHeight)

	l.chat = layoutSpan{top: top, height: chatHeight}
	top += chatHeight

	if in.inspecting {
		inspectHeight := in.height - l.streamInfo.height - in.poll - chatHeight - in.status - in.composer

		l.userInspect = layoutSpan{top: top, height: max(0, inspectHeight)}
		top += l.userInspect.height
	}

	l.composer = layoutSpan{top: top, height: max(0, in.composer)}
	top += l.composer.height

	l.status = layoutSpan{top: top, height: max(0, in.status)}

	return l
}

// viewHeight returns the number of rows a rendered block occupies. An empty
// block occupies no rows, unlike lipgloss.Height which would report one.
func viewHeight(view string) int {
	if view == "" {
		return 0
	}

	return lipgloss.Height(view)
}

// Mode transitions and focus ownership.

// A broadcast tab is a small state machine: which widget owns the keyboard, which
// overlay is open, and what gets rendered all follow from its mode.
//
// Only the transitions declared in tabTransitions are legal. Every mode change
// goes through transit, so the table is the single place that says what may
// follow what, and the entry and exit actions are the single place that moves
// focus. Before this existed the same information was spread over ~30 guards and
// 15 assignments across five files, and "which child is focused" was a convention
// rather than an invariant.

// tabEvent is an input that may move the tab to another mode.
type tabEvent uint8

const (
	// eventCompose starts writing a message. The insert key, reply, copy and the
	// timeout shortcut all mean "compose in this tab".
	eventCompose tabEvent = iota
	// eventSend sends the composed message and returns to viewing.
	eventSend
	// eventCancel is Escape.
	eventCancel
	// eventInspect opens the user inspect window for the selected message.
	eventInspect
	// eventEmoteOverview opens the emote overview.
	eventEmoteOverview
	// eventLeaveInspect closes the user inspect window without returning to the
	// composer, used when the user clicks outside of it.
	eventLeaveInspect
)

// tabTransition is one row of the mode table.
type tabTransition struct {
	to    broadcastTabState
	guard func(*Broadcast) bool
	exit  func(*Broadcast)
	enter func(*Broadcast) tea.Cmd
}

// tabTransitions is the mode table. A pair that is not listed cannot happen: the
// matching key press or click is reported as unhandled instead.
var tabTransitions = map[broadcastTabState]map[tabEvent]tabTransition{
	inChatWindow: {
		eventCompose:       {to: insertMode, guard: canInteract, exit: blurChatWindow, enter: focusMessageInput},
		eventInspect:       {to: userInspectMode, guard: hasInspectWindow, exit: blurChatWindow, enter: focusInspectWindow},
		eventEmoteOverview: {to: emoteOverviewMode, guard: canInteract, exit: blurChatWindow},
	},
	insertMode: {
		eventSend:          {to: inChatWindow, exit: clearMessageInput, enter: focusChatWindow},
		eventCancel:        {to: inChatWindow, exit: cancelMessageInput, enter: focusChatWindow},
		eventInspect:       {to: userInspectMode, guard: hasInspectWindow, exit: cancelMessageInput, enter: focusInspectWindow},
		eventEmoteOverview: {to: emoteOverviewMode, guard: canInteract, exit: cancelMessageInput},
	},
	userInspectMode: {
		eventCompose:       {to: userInspectInsertMode, guard: canInteract, exit: blurChatWindows, enter: focusMessageInput},
		eventCancel:        {to: inChatWindow, exit: dropInspectWindow, enter: focusChatWindow},
		eventInspect:       {to: userInspectMode, guard: hasInspectWindow, exit: blurChatWindow, enter: focusInspectWindow},
		eventEmoteOverview: {to: emoteOverviewMode, guard: canInteract, exit: leaveInspectForOverview},
		eventLeaveInspect:  {to: inChatWindow, exit: leaveInspect, enter: focusChatWindow},
	},
	userInspectInsertMode: {
		eventSend:          {to: userInspectMode, exit: clearMessageInput, enter: focusInspectWindow},
		eventCancel:        {to: userInspectMode, exit: cancelMessageInput, enter: focusInspectWindow},
		eventInspect:       {to: userInspectMode, guard: hasInspectWindow, exit: cancelMessageInput, enter: focusInspectWindow},
		eventLeaveInspect:  {to: inChatWindow, exit: leaveInspect, enter: focusChatWindow},
		eventEmoteOverview: {to: emoteOverviewMode, guard: canInteract, exit: leaveInspectForOverview},
	},
	emoteOverviewMode: {
		eventCancel: {to: inChatWindow, exit: dropInspectWindow, enter: focusChatWindow},
	},
}

// transit applies the declared transition for (mode, event). It reports whether
// the event was accepted and returns the command produced by the entry action.
func (t *Broadcast) transit(event tabEvent) (bool, tea.Cmd) {
	transition, ok := tabTransitions[t.state][event]
	if !ok {
		return false, nil
	}

	if transition.guard != nil && !transition.guard(t) {
		return false, nil
	}

	if transition.exit != nil {
		transition.exit(t)
	}

	t.state = transition.to

	if transition.enter != nil {
		return true, transition.enter(t)
	}

	return true, nil
}

// --- guards ---

// canInteract blocks the transitions that need an authenticated account: anonymous
// accounts can read chat but cannot compose messages or open the emote overview.
func canInteract(t *Broadcast) bool {
	return !t.account.IsAnonymous
}

// hasInspectWindow rejects entering inspect mode without an inspect window. The
// caller builds the window first (it needs the username to do so); without this
// guard a caller that forgot would leave the tab in a mode that renders a nil
// window.
func hasInspectWindow(t *Broadcast) bool {
	return t.userInspect != nil
}

// --- exit actions ---

func blurChatWindow(t *Broadcast) {
	t.chatWindow.Blur()
}

func blurChatWindows(t *Broadcast) {
	t.chatWindow.Blur()

	if t.userInspect != nil {
		t.userInspect.chatWindow.Blur()
	}
}

// clearMessageInput drops the sent message, its reply target and the focus that
// came with it.
func clearMessageInput(t *Broadcast) {
	t.input.Blur()
	t.input.Clear()
	t.clearReplyTarget()
}

// cancelMessageInput keeps the typed text but drops focus and the reply target.
func cancelMessageInput(t *Broadcast) {
	t.input.Blur()
	t.clearReplyTarget()
}

func dropInspectWindow(t *Broadcast) {
	t.userInspect = nil
}

// leaveInspectForOverview drops the inspect window when the emote overview takes
// over the whole tab.
func leaveInspectForOverview(t *Broadcast) {
	blurChatWindows(t)
	dropInspectWindow(t)
	cancelMessageInput(t)
}

// leaveInspect closes the inspect window, including its composer.
func leaveInspect(t *Broadcast) {
	t.userInspect = nil
	t.input.Blur()
	t.clearReplyTarget()
}

// --- entry actions ---

func focusMessageInput(t *Broadcast) tea.Cmd {
	return t.input.Focus()
}

func focusChatWindow(t *Broadcast) tea.Cmd {
	t.chatWindow.Focus()
	return nil
}

func focusInspectWindow(t *Broadcast) tea.Cmd {
	if t.userInspect != nil {
		t.userInspect.chatWindow.Focus()
	}

	return nil
}

// --- mode predicates ---

// isComposing reports whether the message composer owns the keyboard.
func (s broadcastTabState) isComposing() bool {
	return s == insertMode || s == userInspectInsertMode
}

// isInspecting reports whether the user inspect window is open, with or without
// the composer.
func (s broadcastTabState) isInspecting() bool {
	return s == userInspectMode || s == userInspectInsertMode
}

// isViewing reports whether a chat window owns the selection: no composer, no
// overlay. Selection and navigation shortcuts are only live in this state.
func (s broadcastTabState) isViewing() bool {
	return s == inChatWindow || s == userInspectMode
}

// Keyboard interaction.

// handleKeyPress dispatches a key press while the tab is focused and the channel
// data is loaded. It reports whether the key was handled; unhandled keys fall
// through to the message input and child components.
func (t *Broadcast) handleKeyPress(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	// Focus message input, when not in insert mode and not in search mode inside chat window, depending on the current active chat window
	if key.Matches(msg, t.keymap.InsertMode) &&
		(t.state == inChatWindow && t.chatWindow.state != searchChatWindowState || t.state == userInspectMode && t.userInspect.chatWindow.state != searchChatWindowState) {
		return true, t.handleStartInsertMode()
	}

	// Plain-letter shortcuts must not fire while a chat search input is focused.
	searching := t.isChatSearching()

	// Open user inspect mode, where only messages from a specific user are shown
	if key.Matches(msg, t.keymap.InspectMode) && !searching && t.state.isViewing() {
		return true, t.handleOpenUserInspectFromMessage()
	}

	// Open chat in browser
	if key.Matches(msg, t.keymap.ChatPopUp, t.keymap.ChannelPopUp) && !searching && t.state.isViewing() {
		return true, t.handleOpenBrowser(msg)
	}

	// Send message
	if key.Matches(msg, t.keymap.Confirm) && len(t.input.Value()) > 0 && t.state.isComposing() {
		t.input.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // commit history entry
		return true, t.handleMessageSent(false)
	}

	// Send message - quick send
	if key.Matches(msg, t.keymap.QuickSent) && len(t.input.Value()) > 0 && t.state.isComposing() {
		t.input.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // commit history entry
		return true, t.handleMessageSent(true)
	}

	// Message Accept Suggestion Template Replace
	// always allow accept suggestion key so even new texts can be templated
	if t.input.IsAcceptSuggestionKey(msg) && len(t.input.Value()) > 0 && t.state.isComposing() {
		_, heightChanged := t.input.Update(msg)
		cmd := t.replaceInputTemplate()
		if heightChanged {
			t.HandleResize()
		}
		return true, cmd
	}

	// Set quick time out message to message input
	if key.Matches(msg, t.keymap.QuickTimeout) && !searching && t.state.isViewing() {
		t.handleTimeoutShortcut()
		return true, nil
	}

	// Copy selected message to message input
	if key.Matches(msg, t.keymap.CopyMessage) && !searching && t.state.isViewing() {
		t.handleCopyMessage()
		return true, nil
	}

	// Reply to selected message
	if key.Matches(msg, t.keymap.ReplyMessage) && !searching && t.state.isViewing() {
		t.handleReplyMessage()
		return true, nil
	}

	// Close overlay windows
	if key.Matches(msg, t.keymap.Escape) {
		var cmd tea.Cmd

		// first end search in user inspect sub window
		if t.userInspect != nil && t.userInspect.chatWindow.state == searchChatWindowState {
			t.userInspect.chatWindow, cmd = t.userInspect.chatWindow.Update(msg)
			return true, cmd
		}

		// second case, end inspect mode or end insert mode in inspect window
		if t.state.isInspecting() {
			t.handleEscapePressed()
			return true, nil
		}

		// third case, end search in 'main' chat window
		if t.chatWindow.state == searchChatWindowState {
			t.chatWindow, cmd = t.chatWindow.Update(msg)
			return true, cmd
		}

		t.handleEscapePressed()
		return true, nil
	}

	return false, nil
}

// isChatSearching reports whether the active chat window is capturing text for
// search. Plain-letter shortcuts are suppressed while this is true so they can
// be typed into the search input instead.
func (t *Broadcast) isChatSearching() bool {
	if t.state.isInspecting() {
		return t.userInspect != nil && t.userInspect.chatWindow.state == searchChatWindowState
	}

	return t.chatWindow != nil && t.chatWindow.state == searchChatWindowState
}

// leaveUserInspect exits user inspect mode (including its insert mode) and
// returns focus to the main chat window.
func (t *Broadcast) leaveUserInspect() {
	changed, _ := t.transit(eventLeaveInspect)
	if !changed {
		return
	}

	t.HandleResize()
	t.chatWindow.updatePort()
}

func (t *Broadcast) handleEscapePressed() {
	// Leaving a mode that replaced the chat window with a full-sized one changes
	// the layout; leaving the composer does not.
	fullWindow := t.state.isInspecting() || t.state == emoteOverviewMode

	changed, _ := t.transit(eventCancel)
	if !changed {
		return
	}

	if fullWindow && t.state == inChatWindow {
		t.HandleResize()
		t.chatWindow.updatePort()
	}
}

func (t *Broadcast) handleOpenBrowser(msg tea.KeyPressMsg) tea.Cmd {
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

func (t *Broadcast) handleOpenBrowserChatPopUp() tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf(streamChatPopUpFmt, t.channelLogin)

		if err := browser.OpenURL(url); err != nil {
			log.Logger.Error().Err(err).Msg("error while opening twitch channel in browser")
		}
		return nil
	}
}

func (t *Broadcast) handleOpenBrowserChannel() tea.Cmd {
	return func() tea.Msg {
		url := fmt.Sprintf(streamWebFmt, t.channelLogin)

		if err := browser.OpenURL(url); err != nil {
			log.Logger.Error().Err(err).Msg("error while opening twitch channel in browser")
		}
		return nil
	}
}

func (t *Broadcast) handleStartInsertMode() tea.Cmd {
	_, cmd := t.transit(eventCompose)
	return cmd
}

func (t *Broadcast) handleCopyMessage() {
	entry, ok := t.cursorEntry()
	if !ok {
		return
	}

	msg, ok := entry.Event.Message.(*twitchirc.PrivateMessage)
	if !ok {
		return
	}

	// The table rejects this for anonymous accounts, which cannot compose.
	if accepted, _ := t.transit(eventCompose); !accepted {
		return
	}

	t.clearReplyTarget() // copying replaces the armed reply with different content
	t.input.SetValue(strings.ReplaceAll(msg.Message, string(duplicateBypass), ""))
	t.HandleResize() // Recalculate layout after copying message
}

// handleReplyMessage arms a reply to the message under the cursor and starts insert mode.
func (t *Broadcast) handleReplyMessage() {
	if t.account.IsAnonymous {
		return
	}

	entry, ok := t.cursorEntry()
	if !ok {
		return
	}

	msg, ok := entry.Event.Message.(*twitchirc.PrivateMessage)

	// Without an original message ID there is nothing to thread onto, so ignore the
	// keypress instead of arming a reply Twitch would reject.
	if !ok || msg.ID == "" {
		return
	}

	if accepted, _ := t.transit(eventCompose); !accepted {
		return
	}

	t.input.ArmReply(msg.ID, msg.DisplayName, msg.Message)
	t.HandleResize()
}

func (t *Broadcast) handleOpenUserInspectFromMessage() tea.Cmd {
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
	switch msg := e.Event.Message.(type) {
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

func (t *Broadcast) handleTimeoutShortcut() {
	if t.account.IsAnonymous {
		return
	}

	entry, ok := t.cursorEntry()
	if !ok {
		return
	}

	msg, ok := entry.Event.Message.(*twitchirc.PrivateMessage)
	if !ok {
		return
	}

	if accepted, _ := t.transit(eventCompose); !accepted {
		return
	}

	t.input.SetValue("/timeout " + msg.DisplayName + " 600")
	t.HandleResize() // Recalculate layout after setting timeout command
}

// activeChatWindow returns the chat window that owns the selection, or nil when
// the tab shows none.
func (t *Broadcast) activeChatWindow() *chatWindow {
	if t.state.isInspecting() {
		if t.userInspect == nil {
			return nil
		}

		return t.userInspect.chatWindow
	}

	return t.chatWindow
}

// cursorEntry returns the entry under the cursor of the active chat window. An
// active search is stopped first so the cursor resolves against the full list.
func (t *Broadcast) cursorEntry() (*chatEntry, bool) {
	window := t.activeChatWindow()
	if window == nil {
		return nil, false
	}

	if window.state == searchChatWindowState {
		window.handleStopSearchMode()
	}

	_, entry := window.entryForCurrentCursor()
	if entry == nil {
		return nil, false
	}

	t.chatWindow.Blur()

	if t.userInspect != nil {
		t.userInspect.chatWindow.Blur()
	}

	return entry, true
}

func (t *Broadcast) handleOpenEmoteOverview() tea.Cmd {
	if accepted, _ := t.transit(eventEmoteOverview); !accepted {
		return nil
	}

	if t.emoteOverview != nil {
		t.HandleResize()
		return nil
	}

	t.emoteOverview = NewEmoteOverview(t.channelID, t.emoteCache, t.emoteReplacer, graphicRows(t.settings), t.width, t.height)
	t.HandleResize()
	return t.emoteOverview.Init()
}

func (t *Broadcast) handleManualRefreshEmotes() tea.Cmd {
	if t.account.IsAnonymous {
		return nil
	}

	t.emoteCache.RemoveEmoteSetForChannel(t.channelID)

	return t.refreshEmotes(t.channelLogin, t.channelID, true)
}

// Mouse interaction.

// doubleClickWindow is how long two clicks on the same message may be apart to
// count as a double-click. Terminals only report individual presses, so the
// gesture is synthesized here.
const doubleClickWindow = 400 * time.Millisecond

// registerClick records the clicked entry and reports whether it forms a
// double-click with the previous click. Clicks outside the entry's line range
// (for example on the blank padding below the last message) reset the tracker.
func (t *Broadcast) registerClick(entry *chatEntry, line int) bool {
	if entry == nil || line < entry.Position.CursorStart || line > entry.Position.CursorEnd {
		t.lastClickEntry = nil
		t.lastClickAt = time.Time{}
		return false
	}

	now := time.Now()
	double := entry == t.lastClickEntry && now.Sub(t.lastClickAt) <= doubleClickWindow

	t.lastClickEntry = entry
	t.lastClickAt = now

	return double
}

// inspectChatSpan is the clickable history area inside the user inspect window,
// in tab-local rows. The window's own span also covers the info banner.
func (t *Broadcast) inspectChatSpan() layoutSpan {
	chat := t.userInspect.chatSpan()
	chat.top += t.layout.userInspect.top

	return chat
}

func (t *Broadcast) HandleMouseClick(msg tea.MouseClickMsg, x, y int) tea.Cmd {
	if msg.Button != tea.MouseLeft || !t.channelDataLoaded || t.err != nil {
		return nil
	}

	if t.state == emoteOverviewMode {
		return nil
	}

	// Main chat window: select the clicked message.
	if t.layout.chat.contains(y) {
		// Resolve the absolute line before any layout change, then leave
		// insert/inspect mode so subsequent shortcuts act on the main chat.
		line := t.chatWindow.viewportRenderStart() + (y - t.layout.chat.top)
		_, entry := t.chatWindow.entryAtLine(line)
		double := t.registerClick(entry, line)

		switch t.state {
		case insertMode:
			t.handleEscapePressed()
		case userInspectMode, userInspectInsertMode:
			t.leaveUserInspect()
		}

		t.chatWindow.selectLine(line)

		if double {
			return t.handleOpenUserInspectFromMessage()
		}

		return nil
	}

	// User inspect window: select the clicked message. Only the history below
	// the info banner is interactive.
	if t.state.isInspecting() && t.layout.userInspect.visible() {
		if chat := t.inspectChatSpan(); chat.contains(y) {
			if t.state == userInspectInsertMode {
				t.handleEscapePressed()
			}

			row := y - chat.top
			line := t.userInspect.chatWindow.viewportRenderStart() + row
			_, entry := t.userInspect.chatWindow.entryAtLine(line)
			double := t.registerClick(entry, line)

			t.userInspect.chatWindow.selectAtViewportRow(row)

			if double {
				return t.handleOpenUserInspectFromMessage()
			}

			return nil
		}
	}

	// Message composer: click to start typing.
	if !t.account.IsAnonymous && t.layout.composer.contains(y) {
		return t.handleStartInsertMode()
	}

	return nil
}

func (t *Broadcast) HandleMouseWheel(msg tea.MouseWheelMsg, x, y int) tea.Cmd {
	if !t.channelDataLoaded || t.err != nil {
		return nil
	}

	delta := WheelDelta(msg)
	if delta == 0 {
		return nil
	}

	if t.state == emoteOverviewMode {
		if t.emoteOverview == nil {
			return nil
		}

		var cmd tea.Cmd
		t.emoteOverview, cmd = t.emoteOverview.Update(msg)
		return cmd
	}

	if t.layout.chat.contains(y) {
		t.chatWindow.scrollByLines(delta)
		t.chatWindow.snapScroll()
		return nil
	}

	if t.state.isInspecting() && t.layout.userInspect.visible() && t.inspectChatSpan().contains(y) {
		t.userInspect.chatWindow.scrollByLines(delta)
		t.userInspect.chatWindow.snapScroll()
	}

	return nil
}

// Message filtering, sending, and input templates.

func (t *Broadcast) shouldIgnoreMessage(msg twitchirc.IRCer) bool {
	if messageMatchesBlocked(msg, t.settings.BlockSettings) {
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

	if t.isUniqueOnlyChat && isNearDuplicate(cast.Message, t.lastMessages.Keys()) {
		return true
	}

	return false
}

// isNearDuplicate reports whether message is (nearly) identical to one of the
// recently seen messages. A single-word message only matches a case-insensitively
// equal entry; longer messages match when the Levenshtein distance to any recent
// message is small.
func isNearDuplicate(message string, recent []string) bool {
	wordsSrc := strings.Fields(message)
	lenWords := len(wordsSrc)

	// a one-word message is a duplicate when it was recently seen verbatim
	if lenWords == 1 {
		return slices.ContainsFunc(recent, func(e string) bool { return strings.EqualFold(e, message) })
	}

	uniqueWordsSrc := map[string]struct{}{}
	for word := range slices.Values(wordsSrc) {
		uniqueWordsSrc[word] = struct{}{}
	}

	// uniqueWordsTarget := map[string]struct{}{}
	for stored := range slices.Values(recent) {
		distance := fuzzy.LevenshteinDistance(message, stored)
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

	return false
}

// handleMessageSent processes the content of the message input after a send key
// press: slash commands are dispatched to their handlers, everything else is sent
// to the channel as a chat message.
func (t *Broadcast) handleMessageSent(quickSend bool) tea.Cmd {
	input := t.input.Value()

	// Consume the armed reply before the command dispatch below: commands, quick-sends
	// and unsupported entries must never send a stale reply target.
	replyTo := t.takeReplyTarget()

	if !quickSend {
		t.resetAfterSend()
	}

	t.chatWindow.moveToBottom()

	if name, args, ok := parseSlashCommand(input); ok {
		return t.dispatchSlashCommand(name, args)
	}

	input = applyDuplicateBypass(input, t.lastMessageSent)

	cmd := t.sendChatMessageCmd(input, replyTo)

	t.lastMessageSent = input
	t.lastMessageSentAt = time.Now()

	return cmd
}

// resetAfterSend leaves insert mode and clears the message input after a regular
// (non quick) send.
func (t *Broadcast) resetAfterSend() {
	_, _ = t.transit(eventSend)
	t.HandleResize() // Recalculate layout after clearing input
}

// parseSlashCommand splits a chat input of the form "/name arg1 arg2" into its
// command name and arguments. ok is false when the input is not a slash command.
// A command without arguments yields args == [""]; the command handlers rely on
// this historical shape when validating their arguments.
func parseSlashCommand(input string) (name string, args []string, ok bool) {
	if !strings.HasPrefix(input, "/") {
		return "", nil, false
	}

	end := strings.Index(input, " ")
	if end == -1 {
		end = len(input)
	}

	name = input[1:end]
	argStr := strings.TrimSpace(input[end:])

	return name, strings.Split(argStr, " "), true
}

// dispatchSlashCommand routes a parsed slash command to its handler. Commands
// without a local handler fall through to the moderation command handling, which
// is only available to moderators.
func (t *Broadcast) dispatchSlashCommand(name string, args []string) tea.Cmd {
	switch name {
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
	case "join":
		return t.handleJoinCommand(args)
	}

	if !t.isUserMod {
		return func() tea.Msg {
			respMsg := ChatEventMessage{
				IsFakeEvent: true,
				AccountID:   t.account.ID,
				TabID:       t.id,
				Message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Moderator commands are not available since you are not a moderator",
				},
			}
			return respMsg
		}
	}

	// Moderation commands need the account's own token.
	client := t.client.Moderation
	if client == nil {
		return t.localNoticeCmd("This account cannot use moderation commands")
	}

	return handleCommand(name, args, t.channelID, t.channelLogin, t.account.ID, client)
}

// applyDuplicateBypass appends an invisible character when input repeats the
// previously sent message, bypassing Twitch's duplicate message filter.
func applyDuplicateBypass(input, lastSent string) string {
	if strings.EqualFold(input, lastSent) {
		return input + " " + string(duplicateBypass)
	}

	return input
}

// sendChatMessageCmd returns the command that sends input as a chat message via
// the API, delayed so that consecutive sends are at least a second apart.
func (t *Broadcast) sendChatMessageCmd(input, replyTo string) tea.Cmd {
	lastSent := t.lastMessageSentAt
	client := t.client.Chat
	if client == nil {
		return nil
	}

	broadcasterID := t.channelID
	userID := t.account.ID

	return func() tea.Msg {
		const delay = time.Second
		diff := time.Since(lastSent)
		if diff < delay {
			time.Sleep(delay - diff)
		}

		notice := &twitchirc.Notice{
			FakeTimestamp: time.Now(),
		}

		resp := ChatEventMessage{
			IsFakeEvent: true,
			AccountID:   userID,
			Channel:     t.channelLogin,
			ChannelID:   t.channelID,
			TabID:       t.id,
			Message:     notice,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		r, err := client.SendChatMessage(ctx, twitchapi.SendChatMessageRequest{
			BroadcasterID:  broadcasterID,
			SenderID:       userID,
			Message:        input,
			ReplyMessageID: replyTo,
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
}

func (t *Broadcast) replaceInputTemplate() tea.Cmd {
	input := t.input.RawValue()

	notice := &twitchirc.Notice{
		FakeTimestamp: time.Now(),
	}

	resp := ChatEventMessage{
		IsFakeEvent: true,
		AccountID:   t.account.ID,
		Channel:     t.channelLogin,
		ChannelID:   t.channelID,
		TabID:       t.id,
		Message:     notice,
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
		switch msg := e.Event.Message.(type) {
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

	t.input.SetValue(inputText)
	t.HandleResize() // Recalculate layout after template replacement
	return nil
}

// Reply lifecycle.

// takeReplyTarget returns the armed reply's message ID and disarms the reply. The
// layout is recalculated because the indicator row is given back to the chat window.
func (t *Broadcast) takeReplyTarget() string {
	id := t.input.TakeReply()
	if id != "" {
		t.HandleResize()
	}

	return id
}

// clearReplyTarget disarms the armed reply. The reply indicator occupies a row above
// the message input, so the layout is recalculated here: otherwise the chat window
// keeps the height it had while the indicator was visible and the view ends up a row
// short, floating the status bar above the bottom of the terminal.
func (t *Broadcast) clearReplyTarget() {
	// A tab has no composer before its channel data loaded, and therefore no reply
	// to disarm. Guarding here keeps every caller safe rather than only the ones
	// that remember to check.
	if t.input == nil {
		return
	}

	if t.input.ClearReply() {
		t.HandleResize()
	}
}

// Local commands.

func (t *Broadcast) handleJoinCommand(args []string) tea.Cmd {
	if len(args) < 1 || args[0] == "" {
		return func() tea.Msg {
			return ChatEventMessage{
				AccountID: t.account.ID,
				Channel:   t.channelLogin,
				ChannelID: t.channelID,
				TabID:     t.id,
				Message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Expected Usage: /join <channel>",
				},
			}
		}
	}

	channel := strings.ToLower(args[0])
	account := t.account
	client := t.client.API

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resp, err := client.GetUsers(ctx, []string{channel}, nil)
		if err == nil && len(resp.Data) > 0 {
			channel = resp.Data[0].Login
		}

		return JoinChannelMessage{
			TabKind: BroadcastKind,
			Channel: channel,
			Account: account,
		}
	}
}

// handlePyramidMessagesCommand build a message pyramid with the given word and count
// like this:
// word
// word word
// word word word
// word word
// word
func (t *Broadcast) handlePyramidMessagesCommand(args []string) tea.Cmd {
	accountIsStreamer := t.account.ID == t.channelID

	if !accountIsStreamer && t.statusInfo != nil && t.statusInfo.settings.SlowMode {
		return func() tea.Msg {
			return ChatEventMessage{
				AccountID: t.account.ID,
				Channel:   t.channelLogin,
				ChannelID: t.channelID,
				TabID:     t.id,
				Message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Pyramid command is disabled in slow mode",
				},
			}
		}
	}

	if len(args) < 2 {
		return func() tea.Msg {
			return ChatEventMessage{
				AccountID: t.account.ID,
				Channel:   t.channelLogin,
				ChannelID: t.channelID,
				TabID:     t.id,
				Message: &twitchirc.Notice{
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
			return ChatEventMessage{
				AccountID: t.account.ID,
				Channel:   t.channelLogin,
				ChannelID: t.channelID,
				TabID:     t.id,
				Message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Failed to convert count to integer",
				},
			}
		}
	}

	client := t.client.Chat
	if client == nil {
		log.Logger.Warn().Str("account", t.account.DisplayName).Msg("account has no authenticated API client")
		return nil
	}

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

		resp := ChatEventMessage{
			IsFakeEvent: true,
			AccountID:   userID,
			Channel:     t.channelLogin,
			ChannelID:   t.channelID,
			TabID:       t.id,
			Message:     notice,
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

func (t *Broadcast) handleLocalSubCommand(enable bool) tea.Cmd {
	if enable && t.isLocalSub {
		return func() tea.Msg {
			return ChatEventMessage{
				AccountID: t.account.ID,
				Channel:   t.channelLogin,
				ChannelID: t.channelID,
				TabID:     t.id,
				Message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Already in local submode",
				},
			}
		}
	}

	if !enable && !t.isLocalSub {
		return func() tea.Msg {
			return ChatEventMessage{
				AccountID: t.account.ID,
				Channel:   t.channelLogin,
				ChannelID: t.channelID,
				TabID:     t.id,
				Message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Already out of local submode",
				},
			}
		}
	}

	t.isLocalSub = enable

	return nil
}

func (t *Broadcast) handleUniqueOnlyChatCommand(enable bool) tea.Cmd {
	if enable && t.isUniqueOnlyChat {
		return func() tea.Msg {
			return ChatEventMessage{
				AccountID: t.account.ID,
				Channel:   t.channelLogin,
				ChannelID: t.channelID,
				TabID:     t.id,
				Message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Already in unique only chat",
				},
			}
		}
	}

	if !enable && !t.isUniqueOnlyChat {
		return func() tea.Msg {
			return ChatEventMessage{
				AccountID: t.account.ID,
				Channel:   t.channelLogin,
				ChannelID: t.channelID,
				TabID:     t.id,
				Message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Already out of unique only chat",
				},
			}
		}
	}

	t.isUniqueOnlyChat = enable

	return nil
}

func (t *Broadcast) handleCreateClipMessage() tea.Cmd {
	return func() tea.Msg {
		api := t.client.Chat
		if api == nil {
			log.Logger.Warn().Str("broadcast", t.channelLogin).Str("account", t.account.DisplayName).Msg("provided API does not support user authenticated API")
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
		defer cancel()

		clip, err := api.CreateClip(ctx, t.channelID, false)

		notice := &twitchirc.Notice{
			FakeTimestamp: time.Now(),
		}

		resp := ChatEventMessage{
			IsFakeEvent: true,
			AccountID:   t.account.ID,
			Channel:     t.channelLogin,
			ChannelID:   t.channelID,
			TabID:       t.id,
			Message:     notice,
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

func (t *Broadcast) handleOpenUserInspect(args []string) tea.Cmd {
	var cmds []tea.Cmd

	if len(args) < 1 {
		return nil
	}

	username := args[0]

	t.userInspect = newUserInspect(t.id, t.width, t.height, username, t.channelLogin, t.account.ID,
		t.settings, t.theme, t.keymap, t.client.API, t.messageLogger, t.emoteReplacer, t.badgeReplacer)
	t.userInspect.chatWindow.UsePalette(t.chatWindow.palette)

	// The table blurs the chat window and focuses the new inspect window.
	_, _ = t.transit(eventInspect)

	initialEvents := make([]ChatEventMessage, 0, 15)
	for e := range slices.Values(t.chatWindow.entries) {
		initialEvents = append(initialEvents, ChatEventMessage{
			IsFakeEvent:             true,
			AccountID:               t.account.ID,
			Channel:                 t.channelLogin,
			ChannelID:               t.channelID,
			Message:                 e.Event.Message,
			DisplayModifier:         e.Event.DisplayModifier,
			ChannelGuestID:          e.Event.ChannelGuestID,
			ChannelGuestDisplayName: e.Event.ChannelGuestDisplayName,
		})
	}

	cmds = append(cmds, t.userInspect.init(initialEvents))
	t.HandleResize()

	return tea.Batch(cmds...)
}

// EventSub notifications.

func (t *Broadcast) handleEventSubMessage(msg eventsub.Message[eventsub.NotificationPayload]) tea.Cmd {
	if msg.Payload.Subscription.Condition["broadcaster_user_id"] != t.channelID &&
		msg.Payload.Subscription.Condition["from_broadcaster_user_id"] != t.channelID &&
		msg.Payload.Subscription.Condition["to_broadcaster_user_id"] != t.channelID {
		return nil
	}

	createCMDFunc := func(ircer twitchirc.IRCer) tea.Cmd {
		return func() tea.Msg {
			return RequestLocalMessageHandleMessage{
				Message:   ircer,
				AccountID: t.AccountID(),
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

		adDuration := humanizeDuration(time.Duration(msg.Payload.Event.DurationInSeconds) * time.Second)
		if msg.Payload.Event.IsAutomatic {
			chatMsg = fmt.Sprintf("An automatic %s ad just started!", adDuration)
		} else {
			chatMsg = fmt.Sprintf("A %s ad, requested by %s, just started!", adDuration, msg.Payload.Event.RequesterUserName)
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
