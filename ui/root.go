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
	containerHeight := m.height - lipgloss.Height(m.renderTabHeader())
	yPosition := m.height + 2

	return func() tea.Msg {
		return resizeChatContainerMessage{
			Width:     m.width,
			Height:    containerHeight,
			YPosition: yPosition,
		}
	}
}

type Model struct {
	ctx            context.Context
	width, height  int
	logger         zerolog.Logger
	tabs           []*tab
	activeTabIndex int
}

func New(ctx context.Context, logger zerolog.Logger) *Model {
	return &Model{
		ctx:    ctx,
		logger: logger,
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
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "k":
			c := newTab(m.ctx, m.logger, "noway4u_sir", m.width, m.height)
			m.tabs = append(m.tabs, c)
			cmds = append(cmds, computeChatContainerSize(m))
			cmds = append(cmds, c.Init())

		case "tab":
			m.nextTab()
		case "shift+tab":
			m.prevTab()
		}
	}

	for i, tab := range m.tabs {
		m.tabs[i], cmd = tab.Update(msg)
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
	tabParts := make([]string, 0, len(m.tabs))
	for index, tab := range m.tabs {
		style := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Border(lipgloss.RoundedBorder(), true).
			BorderForeground(lipgloss.Color("#FF0000"))

		if index == m.activeTabIndex {
			style = style.Background(lipgloss.Color("#FF0000"))
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
