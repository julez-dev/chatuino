package chat

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/stretchr/testify/require"
)

// newTestRenderer builds a renderer without constructing a chat window, which is
// the point of the extraction: formatting can be exercised on its own.
func newTestRenderer(t *testing.T, width int) *messageRenderer {
	t.Helper()

	return newTestRendererWith(t, width, save.BuildDefaultSettings())
}

// newTestRendererWith is newTestRenderer for a renderer that needs non-default settings.
func newTestRendererWith(t *testing.T, width int, settings save.Settings) *messageRenderer {
	t.Helper()

	return newMessageRenderer(width, lipgloss.Width("> "), settings, save.BuildDefaultTheme(), NewPalette())
}

// testChatError is an error that can travel as a chat event, mirroring the shell's
// connection error type.
type testChatError string

func (e testChatError) Error() string { return string(e) }
func (e testChatError) IRC() string   { return string(e) }

func plain(lines []string) string {
	return StripANSI(strings.Join(lines, "\n"))
}

func Test_messageRenderer_EventKinds(t *testing.T) {
	t.Parallel()

	sentAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.Local)
	userName := "bob"
	banDuration := 600
	userID := "1234"

	tests := []struct {
		name   string
		event  ChatEventMessage
		expect []string
	}{
		{
			name: "private message",
			event: ChatEventMessage{Message: &twitchirc.PrivateMessage{
				DisplayName: "Alice",
				LoginName:   "alice",
				Message:     "hello chat",
				TMISentTS:   sentAt,
			}},
			expect: []string{"05:06:07", "Alice", "hello chat"},
		},
		{
			name: "notice",
			event: ChatEventMessage{Message: &twitchirc.Notice{
				FakeTimestamp: sentAt,
				Message:       "loaded recent messages",
			}},
			expect: []string{"[Notice]", "loaded recent messages"},
		},
		{
			name: "fake notice",
			event: ChatEventMessage{IsFakeEvent: true, Message: &twitchirc.Notice{
				FakeTimestamp: sentAt,
				Message:       "loaded recent messages",
			}},
			expect: []string{"[Fake-Notice]"},
		},
		{
			name:   "error",
			event:  ChatEventMessage{Message: testChatError("connection lost\nretrying")},
			expect: []string{"[Error]", "connection lostretrying"},
		},
		{
			name: "subscription",
			event: ChatEventMessage{Message: &twitchirc.SubMessage{
				UserNotice:       twitchirc.UserNotice{DisplayName: "Alice", Login: "alice", TMISentTS: sentAt, MsgID: "resub"},
				SubPlan:          twitchirc.Prime,
				CumulativeMonths: 7,
				StreakMonths:     3,
			}},
			expect: []string{"[Sub Alert]", "resubscribed", "7 Months", "3 Month Streak"},
		},
		{
			name: "gift subscription",
			event: ChatEventMessage{Message: &twitchirc.SubGiftMessage{
				UserNotice:         twitchirc.UserNotice{DisplayName: "Alice", Login: "alice", TMISentTS: sentAt},
				SubPlan:            twitchirc.Tier1,
				ReceiptDisplayName: "Bob",
				Months:             1,
			}},
			expect: []string{"[Sub Gift Alert]", "gifted a", "Bob"},
		},
		{
			name: "announcement keeps the author in the body",
			event: ChatEventMessage{Message: &twitchirc.AnnouncementMessage{
				UserNotice: twitchirc.UserNotice{DisplayName: "Alice", Login: "alice", TMISentTS: sentAt},
				ParamColor: twitchirc.Primary,
				Message:    "hello chat",
			}},
			expect: []string{"[Announcement]", "Alice: hello chat"},
		},
		{
			name:   "clear chat prevented",
			event:  ChatEventMessage{Message: &twitchirc.ClearChat{TMISentTS: sentAt}},
			expect: []string{"[Clear Chat]", "Clear chat prevented by Chatuino"},
		},
		{
			name: "timeout",
			event: ChatEventMessage{Message: &twitchirc.ClearChat{
				UserName:     &userName,
				TargetUserID: &userID,
				TMISentTS:    sentAt,
				BanDuration:  &banDuration,
			}},
			expect: []string{"[Clear Chat]", "bob was timed out for 10 minutes"},
		},
		{
			name: "message removed",
			event: ChatEventMessage{Message: &twitchirc.ClearMessage{
				Login:     "bob",
				TMISentTS: sentAt,
			}},
			expect: []string{"[Clear Message]", "A message from bob was removed."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := newTestRenderer(t, 200)
			lines := r.Render(tt.event)
			require.NotEmpty(t, lines)

			text := plain(lines)
			for _, want := range tt.expect {
				require.Contains(t, text, want)
			}
		})
	}
}

func Test_messageRenderer_Badges(t *testing.T) {
	t.Parallel()

	event := func() ChatEventMessage {
		return ChatEventMessage{
			Message: &twitchirc.PrivateMessage{
				DisplayName: "Alice",
				LoginName:   "alice",
				Message:     "hello",
			},
			DisplayModifier: MessageContentModifier{
				BadgeReplacement: WordReplacement{"subscriber": "[S]", "moderator": "[M]"},
			},
		}
	}

	t.Run("text badges are bracketed and ordered", func(t *testing.T) {
		t.Parallel()

		lines := newTestRenderer(t, 80).Render(event())
		require.Contains(t, plain(lines), "[[M],[S]] Alice: hello")
	})

	t.Run("badges can be disabled", func(t *testing.T) {
		t.Parallel()

		r := newTestRenderer(t, 80)
		r.settings.Chat.DisableBadges = true

		require.NotContains(t, plain(r.Render(event())), "[M,S]")
	})

	t.Run("graphic badges are concatenated", func(t *testing.T) {
		t.Parallel()

		r := newTestRenderer(t, 80)
		r.settings.Chat.GraphicBadges = true

		require.Contains(t, plain(r.Render(event())), "[M][S] Alice: hello")
	})
}

func Test_messageRenderer_DeletionStyling(t *testing.T) {
	t.Parallel()

	r := newTestRenderer(t, 80)
	lines := r.Render(ChatEventMessage{
		Message: &twitchirc.PrivateMessage{
			DisplayName: "Alice",
			LoginName:   "alice",
			Message:     "deleted text",
		},
		DisplayModifier: MessageContentModifier{Strikethrough: true},
	})

	require.Contains(t, strings.Join(lines, ""), "\x1b[9m", "deleted messages are rendered struck through")
}

func Test_messageRenderer_DuplicateBypassIsStripped(t *testing.T) {
	t.Parallel()

	r := newTestRenderer(t, 80)
	lines := r.Render(ChatEventMessage{
		Message: &twitchirc.PrivateMessage{
			DisplayName: "Alice",
			LoginName:   "alice",
			Message:     "hello " + string(DuplicateBypass) + "world",
		},
	})

	require.Contains(t, plain(lines), "hello world")
	require.NotContains(t, plain(lines), string(DuplicateBypass))
}

func Test_messageRenderer_WrapsToWidth(t *testing.T) {
	t.Parallel()

	settings := save.BuildDefaultSettings()

	tests := []struct {
		name     string
		settings func(*save.Settings)
		content  string
	}{
		{name: "ascii words", content: strings.Repeat("word ", 40)},
		{name: "unicode", content: strings.Repeat("日本語テキスト ", 20)},
		{name: "unbreakable url", content: "https://example.com/" + strings.Repeat("a", 120)},
		{name: "ansi colored", content: "\x1b[31m" + strings.Repeat("colored ", 30) + "\x1b[0m"},
		{
			name:     "unpadded wrapped lines",
			settings: func(s *save.Settings) { s.Chat.DisablePaddingWrappedLines = true },
			content:  strings.Repeat("word ", 40),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := settings
			if tt.settings != nil {
				tt.settings(&s)
			}

			r := newMessageRenderer(40, 2, s, save.BuildDefaultTheme(), NewPalette())
			lines := r.Render(ChatEventMessage{Message: &twitchirc.PrivateMessage{
				DisplayName: "Alice",
				LoginName:   "alice",
				Message:     tt.content,
			}})

			require.Greater(t, len(lines), 1, "the message should wrap")

			for _, line := range lines {
				require.LessOrEqual(t, lipgloss.Width(line), 40, "line exceeds the renderer width: %q", line)
			}
		})
	}
}

// Test_messageRenderer_TimeFormatDrivesDerivedWidths pins the invariant that the
// padding widths cannot drift from the configured timestamp format.
func Test_messageRenderer_TimeFormatDrivesDerivedWidths(t *testing.T) {
	t.Parallel()

	r := newTestRenderer(t, 80)
	require.Equal(t, len("15:04:05"), r.timeFormatWidth)
	require.Equal(t, len("15:04:05")+3, r.timePaddingWidth)

	r.SetTimeFormat("2006-01-02 15:04:05")

	require.Equal(t, len("2006-01-02 15:04:05"), r.timeFormatWidth)
	require.Equal(t, len("2006-01-02 15:04:05")+3, r.timePaddingWidth)

	// The user inspect window uses its own format; the wrapped lines are padded
	// with the recalculated width.
	r.settings.Chat.DisablePaddingWrappedLines = true
	r.SetWidth(60)

	sentAt := time.Date(2026, 3, 4, 5, 6, 7, 0, time.Local)
	lines := r.Render(ChatEventMessage{Message: &twitchirc.PrivateMessage{
		DisplayName: "Alice",
		LoginName:   "alice",
		TMISentTS:   sentAt,
		Message:     strings.Repeat("word ", 30),
	}})

	require.Contains(t, plain(lines), "2026-03-04 05:06:07")
	require.True(t, strings.HasPrefix(plain([]string{lines[1]}), strings.Repeat(" ", r.timePaddingWidth)),
		"continuation lines are padded by the derived timestamp width")
}

func Test_messageRenderer_GraphicRows(t *testing.T) {
	t.Parallel()

	settings := save.BuildDefaultSettings()
	settings.Chat.GraphicEmotes = true
	settings.Chat.GraphicScale = 2

	r := newMessageRenderer(80, 2, settings, save.BuildDefaultTheme(), NewPalette())

	// A line containing a kitty image placeholder expands to the graphic scale.
	image := "\x1b[38;2;0;0;1m\U0010EEEE\u030D\u0305\U0010EEEE\x1b[39m"
	lines := r.Render(ChatEventMessage{Message: &twitchirc.PrivateMessage{
		DisplayName: "Alice",
		LoginName:   "alice",
		Message:     "hi " + image,
	}})

	require.Len(t, lines, 2)
}

func Test_messageRenderer_DoesNotMutateEventData(t *testing.T) {
	t.Parallel()

	palette := NewPalette()
	palette.Renderer("bob", "#ff0000")

	r := newMessageRenderer(80, 2, save.BuildDefaultSettings(), save.BuildDefaultTheme(), palette)

	replacements := WordReplacement{"Kappa": "[k]"}
	event := ChatEventMessage{
		Message: &twitchirc.PrivateMessage{
			DisplayName: "Alice",
			LoginName:   "alice",
			Message:     "Kappa bob",
		},
		DisplayModifier: MessageContentModifier{WordReplacements: replacements},
	}

	lines := r.Render(event)
	require.Contains(t, plain(lines), "[k]")
	require.Contains(t, plain(lines), "bob")

	require.Equal(t, WordReplacement{"Kappa": "[k]"}, replacements,
		"rendering must not write the derived user colors into the event's map")
}

// Test_messageRenderer_UniformStyleIsAppliedPerWrappedLine covers a deleted message
// that wraps on a long link: the strike-through must cover every wrapped line
// without spilling into the next one, where it drew a line across the indentation.
func Test_messageRenderer_UniformStyleIsAppliedPerWrappedLine(t *testing.T) {
	t.Parallel()

	r := newTestRenderer(t, 80)
	event := ChatEventMessage{Message: &twitchirc.PrivateMessage{
		RoomID:          "room-1",
		ChannelUserName: "chan",
		DisplayName:     "xTom_AFGx",
		LoginName:       "xtom_afgx",
		Message:         "Heiko geht richtig ab https://www.tiktok.com/@offstreamde/video/77007159974999297302?is_from_webapp=1&sender_device=pc",
		TMISentTS:       time.Date(2026, 3, 4, 18, 1, 6, 0, time.Local),
	}}
	event.DisplayModifier.Strikethrough = true

	lines := r.Render(event)
	require.Len(t, lines, 3)

	bodies := strings.Split(plain(lines), "\n")
	require.Contains(t, bodies[0], "xTom_AFGx: Heiko geht richtig ab")
	require.True(t, strings.HasPrefix(bodies[1], "                      https://"), "wrapped lines keep their indent")

	indent := len(bodies[1]) - len(strings.TrimLeft(bodies[1], " "))
	require.NotZero(t, indent)

	for i, line := range lines {
		require.Contains(t, line, "\x1b[9m", "line %d carries the strike-through", i)
		require.False(t, strings.HasSuffix(line, "\x1b[9m"), "line %d leaves the strike-through open", i)
	}

	for _, line := range lines[1:] {
		require.Equal(t, indent, strings.Index(line, "\x1b"), "the indentation itself is never styled")
	}
}

// renderReplyEvent renders one event and hands back the renderer, so a test can assert
// against the same width budget the renderer itself used.
func renderReplyEvent(t *testing.T, settings save.Settings, event ChatEventMessage) (*messageRenderer, []string) {
	t.Helper()

	r := newTestRendererWith(t, 60, settings)

	return r, r.Render(event)
}

func renderReply(t *testing.T, settings save.Settings, msg *twitchirc.PrivateMessage) []string {
	t.Helper()

	_, lines := renderReplyEvent(t, settings, ChatEventMessage{Message: msg})

	return lines
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

	t.Run("a long parent name cannot overflow the pane", func(t *testing.T) {
		t.Parallel()

		msg := replyMessage("hello")
		msg.ParentDisplayName = strings.Repeat("LongName", 10)

		r, lines := renderReplyEvent(t, save.BuildDefaultSettings(), ChatEventMessage{Message: msg})

		require.LessOrEqual(t, lipgloss.Width(lines[0]), 60-r.indicatorWidth, "the whole reference stays inside the pane")
		require.Contains(t, ansi.Strip(lines[0]), "…", "the name is cut, not the row")
		require.NotContains(t, ansi.Strip(lines[0]), ": ", "no dangling separator when no room is left for the quote")
	})

	t.Run("a struck reply strikes its quote", func(t *testing.T) {
		t.Parallel()

		msg := replyMessage("hello")

		_, plainLines := renderReplyEvent(t, save.BuildDefaultSettings(), ChatEventMessage{Message: msg})
		_, struckLines := renderReplyEvent(t, save.BuildDefaultSettings(), ChatEventMessage{
			Message:         msg,
			DisplayModifier: MessageContentModifier{Strikethrough: true},
		})

		require.Equal(t, ansi.Strip(plainLines[0]), ansi.Strip(struckLines[0]), "the reference reads the same")
		require.NotEqual(t, plainLines[0], struckLines[0], "but carries the attribute")
		require.Contains(t, struckLines[0], ";9m", "the row carries the strikethrough attribute")
		require.True(t, strings.HasPrefix(struckLines[0], "  "), "the indent stays unstyled, so the selection indicator keeps its columns")
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

func TestApplyWordReplacements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		content      string
		replacements WordReplacement
		want         string
	}{
		{
			name:         "empty replacements returns content unchanged",
			content:      "moon pool",
			replacements: WordReplacement{},
			want:         "moon pool",
		},
		{
			name:         "single-char emote token does not bleed into words",
			content:      "o moon pool",
			replacements: WordReplacement{"o": "[emote]"},
			want:         "[emote] moon pool",
		},
		{
			name:         "no standalone token leaves words untouched",
			content:      "moon pool",
			replacements: WordReplacement{"o": "[emote]"},
			want:         "moon pool",
		},
		{
			name:         "repeated standalone tokens all replaced",
			content:      "o moon o pool o",
			replacements: WordReplacement{"o": "[emote]"},
			want:         "[emote] moon [emote] pool [emote]",
		},
		{
			name:         "multiple distinct emotes replaced per token",
			content:      "Kappa and PogChamp",
			replacements: WordReplacement{"Kappa": "[k]", "PogChamp": "[p]"},
			want:         "[k] and [p]",
		},
		{
			name:         "substring match inside larger word is not replaced",
			content:      "Kappaccino Kappa",
			replacements: WordReplacement{"Kappa": "[k]"},
			want:         "Kappaccino [k]",
		},
		{
			// link producer keys on the whole token, so trailing punctuation is
			// part of the key and is preserved in the annotated value.
			name:         "url token with trailing punctuation matches whole token",
			content:      "see https://x.com! end",
			replacements: WordReplacement{"https://x.com!": "https://x.com [200 OK]!"},
			want:         "see https://x.com [200 OK]! end",
		},
		{
			name:         "url token wrapped in parens matches whole token",
			content:      "(https://x.com) done",
			replacements: WordReplacement{"(https://x.com)": "(https://x.com [200 OK])"},
			want:         "(https://x.com [200 OK]) done",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := &messageRenderer{}
			got := r.applyWordReplacements(tt.content, tt.replacements)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestExpandGraphicRows(t *testing.T) {
	t.Parallel()

	// bottom row (row 1) of a two row image
	image := "\x1b[38;2;0;0;1m\U0010EEEE\u030D\u0305\U0010EEEE\x1b[39m"
	lines := []string{"  12:00 user: hi " + image, "              plain text"}

	t.Run("scale 1 is noop", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, lines, expandGraphicRows(lines, 1))
	})

	t.Run("scale 2 adds rows above lines with images only, images bottom aligned", func(t *testing.T) {
		t.Parallel()
		got := expandGraphicRows(lines, 2)
		require.Equal(t, []string{
			"                 \x1b[38;2;0;0;1m\U0010EEEE\u0305\u0305\U0010EEEE\x1b[39m",
			lines[0],
			lines[1],
		}, got)
	})
}

func TestGraphicRows(t *testing.T) {
	t.Parallel()

	s := save.Settings{Chat: save.ChatSettings{GraphicScale: 2}}
	require.Equal(t, 1, graphicRows(s), "graphics disabled")

	s.Chat.GraphicEmotes = true
	require.Equal(t, 2, graphicRows(s))

	s.Chat.GraphicScale = 0
	require.Equal(t, 1, graphicRows(s), "unset defaults to 1")
}
