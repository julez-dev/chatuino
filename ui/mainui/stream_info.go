package mainui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/muesli/reflow/wordwrap"
)

type setStreamInfo struct {
	target string
	viewer int
	title  string
	game   string
}

type streamInfo struct {
	id        string
	channelID string
	ttvAPI    apiClient
	ctx       context.Context

	width int

	// data
	viewer int
	title  string
	game   string
}

func newStreamInfo(ctx context.Context, channelID string, ttvAPI apiClient, width int) *streamInfo {
	return &streamInfo{
		id:        uuid.New().String(),
		ctx:       ctx,
		width:     width,
		channelID: channelID,
		ttvAPI:    ttvAPI,
	}
}

func (s *streamInfo) Init() tea.Cmd {
	return func() tea.Msg {
		return s.refreshStreamInfo()
	}
}

func (s *streamInfo) Update(msg tea.Msg) (*streamInfo, tea.Cmd) {
	switch msg := msg.(type) {
	case setStreamInfo:
		if msg.target != s.id {
			return s, nil
		}

		s.game = msg.game
		s.title = msg.title
		s.viewer = msg.viewer

		return s, s.doTick
	}
	return s, nil
}

func (s *streamInfo) View() string {
	if s.game == "" && s.viewer == 0 && s.title == "" {
		return ""
	}

	style := lipgloss.NewStyle().AlignHorizontal(lipgloss.Center).Width(s.width).MaxWidth(s.width)
	info := wordwrap.String(fmt.Sprintf("%s - %s (%d)", s.game, s.title, s.viewer), s.width-10)
	return style.Render(info)
}

func (s streamInfo) doTick() tea.Msg {
	timer := time.NewTimer(time.Second * 15)
	defer timer.Stop()

	select {
	case <-timer.C:
		return s.refreshStreamInfo()
	case <-s.ctx.Done():
		return nil
	}
}

func (s streamInfo) refreshStreamInfo() tea.Msg {
	ctx, cancel := context.WithTimeout(s.ctx, time.Second*5)
	defer cancel()

	info, err := s.ttvAPI.GetStreamInfo(ctx, []string{s.channelID})
	if err != nil {
		return nil
	}

	if len(info.Data) < 1 {
		return setStreamInfo{
			target: s.id,
		}
	}

	return setStreamInfo{
		target: s.id,
		viewer: info.Data[0].ViewerCount,
		title:  info.Data[0].Title,
		game:   info.Data[0].GameName,
	}
}
