package mainui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type helpSection struct {
	name  string
	binds []key.Binding
}

type help struct {
	keySections []helpSection
	port        viewport.Model
}

func newHelp(height, width int, deps *DependencyContainer) *help {
	sections := []helpSection{
		{
			"General",
			[]key.Binding{
				deps.Keymap.Up,
				deps.Keymap.Down,
				deps.Keymap.Escape,
				deps.Keymap.Confirm,
				deps.Keymap.Help,
			},
		},
		{
			"App Binds",
			[]key.Binding{
				deps.Keymap.Quit,
				deps.Keymap.Create,
				deps.Keymap.Remove,
				deps.Keymap.CloseTab,
				deps.Keymap.DumpScreen,
			},
		},
		{
			"Tab Binds",
			[]key.Binding{
				deps.Keymap.Next,
				deps.Keymap.Previous,
			},
		},
		{
			"Chat Binds",
			[]key.Binding{
				deps.Keymap.InsertMode,
				deps.Keymap.InspectMode,
				deps.Keymap.ChatPopUp,
				deps.Keymap.ChannelPopUp,
				deps.Keymap.GoToTop,
				deps.Keymap.GoToBottom,
				deps.Keymap.DumpChat,
				deps.Keymap.QuickTimeout,
				deps.Keymap.CopyMessage,
				deps.Keymap.SearchMode,
				deps.Keymap.QuickSent,
			},
		},
		{
			"Account Binds",
			[]key.Binding{
				deps.Keymap.MarkLeader,
			},
		},
	}

	help := &help{port: viewport.New(width, height), keySections: sections}
	help.port.SetContent(help.render())

	return help
}

func (h *help) Init() tea.Cmd {
	return nil
}

func (h *help) Update(msg tea.Msg) (*help, tea.Cmd) {
	var cmd tea.Cmd
	h.port, cmd = h.port.Update(msg)
	return h, cmd
}

func (h *help) View() string {
	return h.port.View()
}

func (h *help) handleResize(width, height int) {
	h.port.Width = width
	h.port.Height = height
	h.port.SetContent(h.render())
}

func (h *help) render() string {
	b := &strings.Builder{}

	head := lipgloss.NewStyle().
		Width(h.port.Width).
		AlignHorizontal(lipgloss.Center).Bold(true).Render("\n\nKeybind Help")

	centered := lipgloss.NewStyle().Width(h.port.Width).AlignHorizontal(lipgloss.Center).Render
	left := lipgloss.NewStyle().Width(h.port.Width / 2).AlignHorizontal(lipgloss.Right).Render
	right := lipgloss.NewStyle().Width(h.port.Width / 2).AlignHorizontal(lipgloss.Left).Render

	_, _ = b.WriteString(head)
	_, _ = b.WriteRune('\n')
	_, _ = b.WriteRune('\n')

	for _, section := range h.keySections {
		_, _ = b.WriteString(centered(lipgloss.NewStyle().Bold(true).Render(section.name)))
		_, _ = b.WriteRune('\n')
		_, _ = b.WriteRune('\n')
		for _, bind := range section.binds {
			line := left(strings.Join(bind.Keys(), "; ")+"  ") + right("  "+bind.Help().Desc)
			_, _ = b.WriteString(line)
			_, _ = b.WriteRune('\n')
		}
		_, _ = b.WriteRune('\n')
	}

	return b.String()
}
