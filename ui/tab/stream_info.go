package tab

import (
	"context"
	"strings"
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/reflow/wordwrap"
)

type streamInfo struct {
	channelID string
	ttvAPI    APIClient
	printer   *message.Printer

	width  int
	loaded bool

	// data
	viewer int
	title  string
	game   string

	// cached rendered view; rebuilt only when data or width changes
	cachedView  string
	cachedDirty bool
}

func newStreamInfo(channelID string, ttvAPI APIClient, width int) *streamInfo {
	return &streamInfo{
		width:       width,
		channelID:   channelID,
		ttvAPI:      ttvAPI,
		printer:     message.NewPrinter(language.English),
		cachedDirty: true,
	}
}

func (s *streamInfo) Init() tea.Cmd {
	return func() tea.Msg {
		return s.refreshStreamInfo()
	}
}

func (s *streamInfo) Update(msg tea.Msg) (*streamInfo, tea.Cmd) {
	switch msg := msg.(type) {
	case SetStreamInfoMessage:
		if msg.Target != s.channelID {
			return s, nil
		}
		s.loaded = true
		s.game = msg.Game
		s.title = msg.Title
		s.viewer = msg.Viewer
		s.cachedDirty = true

		return s, nil
	}
	return s, nil
}

func (s *streamInfo) View() string {
	if !s.loaded {
		return ""
	}

	if s.game == "" && s.viewer == 0 && s.title == "" {
		return ""
	}

	if !s.cachedDirty && s.cachedView != "" {
		return s.cachedView
	}

	info := wordwrap.String(s.printer.Sprintf("%s - %s (%d Viewer)\n", s.game, s.title, s.viewer), s.width-10)
	infoSplit := strings.Split(info, "\n")

	for i, v := range infoSplit {
		infoSplit[i] = centerTextGraphemeAware(s.width, v)
	}

	s.cachedView = strings.Join(infoSplit, "\n")
	s.cachedDirty = false
	return s.cachedView
}

func (s *streamInfo) refreshStreamInfo() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	info, err := s.ttvAPI.GetStreamInfo(ctx, []string{s.channelID})
	if err != nil {
		return nil
	}

	if len(info.Data) < 1 {
		return SetStreamInfoMessage{
			Target: s.channelID,
		}
	}

	return SetStreamInfoMessage{
		Target:   s.channelID,
		Viewer:   info.Data[0].ViewerCount,
		Title:    info.Data[0].Title,
		Game:     info.Data[0].GameName,
		Username: info.Data[0].UserName,
		IsLive:   !info.Data[0].StartedAt.IsZero(),
	}
}
