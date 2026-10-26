package tab

import (
	"testing"
	"time"

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

			require.Equal(t, tt.want, stripReplyMention(tt.body, ctx.ParentName, ctx.ParentLogin))
		})
	}
}

// TestStripReplyMention_UnicodeCaseFolding pins rune-safe matching: the Kelvin sign folds
// to "k" but is three bytes long, so a byte-length prefix would cut it in half and miss
// the mention.
func TestStripReplyMention_UnicodeCaseFolding(t *testing.T) {
	t.Parallel()

	const kelvin = "\u212a"

	require.Equal(t, "hello", stripReplyMention("@k hello", kelvin, "kelvin"))
	require.Equal(t, "hello", stripReplyMention("@"+kelvin+" hello", kelvin, "kelvin"))
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
