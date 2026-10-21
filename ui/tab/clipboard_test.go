package tab

import (
	"testing"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/stretchr/testify/require"
)

func TestChatWindow_ClipboardText(t *testing.T) {
	t.Parallel()

	c := newChatWindow(40, 10, save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap())

	user := "bob"
	banDuration := 600

	tests := []struct {
		name  string
		entry *chatEntry
		want  string
	}{
		{
			name:  "private message strips the duplicate bypass rune",
			entry: &chatEntry{Event: ChatEventMessage{Message: &twitchirc.PrivateMessage{Message: "hello " + string(duplicateBypass) + "world"}}},
			want:  "hello world",
		},
		{
			name:  "notice",
			entry: &chatEntry{Event: ChatEventMessage{Message: &twitchirc.Notice{Message: "notice text"}}},
			want:  "notice text",
		},
		{
			name:  "cleared message",
			entry: &chatEntry{Event: ChatEventMessage{Message: &twitchirc.ClearMessage{Login: "bob"}}},
			want:  "A message from bob was removed.",
		},
		{
			name:  "timeout",
			entry: &chatEntry{Event: ChatEventMessage{Message: &twitchirc.ClearChat{UserName: &user, BanDuration: &banDuration}}},
			want:  "bob was timed out for 10 minutes",
		},
		{
			name:  "announcement includes the author",
			entry: &chatEntry{Event: ChatEventMessage{Message: &twitchirc.AnnouncementMessage{UserNotice: twitchirc.UserNotice{DisplayName: "alice"}, Message: "hello chat"}}},
			want:  "alice: hello chat",
		},
		{
			name:  "nil entry",
			entry: nil,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, c.clipboardText(tt.entry))
		})
	}
}
