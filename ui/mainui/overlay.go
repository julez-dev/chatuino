package mainui

import (
	"charm.land/lipgloss/v2"
)

// centerOverlay draws fg centered over bg and returns the composited result.
//
// The composited string always has the dimensions of bg: a foreground that is
// larger than the background is clipped rather than allowed to overflow the
// terminal.
//
// Position semantics match the old overlay.Composite(fg, bg, Center, Center, 0, 0)
// call exactly, including the integer-division centering: bgWidth/2-fgWidth/2,
// which differs from (bgWidth-fgWidth)/2 when the widths have different parity.
func centerOverlay(fg, bg string) string {
	if fg == "" {
		return bg
	}
	if bg == "" {
		return fg
	}

	bgWidth, bgHeight := lipgloss.Width(bg), lipgloss.Height(bg)
	fgWidth, fgHeight := lipgloss.Width(fg), lipgloss.Height(fg)

	// Centering by halves mirrors the previous implementation. max(0, …) keeps a
	// too-large foreground anchored at the top-left instead of producing a
	// negative offset, so the canvas below clips it.
	x := max(0, bgWidth/2-fgWidth/2)
	y := max(0, bgHeight/2-fgHeight/2)

	canvas := lipgloss.NewCanvas(bgWidth, bgHeight)
	canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(bg),
		lipgloss.NewLayer(fg).X(x).Y(y),
	))

	return canvas.Render()
}
