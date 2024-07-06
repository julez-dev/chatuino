package mainui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/multiplexer"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/twitch/command"
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
	GetUsers(ctx context.Context, logins []string, ids []string) (twitch.UserResponse, error)
	GetStreamInfo(ctx context.Context, broadcastID []string) (twitch.GetStreamsResponse, error)
	GetChatSettings(ctx context.Context, broadcasterID string, moderatorID string) (twitch.GetChatSettingsResponse, error)
	RefreshToken(ctx context.Context, refreshToken string) (string, string, error)
}

type activeScreen int

const (
	mainScreen activeScreen = iota
	inputScreen
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

type setStateMessage struct {
	state    save.AppState
	accounts []save.Account
}

type chatEventMessage struct {
	accountID string
	channel   string
	message   twitch.IRCer
}

type forwardChatMessage struct {
	msg multiplexer.InboundMessage
}

type Root struct {
	logger   zerolog.Logger
	clientID string

	width, height int
	keymap        save.KeyMap

	screenType activeScreen

	// dependencies
	accounts       AccountProvider
	emoteStore     EmoteStore
	serverAPI      APIClientWithRefresh
	buildTTVClient func(clientID string, opts ...twitch.APIOptionFunc) (APIClient, error)
	loadSaveState  func() (save.AppState, error)

	// One API Client per Chatuino User Tab
	ttvAPIUserClient map[string]APIClient

	// chat multiplexer channels
	in  chan multiplexer.InboundMessage
	out <-chan multiplexer.OutboundMessage

	// components
	splash    splash
	header    *tabHeader
	joinInput *join

	tabCursor int
	tabs      []*tab
}

func NewUI(logger zerolog.Logger, provider AccountProvider, emoteStore EmoteStore, clientID string, serverClient APIClientWithRefresh, keymap save.KeyMap) *Root {
	multi := multiplexer.NewMultiplexer(logger, provider)

	in := make(chan multiplexer.InboundMessage)
	out := multi.ListenAndServe(in)

	return &Root{
		clientID: clientID,
		logger:   logger,
		width:    10,
		height:   10,
		keymap:   keymap,

		// components
		splash:    splash{},
		header:    newTabHeader(),
		joinInput: newJoin(provider, 10, 10, keymap),

		// chat multiplexer channels
		in:  in,
		out: out,

		accounts:         provider,
		ttvAPIUserClient: make(map[string]APIClient),
		emoteStore:       emoteStore,
		serverAPI:        serverClient,
		buildTTVClient: func(clientID string, opts ...twitch.APIOptionFunc) (APIClient, error) {
			return twitch.NewAPI(clientID, opts...)
		},
		loadSaveState: save.AppStateFromDisk,
	}
}

func (r *Root) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			state, err := r.loadSaveState()
			if err != nil {
				return nil
			}

			accounts, err := r.accounts.GetAllAccounts()
			if err != nil {
				return nil
			}

			return setStateMessage{state: state, accounts: accounts}
		},
		r.joinInput.Init(),
		r.waitChatEvents(),
	)
}

func (r *Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case setStateMessage:
		cmds = make([]tea.Cmd, 0, len(msg.state.Tabs))

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

			nTab, err := r.createTab(account, t.Channel, t.IRCMessages)
			if err != nil {
				r.logger.Error().Err(err).Send()
				continue
			}

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
		return r, tea.Batch(cmds...)
	case joinChannelMessage:
		r.screenType = mainScreen

		nTab, err := r.createTab(msg.account, msg.channel, nil)
		if err != nil {
			r.logger.Error().Err(err).Send()
			return r, nil
		}

		nTab.focus()

		r.tabs = append(r.tabs, nTab)

		r.tabCursor = len(r.tabs) - 1 // set index to the newest tab
		r.header.selectTab(nTab.id)

		r.joinInput.blur()

		r.handleResize()

		return r, nTab.Init()
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
		cmd := func() tea.Msg {
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

		if key.Matches(msg, r.keymap.Escape) {
			if r.screenType == inputScreen {
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
				r.joinInput = newJoin(r.accounts, r.width, r.height, r.keymap)
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
				if len(r.tabs) > r.tabCursor && r.tabs[r.tabCursor].state == insertMode {
					r.tabs[r.tabCursor], cmd = r.tabs[r.tabCursor].Update(msg)
					return r, cmd
				}

				r.nextTab()
			}

			if key.Matches(msg, r.keymap.Previous) {
				r.prevTab()
			}

			if key.Matches(msg, r.keymap.CloseTab) {
				if len(r.tabs) > r.tabCursor && r.tabs[r.tabCursor].state != insertMode {
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
								r.in <- multiplexer.InboundMessage{
									AccountID: currentTab.account.ID,
									Msg: command.PartMessage{
										Channel: currentTab.channel,
									},
								}
								return nil
							})
						}

						cmds = append(cmds, func() tea.Msg {
							r.in <- multiplexer.InboundMessage{
								AccountID: currentTab.account.ID,
								Msg:       multiplexer.DecrementTabCounter{},
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
			IsFocused:   t.focused,
			Channel:     t.channel,
			IdentityID:  t.account.ID,
			IRCMessages: make([]*command.PrivateMessage, 0, len(t.chatWindow.entries)),
		}

		relevantEntries := t.chatWindow.entries

		// If the chat holds more than 10 times, only persist the latest 10 to save space
		if len(relevantEntries) > 10 {
			relevantEntries = relevantEntries[len(relevantEntries)-10:]
		}

		tabState.SelectedIndex = len(relevantEntries) - 1 // fallback to last entry if known of the filtered were selected
		for i, e := range relevantEntries {
			if msg, ok := e.Message.(*command.PrivateMessage); ok {
				if e.Selected {
					tabState.SelectedIndex = i
				}

				tabState.IRCMessages = append(tabState.IRCMessages, msg)
			}
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

	close(r.in)
	return errors.Join(errs...)
}

func (r *Root) createTab(account save.Account, channel string, initialMessages []*command.PrivateMessage) (*tab, error) {
	identity := account.DisplayName

	if account.IsAnonymous {
		identity = "Anonymous"
	}

	id := r.header.addTab(channel, identity)

	headerHeight := r.getHeaderHeight()

	if _, ok := r.ttvAPIUserClient[account.ID]; !ok {
		var api APIClient
		var err error

		if !account.IsAnonymous {
			api, err = r.buildTTVClient(r.clientID, twitch.WithUserAuthentication(r.accounts, r.serverAPI, account.ID))

			// err -should- never happen
			if err != nil {
				return nil, err
			}
		} else {
			api = r.serverAPI
		}

		r.ttvAPIUserClient[account.ID] = api
	}

	nTab, err := newTab(id, r.logger, r.ttvAPIUserClient[account.ID], channel, r.width, r.height-headerHeight, r.emoteStore, account, r.accounts, initialMessages, r.keymap)
	if err != nil {
		return nil, err
	}

	nTab.focus()

	return nTab, nil
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
