package ui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/common-nighthawk/go-figure"
	"github.com/fatih/color"
	"github.com/rs/zerolog"
)

type resizeChatContainerMessage struct {
	Width, Height int
}

func computeChatContainerSize(m Model) tea.Cmd {
	containerHeight := m.height - lipgloss.Height(m.renderTabHeader()) - 3 // minus border

	return func() tea.Msg {
		return resizeChatContainerMessage{
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
}

func New(ctx context.Context, logger zerolog.Logger) *Model {
	return &Model{
		screenType: mainScreen,
		ctx:        ctx,
		logger:     logger,
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
		cmds = append(cmds, computeChatContainerSize(m))
	case joinChannelCmd:
		c := newTab(m.ctx, m.logger.With().Str("channel", msg.channel).Logger(), msg.channel, m.width, m.height)
		m.tabs = append(m.tabs, c)
		cmds = append(cmds, computeChatContainerSize(m))
		cmds = append(cmds, c.Init())

		m.tabs[m.activeTabIndex].Blur()
		m.activeTabIndex = len(m.tabs) - 1
		m.tabs[m.activeTabIndex].Focus()
		m.screenType = mainScreen
		m.inputScreen.Blur()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "f1":
			m.logger.Info().Int("screen", int(m.screenType)).Send()
			switch m.screenType {
			case mainScreen:
				if len(m.tabs) > 0 {
					m.tabs[m.activeTabIndex].Blur()
				}

				m.screenType = inputScreen
				inputScreen := newChannelInputScreen(m.width, m.height)
				inputScreen.Focus()
				m.inputScreen = inputScreen
			case inputScreen:
				if len(m.tabs) > 0 {
					m.tabs[m.activeTabIndex].Focus()
				}
				m.screenType = mainScreen
			}
		case "tab":
			if m.screenType == mainScreen {
				m.tabs[m.activeTabIndex].Blur()
				m.nextTab()
				m.tabs[m.activeTabIndex].Focus()
			}
		case "shift+tab":
			if m.screenType == mainScreen {
				m.tabs[m.activeTabIndex].Blur()
				m.prevTab()
				m.tabs[m.activeTabIndex].Focus()
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
		if m.tabs[m.activeTabIndex] == nil {
			return nil, false
		}

		return m.tabs[m.activeTabIndex], true
	}

	return nil, false
}

func (m *Model) nextTab() {
	newIndex := m.activeTabIndex + 1

	if newIndex > len(m.tabs)-1 {
		newIndex = 0
	}

	m.activeTabIndex = newIndex
}

func (m *Model) prevTab() {
	newIndex := m.activeTabIndex - 1

	if newIndex < 0 {
		newIndex = len(m.tabs) - 1
	}

	m.activeTabIndex = newIndex
}
