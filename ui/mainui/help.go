package mainui

import (
	"strings"

	"github.com/julez-dev/chatuino/save"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/ui/nav"
)

type helpSection struct {
	name  string
	binds []key.Binding
}

type help struct {
	keySections []helpSection
	port        viewport.Model
}

func newHelp(height, width int, keymap save.KeyMap) *help {
	sections := []helpSection{
		{
			"General",
			[]key.Binding{
				keymap.Up,
				keymap.Down,
				keymap.Escape,
				keymap.Confirm,
				keymap.Help,
			},
		},
		{
			"App Binds",
			[]key.Binding{
				keymap.Quit,
				keymap.Create,
				keymap.QuickJoin,
				keymap.Remove,
				keymap.CloseTab,
				keymap.DumpScreen,
			},
		},
		{
			"Tab Binds",
			[]key.Binding{
				keymap.Next,
				keymap.Previous,
			},
		},
		{
			"Pane Controls (" + strings.Join(keymap.PaneMode.Keys(), "/") + "; pane-mode commands)",
			[]key.Binding{
				keymap.PaneMode,
				keymap.Escape,
				keymap.Create,
				keymap.PaneFocusLeft,
				keymap.PaneFocusDown,
				keymap.PaneFocusUp,
				keymap.PaneFocusRight,
				keymap.PaneSwapLeft,
				keymap.PaneSwapDown,
				keymap.PaneSwapUp,
				keymap.PaneSwapRight,
				keymap.PaneSplitColumns,
				keymap.PaneSplitRows,
				keymap.PaneNarrow,
				keymap.PaneWiden,
				keymap.PaneShorten,
				keymap.PaneTaller,
				keymap.PaneClose,
				keymap.PaneZoom,
				keymap.PaneNext,
				keymap.PanePrevious,
			},
		},
		{
			"Suggestion Binds",
			[]key.Binding{
				keymap.AcceptSuggestion,
				keymap.NextSuggestion,
				keymap.PrevSuggestion,
			},
		},
		{
			"Chat Binds",
			[]key.Binding{
				keymap.InsertMode,
				keymap.InspectMode,
				keymap.ChatPopUp,
				keymap.ChannelPopUp,
				keymap.GoToTop,
				keymap.GoToBottom,
				keymap.PageUp,
				keymap.PageDown,
				keymap.DumpChat,
				keymap.QuickTimeout,
				keymap.CopyMessage,
				keymap.CopyToClipboard,
				keymap.SearchMode,
				keymap.QuickSent,
				keymap.ReplyMessage,
			},
		},
		{
			"Account Binds",
			[]key.Binding{
				keymap.MarkLeader,
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

func (h *help) Update(msg tea.Msg) (nav.Screen, tea.Cmd) {
	var cmd tea.Cmd
	h.port, cmd = h.port.Update(msg)
	return h, cmd
}

func (h *help) View() string {
	return h.port.View()
}

// Screen implementation. Help replaces the view instead of overlaying it and
// never captures text, so '?' keeps toggling it from the chat view.

func (h *help) OverlaysBackground() bool { return false }

func (h *help) Route() string { return helpRoute }

func (h *help) Resize(width, height int) { h.handleResize(width, height) }

// scrollsOnWheel marks this screen as consuming mouse wheel messages.
func (h *help) scrollsOnWheel() {}

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
