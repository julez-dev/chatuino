package ui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rs/zerolog"
)

type resizeChatContainerMessage struct {
	Width, Height, YPosition int
}

func computeChatContainerSize(m Model) tea.Cmd {
	containerHeight := m.Height - lipgloss.Height(m.renderTabHeader())
	yPosition := m.Height + 2

	return func() tea.Msg {
		m.Logger.Info()
		return resizeChatContainerMessage{
			Width:     m.Width,
			Height:    containerHeight,
			YPosition: yPosition,
		}
	}
}

type Model struct {
	ctx            context.Context
	Width, Height  int
	Logger         zerolog.Logger
	Tabs           []*Tab
	activeTabIndex int
}

func New(ctx context.Context, logger zerolog.Logger) *Model {
	return &Model{
		ctx:    ctx,
		Logger: logger,
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
		m.Logger.Info().Int("height", msg.Height).Msg("window height")
		m.Height = msg.Height
		m.Width = msg.Width
		cmds = append(cmds, computeChatContainerSize(m))
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "k":
			c := NewTab(m.ctx, m.Logger, "noway4u_sir", m.Width, m.Height)
			m.Tabs = append(m.Tabs, c)
			cmds = append(cmds, computeChatContainerSize(m))
			cmds = append(cmds, c.Init())

		case "tab":
			m.nextTab()
		case "shift+tab":
			m.prevTab()
		}
	}

	for i, tab := range m.Tabs {
		m.Tabs[i], cmd = tab.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	b := strings.Builder{}

	b.WriteString(m.renderTabHeader())

	if activeTab, ok := m.getActiveTab(); ok {
		b.WriteString("\n")
		b.WriteString(activeTab.View())
	}

	return b.String()
}

func (m Model) renderTabHeader() string {
	tabParts := make([]string, 0, len(m.Tabs))
	for index, tab := range m.Tabs {
		style := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Border(lipgloss.RoundedBorder(), true).
			BorderForeground(lipgloss.Color("#FF0000"))

		if index == m.activeTabIndex {
			style = style.Background(lipgloss.Color("#FF0000"))
		}

		tabParts = append(tabParts, style.Render(tab.Channel))
	}

	return lipgloss.JoinHorizontal(lipgloss.Center, tabParts...)
}

func (m *Model) getActiveTab() (*Tab, bool) {
	if len(m.Tabs) > m.activeTabIndex {
		if m.Tabs[m.activeTabIndex] == nil {
			return nil, false
		}

		return m.Tabs[m.activeTabIndex], true
	}

	return nil, false
}

func (m *Model) nextTab() {
	newIndex := m.activeTabIndex + 1

	if newIndex > len(m.Tabs)-1 {
		newIndex = 0
	}

	m.Logger.Info().Int("new-index", newIndex).Send()

	m.activeTabIndex = newIndex
}

func (m *Model) prevTab() {
	newIndex := m.activeTabIndex - 1

	if newIndex < 0 {
		newIndex = len(m.Tabs) - 1
	}
	m.Logger.Info().Int("new-index", newIndex).Send()

	m.activeTabIndex = newIndex
}
