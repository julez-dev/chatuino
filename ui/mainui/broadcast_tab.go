package mainui

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"golang.org/x/sync/errgroup"

	"github.com/julez-dev/chatuino/save"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/badge"
	"github.com/julez-dev/chatuino/twitch/ivr"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/nav"
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

type broadcastTab struct {
	id      string
	account save.Account

	state            broadcastTabState
	isLocalSub       bool
	isUniqueOnlyChat bool
	lastMessages     *ttlcache.Cache[string, struct{}]

	isUserMod          bool
	isModStatusAssumed bool // true when mod fetch failed; don't show "Mod" in status
	focused            bool
	updateInfo         *UpdateInfo

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
	fullWidth     int // full terminal width (for status bar in vertical mode)

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

func newBroadcastTab(
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
) *broadcastTab {
	cache := ttlcache.New(
		ttlcache.WithTTL[string, struct{}](time.Second * 10),
	)
	go cache.Start()

	return &broadcastTab{
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
		spinner:        spinner.New(spinner.WithSpinner(loadingSpinner)),

		centeredStyle: lipgloss.NewStyle().AlignHorizontal(lipgloss.Center).AlignVertical(lipgloss.Center),
	}
}

// localNoticeCmd returns a command that shows an informational notice only in this
// tab, without sending anything to Twitch.
func (t *broadcastTab) localNoticeCmd(message string) tea.Cmd {
	return func() tea.Msg {
		return requestLocalMessageHandleMessage{
			tabID:     t.id,
			accountID: t.AccountID(),
			message: &twitchirc.Notice{
				FakeTimestamp: time.Now(),
				Message:       message,
			},
		}
	}
}

func (t *broadcastTab) Init() tea.Cmd {
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

func (t *broadcastTab) InitWithUserData(userData twitchapi.UserData) tea.Cmd {
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

func (t *broadcastTab) refreshEmotes(login, channelID string, manually bool) tea.Cmd {
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

func (t *broadcastTab) Focused() bool {
	return t.focused
}

func (t *broadcastTab) AccountID() string {
	return t.account.ID
}

func (t *broadcastTab) Channel() string {
	return t.channelLogin
}

// Title is the header label: the channel as the user entered it.
func (t *broadcastTab) Title() string {
	return t.channel
}

// Identity is the account label shown next to the title.
func (t *broadcastTab) Identity() string {
	if t.account.IsAnonymous {
		return "Anonymous"
	}

	return t.account.DisplayName
}

func (t *broadcastTab) ChannelID() string {
	return t.channelID
}

func (t *broadcastTab) State() broadcastTabState {
	return t.state
}

func (t *broadcastTab) IsSearching() bool {
	// Root asks this while deciding what a key press means, which also happens for a
	// tab that is still loading its channel data and therefore has no chat window.
	if t.chatWindow != nil && t.chatWindow.state == searchChatWindowState {
		return true
	}

	return t.userInspect != nil && t.userInspect.chatWindow.state == searchChatWindowState
}

func (t *broadcastTab) IsDataLoaded() bool {
	return t.channelDataLoaded
}

func (t *broadcastTab) ID() string {
	return t.id
}

func (t *broadcastTab) Kind() TabKind {
	return BroadcastTabKind
}

func (t *broadcastTab) SetSize(width, height int) {
	t.width = width
	t.height = height
}

func (t *broadcastTab) SetFullWidth(width int) {
	t.fullWidth = width
}

func (t *broadcastTab) Focus() {
	t.focused = true

	if !t.channelDataLoaded {
		return
	}

	// The mode decides which child owns the keyboard.
	switch {
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

func (t *broadcastTab) Blur() {
	t.focused = false
	t.lastClickEntry = nil
	t.lastClickAt = time.Time{}

	// Everything below talks to children, which only exist once the channel data
	// loaded. A tab that is still loading (for example a restored tab whose data is
	// fetched after it became active) has none of them yet.
	if t.channelDataLoaded {
		// Leaving the tab disarms the reply, so it cannot be sent from another tab.
		t.clearReplyTarget()

		t.chatWindow.Blur()
		t.input.Blur()

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
