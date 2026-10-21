package tab

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

// chatLayout returns the row offset of the main chat window and its height
// within the tab view. It mirrors the render order in Broadcast.View.
func (t *Broadcast) chatLayout() (top, height int) {
	top = 1 // stream info always occupies at least one row

	if si := t.streamInfo.View(); si != "" {
		top = lipgloss.Height(si) + 1
	}

	if poll := t.poll.View(); poll != "" {
		top += lipgloss.Height(poll) + 1
	}

	return top, t.chatWindow.height
}

// userInspectLayout returns the row offset and height of the user inspect
// window within the tab view. It is only valid in user inspect modes.
func (t *Broadcast) userInspectLayout() (top, height int) {
	chatTop, chatHeight := t.chatLayout()
	top = chatTop + chatHeight + 1 // newline before the inspect window

	return top, lipgloss.Height(t.userInspect.View())
}

// inputLayout returns the row offset and height of the message composer.
func (t *Broadcast) inputLayout() (top, height int) {
	chatTop, chatHeight := t.chatLayout()
	top = chatTop + chatHeight + 1

	if t.state.isInspecting() {
		top += lipgloss.Height(t.userInspect.View()) + 1
	}

	return top, t.input.Height()
}

func (t *Broadcast) HandleMouseClick(msg tea.MouseClickMsg, x, y int) tea.Cmd {
	if msg.Button != tea.MouseLeft || !t.channelDataLoaded || t.err != nil {
		return nil
	}

	if t.state == emoteOverviewMode {
		return nil
	}

	chatTop, chatHeight := t.chatLayout()

	// Main chat window: select the clicked message.
	if y >= chatTop && y < chatTop+chatHeight {
		// Resolve the absolute line before any layout change, then leave
		// insert/inspect mode so subsequent shortcuts act on the main chat.
		line := t.chatWindow.viewportRenderStart() + (y - chatTop)
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

	// User inspect window: select the clicked message.
	if t.state.isInspecting() {
		uiTop, uiHeight := t.userInspectLayout()
		if y >= uiTop && y < uiTop+uiHeight {
			if t.state == userInspectInsertMode {
				t.handleEscapePressed()
			}

			row := y - uiTop
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
	if !t.account.IsAnonymous {
		inputTop, inputHeight := t.inputLayout()
		if y >= inputTop && y < inputTop+inputHeight {
			return t.handleStartInsertMode()
		}
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

	chatTop, chatHeight := t.chatLayout()
	if y >= chatTop && y < chatTop+chatHeight {
		t.chatWindow.scrollByLines(delta)
		t.chatWindow.snapScroll()
		return nil
	}

	if t.state.isInspecting() {
		uiTop, uiHeight := t.userInspectLayout()
		if y >= uiTop && y < uiTop+uiHeight {
			t.userInspect.chatWindow.scrollByLines(delta)
			t.userInspect.chatWindow.snapScroll()
		}
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
