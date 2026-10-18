package mainui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/rs/zerolog/log"
)

func (t *broadcastTab) View() string {
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

	if t.state == userInspectMode || t.state == userInspectInsertMode {
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
func (t *broadcastTab) ViewWithoutStatusBar() string {
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

	if t.state == userInspectMode || t.state == userInspectInsertMode {
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
func (t *broadcastTab) StatusBarView() string {
	if !t.channelDataLoaded || t.statusInfo == nil {
		return ""
	}
	return t.statusInfo.View()
}

func (t *broadcastTab) renderMessageInput() string {
	if t.account.IsAnonymous {
		return ""
	}

	inputView := t.messageInput.View()
	borderStyle := t.inputBorderStyle

	// Labels
	topLabel := "[ Chat ]"
	charCount := fmt.Sprintf("[ %d / %d ]", utf8.RuneCountInString(t.messageInput.InputModel.Value()), t.messageInput.InputModel.CharLimit)

	innerWidth := t.width - 2 // -2 for left/right border chars

	// Top border: ┌─[ Chat ]─────...─┐
	topFill := innerWidth - len(topLabel) - 2
	topBorder := "┌─" + topLabel + strings.Repeat("─", topFill) + "─┐"

	// Bottom border: └─────...─[ 7 / 500 ]─┘ (counter on RIGHT)
	bottomFill := innerWidth - len(charCount) - 2
	bottomBorder := "└─" + strings.Repeat("─", bottomFill) + charCount + "─┘"

	// Wrap input lines with │ borders
	inputLines := strings.Split(inputView, "\n")
	var borderedLines []string
	for _, line := range inputLines {
		padNeeded := max(0, innerWidth-lipgloss.Width(line))
		borderedLines = append(borderedLines, "│"+line+strings.Repeat(" ", padNeeded)+"│")
	}

	// Combine
	result := ""

	if indicator := t.renderReplyIndicator(); indicator != "" {
		result += indicator + "\n"
	}

	result += borderStyle.Render(topBorder) + "\n"
	result += borderStyle.Render(strings.Join(borderedLines, "\n")) + "\n"
	result += borderStyle.Render(bottomBorder)

	return result
}

// renderReplyIndicator renders the bar above the message input that shows which
// message the next send will reply to.
func (t *broadcastTab) renderReplyIndicator() string {
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(t.deps.UserConfig.Theme.InputPromptColor))
	dimmed := lipgloss.NewStyle().Foreground(lipgloss.Color(t.deps.UserConfig.Theme.DimmedTextColor))

	return t.reply.View(t.width, accent, dimmed)
}

func (t *broadcastTab) HandleResize() {
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

		// Set messageInput width BEFORE rendering to ensure correct wrapping
		// -2 for left/right │ border chars
		t.messageInput.SetWidth(t.width - 2)

		messageInput := t.renderMessageInput()
		heightMessageInput := lipgloss.Height(messageInput)

		if messageInput == "" {
			heightMessageInput = 0
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

		if t.state == userInspectMode || t.state == userInspectInsertMode {
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
