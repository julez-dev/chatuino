package tab

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/stretchr/testify/require"
)

func newTestChatInput(t *testing.T, width int) *chatInput {
	t.Helper()

	settings := save.BuildDefaultSettings()
	input := newChatInput(newUserPalette(), settings.BuildCustomSuggestionMap(), save.BuildDefaultTheme(), nil)
	input.SetWidth(width)

	return input
}

// indicatorLine returns the first rendered row, which is the reply indicator when a
// reply is armed.
func indicatorLine(view string) string {
	return strings.SplitN(view, "\n", 2)[0]
}

func Test_chatInput_View_frame(t *testing.T) {
	t.Parallel()

	input := newTestChatInput(t, 40)
	input.SetValue("hello")

	view := input.View()
	lines := strings.Split(view, "\n")

	require.Contains(t, lines[0], "[ Chat ]")
	require.Contains(t, lines[len(lines)-1], "[ 5 / 500 ]")

	for _, line := range lines {
		require.Equal(t, 40, lipgloss.Width(line), "every row spans the full width: %q", line)
	}
}

func Test_chatInput_replyIndicator(t *testing.T) {
	t.Parallel()

	t.Run("no armed reply adds no row", func(t *testing.T) {
		t.Parallel()

		input := newTestChatInput(t, 80)
		require.NotContains(t, input.View(), "Replying to")
	})

	t.Run("shows author snippet and cancel hint padded to width", func(t *testing.T) {
		t.Parallel()

		input := newTestChatInput(t, 80)
		input.ArmReply("msg-1", "otheruser", "nice play")

		got := indicatorLine(input.View())

		require.Contains(t, got, "@otheruser")
		require.Contains(t, got, "nice play")
		require.Contains(t, got, "(esc to cancel)")
		require.Equal(t, 80, lipgloss.Width(got))
	})

	t.Run("long snippet truncated and still fits the width", func(t *testing.T) {
		t.Parallel()

		input := newTestChatInput(t, 80)
		input.ArmReply("msg-1", "otheruser", "this message is far too long to ever fit into the indicator line")

		got := indicatorLine(input.View())

		require.Contains(t, got, "…")
		require.NotContains(t, got, "indicator line")
		require.Equal(t, 80, lipgloss.Width(got))
	})

	t.Run("narrow terminal does not join label and hint", func(t *testing.T) {
		t.Parallel()

		input := newTestChatInput(t, 10)
		input.ArmReply("msg-1", "otheruser", "nice play")

		got := indicatorLine(input.View())

		require.Contains(t, got, "@otheruser")
		require.Contains(t, got, "(esc to cancel)")
	})
}

func Test_chatInput_Height_tracksReply(t *testing.T) {
	t.Parallel()

	input := newTestChatInput(t, 80)
	base := input.Height()

	input.ArmReply("msg-1", "otheruser", "nice play")
	require.Equal(t, base+1, input.Height(), "the indicator adds one row")

	require.Equal(t, "msg-1", input.TakeReply())
	require.False(t, input.ReplyArmed())
	require.Equal(t, base, input.Height(), "taking the reply removes the row")

	require.False(t, input.ClearReply(), "nothing left to clear")
}

func Test_chatInput_Update_reportsHeightChange(t *testing.T) {
	t.Parallel()

	input := newTestChatInput(t, 20)
	input.Focus()

	// type enough to wrap onto a second line
	var changed bool
	for range 40 {
		_, c := input.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
		changed = changed || c
	}

	require.True(t, changed, "wrapping onto a new line is reported as a height change")
	require.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", input.Value())
}

func TestApplyWordReplacements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		content      string
		replacements WordReplacement
		want         string
	}{
		{
			name:         "empty replacements returns content unchanged",
			content:      "moon pool",
			replacements: WordReplacement{},
			want:         "moon pool",
		},
		{
			name:         "single-char emote token does not bleed into words",
			content:      "o moon pool",
			replacements: WordReplacement{"o": "[emote]"},
			want:         "[emote] moon pool",
		},
		{
			name:         "no standalone token leaves words untouched",
			content:      "moon pool",
			replacements: WordReplacement{"o": "[emote]"},
			want:         "moon pool",
		},
		{
			name:         "repeated standalone tokens all replaced",
			content:      "o moon o pool o",
			replacements: WordReplacement{"o": "[emote]"},
			want:         "[emote] moon [emote] pool [emote]",
		},
		{
			name:         "multiple distinct emotes replaced per token",
			content:      "Kappa and PogChamp",
			replacements: WordReplacement{"Kappa": "[k]", "PogChamp": "[p]"},
			want:         "[k] and [p]",
		},
		{
			name:         "substring match inside larger word is not replaced",
			content:      "Kappaccino Kappa",
			replacements: WordReplacement{"Kappa": "[k]"},
			want:         "Kappaccino [k]",
		},
		{
			// link producer keys on the whole token, so trailing punctuation is
			// part of the key and is preserved in the annotated value.
			name:         "url token with trailing punctuation matches whole token",
			content:      "see https://x.com! end",
			replacements: WordReplacement{"https://x.com!": "https://x.com [200 OK]!"},
			want:         "see https://x.com [200 OK]! end",
		},
		{
			name:         "url token wrapped in parens matches whole token",
			content:      "(https://x.com) done",
			replacements: WordReplacement{"(https://x.com)": "(https://x.com [200 OK])"},
			want:         "(https://x.com [200 OK]) done",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := &messageRenderer{}
			got := r.applyWordReplacements(tt.content, tt.replacements)
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
