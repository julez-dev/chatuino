package tab

import tea "charm.land/bubbletea/v2"

// WheelDelta converts a wheel event into a line delta. Positive values scroll
// towards newer content. Tabs and the shell both scroll, so the conversion lives
// next to the interface both of them use.
func WheelDelta(msg tea.MouseWheelMsg) int {
	switch msg.Button {
	case tea.MouseWheelUp:
		return -3
	case tea.MouseWheelDown:
		return 3
	default:
		return 0
	}
}
