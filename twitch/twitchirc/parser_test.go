package twitchirc

import (
	"bufio"
	"fmt"
	"os"
	"testing"
	"time"

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

func Test_parseEmotes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []Emote
	}{
		{
			name:  "empty-string",
			input: "",
			want:  []Emote{},
		},
		{
			name:  "single-emote",
			input: "115847:0-6",
			want: []Emote{
				{
					ID: "115847",
					Positions: []EmotePosition{
						{Start: 0, End: 6},
					},
				},
			},
		},
		{
			name:  "invalid-position",
			input: "115847:0-string",
			want: []Emote{
				{
					ID: "115847",
				},
			},
		},
		{
			name:  "mixed-with-double-usage",
			input: "115847:0-6,8-14/160397:16-20/25:22-26",
			want: []Emote{
				{
					ID: "115847",
					Positions: []EmotePosition{
						{Start: 0, End: 6},
						{Start: 8, End: 14},
					},
				},
				{
					ID: "160397",
					Positions: []EmotePosition{
						{Start: 16, End: 20},
					},
				},
				{
					ID: "25",
					Positions: []EmotePosition{
						{Start: 22, End: 26},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, parseEmotes(tt.input), "expected matching emote map")
		})
	}
}

func Test_parseTimestamp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  time.Time
	}{
		{
			name:  "valid-time",
			input: "1763895624889",
			want:  time.Date(2025, 11, 23, 12, 0, 24, 889000000, time.Local),
		},
		{
			name:  "invalid-input",
			input: "test",
			want:  time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, parseTimestamp(tt.input), "expected matching timestamp")
		})
	}
}

func Test_parsePrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  *prefix
	}{
		{
			name:  "valid-time",
			input: "julezdev!julezdev@julezdev.tmi.twitch.tv",
			want:  &prefix{Name: "julezdev", User: "julezdev", Host: "julezdev.tmi.twitch.tv"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.EqualValues(t, tt.want, parsePrefix(tt.input), "expected matching prefix")
		})
	}
}

func Test_parseTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  tags
	}{
		{
			name:  "valid-input",
			input: "emote-only=0;followers-only=0;r9k=0;room-id=82032862;slow=0;subs-only=0",
			want: tags{
				"emote-only":     "0",
				"followers-only": "0",
				"r9k":            "0",
				"room-id":        "82032862",
				"slow":           "0",
				"subs-only":      "0",
			},
		},
		{
			name:  "dobule-equal-input",
			input: "emote-only==0;followers-only=0;r9k=0;room-id=82032862;slow=0;subs-only=0",
			want: tags{
				"emote-only":     "=0",
				"followers-only": "0",
				"r9k":            "0",
				"room-id":        "82032862",
				"slow":           "0",
				"subs-only":      "0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.EqualValues(t, tt.want, parseTags(tt.input), "expected matching tags")
		})
	}
}

func Test_ParseIRC_Files(t *testing.T) {
	messageFile, err := os.Open("testdata/messages.txt")
	require.NoError(t, err)
	defer messageFile.Close()

	scanner := bufio.NewScanner(messageFile)
	for scanner.Scan() {
		message := scanner.Text()
		irc, err := ParseIRC(message)
		require.NoError(t, err)
		require.NotNil(t, irc)
	}
}

func Fuzz_ParseIRC(f *testing.F) {
	msgLineFmt := `@badge-info=subscriber/21;badges=subscriber/18;client-nonce=3b4d1fa0f6549a0228e5feafc4382755;color=#8A2BE2;display-name=julezdev;emotes=;first-msg=0;flags=;id=60654e92-f779-4e3b-beec-3f2d38031be9;mod=0;returning-chatter=0;room-id=92038375;subscriber=1;tmi-sent-ts=1763899302525;turbo=0;user-id=1;user-type= :julezdev!julezdev@julezdev.tmi.twitch.tv PRIVMSG #julezdev :%s`

	f.Fuzz(func(t *testing.T, input string) {
		irc, err := ParseIRC(fmt.Sprintf(msgLineFmt, input))
		require.NoError(t, err)
		require.NotNil(t, irc)
	})
}
