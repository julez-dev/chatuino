package mainui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/ui/tab"
)

// mouseAware is implemented by tabs that understand cell-relative mouse input.
// Coordinates passed to these methods are relative to the tab's own view.
type mouseAware interface {
	HandleMouseClick(msg tea.MouseClickMsg, x, y int) tea.Cmd
	HandleMouseWheel(msg tea.MouseWheelMsg, x, y int) tea.Cmd
}

// hoverAware is implemented by tabs that react to the pointer moving over them.
// Coordinates are relative to the tab's own view.
type hoverAware interface {
	HandleMouseMotion(x, y int)
	HandleMouseLeave()
}

// emoteTooltipEnabled reports whether pointer motion needs to be tracked.
func (r *Root) emoteTooltipEnabled() bool {
	return r.settings.Mouse && r.settings.Chat.GraphicEmotes && !r.settings.Chat.DisableEmoteTooltip
}

// handleMouseMotion forwards pointer motion to the tab under the pointer and
// tells the previously hovered tab when the pointer left it.
func (r *Root) handleMouseMotion(msg tea.MouseMotionMsg) tea.Cmd {
	if !r.emoteTooltipEnabled() {
		return nil
	}

	var (
		id   string
		x, y int
	)

	if r.hasLoadedSession && r.stack.Len() == 0 && msg.X >= 0 && msg.Y >= 0 && msg.X < r.width && msg.Y < r.height {
		if r.usesWorkspace() {
			if paneID, px, py, content := r.paneMouseTarget(msg.X, msg.Y); paneID != "" && content {
				id, x, y = paneID, px, py
			}
		} else if lx, ly, inTab := r.tabLocalCoords(msg.X, msg.Y); inTab {
			if t := r.activeTab(); t != nil {
				id, x, y = t.ID(), lx, ly
			}
		}
	}

	if r.hoverTabID != "" && r.hoverTabID != id {
		if h, ok := r.tabByID(r.hoverTabID).(hoverAware); ok {
			h.HandleMouseLeave()
		}
	}
	r.hoverTabID = id

	if id != "" {
		if h, ok := r.tabByID(id).(hoverAware); ok {
			h.HandleMouseMotion(x, y)
		}
	}

	return nil
}

// activeTab returns the currently focused tab, or nil when none exists.
func (r *Root) activeTab() tab.Tab {
	t, _ := r.tabs.Active()
	return t
}

// tabLocalCoords translates screen coordinates into coordinates relative to the
// active tab's view. inTab is false when the cell belongs to the tab header.
func (r *Root) tabLocalCoords(x, y int) (localX, localY int, inTab bool) {
	if r.settings.VerticalTabList {
		headerWidth := lipgloss.Width(r.header.View(r.visibleRows()))
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

// switchToTab activates a tab from either header. It never assigns a tab to a
// pane: the active tab's owning workspace, when any, becomes live automatically.
func (r *Root) switchToTab(id string) tea.Cmd {
	r.navigateToTab(id)
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

	if t := r.activeTab(); t != nil && (t.IsComposing() || t.IsSearching()) {
		return true
	}

	return false
}

// handleMouseClick routes a left click to the tab header or the active tab.
func (r *Root) handleMouseClick(msg tea.MouseClickMsg) tea.Cmd {
	// Modals do not take mouse clicks; they are keyboard driven.
	if !r.settings.Mouse || msg.Button != tea.MouseLeft || !r.hasLoadedSession || r.stack.Len() > 0 ||
		msg.X < 0 || msg.Y < 0 || msg.X >= r.width || msg.Y >= r.height {
		return nil
	}

	if r.usesWorkspace() {
		if id, x, y, content := r.paneMouseTarget(msg.X, msg.Y); id != "" {
			r.switchToTab(id)
			if aware, ok := r.tabByID(id).(mouseAware); ok && content {
				return aware.HandleMouseClick(msg, x, y)
			}
			return nil
		}
		bounds := r.workspaceBounds()
		inHeader := (!r.settings.VerticalTabList && msg.Y < bounds.Y) ||
			(r.settings.VerticalTabList && msg.X < bounds.X && msg.Y < bounds.Height)
		if inHeader {
			if id, ok := r.header.TabAt(r.visibleRows(), msg.X, msg.Y); ok {
				return r.switchToTab(id)
			}
		}
		return nil
	}

	localX, localY, inTab := r.tabLocalCoords(msg.X, msg.Y)
	if !inTab {
		if id, ok := r.header.TabAt(r.visibleRows(), msg.X, msg.Y); ok {
			return r.switchToTab(id)
		}

		return nil
	}

	tab, ok := r.activeTab().(mouseAware)
	if !ok {
		return nil
	}

	return tab.HandleMouseClick(msg, localX, localY)
}

// handleMouseWheel routes a wheel event to the help viewport, the vertical tab
// list, or the active tab.
func (r *Root) handleMouseWheel(msg tea.MouseWheelMsg) tea.Cmd {
	if !r.settings.Mouse || !r.hasLoadedSession || msg.X < 0 || msg.Y < 0 || msg.X >= r.width || msg.Y >= r.height {
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

	if r.usesWorkspace() {
		if id, x, y, content := r.paneMouseTarget(msg.X, msg.Y); id != "" {
			if aware, ok := r.tabByID(id).(mouseAware); ok && content {
				return aware.HandleMouseWheel(msg, x, y)
			}
			return nil
		}
		bounds := r.workspaceBounds()
		if r.settings.VerticalTabList && msg.X >= 0 && msg.X < bounds.X && msg.Y >= 0 && msg.Y < bounds.Height {
			switch tab.WheelDelta(msg) {
			case -3:
				r.prevTab()
			case 3:
				r.nextTab()
			}
		}
		return nil
	}

	localX, localY, inTab := r.tabLocalCoords(msg.X, msg.Y)
	if !inTab {
		// Bubbles' list has no wheel support, so wheel over the vertical tab
		// list moves the active tab instead.
		if r.settings.VerticalTabList {
			switch tab.WheelDelta(msg) {
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

	return tab.HandleMouseWheel(msg, localX, localY)
}
