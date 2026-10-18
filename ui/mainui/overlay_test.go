package mainui

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/stretchr/testify/require"
)

// grid builds a w x h block of a repeated character.
func grid(w, h int, fill string) string {
	lines := make([]string, h)
	for i := range lines {
		lines[i] = strings.Repeat(fill, w)
	}

	return strings.Join(lines, "\n")
}

func assertSize(t *testing.T, s string, w, h int) {
	t.Helper()

	lines := strings.Split(s, "\n")
	require.Len(t, lines, h, "height must match the background")

	for i, line := range lines {
		require.Equal(t, w, lipgloss.Width(line), "line %d must match the background width", i)
	}
}

var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*[a-zA-Z]")

func stripANSI(s string) string { return ansiPattern.ReplaceAllString(s, "") }

func Test_centerOverlay(t *testing.T) {
	t.Parallel()

	t.Run("centers the foreground and keeps background dimensions", func(t *testing.T) {
		t.Parallel()

		got := centerOverlay(grid(4, 3, "#"), grid(20, 7, "."))
		assertSize(t, got, 20, 7)

		lines := strings.Split(got, "\n")
		// x = 20/2 - 4/2 = 8, y = 7/2 - 3/2 = 2
		centered := strings.Repeat(".", 8) + "####" + strings.Repeat(".", 8)
		require.Equal(t, centered, lines[2])
		require.Equal(t, centered, lines[3])
		require.Equal(t, centered, lines[4])

		// Rows outside the foreground are untouched background.
		require.Equal(t, strings.Repeat(".", 20), lines[0])
		require.Equal(t, strings.Repeat(".", 20), lines[6])
	})

	// The previous implementation centered with bgWidth/2 - fgWidth/2, which is
	// not (bgWidth-fgWidth)/2 when the parities differ. Locked in so the
	// migration stays pixel-identical.
	t.Run("integer-division centering keeps the old parity behaviour", func(t *testing.T) {
		t.Parallel()

		lines := strings.Split(centerOverlay(grid(5, 3, "#"), grid(10, 7, ".")), "\n")

		// 10/2 - 5/2 = 3, whereas (10-5)/2 would be 2.
		require.Equal(t, strings.Repeat(".", 3)+"#####"+strings.Repeat(".", 2), lines[2])
	})

	t.Run("keeps the background when the foreground is empty", func(t *testing.T) {
		t.Parallel()

		bg := grid(20, 7, ".")
		require.Equal(t, bg, centerOverlay("", bg))
	})

	t.Run("returns the foreground when the background is empty", func(t *testing.T) {
		t.Parallel()

		fg := grid(4, 3, "#")
		require.Equal(t, fg, centerOverlay(fg, ""))
	})

	t.Run("clips a foreground wider than the background", func(t *testing.T) {
		t.Parallel()

		// The replaced implementation emitted 30 columns here, overflowing the
		// background.
		assertSize(t, centerOverlay(grid(30, 3, "#"), grid(10, 5, ".")), 10, 5)
	})

	t.Run("clips a foreground taller than the background", func(t *testing.T) {
		t.Parallel()

		assertSize(t, centerOverlay(grid(4, 20, "#"), grid(10, 5, ".")), 10, 5)
	})

	t.Run("preserves ansi-styled backgrounds", func(t *testing.T) {
		t.Parallel()

		bg := lipgloss.NewStyle().Faint(true).Render(grid(20, 7, "."))
		got := centerOverlay(grid(4, 3, "#"), bg)

		assertSize(t, got, 20, 7)
		require.Contains(t, got, "####", "foreground must survive compositing")
	})
}

// Test_Root_modalKeepsScreenDimensions guards the wiring in renderView: opening a
// modal must composite over the screen without changing its dimensions, otherwise
// the terminal wraps or the layout jumps.
func Test_Root_modalKeepsScreenDimensions(t *testing.T) {
	deps := &DependencyContainer{
		UserConfig: UserConfiguration{
			Settings: save.BuildDefaultSettings(),
			Theme:    save.BuildDefaultTheme(),
		},
		Keymap:         save.BuildDefaultKeyMap(),
		APIUserClients: map[string]APIClient{},
	}

	for _, tc := range []struct {
		name   string
		screen activeScreen
		marker string
	}{
		{name: "join modal", screen: inputScreen, marker: "Create new Tab"},
		{name: "quick join modal", screen: quickJoinScreen, marker: "Join"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := NewUI(nil, deps, nil)
			root.hasLoadedSession = true
			root.screenType = tc.screen

			background := root.dimmedScreenBackground()
			got := root.renderView()

			require.Equal(t, lipgloss.Width(background), lipgloss.Width(got))
			require.Equal(t, lipgloss.Height(background), lipgloss.Height(got))
			require.Contains(t, stripANSI(got), tc.marker)
		})
	}
}
