package tab

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// View renders the tab including the status bar.
func (t *Broadcast) View() string {
	return t.render(true)
}

// ViewWithoutStatusBar returns the view without the status bar (for vertical tab
// mode, where Root renders the status bar at full terminal width).
func (t *Broadcast) ViewWithoutStatusBar() string {
	return t.render(false)
}

// render is the single rendering path of a broadcast tab. Region placement comes
// from the layout computed by HandleResize, so rendering, child sizing and
// hit-testing cannot drift apart.
func (t *Broadcast) render(includeStatus bool) string {
	if t.err != nil {
		return t.centeredStyle.
			Width(t.width).Height(t.height).MaxWidth(t.width).MaxHeight(t.height).
			Render(t.err.Error())
	}

	if !t.channelDataLoaded {
		return t.centeredStyle.
			Width(t.width).Height(t.height).MaxWidth(t.width).MaxHeight(t.height).
			Render(t.spinner.View() + " Loading channel")
	}

	// In emote overview mode the overview replaces the whole content area and
	// only the status bar is appended.
	if t.state == emoteOverviewMode {
		var blocks []string
		if t.layout.emoteOverview.visible() {
			blocks = append(blocks, t.emoteOverview.View())
		}
		if includeStatus && t.layout.status.visible() {
			blocks = append(blocks, t.statusInfo.View())
		}
		return strings.Join(blocks, "\n")
	}

	blocks := make([]string, 0, 6)

	// The stream info slot keeps its row even when the channel is offline, so
	// the chat does not jump when the info appears.
	blocks = append(blocks, t.streamInfo.View())

	if t.layout.poll.visible() {
		blocks = append(blocks, t.poll.View())
	}

	if t.layout.chat.visible() {
		blocks = append(blocks, t.chatWindow.View())
	}

	if t.layout.userInspect.visible() && t.userInspect != nil {
		blocks = append(blocks, t.userInspect.View())
	}

	if t.layout.composer.visible() {
		blocks = append(blocks, t.input.View())
	}

	if includeStatus && t.layout.status.visible() {
		blocks = append(blocks, t.statusInfo.View())
	}

	return strings.Join(blocks, "\n")
}

// StatusBarView returns just the status bar view (for vertical tab mode)
func (t *Broadcast) StatusBarView() string {
	if !t.channelDataLoaded || t.statusInfo == nil {
		return ""
	}
	return t.statusInfo.View()
}

// renderMessageInput renders the message composer. Anonymous accounts cannot
// send messages, so nothing is rendered for them.
func (t *Broadcast) renderMessageInput() string {
	if t.account.IsAnonymous {
		return ""
	}

	return t.input.View()
}

// HandleResize sizes the children and records the layout the view and the mouse
// handlers use. Widths are applied first because the composer and the stream
// info wrap, and their height depends on the width they are rendered at.
func (t *Broadcast) HandleResize() {
	if !t.channelDataLoaded {
		return
	}

	t.applyWidths()

	var statusView string
	if t.fullWidth == 0 {
		// In vertical tab mode Root renders the status bar below the tab, so it
		// does not take rows away from the chat.
		statusView = t.statusInfo.View()
	}

	in := layoutInputs{
		height:        t.height,
		streamInfo:    lipgloss.Height(t.streamInfo.View()),
		poll:          viewHeight(t.poll.View()),
		status:        viewHeight(statusView),
		inspecting:    t.state.isInspecting(),
		emoteOverview: t.state == emoteOverviewMode,
	}
	if !t.account.IsAnonymous {
		in.composer = t.input.Height()
	}

	t.layout = allocateBroadcastLayout(in)

	t.chatWindow.Resize(t.width, t.layout.chat.height)

	if t.layout.userInspect.visible() {
		t.userInspect.width = t.width
		t.userInspect.height = t.layout.userInspect.height
		t.userInspect.handleResize()
	}

	if t.state == emoteOverviewMode && t.emoteOverview != nil {
		t.emoteOverview.resize(t.width, t.layout.emoteOverview.height)
	}
}

// applyWidths propagates the tab width to the children that wrap their content.
func (t *Broadcast) applyWidths() {
	if t.fullWidth > 0 {
		t.statusInfo.width = t.fullWidth
	} else {
		t.statusInfo.width = t.width
	}

	t.streamInfo.width = t.width
	t.streamInfo.cachedDirty = true
	t.poll.setWidth(t.width)

	// Set the input width before measuring, so its height reflects wrapping at
	// the tab width.
	t.input.SetWidth(t.width)
}
