package mainui

import (
	"context"
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
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/server"
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
	GetAllForUser(id string) emote.EmoteSet
}

type AppKeyMap struct {
	Quit             key.Binding
	ToggleJoinScreen key.Binding
	CloseTab         key.Binding
	EscapeJoinScreen key.Binding
	DumpScreen       key.Binding
}

type HeaderKeyMap struct {
	Next     key.Binding
	Previous key.Binding
}

func buildDefaultKeyMap() AppKeyMap {
	return AppKeyMap{
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "Quit the program"),
		),
		ToggleJoinScreen: key.NewBinding(
			key.WithKeys("f1"),
			key.WithHelp("f1", "Toggle join channel scrren"),
		),
		CloseTab: key.NewBinding(
			key.WithKeys("q", "ctrl+w"),
			key.WithHelp("q/ctrl+w", "Close Tab"),
		),
		EscapeJoinScreen: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "Close join input"),
		),
		DumpScreen: key.NewBinding(
			key.WithKeys("f12"),
			key.WithHelp("f12", "Dump curren buffer"),
		),
	}
}

func buildDefaultHeaderKeyMap() HeaderKeyMap {
	return HeaderKeyMap{
		Next: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "Select next tab"),
		),
		Previous: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "Select previous tab"),
		),
	}
}

type activeScreen int

const (
	mainScreen activeScreen = iota
	inputScreen
)

type setStateMessage struct {
	state    save.AppState
	accounts []save.Account
}

type Root struct {
	logger   zerolog.Logger
	clientID string

	width, height int
	keymap        AppKeyMap
	headerKeymap  HeaderKeyMap

	screenType activeScreen

	// dependencies
	accounts   AccountProvider
	emoteStore EmoteStore
	serverAPI  *server.Client

	// components
	splash    splash
	header    tabHeader
	joinInput join

	tabCursor int
	tabs      []tab
}

func NewUI(logger zerolog.Logger, provider AccountProvider, emoteStore EmoteStore, clientID string, serverClient *server.Client) Root {
	return Root{
		clientID:     clientID,
		logger:       logger,
		width:        10,
		height:       10,
		keymap:       buildDefaultKeyMap(),
		headerKeymap: buildDefaultHeaderKeyMap(),

		// components
		splash:    splash{},
		header:    newTabHeader(),
		joinInput: newJoin(provider, 10, 10),

		accounts:   provider,
		emoteStore: emoteStore,
		serverAPI:  serverClient,
	}
}

func (r Root) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			state, err := save.AppStateFromDisk()
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
	)
}

func (r Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

			tabCmds := make([]tea.Cmd, 0, len(t.IRCMessages)+1) // lengths of messages plus length for init message

			identity := account.DisplayName

			if account.IsAnonymous {
				identity = "Anonymous"
			}

			id := r.header.addTab(t.Channel, identity)
			headerHeight := r.getHeaderHeight()
			nTab, err := newTab(id, r.logger, r.clientID, r.serverAPI, t.Channel, r.width, r.height-headerHeight, r.emoteStore, account, r.accounts, t.IRCMessages)
			if err != nil {
				r.logger.Error().Err(err).Send()
				continue
			}

			if t.IsFocused {
				nTab.focus()
			}

			tabCmds = append(tabCmds, nTab.Init())

			r.tabs = append(r.tabs, nTab)

			if t.IsFocused {
				r.tabCursor = len(r.tabs) - 1 // set index to the newest tab
				r.header.selectTab(id)
			}

			cmds = append(cmds, tea.Sequence(tabCmds...))
		}

		r.handleResize()
		return r, tea.Sequence(cmds...)
	case tea.WindowSizeMsg:
		r.width = msg.Width
		r.height = msg.Height
		r.handleResize()
	case joinChannelMessage:
		r.screenType = mainScreen

		identity := msg.account.DisplayName

		if msg.account.IsAnonymous {
			identity = "Anonymous"
		}

		id := r.header.addTab(msg.channel, identity)

		headerHeight := r.getHeaderHeight()

		nTab, err := newTab(id, r.logger, r.clientID, r.serverAPI, msg.channel, r.width, r.height-headerHeight, r.emoteStore, msg.account, r.accounts, nil)
		if err != nil {
			r.logger.Error().Err(err).Send()
			return r, nil
		}

		nTab.focus()

		r.tabs = append(r.tabs, nTab)

		r.tabCursor = len(r.tabs) - 1 // set index to the newest tab
		r.header.selectTab(id)

		r.joinInput.blur()

		r.handleResize()

		return r, nTab.Init()
	case tea.KeyMsg:

		// Intentionally block the app until all items are saved
		// TODO: The tea.Run() function returns the final model
		// 	so we could get the state there which maybe the better way to handle this
		if key.Matches(msg, r.keymap.Quit) {
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

			err := appState.Save()
			if err != nil {
				panic(err)
			}

			return r, tea.Quit
		}

		if key.Matches(msg, r.keymap.EscapeJoinScreen) {
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

			defer f.Close()

			io.Copy(f, strings.NewReader(stripAnsi(r.View())))

			return r, nil
		}

		if key.Matches(msg, r.keymap.ToggleJoinScreen) {
			switch r.screenType {
			case mainScreen:
				if len(r.tabs) > r.tabCursor {
					r.tabs[r.tabCursor].blur()
				}

				r.screenType = inputScreen
				r.joinInput = newJoin(r.accounts, r.width, r.height)
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
			if key.Matches(msg, r.headerKeymap.Next) {
				if len(r.tabs) > r.tabCursor && r.tabs[r.tabCursor].state == insertMode {
					r.tabs[r.tabCursor], cmd = r.tabs[r.tabCursor].Update(msg)
					return r, cmd
				}

				r.nextTab()
			}

			if key.Matches(msg, r.headerKeymap.Previous) {
				r.prevTab()
			}

			if key.Matches(msg, r.keymap.CloseTab) {
				if len(r.tabs) > r.tabCursor && r.tabs[r.tabCursor].state == inChatWindow {
					r.closeTab()
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

func (r Root) View() string {
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

	r.logger.Info().Int("header-height", headerHeight).Send()

	for i := range r.tabs {
		r.tabs[i].height = r.height - headerHeight
		r.logger.Info().Int("tab-height", r.tabs[i].height).Send()

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

func (r *Root) closeTab() {
	if len(r.tabs) > r.tabCursor {
		tabID := r.tabs[r.tabCursor].id
		r.header.removeTab(tabID)
		r.tabs[r.tabCursor].Close()
		r.tabs = slices.DeleteFunc(r.tabs, func(t tab) bool {
			return t.id == tabID
		})
		r.prevTab()
		r.handleResize()
	}
}
