package mainui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// mouseAware is implemented by tabs that understand cell-relative mouse input.
// Coordinates passed to these methods are relative to the tab's own view.
type mouseAware interface {
	handleMouseClick(msg tea.MouseClickMsg, x, y int) tea.Cmd
	handleMouseWheel(msg tea.MouseWheelMsg, x, y int) tea.Cmd
}

// wheelDelta converts a wheel event into a line delta. Positive values scroll
// towards newer content.
func wheelDelta(msg tea.MouseWheelMsg) int {
	switch msg.Button {
	case tea.MouseWheelUp:
		return -3
	case tea.MouseWheelDown:
		return 3
	default:
		return 0
	}
}

// activeTab returns the currently focused tab, or nil when none exists.
func (r *Root) activeTab() tab {
	tab, _ := r.tabs.Active()
	return tab
}

// tabLocalCoords translates screen coordinates into coordinates relative to the
// active tab's view. inTab is false when the cell belongs to the tab header.
func (r *Root) tabLocalCoords(x, y int) (localX, localY int, inTab bool) {
	if r.dependencies.UserConfig.Settings.VerticalTabList {
		headerWidth := lipgloss.Width(r.header.View(r.tabs.Rows()))
		if x < headerWidth {
			return 0, 0, false
		}

		return x - headerWidth, y, true
	}

	headerHeight := r.getHeaderHeight()
	if y < headerHeight {
		return 0, 0, false
	}

	return x, y - headerHeight, true
}

// switchToTab focuses the tab with the given id.
func (r *Root) switchToTab(id string) tea.Cmd {
	// SetActive blurs the previous tab, focuses the new one and drops its
	// notification flag; unknown ids are ignored.
	r.tabs.SetActive(id)

	return nil
}

// isTyping reports whether a text input (composer, modal, or chat search) is
// currently capturing printable characters.
func (r *Root) isTyping() bool {
	// A modal owns the keyboard while it is open, so it is the only thing that
	// can be capturing text.
	if top, ok := r.stack.Top(); ok {
		capturer, ok := top.(textCapturer)
		return ok && capturer.CapturesText()
	}

	if t := r.activeTab(); t != nil {
		state := t.State()
		if state == insertMode || state == userInspectInsertMode || t.IsSearching() {
			return true
		}
	}

	return false
}

// handleMouseClick routes a left click to the tab header or the active tab.
func (r *Root) handleMouseClick(msg tea.MouseClickMsg) tea.Cmd {
	// Modals do not take mouse clicks; they are keyboard driven.
	if msg.Button != tea.MouseLeft || !r.hasLoadedSession || r.stack.Len() > 0 {
		return nil
	}

	localX, localY, inTab := r.tabLocalCoords(msg.X, msg.Y)
	if !inTab {
		if id, ok := r.header.TabAt(r.tabs.Rows(), msg.X, msg.Y); ok {
			return r.switchToTab(id)
		}

		return nil
	}

	tab, ok := r.activeTab().(mouseAware)
	if !ok {
		return nil
	}

	return tab.handleMouseClick(msg, localX, localY)
}

// handleMouseWheel routes a wheel event to the help viewport, the vertical tab
// list, or the active tab.
func (r *Root) handleMouseWheel(msg tea.MouseWheelMsg) tea.Cmd {
	if !r.hasLoadedSession {
		return nil
	}

	if r.stack.Len() > 0 {
		// Only screens that declare wheel scrolling consume the event; the rest
		// swallow it so nothing scrolls behind the modal.
		top, _ := r.stack.Top()
		if _, scrolls := top.(wheelScroller); scrolls {
			return r.stack.Update(msg)
		}

		return nil
	}

	localX, localY, inTab := r.tabLocalCoords(msg.X, msg.Y)
	if !inTab {
		// Bubbles' list has no wheel support, so wheel over the vertical tab
		// list moves the active tab instead.
		if r.dependencies.UserConfig.Settings.VerticalTabList {
			switch wheelDelta(msg) {
			case -3:
				r.prevTab()
			case 3:
				r.nextTab()
			}
		}

		return nil
	}

	tab, ok := r.activeTab().(mouseAware)
	if !ok {
		return nil
	}

	return tab.handleMouseWheel(msg, localX, localY)
}
