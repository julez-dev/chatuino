package mainui

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/multiplex"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/save/messagelog"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/twitch/command"
	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/rs/zerolog"
)

type UserConfiguration struct {
	Settings save.Settings
	Theme    save.Theme
}

type AccountProvider interface {
	GetAllAccounts() ([]save.Account, error)
	UpdateTokensFor(id, accessToken, refreshToken string) error
	GetAccountBy(id string) (save.Account, error)
}

type EmoteStore interface {
	GetByText(channel, text string) (emote.Emote, bool)
	RefreshLocal(ctx context.Context, channelID string) error
	RefreshGlobal(ctx context.Context) error
	GetAllForUser(id string) emote.EmoteSet
}

type EmoteReplacer interface {
	Replace(content string) (string, string, error)
}

type APIClient interface {
	GetUsers(ctx context.Context, logins []string, ids []string) (twitch.UserResponse, error)
	GetStreamInfo(ctx context.Context, broadcastID []string) (twitch.GetStreamsResponse, error)
	GetChatSettings(ctx context.Context, broadcasterID string, moderatorID string) (twitch.GetChatSettingsResponse, error)
}

type APIClientWithRefresh interface {
	APIClient
	RefreshToken(ctx context.Context, refreshToken string) (string, string, error)
}

type ChatPool interface {
	ListenAndServe(inbound <-chan multiplex.InboundMessage) <-chan multiplex.OutboundMessage
}

type EventSubPool interface {
	ListenAndServe(inbound <-chan multiplex.EventSubInboundMessage) error
}

type RecentMessageService interface {
	GetRecentMessagesFor(ctx context.Context, channelLogin string) ([]twitch.IRCer, error)
}

type MessageLogger interface {
	MessagesFromUserInChannel(username string, broadcasterChannel string) ([]messagelog.LogEntry, error)
}

type tabKind int

const (
	broadcastTabKind tabKind = iota
	mentionTabKind
	liveNotificationTabKind
)

func (t tabKind) String() string {
	switch t {
	case broadcastTabKind:
		return "Channel (Default)"
	case mentionTabKind:
		return "Mention"
	case liveNotificationTabKind:
		return "Live Notifications"
	}

	return "<not implemented>"
}

type tab interface {
	Init() tea.Cmd
	InitWithUserData(twitch.UserData) tea.Cmd
	Update(tea.Msg) (tab, tea.Cmd)
	View() string
	Focus()
	Blur()
	AccountID() string
	Channel() string
	State() broadcastTabState
	IsDataLoaded() bool
	ID() string
	Focused() bool
	ChannelID() string
	HandleResize()
	SetSize(width, height int)
	Kind() tabKind
}

type activeScreen int

const (
	mainScreen activeScreen = iota
	inputScreen
	helpScreen
)

type ircConnectionError struct {
	err error
}

func (e ircConnectionError) Error() string {
	return fmt.Sprintf("Disconnected from chat server. (%s)", e.err.Error())
}

func (e ircConnectionError) Unwrap() error {
	return e.err
}

func (e ircConnectionError) IRC() string {
	return e.Error()
}

type persistedDataLoadedMessage struct {
	err      error
	ttvUsers map[string]twitch.UserData
	state    save.AppState
	accounts []save.Account
	clients  map[string]APIClient
}

type chatEventMessage struct {
	// If the event was not created by twitch IRC connection but instead locally by message input chat load etc.
	// This indicates that the root will not start a new wait message command.
	// All messages requested by requestLocalMessageHandleMessage will have this flag set to true.
	isFakeEvent bool
	accountID   string
	channel     string

	message twitch.IRCer
	// the original twitch.IRC message with it's content overwritten by emote unicodes or colors
	messageContentEmoteOverride string

	// if message should only be sent to a specific tab ID
	// if empty send to all
	tabID string
}

type requestLocalMessageHandleMessage struct {
	message   twitch.IRCer
	accountID string
	tabID     string
}

type requestLocalMessageHandleMessageBatch struct {
	messages  []twitch.IRCer
	accountID string
	tabID     string
}

type forwardChatMessage struct {
	msg multiplex.InboundMessage
}

type forwardEventSubMessage struct {
	accountID string
	msg       eventsub.InboundMessage
}

type EventSubMessage struct {
	Payload eventsub.Message[eventsub.NotificationPayload]
}

type polledStreamInfo struct {
	streamInfos []setStreamInfo
}

type Root struct {
	logger   zerolog.Logger
	clientID string

	width, height int
	keymap        save.KeyMap
	userConfig    UserConfiguration

	hasLoadedSession bool
	screenType       activeScreen

	// dependencies
	accounts             AccountProvider
	emoteStore           EmoteStore
	emoteReplacer        EmoteReplacer
	serverAPI            APIClientWithRefresh
	recentMessageService RecentMessageService
	messageLogger        MessageLogger
	buildTTVClient       func(clientID string, opts ...twitch.APIOptionFunc) (APIClient, error)
	loadSaveState        func() (save.AppState, error)

	// One API Client per Chatuino User Tab
	ttvAPIUserClients map[string]APIClient

	// chat multiplexer channels
	closerWG *sync.WaitGroup
	in       chan multiplex.InboundMessage
	out      <-chan multiplex.OutboundMessage

	// event sub
	initErr            error
	eventSub           EventSubPool
	eventSubInInFlight *sync.WaitGroup
	eventSubIn         chan multiplex.EventSubInboundMessage

	// message logge
	messageLoggerChan chan<- *command.PrivateMessage

	// components
	splash    splash
	header    *tabHeader
	joinInput *join
	help      *help

	tabCursor int
	tabs      []tab
}

func NewUI(
	logger zerolog.Logger,
	provider AccountProvider,
	chatPool ChatPool,
	emoteStore EmoteStore,
	clientID string,
	serverClient APIClientWithRefresh,
	keymap save.KeyMap,
	recentMessageService RecentMessageService,
	eventSub EventSubPool,
	messageLoggerChan chan<- *command.PrivateMessage,
	emoteReplacer EmoteReplacer,
	messageLogger MessageLogger,
	userConfig UserConfiguration,
) *Root {
	inChat := make(chan multiplex.InboundMessage)
	outChat := chatPool.ListenAndServe(inChat)
	inEventSub := make(chan multiplex.EventSubInboundMessage)

	clients := map[string]APIClient{}
	return &Root{
		clientID: clientID,
		logger:   logger,
		width:    10,
		height:   10,
		keymap:   keymap,

		// components
		splash: splash{
			keymap:            keymap,
			userConfiguration: userConfig,
		},
		header:    newTabHeader(userConfig),
		help:      newHelp(10, 10, keymap),
		joinInput: newJoin(provider, clients, 10, 10, keymap, userConfig),

		// chat multiplex channels
		closerWG: &sync.WaitGroup{},
		in:       inChat,
		out:      outChat,

		// event sub
		eventSubInInFlight: &sync.WaitGroup{},
		eventSub:           eventSub,
		eventSubIn:         inEventSub,

		messageLogger:        messageLogger,
		emoteReplacer:        emoteReplacer,
		messageLoggerChan:    messageLoggerChan,
		accounts:             provider,
		ttvAPIUserClients:    clients,
		emoteStore:           emoteStore,
		serverAPI:            serverClient,
		recentMessageService: recentMessageService,
		buildTTVClient: func(clientID string, opts ...twitch.APIOptionFunc) (APIClient, error) {
			return twitch.NewAPI(clientID, opts...)
		},
		loadSaveState: save.AppStateFromDisk,
		userConfig:    userConfig,
	}
}

func (r *Root) Init() tea.Cmd {
	return tea.Batch(
		tea.SetWindowTitle("Chatuino"),
		func() tea.Msg {
			r.closerWG.Add(1)
			go func() {
				defer r.closerWG.Done()
				if err := r.eventSub.ListenAndServe(r.eventSubIn); err != nil {
					r.logger.Err(err).Msg("failed to connect to eventsub")
					return
				}

				r.logger.Info().Msg("init event sub routine done")
			}()

			accounts, err := r.accounts.GetAllAccounts()
			if err != nil {
				return persistedDataLoadedMessage{
					err: fmt.Errorf("failed to load accounts: %w", err),
				}
			}

			// pre populate the API clients for each account
			clients := make(map[string]APIClient, len(accounts))
			for _, acc := range accounts {
				var api APIClient

				if !acc.IsAnonymous {
					api, _ = r.buildTTVClient(r.clientID, twitch.WithUserAuthentication(r.accounts, r.serverAPI, acc.ID))
				} else {
					api = r.serverAPI
				}

				clients[acc.ID] = api
			}

			state, err := r.loadSaveState()
			if err != nil {
				return persistedDataLoadedMessage{
					accounts: accounts,
					clients:  clients,
					err:      fmt.Errorf("failed to load save state: %w", err),
				}
			}

			// pre fetch all of tabs twitch users in one single call, this saves a lot of calls if the app was previously closed with a lot of tabs
			ttvUsers := make(map[string]twitch.UserData, len(state.Tabs))
			loginsUnique := make(map[string]struct{}, len(state.Tabs))
			logins := make([]string, 0, len(state.Tabs))

			for _, tab := range state.Tabs {
				if tab.Kind != int(broadcastTabKind) {
					continue
				}
				loginsUnique[tab.Channel] = struct{}{}
			}

			logins = slices.AppendSeq(logins, maps.Keys(loginsUnique))

			if len(logins) > 0 {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
				defer cancel()

				resp, err := r.serverAPI.GetUsers(ctx, logins, nil)
				if err != nil {
					r.logger.Err(err).Msg("failed to connect to load users")
					return persistedDataLoadedMessage{
						accounts: accounts,
						clients:  clients,
						err:      fmt.Errorf("failed to fetch users: %w", err),
					}
				}

				for _, data := range resp.Data {
					ttvUsers[data.Login] = data
				}
			}

			return persistedDataLoadedMessage{
				state:    state,
				accounts: accounts,
				clients:  clients,
				ttvUsers: ttvUsers,
			}
		},
		r.waitChatEvents(),
		r.tickPollStreamInfos(),
	)
}

func (r *Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case persistedDataLoadedMessage:
		return r, r.handlePersistedDataLoaded(msg)
	case joinChannelMessage:
		r.screenType = mainScreen
		r.initErr = nil

		nTab := r.createTab(msg.account, msg.channel, msg.tabKind)
		nTab.Focus()

		r.tabs = append(r.tabs, nTab)

		r.tabCursor = len(r.tabs) - 1 // set index to the newest tab
		r.header.selectTab(nTab.ID())

		r.joinInput.blur()
		r.joinInput.input.SetSuggestions(nil) // free up some memory

		r.handleResize()

		return r, nTab.Init()
	case forwardEventSubMessage:
		r.eventSubInInFlight.Add(1)
		cmd := func() tea.Msg {
			defer r.eventSubInInFlight.Done()
			r.eventSubIn <- multiplex.EventSubInboundMessage{
				AccountID: msg.accountID,
				Msg:       msg.msg,
			}
			return nil
		}
		return r, cmd
	case chatEventMessage:
		for i := range r.tabs {
			if msg.tabID != "" && msg.tabID != r.tabs[i].ID() {
				continue
			}

			r.tabs[i], cmd = r.tabs[i].Update(msg)
			cmds = append(cmds, cmd)
		}

		// only start new wait command when event was actually from the websocket/twitch connection
		if !msg.isFakeEvent {
			cmds = append(cmds, r.waitChatEvents())
		}
		return r, tea.Batch(cmds...)
	case requestLocalMessageHandleMessage:
		return r, func() tea.Msg {
			return r.buildChatEventMessage(msg.accountID, msg.tabID, msg.message, true)
		}
	case requestLocalMessageHandleMessageBatch:
		batched := make([]tea.Cmd, 0, len(msg.messages))

		for ircer := range slices.Values(msg.messages) {
			batched = append(batched, func() tea.Msg {
				return r.buildChatEventMessage(msg.accountID, msg.tabID, ircer, true)
			})
		}

		cmds = append(cmds, tea.Sequence(batched...))
		return r, tea.Batch(cmds...)
	case forwardChatMessage:
		r.eventSubInInFlight.Add(1)
		cmd := func() tea.Msg {
			defer r.eventSubInInFlight.Done()
			r.in <- msg.msg
			return nil
		}

		return r, cmd
	case polledStreamInfo:
		return r, r.handlePolledStreamInfo(msg)
	case tea.WindowSizeMsg:
		r.width = msg.Width
		r.height = msg.Height
		r.handleResize()
	case tea.KeyMsg:
		if key.Matches(msg, r.keymap.Quit) {
			return r, tea.Quit
		}

		if !r.hasLoadedSession {
			return r, tea.Batch(cmds...)
		}

		if key.Matches(msg, r.keymap.Help) {
			var isInsertMode bool
			if len(r.tabs) > r.tabCursor {
				isInsertMode = (r.tabs[r.tabCursor].State() == insertMode || r.tabs[r.tabCursor].State() == userInspectInsertMode)
			}

			if !isInsertMode && r.screenType == inputScreen && r.joinInput.input.InputModel.Focused() {
				isInsertMode = true
			}

			if !isInsertMode {
				r.screenType = helpScreen
				r.joinInput.blur()
				if len(r.tabs) > r.tabCursor {
					r.tabs[r.tabCursor].Blur()
				}
				return r, nil
			}
		}

		if key.Matches(msg, r.keymap.Escape) {
			if (r.screenType == inputScreen && r.joinInput.state == joinViewMode) || r.screenType == helpScreen {
				if len(r.tabs) > r.tabCursor {
					r.tabs[r.tabCursor].Focus()
				}

				r.joinInput.blur()
				r.screenType = mainScreen

				return r, nil
			}
		}

		if key.Matches(msg, r.keymap.DumpScreen) {
			f, err := os.Create(fmt.Sprintf("%s_dump.txt", time.Now().Format("2006-01-02_15_04_05")))
			if err != nil {
				return r, nil
			}

			defer func() {
				_ = f.Close()
			}()

			_, _ = io.Copy(f, strings.NewReader(stripAnsi(r.View())))

			return r, nil
		}

		if key.Matches(msg, r.keymap.Create) {
			switch r.screenType {
			case mainScreen:
				if len(r.tabs) > r.tabCursor {
					r.tabs[r.tabCursor].Blur()
				}

				r.screenType = inputScreen
				r.joinInput = newJoin(r.accounts, r.ttvAPIUserClients, r.width, r.height, r.keymap, r.userConfig)
				hasMentionTab := slices.ContainsFunc(r.tabs, func(t tab) bool {
					return t.Kind() == mentionTabKind
				})

				hasNotificationTab := slices.ContainsFunc(r.tabs, func(t tab) bool {
					return t.Kind() == liveNotificationTabKind
				})

				var validTabKinds []tabKind
				validTabKinds = append(validTabKinds, broadcastTabKind)

				if !hasMentionTab {
					validTabKinds = append(validTabKinds, mentionTabKind)
				}

				if !hasNotificationTab {
					validTabKinds = append(validTabKinds, liveNotificationTabKind)
				}

				r.joinInput.setTabOptions(validTabKinds...)
				r.joinInput.focus()
				return r, r.joinInput.Init()
			case inputScreen:
				if len(r.tabs) > r.tabCursor {
					r.tabs[r.tabCursor].Focus()
				}

				r.joinInput.blur()
				r.screenType = mainScreen
			}

			return r, nil
		}

		if r.screenType == mainScreen {

			if key.Matches(msg, r.keymap.Next) {
				if len(r.tabs) > r.tabCursor && (r.tabs[r.tabCursor].State() == insertMode || r.tabs[r.tabCursor].State() == userInspectInsertMode) {
					r.tabs[r.tabCursor], cmd = r.tabs[r.tabCursor].Update(msg)
					return r, cmd
				}

				r.nextTab()
			}

			if key.Matches(msg, r.keymap.Previous) {
				if len(r.tabs) > r.tabCursor && (r.tabs[r.tabCursor].State() == insertMode || r.tabs[r.tabCursor].State() == userInspectInsertMode) {
					r.tabs[r.tabCursor], cmd = r.tabs[r.tabCursor].Update(msg)
					return r, cmd
				}

				r.prevTab()
			}

			if key.Matches(msg, r.keymap.CloseTab) {
				if len(r.tabs) > r.tabCursor && !(r.tabs[r.tabCursor].State() == insertMode || r.tabs[r.tabCursor].State() == userInspectInsertMode) {
					currentTab := r.tabs[r.tabCursor]
					r.closeTab()

					// if tab was connected to IRC, disconnect it
					if currentTab.IsDataLoaded() && currentTab.Kind() == broadcastTabKind {
						cmds := make([]tea.Cmd, 0, 2)

						// if there is another tab for the same channel and the same account
						hasOther := slices.ContainsFunc(r.tabs, func(t tab) bool {
							return t.ID() != currentTab.ID() &&
								t.AccountID() == currentTab.AccountID() &&
								t.Channel() == currentTab.Channel()
						})

						if !hasOther {
							// send part message
							r.logger.Info().Str("channel", currentTab.Channel()).Str("id", currentTab.AccountID()).Msg("sending part message")
							cmds = append(cmds, func() tea.Msg {
								r.in <- multiplex.InboundMessage{
									AccountID: currentTab.AccountID(),
									Msg: command.PartMessage{
										Channel: currentTab.Channel(),
									},
								}
								return nil
							})
						}

						r.closerWG.Add(1)
						cmds = append(cmds, func() tea.Msg {
							defer r.closerWG.Done()
							r.in <- multiplex.InboundMessage{
								AccountID: currentTab.AccountID(),
								Msg:       multiplex.DecrementTabCounter{},
							}
							return nil
						})

						return r, tea.Sequence(cmds...)
					}

					return r, nil
				}
			}
		}
	}

	r.header, cmd = r.header.Update(msg)
	cmds = append(cmds, cmd)

	for i, tab := range r.tabs {
		r.tabs[i], cmd = tab.Update(msg)
		cmds = append(cmds, cmd)
	}

	if r.screenType == inputScreen {
		r.joinInput, cmd = r.joinInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	if r.screenType == helpScreen {
		r.help, cmd = r.help.Update(msg)
		cmds = append(cmds, cmd)
	}

	return r, tea.Batch(cmds...)
}

func (r *Root) View() string {
	if !r.hasLoadedSession {
		return r.splash.ViewLoading()
	}

	switch r.screenType {
	case mainScreen:
		if len(r.tabs) == 0 {
			if r.initErr != nil {
				return r.splash.ViewError(r.initErr)
			}

			return r.splash.View()
		}

		return r.header.View() + "\n" + r.tabs[r.tabCursor].View()
	case inputScreen:
		return r.joinInput.View()
	case helpScreen:
		return r.help.View()
	}

	return ""
}

func (r *Root) TakeStateSnapshot() save.AppState {
	appState := save.AppState{}

	for _, t := range r.tabs {
		tabState := save.TabState{
			IsFocused:  t.Focused(),
			Channel:    t.Channel(),
			IdentityID: t.AccountID(),
			Kind:       int(t.Kind()),
		}

		if t.Kind() == broadcastTabKind {
			tabState.IsLocalUnique = t.(*broadcastTab).isUniqueOnlyChat
			tabState.IsLocalSub = t.(*broadcastTab).isLocalSub
		}

		appState.Tabs = append(appState.Tabs, tabState)
	}

	return appState
}

func (r *Root) Close() error {
	if r.eventSubIn != nil {
		r.eventSubInInFlight.Wait()
		close(r.eventSubIn)
	}

	r.closerWG.Wait() // wait for all inbound messages to be processed

	close(r.in)

	return nil
}

func (r *Root) tickPollStreamInfos() tea.Cmd {
	clients := maps.Clone(r.ttvAPIUserClients)

	// collect all open broadcasters
	openBroadcasts := map[string]struct{}{}
	channelIDNames := map[string]string{}

	for _, tab := range r.tabs {
		if tab.Kind() != broadcastTabKind {
			continue
		}
		openBroadcasts[tab.ChannelID()] = struct{}{}
		channelIDNames[tab.ChannelID()] = tab.Channel()
	}

	if len(openBroadcasts) == 0 {
		return func() tea.Msg {
			time.Sleep(time.Second * 90)
			return polledStreamInfo{}
		}
	}

	broadcastIDs := []string{}
	for broadcast := range openBroadcasts {
		broadcastIDs = append(broadcastIDs, broadcast)
	}

	return func() tea.Msg {
		accounts, err := r.accounts.GetAllAccounts()
		if err != nil {
			return polledStreamInfo{}
		}

		var mainAccountID string

		for _, account := range accounts {
			if account.IsMain {
				mainAccountID = account.ID
			}
		}

		var fetcher APIClient
		if _, has := clients[mainAccountID]; has {
			fetcher = clients[mainAccountID]
		} else {
			fetcher = r.serverAPI
		}

		timer := time.NewTimer(time.Second * 90)

		defer func() {
			timer.Stop()
		}()

		<-timer.C

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		resp, err := fetcher.GetStreamInfo(ctx, broadcastIDs)
		if err != nil {
			r.logger.Err(err).Msg("failed polling streamer info")
			return polledStreamInfo{}
		}

		polled := polledStreamInfo{
			streamInfos: make([]setStreamInfo, 0, len(broadcastIDs)),
		}

		// Update status for all streams
		// If a stream is offline the twitch API does not return any item for this broadcaster_id
		// To still update the title etc. to empty value, send an empty info to component.
		for ib := range broadcastIDs {
			info := setStreamInfo{
				target:   broadcastIDs[ib],
				username: channelIDNames[broadcastIDs[ib]], // fall back channel name, so it can still be displayed when offline
			}

			for id := range resp.Data {
				// data's user does not match broadcaster
				if resp.Data[id].UserID != broadcastIDs[ib] {
					continue
				}

				info.viewer = resp.Data[id].ViewerCount
				info.username = resp.Data[id].UserName
				info.title = resp.Data[id].Title
				info.game = resp.Data[id].GameName
				info.isLive = !resp.Data[id].StartedAt.IsZero()
			}

			polled.streamInfos = append(polled.streamInfos, info)
		}

		return polled
	}
}

func (r *Root) handlePolledStreamInfo(polled polledStreamInfo) tea.Cmd {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	for _, info := range polled.streamInfos {
		for it, tab := range r.tabs {
			if tab.IsDataLoaded() {
				r.tabs[it], cmd = tab.Update(info)
				cmds = append(cmds, cmd)
			}
		}
	}

	cmds = append(cmds, r.tickPollStreamInfos())
	return tea.Batch(cmds...)
}

func (r *Root) createTab(account save.Account, channel string, kind tabKind) tab {
	switch kind {
	case broadcastTabKind:
		identity := account.DisplayName

		if account.IsAnonymous {
			identity = "Anonymous"
		}

		id := r.header.addTab(channel, identity)

		headerHeight := r.getHeaderHeight()

		nTab := newBroadcastTab(id, r.logger, r.ttvAPIUserClients[account.ID], channel, r.width, r.height-headerHeight, r.emoteStore, account, r.accounts, r.recentMessageService, r.keymap, r.emoteReplacer, r.messageLogger, r.userConfig)
		return nTab
	case mentionTabKind:
		id := r.header.addTab("mentioned", "all")
		headerHeight := r.getHeaderHeight()
		nTab := newMentionTab(id, r.logger, r.keymap, r.accounts, r.emoteStore, r.width, r.height-headerHeight, r.userConfig)
		return nTab
	case liveNotificationTabKind:
		id := r.header.addTab("live notifications", "all")
		headerHeight := r.getHeaderHeight()
		nTab := newLiveNotificationTab(id, r.logger, r.keymap, r.emoteStore, r.width, r.height-headerHeight, r.userConfig)
		return nTab
	}

	return nil
}

func (r *Root) getHeaderHeight() int {
	headerView := r.header.View()
	return lipgloss.Height(headerView)
}

func (r *Root) handleResize() {
	// splash screen
	r.splash.width = r.width
	r.splash.height = r.height

	// tab header
	r.header.width = r.width

	// channel join input
	r.joinInput.width = r.width
	r.joinInput.height = r.height
	r.joinInput.accountList.SetHeight(r.height / 2)

	// help
	r.help.handleResize(r.width, r.height)

	// tab
	headerHeight := r.getHeaderHeight()

	for i := range r.tabs {
		r.tabs[i].SetSize(r.width, r.height-headerHeight)
		r.tabs[i].HandleResize()
	}
}

func (r *Root) nextTab() {
	if len(r.tabs) > r.tabCursor {
		r.tabs[r.tabCursor].Blur()
	}

	newIndex := r.tabCursor + 1

	if newIndex >= len(r.tabs) {
		newIndex = 0
	}

	r.tabCursor = newIndex

	if len(r.tabs) > r.tabCursor {
		r.header.selectTab(r.tabs[r.tabCursor].ID())
		r.tabs[r.tabCursor].Focus()
	}
}

func (r *Root) prevTab() {
	if len(r.tabs) > r.tabCursor {
		r.tabs[r.tabCursor].Blur()
	}

	newIndex := r.tabCursor - 1

	if newIndex < 0 {
		newIndex = len(r.tabs) - 1

		if newIndex < 0 {
			newIndex = 0
		}
	}

	r.tabCursor = newIndex

	if len(r.tabs) > r.tabCursor {
		r.header.selectTab(r.tabs[r.tabCursor].ID())
		r.tabs[r.tabCursor].Focus()
	}
}

func (r *Root) handlePersistedDataLoaded(msg persistedDataLoadedMessage) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(msg.state.Tabs))
	r.ttvAPIUserClients = msg.clients
	r.hasLoadedSession = true

	if msg.err != nil {
		r.initErr = msg.err
		return nil
	}

	// restore tabs
	var hasActiveTab bool
	for _, t := range msg.state.Tabs {
		r.screenType = mainScreen

		var newTab tab
		switch tabKind(t.Kind) {
		case broadcastTabKind:
			var account save.Account

			for _, a := range msg.accounts {
				if a.ID == t.IdentityID {
					account = a
				}
			}

			if account.ID == "" {
				continue
			}

			newTab = r.createTab(account, t.Channel, broadcastTabKind)
			newTab.(*broadcastTab).isUniqueOnlyChat = t.IsLocalUnique
			newTab.(*broadcastTab).isLocalSub = t.IsLocalSub
		case mentionTabKind:
			// don't load mention tab, when there are no longer any non-anonymous accounts
			hasNormalAccount := slices.ContainsFunc(msg.accounts, func(e save.Account) bool {
				return !e.IsAnonymous
			})

			if !hasNormalAccount {
				continue
			}

			newTab = r.createTab(save.Account{}, "", mentionTabKind)
		case liveNotificationTabKind:
			newTab = r.createTab(save.Account{}, "", liveNotificationTabKind)
		}

		if t.IsFocused {
			newTab.Focus()
		}

		r.tabs = append(r.tabs, newTab)

		if t.IsFocused {
			hasActiveTab = true
			r.tabCursor = len(r.tabs) - 1 // set index to the newest tab
			r.header.selectTab(newTab.ID())
		}
		cmds = append(cmds, newTab.InitWithUserData(msg.ttvUsers[t.Channel]))
	}

	// if for some reason there were tabs persisted but non tab has the focus flag set
	// focus the first tab
	if len(r.tabs) > 0 && !hasActiveTab {
		r.header.selectTab(r.tabs[0].ID())
		r.tabCursor = 0
		r.tabs[0].Focus()
	}

	r.handleResize()
	return tea.Batch(cmds...)
}

func (r *Root) buildChatEventMessage(accountID string, tabID string, ircer twitch.IRCer, isFakeEvent bool) chatEventMessage {
	var (
		channel          string
		contentOverwrite string
		prepare          string
	)

	switch ircMessage := ircer.(type) {
	case *command.PrivateMessage:
		channel = ircMessage.ChannelUserName
		prepare, contentOverwrite, _ = r.emoteReplacer.Replace(ircMessage.Message)
		io.WriteString(os.Stdout, prepare)
	case *command.RoomState:
		channel = ircMessage.ChannelUserName
	case *command.UserNotice:
		channel = ircMessage.ChannelUserName
	case *command.UserState:
		channel = ircMessage.ChannelUserName
	case *command.ClearChat:
		channel = ircMessage.ChannelUserName
	case *command.ClearMessage:
		channel = ircMessage.ChannelUserName
	case *command.SubMessage:
		channel = ircMessage.ChannelUserName
		prepare, contentOverwrite, _ = r.emoteReplacer.Replace(ircMessage.Message)
		io.WriteString(os.Stdout, prepare)
	case *command.RaidMessage:
		channel = ircMessage.ChannelUserName
	case *command.SubGiftMessage:
		channel = ircMessage.ChannelUserName
	case *command.RitualMessage:
		channel = ircMessage.ChannelUserName
		prepare, contentOverwrite, _ = r.emoteReplacer.Replace(ircMessage.Message)
		io.WriteString(os.Stdout, prepare)
	case *command.AnnouncementMessage:
		channel = ircMessage.ChannelUserName
		prepare, contentOverwrite, _ = r.emoteReplacer.Replace(ircMessage.Message)
		io.WriteString(os.Stdout, prepare)
	}

	return chatEventMessage{
		isFakeEvent:                 isFakeEvent,
		accountID:                   accountID,
		channel:                     channel,
		tabID:                       tabID,
		message:                     ircer,
		messageContentEmoteOverride: contentOverwrite,
	}
}

func (r *Root) waitChatEvents() tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-r.out

		if !ok {
			return nil
		}

		if msg.Err != nil {
			return chatEventMessage{
				accountID: msg.ID,
				channel:   "",
				message:   ircConnectionError{err: msg.Err},
			}
		}

		if privateMsg, ok := msg.Msg.(*command.PrivateMessage); ok {
			r.messageLoggerChan <- privateMsg.Clone()
		}

		return r.buildChatEventMessage(msg.ID, "", msg.Msg, false)
	}
}

func (r *Root) closeTab() {
	if len(r.tabs) > r.tabCursor {
		tabID := r.tabs[r.tabCursor].ID()
		if r.tabs[r.tabCursor].Kind() == broadcastTabKind {
			r.tabs[r.tabCursor].(*broadcastTab).close()
		}
		r.header.removeTab(tabID)
		r.tabs = slices.DeleteFunc(r.tabs, func(t tab) bool {
			return t.ID() == tabID
		})
		r.prevTab()
		r.handleResize()
	}
}
