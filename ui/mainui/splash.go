package mainui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/common-nighthawk/go-figure"
	"github.com/fatih/color"
)

type splash struct {
	width, height int
}

func (s splash) Init() tea.Cmd {
	return nil
}

func (s splash) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return s, nil
}

func (s splash) View() string {
	style := lipgloss.NewStyle().AlignHorizontal(lipgloss.Center).AlignVertical(lipgloss.Center).Width(s.width).Height(s.height)

	name := color.New(color.FgHiMagenta).Sprint("ChaTUIno")
	help := "Use " + color.New(color.FgHiMagenta).Sprint("F1") + " to create a new tab and join a channel"
	logo := style.Render(figure.NewFigure("CHATUINO", "isometric1", true).String() + "\n" + "Welcome to " + name + "!\n" + help)

	return logo
}
