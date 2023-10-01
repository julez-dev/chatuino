package mainui

import (
	"context"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/save"
	"github.com/rs/zerolog"
	"slices"
)

type AccountProvider interface {
	GetAllWithAnonymous() []save.Account
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

type Root struct {
	logger        zerolog.Logger
	width, height int
	keymap        AppKeyMap
	headerKeymap  HeaderKeyMap

	screenType activeScreen

	// dependencies
	accounts   AccountProvider
	emoteStore EmoteStore

	// components
	splash    splash
	header    tabHeader
	joinInput join

	tabCursor int
	tabs      []tab
}

func NewUI(logger zerolog.Logger, provider AccountProvider, emoteStore EmoteStore) Root {
	return Root{
		logger:       logger,
		width:        10,
		height:       10,
		keymap:       buildDefaultKeyMap(),
		headerKeymap: buildDefaultHeaderKeyMap(),

		// components
		splash:    splash{},
		header:    newTabHeader(),
		joinInput: newJoin(provider.GetAllWithAnonymous(), 10, 10),

		accounts:   provider,
		emoteStore: emoteStore,
	}
}

func (r Root) Init() tea.Cmd {
	return nil
}

func (r Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.width = msg.Width
		r.height = msg.Height
		r.handleResize()
	case joinChannelMessage:
		r.screenType = mainScreen
		id := r.header.addTab(msg.channel, msg.account.DisplayName)

		headerHeight := r.getHeaderHeigth()

		nTab := newTab(id, msg.channel, r.width, r.height-headerHeight, r.emoteStore, msg.account)
		r.tabs = append(r.tabs, nTab)

		r.tabCursor = len(r.tabs) - 1 // set index to the newest tab
		r.header.selectTab(id)
		nTab.focus()
		r.joinInput.blur()

		r.handleResize()

		return r, nTab.Init()
	case tea.KeyMsg:
		if key.Matches(msg, r.keymap.Quit) {
			return r, tea.Quit
		}

		if key.Matches(msg, r.keymap.EscapeJoinScreen) {
			if r.screenType == inputScreen {
				if len(r.tabs) > r.tabCursor {
					r.tabs[r.tabCursor].focus()
				}

				r.joinInput.blur()
				r.screenType = mainScreen
			}
		}

		if key.Matches(msg, r.keymap.ToggleJoinScreen) {
			switch r.screenType {
			case mainScreen:
				if len(r.tabs) > r.tabCursor {
					r.tabs[r.tabCursor].blur()
				}

				r.screenType = inputScreen
				r.joinInput = newJoin(r.accounts.GetAllWithAnonymous(), r.width, r.height)
				r.joinInput.focus()
			case inputScreen:
				if len(r.tabs) > r.tabCursor {
					r.tabs[r.tabCursor].blur()
				}

				r.joinInput.blur()
				r.screenType = mainScreen
			}

			return r, nil
		}

		if r.screenType == mainScreen {
			if key.Matches(msg, r.headerKeymap.Next) {
				r.nextTab()
			}

			if key.Matches(msg, r.headerKeymap.Previous) {
				r.prevTab()
			}

			if key.Matches(msg, r.keymap.CloseTab) {
				r.closeTab()
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

		return lipgloss.JoinVertical(lipgloss.Top, r.header.View(), r.tabs[r.tabCursor].View())
	case inputScreen:
		return r.joinInput.View()
	}

	return ""
}

func (r *Root) getHeaderHeigth() int {
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
	headerHeight := r.getHeaderHeigth()

	r.logger.Info().Int("header-height", headerHeight).Send()

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

func (r *Root) closeTab() {
	if len(r.tabs) > r.tabCursor {
		tabID := r.tabs[r.tabCursor].id
		r.header.removeTab(tabID)
		r.tabs[r.tabCursor].Close()
		r.tabs = slices.DeleteFunc(r.tabs, func(t tab) bool {
			return t.id == tabID
		})
		r.prevTab()
	}
}
