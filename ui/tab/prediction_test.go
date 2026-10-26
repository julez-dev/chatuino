package tab

import (
	"testing"

	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/stretchr/testify/require"
)

func predictionMessage(event eventsub.Event) eventsub.Message[eventsub.NotificationPayload] {
	return eventsub.Message[eventsub.NotificationPayload]{
		Payload: eventsub.NotificationPayload{Event: event},
	}
}

func TestPrediction_SetPredictionData(t *testing.T) {
	t.Parallel()

	p := newPrediction(80)
	p.setPredictionData(predictionMessage(eventsub.Event{
		Title: "Will we win?",
		Outcomes: []eventsub.Outcome{
			{ID: "yes", Title: "Yes", ChannelPoints: 500},
			{ID: "no", Title: "No", ChannelPoints: 200},
		},
	}))

	require.Equal(t, "Will we win?", p.title)
	require.Len(t, p.outcomes, 2)
	require.InDelta(t, 5.0/7.0, p.outcomes[0].percent, 0.0001)
	require.InDelta(t, 2.0/7.0, p.outcomes[1].percent, 0.0001)
}

func TestPrediction_View(t *testing.T) {
	t.Parallel()

	p := newPrediction(80)
	p.enabled = true
	p.locked = true
	p.title = "Will we win?"
	p.outcomes = []predictionOutcome{
		{title: "Yes", points: 500},
		{title: "No", points: 200},
	}
	p.setWidth(80)

	text := StripANSI(p.View())
	require.Contains(t, text, `Prediction: "Will we win?" (locked)`)
	require.Contains(t, text, "Yes (500 points)")
	require.Contains(t, text, "No (200 points)")

	p.enabled = false
	require.Empty(t, p.View())
}

func TestPrediction_Winner(t *testing.T) {
	t.Parallel()

	p := newPrediction(80)
	p.outcomes = []predictionOutcome{
		{id: "yes", title: "Yes", points: 500},
		{id: "no", title: "No", points: 700},
	}

	winner, ok := p.winner("yes")
	require.True(t, ok)
	require.Equal(t, "Yes", winner.title)

	winner, ok = p.winner("")
	require.True(t, ok)
	require.Equal(t, "No", winner.title)

	_, ok = p.winner("missing")
	require.True(t, ok, "an unknown id falls back to the highest outcome")
	require.Equal(t, "No", winner.title)

	p.outcomes = nil
	_, ok = p.winner("")
	require.False(t, ok)
}
