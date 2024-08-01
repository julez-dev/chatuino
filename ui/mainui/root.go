package mainui

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/twitch/command"
	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/rs/zerolog"
)

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
	state    save.AppState
	accounts []save.Account
}

type chatEventMessage struct {
	accountID string
	channel   string
	message   twitch.IRCer
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

type Root struct {
	logger   zerolog.Logger
	clientID string

	width, height int
	keymap        save.KeyMap

	screenType activeScreen

	// dependencies
	accounts             AccountProvider
	emoteStore           EmoteStore
	serverAPI            APIClientWithRefresh
	recentMessageService RecentMessageService
	buildTTVClient       func(clientID string, opts ...twitch.APIOptionFunc) (APIClient, error)
	loadSaveState        func() (save.AppState, error)

	// One API Client per Chatuino User Tab
	ttvAPIUserClients map[string]APIClient

	// chat multiplexer channels
	closerWG *sync.WaitGroup
	in       chan multiplex.InboundMessage
	out      <-chan multiplex.OutboundMessage

	// event sub
	eventSub           EventSubPool
	eventSubInInFlight *sync.WaitGroup
	eventSubIn         chan multiplex.EventSubInboundMessage

	// components
	splash    splash
	header    *tabHeader
	joinInput *join
	help      *help

	tabCursor int
	tabs      []*tab
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
			keymap: keymap,
		},
		header:    newTabHeader(),
		help:      newHelp(10, 10, keymap),
		joinInput: newJoin(provider, clients, 10, 10, keymap),

		// chat multiplex channels
		closerWG: &sync.WaitGroup{},
		in:       inChat,
		out:      outChat,

		// event sub
		eventSubInInFlight: &sync.WaitGroup{},
		eventSub:           eventSub,
		eventSubIn:         inEventSub,

		accounts:             provider,
		ttvAPIUserClients:    clients,
		emoteStore:           emoteStore,
		serverAPI:            serverClient,
		recentMessageService: recentMessageService,
		buildTTVClient: func(clientID string, opts ...twitch.APIOptionFunc) (APIClient, error) {
			return twitch.NewAPI(clientID, opts...)
		},
		loadSaveState: save.AppStateFromDisk,
	}
}

func (r *Root) Init() tea.Cmd {
	return tea.Batch(
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

			state, err := r.loadSaveState()
			if err != nil {
				return nil
			}

			accounts, err := r.accounts.GetAllAccounts()
			if err != nil {
				return nil
			}

			return persistedDataLoadedMessage{state: state, accounts: accounts}
		},
		r.waitChatEvents(),
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

		nTab := r.createTab(msg.account, msg.channel)
		nTab.focus()

		r.tabs = append(r.tabs, nTab)

		r.tabCursor = len(r.tabs) - 1 // set index to the newest tab
		r.header.selectTab(nTab.id)

		r.joinInput.blur()

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
		for i, t := range r.tabs {
			if t.account.ID == msg.accountID && (t.channel == msg.channel || msg.channel == "") {
				r.tabs[i], cmd = r.tabs[i].Update(msg)
				cmds = append(cmds, cmd)
			}
		}

		cmds = append(cmds, r.waitChatEvents())
		return r, tea.Batch(cmds...)
	case forwardChatMessage:
		r.eventSubInInFlight.Add(1)
		cmd := func() tea.Msg {
			defer r.eventSubInInFlight.Done()
			r.in <- msg.msg
			return nil
		}

		return r, cmd
	case tea.WindowSizeMsg:
		r.width = msg.Width
		r.height = msg.Height
		r.handleResize()
	case tea.KeyMsg:
		if key.Matches(msg, r.keymap.Quit) {
			return r, tea.Quit
		}

		if key.Matches(msg, r.keymap.Help) {
			var isInsertMode bool
			if len(r.tabs) > r.tabCursor {
				isInsertMode = (r.tabs[r.tabCursor].state == insertMode || r.tabs[r.tabCursor].state == userInspectInsertMode)
			}

			if !isInsertMode {
				r.screenType = helpScreen
				r.joinInput.blur()
				if len(r.tabs) > r.tabCursor {
					r.tabs[r.tabCursor].blur()
				}
				return r, nil
			}
		}

		if key.Matches(msg, r.keymap.Escape) {
			if r.screenType == inputScreen || r.screenType == helpScreen {
				if len(r.tabs) > r.tabCursor {
					r.tabs[r.tabCursor].focus()
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
					r.tabs[r.tabCursor].blur()
				}

				r.screenType = inputScreen
				r.joinInput = newJoin(r.accounts, r.ttvAPIUserClients, r.width, r.height, r.keymap)
				r.joinInput.focus()
				return r, r.joinInput.Init()
			case inputScreen:
				if len(r.tabs) > r.tabCursor {
					r.tabs[r.tabCursor].focus()
				}

				r.joinInput.blur()
				r.screenType = mainScreen
			}

			return r, nil
		}

		if r.screenType == mainScreen {

			if key.Matches(msg, r.keymap.Next) {
				if len(r.tabs) > r.tabCursor && (r.tabs[r.tabCursor].state == insertMode || r.tabs[r.tabCursor].state == userInspectInsertMode) {
					r.tabs[r.tabCursor], cmd = r.tabs[r.tabCursor].Update(msg)
					return r, cmd
				}

				r.nextTab()
			}

			if key.Matches(msg, r.keymap.Previous) {
				if len(r.tabs) > r.tabCursor && (r.tabs[r.tabCursor].state == insertMode || r.tabs[r.tabCursor].state == userInspectInsertMode) {
					r.tabs[r.tabCursor], cmd = r.tabs[r.tabCursor].Update(msg)
					return r, cmd
				}

				r.prevTab()
			}

			if key.Matches(msg, r.keymap.CloseTab) {
				if len(r.tabs) > r.tabCursor && !(r.tabs[r.tabCursor].state == insertMode || r.tabs[r.tabCursor].state == userInspectInsertMode) {
					currentTab := r.tabs[r.tabCursor]
					r.closeTab()

					// if tab was connected to IRC, disconnect it
					if currentTab.channelDataLoaded {
						cmds := make([]tea.Cmd, 0, 2)

						// if there is another tab for the same channel and the same account
						hasOther := slices.ContainsFunc(r.tabs, func(t *tab) bool {
							return t.id != currentTab.id &&
								t.account.ID == currentTab.account.ID &&
								t.channel == currentTab.channel
						})

						if !hasOther {
							// send part message
							r.logger.Info().Str("channel", currentTab.channel).Str("id", currentTab.account.ID).Msg("sending part message")
							cmds = append(cmds, func() tea.Msg {
								r.in <- multiplex.InboundMessage{
									AccountID: currentTab.account.ID,
									Msg: command.PartMessage{
										Channel: currentTab.channel,
									},
								}
								return nil
							})
						}

						r.closerWG.Add(1)
						cmds = append(cmds, func() tea.Msg {
							defer r.closerWG.Done()
							r.in <- multiplex.InboundMessage{
								AccountID: currentTab.account.ID,
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
	switch r.screenType {
	case mainScreen:
		if len(r.tabs) == 0 {
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
		if t.chatWindow == nil {
			continue
		}

		tabState := save.TabState{
			IsFocused:  t.focused,
			Channel:    t.channel,
			IdentityID: t.account.ID,
		}

		appState.Tabs = append(appState.Tabs, tabState)
	}

	return appState
}

func (r *Root) Close() error {
	var errs []error
	for _, t := range r.tabs {
		errs = append(errs, t.Close())
	}

	if r.eventSubIn != nil {
		r.eventSubInInFlight.Wait()
		close(r.eventSubIn)
	}

	r.closerWG.Wait() // wait for all inbound messages to be processed

	close(r.in)

	return errors.Join(errs...)
}

func (r *Root) createTab(account save.Account, channel string) *tab {
	identity := account.DisplayName

	if account.IsAnonymous {
		identity = "Anonymous"
	}

	id := r.header.addTab(channel, identity)

	headerHeight := r.getHeaderHeight()

	nTab := newTab(id, r.logger, r.ttvAPIUserClients[account.ID], channel, r.width, r.height-headerHeight, r.emoteStore, account, r.accounts, r.recentMessageService, r.keymap)
	return nTab
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
	r.joinInput.list.SetHeight(r.height / 2)

	// help
	r.help.handleResize(r.width, r.height)

	// tab
	headerHeight := r.getHeaderHeight()

	for i := range r.tabs {
		r.tabs[i].height = r.height - headerHeight
		r.tabs[i].width = r.width
		r.tabs[i].handleResize()
	}
}

func (r *Root) nextTab() {
	if len(r.tabs) > r.tabCursor {
		r.tabs[r.tabCursor].blur()
	}

	newIndex := r.tabCursor + 1

	if newIndex >= len(r.tabs) {
		newIndex = 0
	}

	r.tabCursor = newIndex

	if len(r.tabs) > r.tabCursor {
		r.header.selectTab(r.tabs[r.tabCursor].id)
		r.tabs[r.tabCursor].focus()
	}
}

func (r *Root) prevTab() {
	if len(r.tabs) > r.tabCursor {
		r.tabs[r.tabCursor].blur()
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
		r.header.selectTab(r.tabs[r.tabCursor].id)
		r.tabs[r.tabCursor].focus()
	}
}

func (r *Root) handlePersistedDataLoaded(msg persistedDataLoadedMessage) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(msg.state.Tabs))

	// pre populate the API clients for each account
	for _, acc := range msg.accounts {
		var api APIClient

		if !acc.IsAnonymous {
			api, _ = r.buildTTVClient(r.clientID, twitch.WithUserAuthentication(r.accounts, r.serverAPI, acc.ID))
		} else {
			api = r.serverAPI
		}

		r.ttvAPIUserClients[acc.ID] = api
	}

	// restore tabs
	for _, t := range msg.state.Tabs {
		r.screenType = mainScreen

		var account save.Account

		for _, a := range msg.accounts {
			if a.ID == t.IdentityID {
				account = a
			}
		}

		if account.ID == "" {
			continue
		}

		nTab := r.createTab(account, t.Channel)

		if t.IsFocused {
			nTab.focus()
		}

		r.tabs = append(r.tabs, nTab)

		if t.IsFocused {
			r.tabCursor = len(r.tabs) - 1 // set index to the newest tab
			r.header.selectTab(nTab.id)
		}
		cmds = append(cmds, nTab.Init())
	}

	r.handleResize()
	return tea.Batch(cmds...)
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

		var channel string

		switch msg.Msg.(type) {
		case *command.PrivateMessage:
			channel = msg.Msg.(*command.PrivateMessage).ChannelUserName
		case *command.RoomState:
			channel = msg.Msg.(*command.RoomState).ChannelUserName
		case *command.UserNotice:
			channel = msg.Msg.(*command.UserNotice).ChannelUserName
		case *command.UserState:
			channel = msg.Msg.(*command.UserState).ChannelUserName
		case *command.ClearChat:
			channel = msg.Msg.(*command.ClearChat).ChannelUserName
		case *command.ClearMessage:
			channel = msg.Msg.(*command.ClearMessage).ChannelUserName
		case *command.SubMessage:
			channel = msg.Msg.(*command.SubMessage).ChannelUserName
		case *command.RaidMessage:
			channel = msg.Msg.(*command.RaidMessage).ChannelUserName
		case *command.SubGiftMessage:
			channel = msg.Msg.(*command.SubGiftMessage).ChannelUserName
		case *command.RitualMessage:
			channel = msg.Msg.(*command.RitualMessage).ChannelUserName
		case *command.AnnouncementMessage:
			channel = msg.Msg.(*command.AnnouncementMessage).ChannelUserName
		}

		return chatEventMessage{
			accountID: msg.ID,
			channel:   channel,
			message:   msg.Msg,
		}
	}
}

func (r *Root) closeTab() {
	if len(r.tabs) > r.tabCursor {
		tabID := r.tabs[r.tabCursor].id
		r.header.removeTab(tabID)
		r.tabs[r.tabCursor].Close()
		r.tabs = slices.DeleteFunc(r.tabs, func(t *tab) bool {
			return t.id == tabID
		})
		r.prevTab()
		r.handleResize()
	}
}
