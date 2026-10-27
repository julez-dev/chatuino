package tab

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/chat"
	"github.com/stretchr/testify/require"
)

func newTestComposer(t *testing.T, width int) *composer {
	t.Helper()

	settings := save.BuildDefaultSettings()
	input := newComposer(chat.NewPalette(), settings.BuildCustomSuggestionMap(), save.BuildDefaultTheme(), nil)
	input.SetWidth(width)

	return input
}

// indicatorLine returns the first rendered row, which is the reply indicator when a
// reply is armed.
func indicatorLine(view string) string {
	return strings.SplitN(view, "\n", 2)[0]
}

func Test_composer_ViewRendersFrameAndCounter(t *testing.T) {
	t.Parallel()

	input := newTestComposer(t, 40)
	input.SetValue("hello")

	view := input.View()
	lines := strings.Split(view, "\n")

	require.Contains(t, lines[0], "[ Chat ]")
	require.Contains(t, lines[len(lines)-1], "[ 5 / 500 ]")

	for _, line := range lines {
		require.Equal(t, 40, lipgloss.Width(line), "every row spans the full width: %q", line)
	}
}

func Test_composer_ViewRendersReplyIndicator(t *testing.T) {
	t.Parallel()

	t.Run("no armed reply adds no row", func(t *testing.T) {
		t.Parallel()

		input := newTestComposer(t, 80)
		require.NotContains(t, input.View(), "Replying to")
	})

	t.Run("shows author snippet and cancel hint padded to width", func(t *testing.T) {
		t.Parallel()

		input := newTestComposer(t, 80)
		input.ArmReply("msg-1", "otheruser", "nice play")

		got := indicatorLine(input.View())

		require.Contains(t, got, "@otheruser")
		require.Contains(t, got, "nice play")
		require.Contains(t, got, "(esc to cancel)")
		require.Equal(t, 80, lipgloss.Width(got))
	})

	t.Run("long snippet truncated and still fits the width", func(t *testing.T) {
		t.Parallel()

		input := newTestComposer(t, 80)
		input.ArmReply("msg-1", "otheruser", "this message is far too long to ever fit into the indicator line")

		got := indicatorLine(input.View())

		require.Contains(t, got, "…")
		require.NotContains(t, got, "indicator line")
		require.Equal(t, 80, lipgloss.Width(got))
	})

	t.Run("narrow terminal does not join label and hint", func(t *testing.T) {
		t.Parallel()

		input := newTestComposer(t, 10)
		input.ArmReply("msg-1", "otheruser", "nice play")

		got := indicatorLine(input.View())

		require.Contains(t, got, "@otheruser")
		require.Contains(t, got, "(esc to cancel)")
	})
}

func Test_composer_HeightTracksReply(t *testing.T) {
	t.Parallel()

	input := newTestComposer(t, 80)
	base := input.Height()

	input.ArmReply("msg-1", "otheruser", "nice play")
	require.Equal(t, base+1, input.Height(), "the indicator adds one row")

	require.Equal(t, "msg-1", input.TakeReply())
	require.False(t, input.ReplyArmed())
	require.Equal(t, base, input.Height(), "taking the reply removes the row")

	require.False(t, input.ClearReply(), "nothing left to clear")
}

func Test_composer_UpdateReportsHeightChange(t *testing.T) {
	t.Parallel()

	input := newTestComposer(t, 20)
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

func TestUserPalette_ComposerSeesUpdatesThroughReadOnlyInterface(t *testing.T) {
	t.Parallel()

	palette := chat.NewPalette()
	input := newComposer(palette, nil, save.BuildDefaultTheme(), nil)
	input.SetWidth(80)
	input.SetValue("@al")
	require.NotContains(t, input.View(), "@alice")

	// A user discovered by message rendering becomes available to the existing
	// composer, without sharing a mutable map or rebuilding the composer.
	render := palette.Renderer("alice", "#ff0000")
	input.SetValue("@al")
	require.Contains(t, input.View(), render("@alice"))

	names := palette.Names()
	names[0] = "changed"
	_, ok := palette.Lookup("alice")
	require.True(t, ok, "enumerating names must not expose mutable cache state")

	palette.Prune(nil)
	input.SetValue("@al")
	require.NotContains(t, input.View(), "@alice")
}

func Test_composer_SubmitConsumesReply(t *testing.T) {
	t.Parallel()

	c := newTestComposer(t, 80)

	require.Empty(t, c.Submit().ReplyTo, "no armed reply yields no message ID")

	c.ArmReply("msg-1", "other", "hi")
	require.Equal(t, "msg-1", c.Submit().ReplyTo)
	require.False(t, c.ReplyArmed(), "taking disarms the reply")
	require.Empty(t, c.Submit().ReplyTo, "a reply target is only used once")
}
