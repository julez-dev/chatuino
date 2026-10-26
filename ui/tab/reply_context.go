package tab

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
)

// replyContext is the message a reply answers, derived from the reply tags Twitch
// attaches to the replying message. Deriving it from the tags instead of looking the
// parent up keeps the display stateless: it needs no history buffer, it survives the
// parent scrolling out of the window, and replayed history renders the same way.
type replyContext struct {
	// ParentID is the message ID of the parent, used to find it in the buffer.
	ParentID string
	// ParentLogin is the parent's login, the key into the user palette.
	ParentLogin string
	// ParentName is what the reference shows, the display name when Twitch sent one.
	ParentName string
	// Snippet is the parent's text, whitespace-normalized and not yet truncated.
	Snippet string
}

const (
	replyMarker = "↩ "
	replySep    = ": "
)

// replyContextFor returns the quoted parent of a reply. It returns false for messages
// that are not replies and when the setting hides the reference, so rendering and click
// handling agree on whether a context row exists.
func (r *messageRenderer) replyContextFor(msg *twitchirc.PrivateMessage) (replyContext, bool) {
	if msg == nil || msg.ParentMsgID == "" || !r.settings.Chat.ReplyContext {
		return replyContext{}, false
	}

	name := msg.ParentDisplayName
	if name == "" {
		name = msg.ParentUserLogin
	}

	// Without a name there is nothing to attribute the quote to, so the message is
	// rendered as a plain one instead of quoting an anonymous parent.
	if name == "" {
		return replyContext{}, false
	}

	return replyContext{
		ParentID:    msg.ParentMsgID,
		ParentLogin: msg.ParentUserLogin,
		ParentName:  name,
		Snippet:     replySnippet(msg.ParentMsgBody),
	}, true
}

// stripReplyMention removes the "@parent " that Twitch's own client writes into the text
// of a reply, so the reference and the body do not name the same user twice. Only an
// exact match of the parent's display name or login, followed by a space, is dropped;
// any other mention, and a name that is merely a prefix of the mentioned one, stays.
func stripReplyMention(body, parentName, parentLogin string) string {
	bodyRunes := []rune(body)

	for _, name := range []string{parentName, parentLogin} {
		mention := []rune("@" + name)
		if name == "" || len(bodyRunes) <= len(mention) {
			continue
		}

		// Compare runes, not bytes: case folding can map runes of different byte lengths
		// onto each other, so a byte prefix could split one in half.
		if !strings.EqualFold(string(bodyRunes[:len(mention)]), string(mention)) {
			continue
		}

		rest := bodyRunes[len(mention):]
		if rest[0] != ' ' && rest[0] != '\t' {
			continue
		}

		return strings.TrimLeft(string(rest), " \t")
	}

	return body
}

// handleReplyReferenceClick selects the parent message when the click lands on a reply's
// reference row and the parent is still in the buffer, and reports whether it did. The
// indent in front of the reference still belongs to the reply itself, so a click there
// selects the reply like any other message row.
func (c *chatWindow) handleReplyReferenceClick(line, col int) bool {
	if col < c.renderer.timePaddingWidth {
		return false
	}

	_, entry := c.entryAtLine(line)
	if entry == nil || line != entry.Position.CursorStart {
		return false
	}

	msg, ok := entry.Event.Message.(*twitchirc.PrivateMessage)
	if !ok {
		return false
	}

	ctx, ok := c.renderer.replyContextFor(msg)
	if !ok {
		return false
	}

	for _, candidate := range c.activeEntries() {
		if parent, ok := candidate.Event.Message.(*twitchirc.PrivateMessage); ok && parent.ID == ctx.ParentID {
			c.goToEntry(candidate)

			return true
		}
	}

	return false
}

// replyContextRow renders the quoted parent as one row above the reply. The row starts in
// the message's name column, and name and quote share the width the pane has left, so a
// long display name cannot push the reference past the pane edge.
func (r *messageRenderer) replyContextRow(ctx replyContext, modifier MessageContentModifier) string {
	indent := strings.Repeat(" ", r.timePaddingWidth)
	budget := r.width - r.indicatorWidth - r.timePaddingWidth - lipgloss.Width(replyMarker) - lipgloss.Width(replySep)

	name := truncateToWidth(ctx.ParentName, min(lipgloss.Width(ctx.ParentName), budget))
	snippet := truncateToWidth(ctx.Snippet, max(0, budget-lipgloss.Width(name)))

	// A struck or italic reply carries the attribute into its quote as well: a clean
	// reference above a struck message reads like a rendering fault. The parent's color
	// is dropped in that case, since a per-user color cannot carry the attribute.
	if modifier.Strikethrough || modifier.Italic {
		// StrikethroughSpaces(false) matches how the message body is struck: the gap in
		// "carol: text" keeps its color instead of being lined through.
		style := r.dimmedStyle.
			Strikethrough(modifier.Strikethrough).
			StrikethroughSpaces(false).
			Italic(modifier.Italic)

		// The indent stays unstyled: the selection indicator replaces the two leading
		// spaces, and a row that starts with an escape sequence would shift by two cells.
		return indent + style.Render(replyMarker+name+replySep+snippet)
	}

	if render, ok := r.palette.Lookup(strings.ToLower(ctx.ParentLogin)); ok {
		name = render(name)
	} else {
		// The parent is not in the palette yet: the accent marks it as a reference.
		name = r.replyStyle.Render(name)
	}

	if snippet == "" {
		return indent + r.dimmedStyle.Render(replyMarker) + name
	}

	return indent + r.dimmedStyle.Render(replyMarker) + name + r.dimmedStyle.Render(replySep+snippet)
}
