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
// exact match of the parent's name followed by a space is dropped; any other mention,
// and a name that is merely a prefix of the mentioned one, is left alone.
func stripReplyMention(body string, ctx replyContext) string {
	for _, name := range []string{ctx.ParentName, ctx.ParentLogin} {
		mention := "@" + name
		// Logins are ASCII and a display name only ever appears here as the very bytes we
		// are comparing, so slicing by byte length cannot split a rune.
		if len(body) <= len(mention) || !strings.EqualFold(body[:len(mention)], mention) {
			continue
		}

		rest := body[len(mention):]
		if rest[0] != ' ' && rest[0] != '\t' {
			continue
		}

		return strings.TrimLeft(rest, " \t")
	}

	return body
}

// handleChatClick resolves the controls that sit on top of the chat rows and reports
// whether the click was consumed by one of them: the new-message badge, then a reply
// reference. Every tab that shows a chat buffer routes its clicks through here, so the
// controls behave the same everywhere.
func (c *chatWindow) handleChatClick(row, col int) bool {
	if c.handleNewMessageBadgeClick(row, col) {
		return true
	}

	return c.handleReplyReferenceClick(c.viewportRenderStart()+row, col)
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

// replyContextRow renders the quoted parent as one row above the reply. The row starts
// in the message's name column and the snippet is cut to what the pane can still show,
// so the reference reads as an annotation of the message below it.
func (r *messageRenderer) replyContextRow(ctx replyContext) string {
	const (
		marker = "↩ "
		sep    = ": "
	)

	// Marks replace the first two cells, so the reference keeps its column: with the
	// selection indicator the row is shifted by exactly the cells the marker frees.
	indent := strings.Repeat(" ", r.timePaddingWidth)

	name := r.replyStyle.Render(ctx.ParentName)
	if render, ok := r.palette.Lookup(strings.ToLower(ctx.ParentLogin)); ok {
		name = render(ctx.ParentName)
	}

	labelWidth := r.timePaddingWidth + lipgloss.Width(marker) + lipgloss.Width(name) + lipgloss.Width(sep)
	snippet := truncateToWidth(ctx.Snippet, r.width-r.indicatorWidth-labelWidth)

	return indent + r.dimmedStyle.Render(marker) + name + r.dimmedStyle.Render(sep+snippet)
}
