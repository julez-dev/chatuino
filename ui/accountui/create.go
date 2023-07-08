package accountui

import tea "github.com/charmbracelet/bubbletea"

type createModel struct{}

func newCreateModel() createModel {
	return createModel{}
}

func (c createModel) Init() tea.Cmd {
	return nil
}

func (c createModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	return c, cmd
}

func (c createModel) View() string {
	return ""
}
