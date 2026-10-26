package tab

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/stretchr/testify/require"
)

var replyTestTime = time.Date(2026, 3, 4, 5, 6, 7, 0, time.Local)

// replyMessage builds a reply from carol to Alice's "that was a great play".
func replyMessage(body string) *twitchirc.PrivateMessage {
	return &twitchirc.PrivateMessage{
		ID:                "reply-1",
		DisplayName:       "carol",
		LoginName:         "carol",
		Color:             "#a3be8c",
		Message:           body,
		TMISentTS:         replyTestTime,
		ParentMsgID:       "parent-1",
		ParentUserID:      "user-1",
		ParentUserLogin:   "alice",
		ParentDisplayName: "Alice",
		ParentMsgBody:     "that was a great play",
	}
}

func renderReply(t *testing.T, settings save.Settings, msg *twitchirc.PrivateMessage) []string {
	t.Helper()

	r := newMessageRenderer(60, 0, settings, save.BuildDefaultTheme(), newUserPalette())

	return r.Render(ChatEventMessage{Message: msg})
}

func TestMessageRenderer_ReplyContextRow(t *testing.T) {
	t.Parallel()

	t.Run("quotes the parent above the message", func(t *testing.T) {
		t.Parallel()

		lines := renderReply(t, save.BuildDefaultSettings(), replyMessage("@Alice no it wasn't, watch the vod"))

		require.Len(t, lines, 2, "a reply is the reference row plus the message row")
		require.Equal(t, "↩ Alice: that was a great play", strings.TrimSpace(ansi.Strip(lines[0])))
		require.Contains(t, ansi.Strip(lines[1]), "carol: no it wasn't, watch the vod", "the redundant mention is stripped")
		require.NotContains(t, ansi.Strip(lines[1]), "@Alice")
	})

	t.Run("starts in the message name column", func(t *testing.T) {
		t.Parallel()

		lines := renderReply(t, save.BuildDefaultSettings(), replyMessage("hello"))

		require.True(t, strings.HasPrefix(lines[0], strings.Repeat(" ", 11)), "the reference aligns with the first message column")
		require.NotContains(t, ansi.Strip(lines[0]), "\n")
	})

	t.Run("falls back to the login without a display name", func(t *testing.T) {
		t.Parallel()

		msg := replyMessage("hello")
		msg.ParentDisplayName = ""

		lines := renderReply(t, save.BuildDefaultSettings(), msg)

		require.Equal(t, "↩ alice: that was a great play", strings.TrimSpace(ansi.Strip(lines[0])))
	})

	t.Run("truncates a long parent to the pane", func(t *testing.T) {
		t.Parallel()

		msg := replyMessage("hello")
		msg.ParentMsgBody = strings.Repeat("a long parent message ", 8)

		lines := renderReply(t, save.BuildDefaultSettings(), msg)

		require.Contains(t, ansi.Strip(lines[0]), "…")
		require.LessOrEqual(t, lipgloss.Width(lines[0]), 60, "the reference never exceeds the pane")
	})

	t.Run("normalizes whitespace in the parent", func(t *testing.T) {
		t.Parallel()

		msg := replyMessage("hello")
		msg.ParentMsgBody = "  spaced   out\ntext  "

		lines := renderReply(t, save.BuildDefaultSettings(), msg)

		require.Equal(t, "↩ Alice: spaced out text", strings.TrimSpace(ansi.Strip(lines[0])))
	})

	for _, tt := range []struct {
		name      string
		configure func(msg *twitchirc.PrivateMessage, settings *save.Settings)
	}{
		{
			name: "setting disabled",
			configure: func(_ *twitchirc.PrivateMessage, settings *save.Settings) {
				settings.Chat.ReplyContext = false
			},
		},
		{
			name: "not a reply",
			configure: func(msg *twitchirc.PrivateMessage, _ *save.Settings) {
				msg.ParentMsgID = ""
			},
		},
		{
			name: "parent without a name",
			configure: func(msg *twitchirc.PrivateMessage, _ *save.Settings) {
				msg.ParentDisplayName = ""
				msg.ParentUserLogin = ""
			},
		},
	} {
		t.Run("no reference when "+tt.name, func(t *testing.T) {
			t.Parallel()

			settings := save.BuildDefaultSettings()
			msg := replyMessage("hello")
			tt.configure(msg, &settings)

			lines := renderReply(t, settings, msg)

			require.Len(t, lines, 1, "the message renders as a plain one")
			require.NotContains(t, ansi.Strip(lines[0]), "↩")
		})
	}
}

func TestStripReplyMention(t *testing.T) {
	t.Parallel()

	ctx := replyContext{ParentLogin: "alice", ParentName: "Alice"}

	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{name: "display name", body: "@Alice hello", want: "hello"},
		{name: "login", body: "@alice hello", want: "hello"},
		{name: "separated by a tab", body: "@Alice\thello", want: "hello"},
		{name: "other user stays", body: "@Bob hello", want: "@Bob hello"},
		{name: "prefix of another name stays", body: "@Alice2 hello", want: "@Alice2 hello"},
		{name: "mention only stays", body: "@Alice", want: "@Alice"},
		{name: "mention in the middle stays", body: "hey @Alice hello", want: "hey @Alice hello"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, stripReplyMention(tt.body, ctx))
		})
	}
}

// replyTestWindow returns a window holding a parent message and a reply to it, with the
// reply's entries reachable through the viewport.
func replyTestWindow(t *testing.T, settings save.Settings) (*chatWindow, *chatEntry) {
	t.Helper()

	c := newChatWindow(60, 10, settings, save.BuildDefaultTheme(), save.BuildDefaultKeyMap())

	c.handleMessage(ChatEventMessage{Message: &twitchirc.PrivateMessage{
		ID: "parent-1", DisplayName: "Alice", LoginName: "alice", Color: "#d08770",
		Message: "that was a great play", TMISentTS: replyTestTime,
	}})

	reply := replyMessage("@Alice no it wasn't, watch the vod")
	c.handleMessage(ChatEventMessage{Message: reply})

	entries := c.activeEntries()
	require.Len(t, entries, 2)

	return c, entries[len(entries)-1]
}

func TestChatWindow_ReplyReferenceClick(t *testing.T) {
	t.Parallel()

	t.Run("clicking the reference selects the parent", func(t *testing.T) {
		t.Parallel()

		c, reply := replyTestWindow(t, save.BuildDefaultSettings())
		row := reply.Position.CursorStart - c.viewportRenderStart()

		require.True(t, c.handleChatClick(row, c.renderer.timePaddingWidth))

		_, selected := c.entryForCurrentCursor()
		require.NotNil(t, selected)
		require.Equal(t, "parent-1", selected.Event.Message.(*twitchirc.PrivateMessage).ID)
	})

	t.Run("the indent before the reference still selects the reply", func(t *testing.T) {
		t.Parallel()

		c, reply := replyTestWindow(t, save.BuildDefaultSettings())
		row := reply.Position.CursorStart - c.viewportRenderStart()

		require.False(t, c.handleChatClick(row, c.renderer.timePaddingWidth-1))

		_, selected := c.entryForCurrentCursor()
		require.Equal(t, "reply-1", selected.Event.Message.(*twitchirc.PrivateMessage).ID, "the reply keeps its own rows")
	})

	t.Run("the message row below the reference selects the reply", func(t *testing.T) {
		t.Parallel()

		c, reply := replyTestWindow(t, save.BuildDefaultSettings())
		row := reply.Position.CursorEnd - c.viewportRenderStart()

		require.False(t, c.handleChatClick(row, c.renderer.timePaddingWidth+3))
	})

	t.Run("a parent outside the buffer is not a reference hit", func(t *testing.T) {
		t.Parallel()

		c := newChatWindow(60, 10, save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap())

		reply := replyMessage("hello")
		c.handleMessage(ChatEventMessage{Message: reply})

		row := 0
		require.False(t, c.handleChatClick(row, c.renderer.timePaddingWidth), "nothing to jump to")
	})

	t.Run("no reference rows when the setting is off", func(t *testing.T) {
		t.Parallel()

		settings := save.BuildDefaultSettings()
		settings.Chat.ReplyContext = false

		c, reply := replyTestWindow(t, settings)
		row := reply.Position.CursorStart - c.viewportRenderStart()

		require.False(t, c.handleChatClick(row, c.renderer.timePaddingWidth))
		require.Equal(t, reply.Position.CursorStart, reply.Position.CursorEnd, "a reply is a single row")
	})
}

// TestChatWindow_ReplyContextKeepsItsRow pins that the reference row belongs to the reply
// entry, so selection, wrapping and cleanup treat it as part of the message.
func TestChatWindow_ReplyContextKeepsItsRow(t *testing.T) {
	t.Parallel()

	c, reply := replyTestWindow(t, save.BuildDefaultSettings())

	require.Equal(t, reply.Position.CursorStart+1, reply.Position.CursorEnd)

	rows := viewRows(t, c)
	reference := rows[reply.Position.CursorStart-c.viewportRenderStart()]

	require.Contains(t, ansi.Strip(reference), "↩ Alice: that was a great play")
	require.Contains(t, ansi.Strip(rows[reply.Position.CursorEnd-c.viewportRenderStart()]), fmt.Sprintf("%s: no it wasn't", "carol"))
}
