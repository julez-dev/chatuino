package twitchirc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_parseBadges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []Badge
	}{
		{
			name:  "empty-string",
			input: "",
		},
		{
			name:  "single-badge",
			input: "twitch-recap-2024/1",
			want: []Badge{
				{
					Name:    "twitch-recap-2024",
					Version: "1",
				},
			},
		},
		{
			name:  "multiple-badges",
			input: "subscriber/6,battlefield-6/1",
			want: []Badge{
				{
					Name:    "subscriber",
					Version: "6",
				},
				{
					Name:    "battlefield-6",
					Version: "1",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, parseBadges(tt.input), "expected matching badge map")
		})
	}
}
