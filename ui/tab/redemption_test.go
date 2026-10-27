package tab

import (
	"testing"

	"github.com/julez-dev/chatuino/twitch/live"
	"github.com/stretchr/testify/require"
)

func TestRedemptionText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event live.Redemption
		want  string
	}{
		{
			name: "custom reward",
			event: live.Redemption{
				User:      "Alice",
				UserInput: "make it purple",
				Status:    "unfulfilled",
				Reward:    "Highlight My Message",
				Cost:      500,
			},
			want: `Alice redeemed "Highlight My Message" (500 points): make it purple`,
		},
		{
			name: "fulfilled custom reward",
			event: live.Redemption{
				User:   "Alice",
				Status: "fulfilled",
				Reward: "Highlight My Message",
				Cost:   500,
			},
			want: `Alice redeemed "Highlight My Message" (500 points) [fulfilled]`,
		},
		{
			name: "automatic reward uses the message text",
			event: live.Redemption{
				User:        "alice",
				MessageText: "sparkles",
				Reward:      "Message Effect",
				Cost:        200,
			},
			want: `alice redeemed "Message Effect" (200 points): sparkles`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, redemptionText(tt.event))
		})
	}
}

func TestRedemptionStatusTerminal(t *testing.T) {
	t.Parallel()

	require.True(t, redemptionStatusTerminal("fulfilled"))
	require.True(t, redemptionStatusTerminal("canceled"))
	require.False(t, redemptionStatusTerminal("unfulfilled"))
	require.False(t, redemptionStatusTerminal(""))
}
