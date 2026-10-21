package tab

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// doubleClickWindow is how long two clicks on the same message may be apart to
// count as a double-click. Terminals only report individual presses, so the
// gesture is synthesized here.
const doubleClickWindow = 400 * time.Millisecond

// registerClick records the clicked entry and reports whether it forms a
// double-click with the previous click. Clicks outside the entry's line range
// (for example on the blank padding below the last message) reset the tracker.
func (t *Broadcast) registerClick(entry *chatEntry, line int) bool {
	if entry == nil || line < entry.Position.CursorStart || line > entry.Position.CursorEnd {
		t.lastClickEntry = nil
		t.lastClickAt = time.Time{}
		return false
	}

	now := time.Now()
	double := entry == t.lastClickEntry && now.Sub(t.lastClickAt) <= doubleClickWindow

	t.lastClickEntry = entry
	t.lastClickAt = now

	return double
}

// inspectChatSpan is the clickable history area inside the user inspect window,
// in tab-local rows. The window's own span also covers the info banner.
func (t *Broadcast) inspectChatSpan() layoutSpan {
	chat := t.userInspect.chatSpan()
	chat.top += t.layout.userInspect.top

	return chat
}

func (t *Broadcast) HandleMouseClick(msg tea.MouseClickMsg, x, y int) tea.Cmd {
	if msg.Button != tea.MouseLeft || !t.channelDataLoaded || t.err != nil {
		return nil
	}

	if t.state == emoteOverviewMode {
		return nil
	}

	// Main chat window: select the clicked message.
	if t.layout.chat.contains(y) {
		// Resolve the absolute line before any layout change, then leave
		// insert/inspect mode so subsequent shortcuts act on the main chat.
		line := t.chatWindow.viewportRenderStart() + (y - t.layout.chat.top)
		_, entry := t.chatWindow.entryAtLine(line)
		double := t.registerClick(entry, line)

		switch t.state {
		case insertMode:
			t.handleEscapePressed()
		case userInspectMode, userInspectInsertMode:
			t.leaveUserInspect()
		}

		t.chatWindow.selectLine(line)

		if double {
			return t.handleOpenUserInspectFromMessage()
		}

		return nil
	}

	// User inspect window: select the clicked message. Only the history below
	// the info banner is interactive.
	if t.state.isInspecting() && t.layout.userInspect.visible() {
		if chat := t.inspectChatSpan(); chat.contains(y) {
			if t.state == userInspectInsertMode {
				t.handleEscapePressed()
			}

			row := y - chat.top
			line := t.userInspect.chatWindow.viewportRenderStart() + row
			_, entry := t.userInspect.chatWindow.entryAtLine(line)
			double := t.registerClick(entry, line)

			t.userInspect.chatWindow.selectAtViewportRow(row)

			if double {
				return t.handleOpenUserInspectFromMessage()
			}

			return nil
		}
	}

	// Message composer: click to start typing.
	if !t.account.IsAnonymous && t.layout.composer.contains(y) {
		return t.handleStartInsertMode()
	}

	return nil
}

func (t *Broadcast) HandleMouseWheel(msg tea.MouseWheelMsg, x, y int) tea.Cmd {
	if !t.channelDataLoaded || t.err != nil {
		return nil
	}

	delta := WheelDelta(msg)
	if delta == 0 {
		return nil
	}

	if t.state == emoteOverviewMode {
		if t.emoteOverview == nil {
			return nil
		}

		var cmd tea.Cmd
		t.emoteOverview, cmd = t.emoteOverview.Update(msg)
		return cmd
	}

	if t.layout.chat.contains(y) {
		t.chatWindow.scrollByLines(delta)
		t.chatWindow.snapScroll()
		return nil
	}

	if t.state.isInspecting() && t.layout.userInspect.visible() && t.inspectChatSpan().contains(y) {
		t.userInspect.chatWindow.scrollByLines(delta)
		t.userInspect.chatWindow.snapScroll()
	}

	return nil
}

// mentionTab and liveNotificationTab render only a chat window, so their local
// mouse coordinates map directly onto it.

func (m *mentionTab) HandleMouseClick(msg tea.MouseClickMsg, x, y int) tea.Cmd {
	if msg.Button == tea.MouseLeft {
		m.chatWindow.selectAtViewportRow(y)
	}

	return nil
}

func (m *mentionTab) HandleMouseWheel(msg tea.MouseWheelMsg, x, y int) tea.Cmd {
	if delta := WheelDelta(msg); delta != 0 {
		m.chatWindow.scrollByLines(delta)
		m.chatWindow.snapScroll()
	}

	return nil
}

func (l *liveNotificationTab) HandleMouseClick(msg tea.MouseClickMsg, x, y int) tea.Cmd {
	if msg.Button == tea.MouseLeft {
		l.chatWindow.selectAtViewportRow(y)
	}

	return nil
}

func (l *liveNotificationTab) HandleMouseWheel(msg tea.MouseWheelMsg, x, y int) tea.Cmd {
	if delta := WheelDelta(msg); delta != 0 {
		l.chatWindow.scrollByLines(delta)
		l.chatWindow.snapScroll()
	}

	return nil
}
