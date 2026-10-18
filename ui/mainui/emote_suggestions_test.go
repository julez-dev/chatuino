package mainui

import (
	"testing"

	"github.com/julez-dev/chatuino/emote"
	"github.com/stretchr/testify/require"
)

func Test_emoteSuggestions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		userEmotes   []emote.Emote
		channelSet   emote.EmoteSet
		want         []string
		wantContains []string
	}{
		{
			name: "user emotes are always included",
			userEmotes: []emote.Emote{
				{Text: "Kappa", Platform: emote.Twitch},
				{Text: "mySubEmote", Platform: emote.Twitch},
			},
			want: []string{"Kappa", "mySubEmote"},
		},
		{
			name: "channel twitch emotes are excluded",
			channelSet: emote.EmoteSet{
				{Text: "channelSubEmote", Platform: emote.Twitch},
				{Text: "catJAM", Platform: emote.BTTV},
				{Text: "modCheck", Platform: emote.SevenTV},
				{Text: "monkaS", Platform: emote.FFZ},
			},
			want: []string{"catJAM", "modCheck", "monkaS"},
		},
		{
			name:       "duplicates collapse",
			userEmotes: []emote.Emote{{Text: "catJAM", Platform: emote.Twitch}},
			channelSet: emote.EmoteSet{{Text: "catJAM", Platform: emote.BTTV}},
			want:       []string{"catJAM"},
		},
		{
			name: "empty sets yield no suggestions",
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := emoteSuggestions(tt.userEmotes, tt.channelSet)
			require.ElementsMatch(t, tt.want, got)
		})
	}
}
