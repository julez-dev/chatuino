package tab

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/rs/zerolog/log"
)

func (t *Broadcast) View() string {
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

	builder := strings.Builder{}

	// In Emote Overview Mode only render emote overview + status info
	if t.state == emoteOverviewMode {
		builder.WriteString(t.emoteOverview.View())
		statusInfo := t.statusInfo.View()
		if statusInfo != "" {
			builder.WriteString("\n")
			builder.WriteString(statusInfo)
		}

		return builder.String()
	}

	// Render Order:
	// Stream Info
	// Poll
	// Chat Window
	// User Inspect Window (if in user inspect mode)
	// Message Input
	// Status Info

	si := t.streamInfo.View()
	if si != "" {
		builder.WriteString(si)
		builder.WriteString("\n")
	} else {
		builder.WriteString("\n")
	}

	pollView := t.poll.View()
	if pollView != "" {
		builder.WriteString(pollView)
		builder.WriteString("\n")
	}

	cw := t.chatWindow.View()
	builder.WriteString(cw)

	if t.state.isInspecting() {
		uiView := t.userInspect.View()
		builder.WriteString("\n")
		builder.WriteString(uiView)
	}

	mi := t.renderMessageInput()
	if mi != "" {
		builder.WriteString("\n")
		builder.WriteString(mi)
	}

	statusInfo := t.statusInfo.View()
	if statusInfo != "" {
		builder.WriteString("\n")
		builder.WriteString(statusInfo)
	}

	return builder.String()
}

// ViewWithoutStatusBar returns the view without the status bar (for vertical tab mode)
func (t *Broadcast) ViewWithoutStatusBar() string {
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

	builder := strings.Builder{}

	// In Emote Overview Mode only render emote overview (status bar rendered separately)
	if t.state == emoteOverviewMode {
		builder.WriteString(t.emoteOverview.View())
		return builder.String()
	}

	// Render Order (without status bar):
	// Stream Info
	// Poll
	// Chat Window
	// User Inspect Window (if in user inspect mode)
	// Message Input

	si := t.streamInfo.View()
	if si != "" {
		builder.WriteString(si)
		builder.WriteString("\n")
	} else {
		builder.WriteString("\n")
	}

	pollView := t.poll.View()
	if pollView != "" {
		builder.WriteString(pollView)
		builder.WriteString("\n")
	}

	cw := t.chatWindow.View()
	builder.WriteString(cw)

	if t.state.isInspecting() {
		uiView := t.userInspect.View()
		builder.WriteString("\n")
		builder.WriteString(uiView)
	}

	mi := t.renderMessageInput()
	if mi != "" {
		builder.WriteString("\n")
		builder.WriteString(mi)
	}

	return builder.String()
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

func (t *Broadcast) HandleResize() {
	if t.channelDataLoaded {
		// Use fullWidth for status bar if set (vertical tab mode), otherwise use tab width
		if t.fullWidth > 0 {
			t.statusInfo.width = t.fullWidth
		} else {
			t.statusInfo.width = t.width
		}
		t.streamInfo.width = t.width
		t.streamInfo.cachedDirty = true
		t.poll.setWidth(t.width)

		// Set input width BEFORE measuring to ensure correct wrapping
		t.input.SetWidth(t.width)

		var heightMessageInput int
		if !t.account.IsAnonymous {
			heightMessageInput = t.input.Height()
		}

		// In vertical mode (fullWidth > 0), status bar is rendered at root level, so don't count its height
		var heightStatusInfo int
		if t.fullWidth == 0 {
			statusInfo := t.statusInfo.View()
			heightStatusInfo = lipgloss.Height(statusInfo)
			if statusInfo == "" {
				heightStatusInfo = 0
			}
		}

		streamInfo := t.streamInfo.View()
		heightStreamInfo := lipgloss.Height(streamInfo)
		if streamInfo == "" {
			heightStreamInfo = 1
		}

		pollView := t.poll.View()
		pollHeight := lipgloss.Height(pollView)
		if pollView == "" {
			pollHeight = 0
		}

		if t.state.isInspecting() {
			chatHeight := (t.height - heightStreamInfo - pollHeight - heightStatusInfo) / 2

			t.userInspect.height = t.height - heightStreamInfo - pollHeight - chatHeight - heightStatusInfo - heightMessageInput
			t.userInspect.width = t.width
			t.userInspect.handleResize()
			t.chatWindow.Resize(t.width, chatHeight)
		} else {
			chatHeight := t.height - pollHeight - heightMessageInput - heightStreamInfo - heightStatusInfo
			if chatHeight < 0 {
				chatHeight = 0
			}

			log.Logger.Info().Int("chatHeight", chatHeight).Int("height", t.height).Int("heightStreamInfo", heightStreamInfo).Int("heightStatusInfo", heightStatusInfo).Msg("handleResize")

			t.chatWindow.Resize(t.width, chatHeight)
		}

		if t.state == emoteOverviewMode {
			log.Logger.Info().Int("width", t.width).Int("height", t.height-heightStatusInfo).Msg("resize emoteOverview")
			t.emoteOverview.resize(t.width, t.height-heightStatusInfo)
		}
	}
}
