package tab

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// replyTarget is the message the user armed a reply to. It is rendered above the
// message input and consumed by the next send attempt.
type replyTarget struct {
	MessageID   string
	DisplayName string
	Snippet     string
}

// replyState tracks the reply the user armed for the next send attempt and renders
// the indicator shown above the message input.
type replyState struct {
	target *replyTarget
}

// Arm targets the given message for the next send attempt. The message content is
// normalized into a single-line snippet for the indicator.
func (r *replyState) Arm(messageID, displayName, message string) {
	r.target = &replyTarget{
		MessageID:   messageID,
		DisplayName: displayName,
		Snippet:     replySnippet(message),
	}
}

// Armed reports whether a reply is currently armed.
func (r *replyState) Armed() bool {
	return r.target != nil
}

// Take returns the armed reply's message ID and disarms the reply, so it can only
// ever be used by a single send attempt. It returns "" when no reply is armed.
func (r *replyState) Take() string {
	if r.target == nil {
		return ""
	}

	id := r.target.MessageID
	r.target = nil

	return id
}

// Clear disarms the armed reply. It reports whether a reply was armed, so the
// caller can recalculate the layout that the indicator row occupied.
func (r *replyState) Clear() bool {
	if r.target == nil {
		return false
	}

	r.target = nil

	return true
}

// View renders the indicator bar that shows which message the next send will reply
// to, padded to width. It renders nothing when no reply is armed.
func (r *replyState) View(width int, accent, dimmed lipgloss.Style) string {
	if r.target == nil {
		return ""
	}

	const hint = "(esc to cancel)"

	label := " ↩ Replying to @" + r.target.DisplayName + ": "
	snippet := truncateToWidth(r.target.Snippet, width-lipgloss.Width(label)-lipgloss.Width(hint)-1)
	gap := max(1, width-lipgloss.Width(label)-lipgloss.Width(snippet)-lipgloss.Width(hint))

	return accent.Render(label) + dimmed.Render(snippet) + strings.Repeat(" ", gap) + dimmed.Render(hint)
}

// takeReplyTarget returns the armed reply's message ID and disarms the reply. The
// layout is recalculated because the indicator row is given back to the chat window.
func (t *Broadcast) takeReplyTarget() string {
	id := t.input.TakeReply()
	if id != "" {
		t.HandleResize()
	}

	return id
}

// clearReplyTarget disarms the armed reply. The reply indicator occupies a row above
// the message input, so the layout is recalculated here: otherwise the chat window
// keeps the height it had while the indicator was visible and the view ends up a row
// short, floating the status bar above the bottom of the terminal.
func (t *Broadcast) clearReplyTarget() {
	// A tab has no composer before its channel data loaded, and therefore no reply
	// to disarm. Guarding here keeps every caller safe rather than only the ones
	// that remember to check.
	if t.input == nil {
		return
	}

	if t.input.ClearReply() {
		t.HandleResize()
	}
}

// replySnippet normalizes a chat message into a single line for the reply indicator.
func replySnippet(message string) string {
	message = strings.ReplaceAll(message, string(duplicateBypass), "")

	return strings.Join(strings.Fields(message), " ")
}

// truncateToWidth shortens s so that it renders in at most width cells, appending
// an ellipsis when it had to cut.
func truncateToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}

	if lipgloss.Width(s) <= width {
		return s
	}

	var b strings.Builder
	used := 0

	for _, r := range s {
		rw := lipgloss.Width(string(r))

		if used+rw > width-1 {
			break
		}

		b.WriteRune(r)
		used += rw
	}

	return b.String() + "…"
}
