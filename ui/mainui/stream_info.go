package mainui

import (
	"context"
	"strings"
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/reflow/wordwrap"
	"github.com/rs/zerolog/log"
)

type setStreamInfo struct {
	target string
	viewer int
	title  string
	game   string
}

type streamInfo struct {
	channelID string
	ttvAPI    APIClient
	printer   *message.Printer
	done      chan struct{}

	width int

	// data
	viewer int
	title  string
	game   string
}

func newStreamInfo(channelID string, ttvAPI APIClient, width int) *streamInfo {
	return &streamInfo{
		width:     width,
		channelID: channelID,
		done:      make(chan struct{}, 1),
		ttvAPI:    ttvAPI,
		printer:   message.NewPrinter(language.English),
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
		if msg.target != s.channelID {
			return s, nil
		}
		log.Logger.Info().Msg("updating stream info")

		s.game = msg.game
		s.title = msg.title
		s.viewer = msg.viewer

		return s, nil
	}
	return s, nil
}

func (s *streamInfo) View() string {
	if s.game == "" && s.viewer == 0 && s.title == "" {
		return ""
	}

	info := wordwrap.String(s.printer.Sprintf("%s - %s (%d Viewer)\n", s.game, s.title, s.viewer), s.width-10)
	infoSplit := strings.Split(info, "\n")

	for i, v := range infoSplit {
		infoSplit[i] = centerTextGraphemeAware(s.width, v)
	}

	return strings.Join(infoSplit, "\n")
}

func (s *streamInfo) doTick() tea.Msg {
	timer := time.NewTimer(time.Second * 90)

	defer func() {
		timer.Stop()
		select {
		case <-timer.C:
		default:
		}
	}()

	select {
	case <-timer.C:
		return s.refreshStreamInfo()
	case <-s.done:
		return nil
	}
}

func (s *streamInfo) refreshStreamInfo() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	info, err := s.ttvAPI.GetStreamInfo(ctx, []string{s.channelID})
	if err != nil {
		return nil
	}

	if len(info.Data) < 1 {
		return setStreamInfo{
			target: s.channelID,
		}
	}

	return setStreamInfo{
		target: s.channelID,
		viewer: info.Data[0].ViewerCount,
		title:  info.Data[0].Title,
		game:   info.Data[0].GameName,
	}
}
