package mainui

import (
	"testing"

	"github.com/julez-dev/chatuino/save"

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

func TestExpandGraphicRows(t *testing.T) {
	t.Parallel()

	// bottom row (row 1) of a two row image
	image := "\x1b[38;2;0;0;1m\U0010EEEE\u030D\u0305\U0010EEEE\x1b[39m"
	lines := []string{"  12:00 user: hi " + image, "              plain text"}

	t.Run("scale 1 is noop", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, lines, expandGraphicRows(lines, 1))
	})

	t.Run("scale 2 adds rows above lines with images only, images bottom aligned", func(t *testing.T) {
		t.Parallel()
		got := expandGraphicRows(lines, 2)
		require.Equal(t, []string{
			"                 \x1b[38;2;0;0;1m\U0010EEEE\u0305\u0305\U0010EEEE\x1b[39m",
			lines[0],
			lines[1],
		}, got)
	})
}

func TestGraphicRows(t *testing.T) {
	t.Parallel()

	s := save.Settings{Chat: save.ChatSettings{GraphicScale: 2}}
	require.Equal(t, 1, graphicRows(s), "graphics disabled")

	s.Chat.GraphicEmotes = true
	require.Equal(t, 2, graphicRows(s))

	s.Chat.GraphicScale = 0
	require.Equal(t, 1, graphicRows(s), "unset defaults to 1")
}
