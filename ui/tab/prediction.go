package tab

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/twitch/live"
)

type predictionOutcome struct {
	id      string
	title   string
	points  int
	percent float64
	bar     progress.Model
}

type prediction struct {
	title    string
	enabled  bool
	locked   bool
	locksAt  time.Time
	width    int
	outcomes []predictionOutcome
}

// predictionTickMsg is scoped to one prediction. Bubbletea messages fan out to
// every tab, so without the owner every active prediction would schedule a new
// ticker for every tick from any other tab.
type predictionTickMsg struct {
	owner *prediction
}

func predictionTickCmd(owner *prediction) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return predictionTickMsg{owner: owner}
	})
}

// needsCountdown reports whether a one-second tick is still useful. A locked or
// finished prediction stops ticking; an expired lock time stops as well, because
// the view then says it is waiting for the lock event.
func (p *prediction) needsCountdown(now time.Time) bool {
	return p.enabled && !p.locked && !p.locksAt.IsZero() && now.Before(p.locksAt)
}

func newPrediction(width int) *prediction {
	return &prediction{width: width}
}

func (p *prediction) Init() tea.Cmd {
	return nil
}

func (p *prediction) Update(msg tea.Msg) (*prediction, tea.Cmd) {
	return p, nil
}

func (p *prediction) View() string {
	if !p.enabled {
		return ""
	}

	padding := lipgloss.NewStyle().PaddingLeft(2).PaddingRight(2)

	sb := strings.Builder{}
	_, _ = fmt.Fprintf(&sb, "Prediction: %q", p.title)

	switch {
	case p.locked:
		_, _ = sb.WriteString(" (locked)")
	case !p.locksAt.IsZero():
		if remaining := time.Until(p.locksAt); remaining > 0 {
			_, _ = fmt.Fprintf(&sb, " (locks in %s)", remaining.Round(time.Second))
		} else {
			_, _ = sb.WriteString(" (locking...)")
		}
	}

	_, _ = sb.WriteString("\n\n")

	for i, outcome := range p.outcomes {
		_, _ = fmt.Fprintf(&sb, "%s (%d points)\n", outcome.title, outcome.points)
		_, _ = fmt.Fprintf(&sb, "%s\n", outcome.bar.ViewAs(outcome.percent))

		// no new line on last item
		if i != len(p.outcomes)-1 {
			_, _ = sb.WriteRune('\n')
		}
	}

	return padding.Render(sb.String())
}

func (p *prediction) setWidth(width int) {
	p.width = width
	for i := range p.outcomes {
		p.outcomes[i].bar.SetWidth(clamp(width-4, 0, width)) // total width - padding
	}
}

func (p *prediction) setPredictionData(prediction live.Prediction) {
	p.title = prediction.Title
	p.locksAt = prediction.LocksAt
	p.outcomes = make([]predictionOutcome, 0, len(prediction.Outcomes))

	var totalPoints int
	for _, outcome := range prediction.Outcomes {
		p.outcomes = append(p.outcomes, predictionOutcome{
			id:     outcome.ID,
			title:  outcome.Title,
			points: outcome.Points,
			bar:    progress.New(progress.WithWidth(clamp(p.width-4, 0, p.width))),
		})
		totalPoints += outcome.Points
	}

	for i := range p.outcomes {
		if totalPoints == 0 {
			p.outcomes[i].percent = 0
			continue
		}

		p.outcomes[i].percent = float64(p.outcomes[i].points) / float64(totalPoints)
	}
}

// winner returns the outcome with the most channel points. The EventSub payload
// names the winning outcome on end, but progress events do not, so callers use
// this for the fallback.
func (p *prediction) winner(id string) (predictionOutcome, bool) {
	if len(p.outcomes) == 0 {
		return predictionOutcome{}, false
	}

	if id != "" {
		for _, outcome := range p.outcomes {
			if outcome.id == id {
				return outcome, true
			}
		}
	}

	winner := p.outcomes[0]
	for _, outcome := range p.outcomes[1:] {
		if outcome.points > winner.points {
			winner = outcome
		}
	}

	return winner, true
}
