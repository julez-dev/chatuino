package chatui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/common-nighthawk/go-figure"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/rs/zerolog"
	"golang.org/x/exp/slices"
)

type emoteStore interface {
	GetByText(channel, text string) (emote.Emote, bool)
	RefreshLocal(ctx context.Context, channelID string) error
}

type twitchAPI interface {
	GetUsers(ctx context.Context, logins []string, ids []string) (twitch.UserResponse, error)
	GetStreamInfo(ctx context.Context, broadcastID []string) (twitch.GetStreamsResponse, error)
}

type resizeTabContainerMessage struct {
	Width, Height int
}

func computeTabContainerSize(m Model) tea.Cmd {
	containerHeight := m.height - lipgloss.Height(m.renderTabHeader())

	return func() tea.Msg {
		return resizeTabContainerMessage{
			Width:  m.width,
			Height: containerHeight,
		}
	}
}

type activeScreen int

const (
	mainScreen activeScreen = iota
	inputScreen
)

type Model struct {
	screenType     activeScreen
	ctx            context.Context
	width, height  int
	logger         zerolog.Logger
	tabs           []*tab
	activeTabIndex int

	inputScreen *channelInputScreen

	emoteStore      emoteStore
	ttvAPI          twitchAPI
	accountProvider accountProvider
}

func New(ctx context.Context, logger zerolog.Logger, emoteStore emoteStore, ttvAPI twitchAPI, accountProvider accountProvider) *Model {
	return &Model{
		screenType:      mainScreen,
		ctx:             ctx,
		logger:          logger,
		emoteStore:      emoteStore,
		ttvAPI:          ttvAPI,
		accountProvider: accountProvider,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		cmds = append(cmds, computeTabContainerSize(m))
	case joinChannelCmd:
		c := newTab(m.ctx, m.logger.With().Str("channel", msg.channel).Logger(), msg.channel, m.emoteStore, msg.account)
		m.tabs = append(m.tabs, c)
		cmds = append(cmds, computeTabContainerSize(m))
		cmds = append(cmds, c.Init())

		// Blur active tab, if exists
		if _, ok := m.getActiveTab(); ok {
			m.tabs[m.activeTabIndex].Blur()
		}

		m.activeTabIndex = len(m.tabs) - 1 // set active index to newest tab
		m.tabs[m.activeTabIndex].Focus()
		m.screenType = mainScreen
		m.inputScreen.Blur()
	case removeTabMessage:
		m.removeTab(msg.id)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.screenType == inputScreen {
				if len(m.tabs) > m.activeTabIndex {
					m.tabs[m.activeTabIndex].Focus()
				}
				m.screenType = mainScreen
			}
		case "f1":
			switch m.screenType {
			case mainScreen:
				if len(m.tabs) > m.activeTabIndex {
					m.tabs[m.activeTabIndex].Blur()
				}

				m.screenType = inputScreen
				inputScreen := newChannelInputScreen(m.width, m.height, m.accountProvider)
				inputScreen.Focus()
				m.inputScreen = inputScreen
			case inputScreen:
				if len(m.tabs) > m.activeTabIndex {
					m.tabs[m.activeTabIndex].Focus()
				}
				m.screenType = mainScreen
			}
		case "tab":
			if m.screenType == mainScreen {
				m.nextTab()
			}
		case "shift+tab":
			if m.screenType == mainScreen {
				m.prevTab()
			}
		}
	}

	if m.screenType == inputScreen {
		m.inputScreen, cmd = m.inputScreen.Update(msg)
		cmds = append(cmds, cmd)
	}

	for i, tab := range m.tabs {
		m.tabs[i], cmd = tab.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	b := strings.Builder{}

	if m.screenType == mainScreen {
		b.WriteString(m.renderTabHeader())

		if activeTab, ok := m.getActiveTab(); ok {
			b.WriteString("\n")
			b.WriteString(activeTab.View())
		}

		if len(m.tabs) == 0 {
			style := lipgloss.NewStyle().AlignHorizontal(lipgloss.Center).AlignVertical(lipgloss.Center).Width(m.width).Height(m.height)
			name := color.New(color.FgHiMagenta).Sprint("ChaTUIno")
			help := "Use " + color.New(color.FgHiMagenta).Sprint("F1") + " to create a new tab and join a channel"
			logo := style.Render(figure.NewFigure("CHATUINO", "isometric1", true).String() + "\n" + "Welcome to " + name + "!\n" + help)
			b.WriteString(logo)
		}
	}

	if m.screenType == inputScreen {
		b.WriteString(m.inputScreen.View())
	}

	return b.String()
}

func (m *Model) removeTab(id uuid.UUID) {
	for i, t := range m.tabs {
		m.logger.Info().Any("have", t.id).Any("search", id).Send()

		if t.id != id {
			continue
		}

		m.tabs[i] = nil
		m.tabs = slices.Delete(m.tabs, i, i+1)
		m.tabs = slices.Clip(m.tabs)

		if i == m.activeTabIndex {
			m.prevTab()
		}

		return
	}
}

func (m Model) renderTabHeader() string {
	tabParts := make([]string, 0, len(m.tabs))
	for index, tab := range m.tabs {
		style := lipgloss.NewStyle().
			Bold(true).
			Border(lipgloss.HiddenBorder()).
			BorderForeground(lipgloss.Color("135"))

		if index == m.activeTabIndex {
			style = style.Background(lipgloss.Color("135"))
		}

		tabParts = append(tabParts, style.Render(tab.channel))
	}

	return lipgloss.JoinHorizontal(lipgloss.Center, tabParts...)
}

func (m *Model) getActiveTab() (*tab, bool) {
	if len(m.tabs) > m.activeTabIndex {
		return m.tabs[m.activeTabIndex], true
	}

	return nil, false
}

func (m *Model) nextTab() {
	if len(m.tabs) > m.activeTabIndex {
		m.tabs[m.activeTabIndex].Blur()
	}

	newIndex := m.activeTabIndex + 1

	if newIndex >= len(m.tabs) {
		newIndex = 0
	}

	m.activeTabIndex = newIndex

	if len(m.tabs) > m.activeTabIndex {
		m.tabs[m.activeTabIndex].Focus()
	}
}

func (m *Model) prevTab() {
	if len(m.tabs) > m.activeTabIndex {
		m.tabs[m.activeTabIndex].Blur()
	}

	newIndex := m.activeTabIndex - 1

	if newIndex < 0 {
		newIndex = len(m.tabs) - 1

		if newIndex < 0 {
			newIndex = 0
		}
	}

	m.activeTabIndex = newIndex

	if len(m.tabs) > m.activeTabIndex {
		m.tabs[m.activeTabIndex].Focus()
	}
}
