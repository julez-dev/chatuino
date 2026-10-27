package mainui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/nav"
	"github.com/stretchr/testify/require"
)

func newTestPanePicker(width, height int) *panePicker {
	return newPanePicker(width, height, save.BuildDefaultTheme(), save.BuildDefaultKeyMap(), 1,
		[]nav.Row{{ID: "tab-1", Title: "julezdev", Identity: "julezdev"}})
}

// The picker is composited by its View width. Padding that width out to the
// maximum the picker may occupy counts as width but is invisible, so the drawn
// box ended up half that padding left of centre.
func Test_panePicker_centeredOverBackground(t *testing.T) {
	t.Parallel()

	for _, bgWidth := range []int{80, 120, 121} {
		view := newTestPanePicker(bgWidth, 40).View()
		boxWidth := lipgloss.Width(view)

		composited := stripANSI(centerOverlay(view, grid(bgWidth, 40, ".")))
		require.Equal(t, 40, lipgloss.Height(composited))

		// Same integer-division centring centerOverlay uses, so the box is
		// exactly where a background of its own width would put it.
		want := bgWidth/2 - boxWidth/2

		lines := strings.Split(composited, "\n")
		boxLines := strings.Split(stripANSI(view), "\n")
		require.Len(t, lines, 40)
		require.Len(t, boxLines, lipgloss.Height(view))

		// Rows above and below the box stay untouched background.
		y := 40/2 - lipgloss.Height(view)/2
		for i, line := range lines {
			expected := strings.Repeat(".", bgWidth)
			if i >= y && i < y+len(boxLines) {
				expected = strings.Repeat(".", want) + boxLines[i-y] + strings.Repeat(".", bgWidth-want-boxWidth)
			}

			require.Equal(t, expected, line, "bg width %d: row %d", bgWidth, i)
		}
	}
}

// The picker must not pad itself to its maximum size just to be centred, but it
// still has to stay inside the terminal.
func Test_panePicker_blockIsBounded(t *testing.T) {
	t.Parallel()

	p := newTestPanePicker(120, 40)
	view := p.View()

	require.LessOrEqual(t, lipgloss.Width(view), p.width)
	require.LessOrEqual(t, lipgloss.Height(view), p.height)
	require.Contains(t, stripANSI(view), "Choose a tab for the new pane")

	lines := strings.Split(view, "\n")
	for i, line := range lines {
		require.Equal(t, lipgloss.Width(lines[0]), lipgloss.Width(line), "line %d must match the block width", i)
	}

	narrow := newTestPanePicker(10, 5).View()
	require.LessOrEqual(t, lipgloss.Width(narrow), 10)
	require.LessOrEqual(t, lipgloss.Height(narrow), 5)
}
