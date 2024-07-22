package mainui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/common-nighthawk/go-figure"
	"github.com/fatih/color"
	"github.com/julez-dev/chatuino/save"
)

type splash struct {
	width, height int
	keymap        save.KeyMap
}

func (s splash) Init() tea.Cmd {
	return nil
}

func (s splash) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return s, nil
}

func (s splash) View() string {
	style := lipgloss.NewStyle().AlignHorizontal(lipgloss.Center).AlignVertical(lipgloss.Center).Width(s.width).Height(s.height)

	keyDisplay := strings.Join(s.keymap.Create.Keys(), ", ")
	name := color.New(color.FgHiMagenta).Sprint("Chatuino")
	help := "Use " + color.New(color.FgHiMagenta).Sprint(keyDisplay) + " to create a new tab and join a channel"
	logo := style.Render(figure.NewFigure("CHATUINO", "isometric1", true).String() + "\n" + "Welcome to " + name + "!\n" + help)

	return logo
}
