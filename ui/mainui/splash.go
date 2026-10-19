package mainui

import (
	"strings"

	_ "embed"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
)

//go:embed splash.txt
var splashArt string

type splash struct {
	width, height     int
	keymap            save.KeyMap
	userConfiguration UserConfiguration
	spinner           spinner.Model
	updateInfo        *UpdateInfo
}

func (s splash) Init() tea.Cmd {
	return s.spinner.Tick
}

func (s splash) Update(msg tea.Msg) (splash, tea.Cmd) {
	var cmd tea.Cmd
	s.spinner, cmd = s.spinner.Update(msg)
	return s, cmd
}

func (s splash) view(loading bool, err error) string {
	style := lipgloss.NewStyle().AlignHorizontal(lipgloss.Center).AlignVertical(lipgloss.Center).Width(s.width).Height(s.height)

	highlight := func(keys string) string {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(s.userConfiguration.Theme.SplashHighlightColor)).Render(keys)
	}

	// Mention both ways to open a channel. Either binding may be unbound in a
	// user's keymap, so only render hints for the ones that have keys.
	hints := make([]string, 0, 2)
	if keys := strings.Join(s.keymap.Create.Keys(), ", "); keys != "" {
		hints = append(hints, "Use "+highlight(keys)+" to create a new tab and join a channel")
	}
	if keys := strings.Join(s.keymap.QuickJoin.Keys(), ", "); keys != "" {
		hints = append(hints, "Use "+highlight(keys)+" to quick join a channel")
	}
	joinHint := strings.Join(hints, "\n")

	name := strings.Builder{}
	_, _ = name.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(s.userConfiguration.Theme.SplashHighlightColor)).Render("Cha"))
	_, _ = name.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(s.userConfiguration.Theme.ChatuinoSplashColor)).Bold(true).Render("tui"))
	_, _ = name.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(s.userConfiguration.Theme.SplashHighlightColor)).Render("no"))

	var help string
	switch {
	case loading:
		help = s.spinner.View() + " Loading initial state"
	case err != nil:
		help = err.Error()
		if joinHint != "" {
			help += "\n" + joinHint
		}
	default:
		help = joinHint
	}

	var updateLine string
	if s.updateInfo != nil && s.updateInfo.HasUpdate {
		highlightStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(s.userConfiguration.Theme.SplashHighlightColor))
		updateLine = "\n" + highlightStyle.Render("New update available: "+s.updateInfo.LatestVersion) + " (current: " + s.updateInfo.CurrentVersion + ")"
	}

	logo := splashArt
	splash := style.Render(logo + "\n" + "Welcome to " + name.String() + "!\n" + help + updateLine)

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
