package chatui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/reflow/wrap"
	"github.com/rs/zerolog"
)

type setChannelInfoMessage struct {
	target  uuid.UUID
	gotData bool
	viewer  int
	title   string
	game    string
}

type channelInfo struct {
	id     uuid.UUID
	ttv    twitchAPI
	logger zerolog.Logger
	ctx    context.Context
	width  int

	channel   string
	channelID string
	title     string
	viewer    int
	game      string

	hasData bool
}

func newChannelInfo(ctx context.Context, logger zerolog.Logger, ttv twitchAPI, channel string) *channelInfo {
	return &channelInfo{
		id:      uuid.New(),
		ctx:     ctx,
		logger:  logger,
		ttv:     ttv,
		channel: channel,
	}
}

func (c *channelInfo) Init() tea.Cmd {
	return func() tea.Msg {
		userData, err := c.ttv.GetUsers(c.ctx, []string{c.channel}, nil)
		if err != nil {
			c.logger.Err(err).Send()
			return nil
		}

		return setChannelIDMessage{
			target:    c.id,
			channelID: userData.Data[0].ID,
		}
	}
}

func (c *channelInfo) Update(msg tea.Msg) (*channelInfo, tea.Cmd) {
	var (
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case setChannelIDMessage:
		if msg.target == c.id {
			c.channelID = msg.channelID
			cmds = append(cmds, func() tea.Msg {
				return fetchStreamData(c)
			})
		}
	case setChannelInfoMessage:
		if msg.target == c.id {
			c.hasData = msg.gotData
			c.title = msg.title
			c.viewer = msg.viewer
			c.game = msg.game
			c.logger.Info().Str("title", msg.title).Int("viewer", msg.viewer).Send()
			cmds = append(cmds, doTick(c))
		}
	}

	return c, tea.Batch(cmds...)
}

func (c *channelInfo) View() string {
	if !c.hasData {
		return ""
	}

	style := lipgloss.NewStyle().AlignHorizontal(lipgloss.Center).Width(c.width)
	info := wrap.String(wordwrap.String(fmt.Sprintf("%s - %s (%d)", c.game, c.title, c.viewer), c.width-5), c.width-5)
	return style.Render(info)
}

func fetchStreamData(c *channelInfo) tea.Msg {
	ctx, cancel := context.WithTimeout(c.ctx, time.Second*5)
	defer cancel()

	resp, err := c.ttv.GetStreamInfo(ctx, []string{
		c.channelID,
	})

	if err != nil {
		c.logger.Err(err).Msg("failed to get channel info")
		return nil
	}

	if len(resp.Data) < 1 {
		return setChannelInfoMessage{
			target:  c.id,
			gotData: false,
		}
	}

	return setChannelInfoMessage{
		target:  c.id,
		title:   resp.Data[0].Title,
		game:    resp.Data[0].GameName,
		viewer:  resp.Data[0].ViewerCount,
		gotData: true,
	}
}

func doTick(c *channelInfo) tea.Cmd {
	return tea.Tick(time.Second*45, func(_ time.Time) tea.Msg {
		return fetchStreamData(c)
	})
}
