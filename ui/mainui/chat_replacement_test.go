package mainui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyWordReplacements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		content      string
		replacements wordReplacement
		want         string
	}{
		{
			name:         "empty replacements returns content unchanged",
			content:      "moon pool",
			replacements: wordReplacement{},
			want:         "moon pool",
		},
		{
			name:         "single-char emote token does not bleed into words",
			content:      "o moon pool",
			replacements: wordReplacement{"o": "[emote]"},
			want:         "[emote] moon pool",
		},
		{
			name:         "no standalone token leaves words untouched",
			content:      "moon pool",
			replacements: wordReplacement{"o": "[emote]"},
			want:         "moon pool",
		},
		{
			name:         "repeated standalone tokens all replaced",
			content:      "o moon o pool o",
			replacements: wordReplacement{"o": "[emote]"},
			want:         "[emote] moon [emote] pool [emote]",
		},
		{
			name:         "multiple distinct emotes replaced per token",
			content:      "Kappa and PogChamp",
			replacements: wordReplacement{"Kappa": "[k]", "PogChamp": "[p]"},
			want:         "[k] and [p]",
		},
		{
			name:         "substring match inside larger word is not replaced",
			content:      "Kappaccino Kappa",
			replacements: wordReplacement{"Kappa": "[k]"},
			want:         "Kappaccino [k]",
		},
		{
			// link producer keys on the whole token, so trailing punctuation is
			// part of the key and is preserved in the annotated value.
			name:         "url token with trailing punctuation matches whole token",
			content:      "see https://x.com! end",
			replacements: wordReplacement{"https://x.com!": "https://x.com [200 OK]!"},
			want:         "see https://x.com [200 OK]! end",
		},
		{
			name:         "url token wrapped in parens matches whole token",
			content:      "(https://x.com) done",
			replacements: wordReplacement{"(https://x.com)": "(https://x.com [200 OK])"},
			want:         "(https://x.com [200 OK]) done",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &chatWindow{}
			got := c.applyWordReplacements(tt.content, tt.replacements)
			require.Equal(t, tt.want, got)
		})
	}
}
