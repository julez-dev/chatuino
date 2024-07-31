package mainui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/twitch/eventsub"
)

type pollItem struct {
	id      string
	title   string
	percent float64
	votes   int
	bar     progress.Model
}

type poll struct {
	title   string
	enabled bool
	width   int
	items   []pollItem
}

func newPoll(width int) *poll {
	return &poll{width: width}
}

func (p *poll) Init() tea.Cmd {
	return nil
}

func (p *poll) Update(msg tea.Msg) (*poll, tea.Cmd) {
	return p, nil
}

func (p *poll) View() string {
	if !p.enabled {
		return ""
	}

	padding := lipgloss.NewStyle().PaddingLeft(2).PaddingRight(2)

	sb := strings.Builder{}
	_, _ = fmt.Fprintf(&sb, "Poll: %q\n\n", p.title)

	for i, item := range p.items {
		_, _ = fmt.Fprintf(&sb, "%s\n", item.title)
		_, _ = fmt.Fprintf(&sb, "%s\n", item.bar.ViewAs(item.percent))

		// no new line on last item
		if i != len(p.items)-1 {
			_, _ = sb.WriteRune('\n')
		}
	}

	return padding.Render(sb.String())
}

func (p *poll) setWidth(width int) {
	p.width = width
	for i := range p.items {
		p.items[i].bar.Width = clamp(width-4, 0, width) // total width - padding
	}
}

func (p *poll) setPollData(event eventsub.Message[eventsub.NotificationPayload]) {
	p.items = nil
	p.items = make([]pollItem, 0, len(event.Payload.Event.Choices))

	p.title = event.Payload.Event.Title

	var totalPoints int
	for _, choice := range event.Payload.Event.Choices {
		item := pollItem{
			id:    choice.ID,
			title: choice.Title,
			votes: choice.Votes,
			bar:   progress.New(progress.WithWidth(clamp(p.width-4, 0, p.width))),
		}

		p.items = append(p.items, item)
		totalPoints += choice.Votes
	}

	for i, item := range p.items {
		if totalPoints == 0 {
			p.items[i].percent = 0
			continue
		}

		p.items[i].percent = float64(item.votes) / float64(totalPoints)
	}

}
