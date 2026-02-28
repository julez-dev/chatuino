package mainui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
				deps.Keymap.QuickJoin,
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

	help := &help{port: viewport.New(viewport.WithWidth(width), viewport.WithHeight(height)), keySections: sections}
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
	h.port.SetWidth(width)
	h.port.SetHeight(height)
	h.port.SetContent(h.render())
}

func (h *help) render() string {
	b := &strings.Builder{}

	head := lipgloss.NewStyle().
		Width(h.port.Width()).
		AlignHorizontal(lipgloss.Center).Bold(true).Render("\n\nKeybind Help")

	centered := lipgloss.NewStyle().Width(h.port.Width()).AlignHorizontal(lipgloss.Center).Render
	left := lipgloss.NewStyle().Width(h.port.Width() / 2).AlignHorizontal(lipgloss.Right).Render
	right := lipgloss.NewStyle().Width(h.port.Width() / 2).AlignHorizontal(lipgloss.Left).Render

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

	// Search syntax reference
	_, _ = b.WriteString(centered(lipgloss.NewStyle().Bold(true).Render("Search Syntax")))
	_, _ = b.WriteRune('\n')
	_, _ = b.WriteRune('\n')

	searchEntries := []struct{ filter, desc string }{
		{"hello", "message or username contains \"hello\""},
		{"content:term", "message content contains term"},
		{"user:term", "username contains term"},
		{"badge:name", "user has badge (e.g. badge:moderator)"},
		{"is:mod|sub|vip|first", "filter by user property"},
		{"/pattern/", "regex on content and username"},
		{"regex:pattern", "regex on content and username"},
		{"user:/pattern/", "regex scoped to username"},
		{"content:/pattern/", "regex scoped to content"},
		{"-filter", "negate any filter (e.g. -user:bot)"},
		{"\"quoted value\"", "match phrase with spaces"},
		{"filter1 filter2", "combine filters (AND)"},
	}

	for _, entry := range searchEntries {
		line := left(entry.filter+"  ") + right("  "+entry.desc)
		_, _ = b.WriteString(line)
		_, _ = b.WriteRune('\n')
	}

	_, _ = b.WriteRune('\n')

	return b.String()
}
