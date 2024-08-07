package mainui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/common-nighthawk/go-figure"
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

func (s splash) view(loading bool, err error) string {
	style := lipgloss.NewStyle().AlignHorizontal(lipgloss.Center).AlignVertical(lipgloss.Center).Width(s.width).Height(s.height)

	keyDisplay := strings.Join(s.keymap.Create.Keys(), ", ")

	name := lipgloss.NewStyle().Foreground(lipgloss.Color("#8bd5ca")).Render("Chatuino")

	var help string
	if loading {
		help = "Loading state from disk..."
	} else if err != nil {
		help = err.Error() + "\n"
		help += "Use " + lipgloss.NewStyle().Foreground(lipgloss.Color("#8bd5ca")).Render(keyDisplay) + " to create a new tab and join a channel"
	} else {
		help = "Use " + lipgloss.NewStyle().Foreground(lipgloss.Color("#8bd5ca")).Render(keyDisplay) + " to create a new tab and join a channel"
	}

	logo := lipgloss.NewStyle().Foreground(lipgloss.Color("#c6a0f6")).Render(figure.NewFigure("CHATUINO", "isometric1", true).String())
	splash := style.Render(logo + "\n" + "Welcome to " + name + "!\n" + help)

	return splash
}

func (s splash) View() string {
	return s.view(false, nil)
}

func (s splash) ViewLoading() string {
	return s.view(true, nil)
}

func (s splash) ViewError(err error) string {
	return s.view(false, err)
}
