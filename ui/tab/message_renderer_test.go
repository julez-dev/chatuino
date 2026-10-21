package tab

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/stretchr/testify/require"
)

// newTestRenderer builds a renderer without constructing a chat window, which is
// the point of the extraction: formatting can be exercised on its own.
func newTestRenderer(t *testing.T, width int) *messageRenderer {
	t.Helper()

	return newMessageRenderer(width, lipgloss.Width("> "), save.BuildDefaultSettings(), save.BuildDefaultTheme(), newUserPalette())
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
			Message:     "hello " + string(duplicateBypass) + "world",
		},
	})

	require.Contains(t, plain(lines), "hello world")
	require.NotContains(t, plain(lines), string(duplicateBypass))
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

			r := newMessageRenderer(40, 2, s, save.BuildDefaultTheme(), newUserPalette())
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

	r := newMessageRenderer(80, 2, settings, save.BuildDefaultTheme(), newUserPalette())

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

	palette := newUserPalette()
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

func TestUserPalette(t *testing.T) {
	t.Parallel()

	p := newUserPalette()

	render, ok := p.Lookup("alice")
	require.False(t, ok)
	require.Nil(t, render)

	first := p.Renderer("alice", "#123456")
	second := p.Renderer("alice", "#abcdef")
	require.NotNil(t, first)

	require.Equal(t, fmt.Sprintf("%p", first), fmt.Sprintf("%p", second), "the cached renderer is reused")

	p.Renderer("bob", "")

	require.ElementsMatch(t, []string{"alice", "bob"}, p.Names())

	p.Prune(map[string]struct{}{"alice": {}})
	require.ElementsMatch(t, []string{"alice"}, p.Names())
}
