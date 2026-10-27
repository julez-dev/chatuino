package tab

import (
	"context"
	"testing"

	"github.com/julez-dev/chatuino/save/messagelog"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/stretchr/testify/require"
)

type testEmoteReplacer struct{}

func (testEmoteReplacer) Replace(context.Context, string, string, []twitchirc.Emote) (string, map[string]string, error) {
	return "", map[string]string{"hi": "[hi]"}, nil
}

type testBadgeReplacer struct {
	injected bool
}

func (r *testBadgeReplacer) Replace(context.Context, string, []twitchirc.Badge) (string, map[string]string, error) {
	return "", map[string]string{"moderator": "[mod]"}, nil
}

func (r *testBadgeReplacer) InjectContributorBadge(_ context.Context, _ string, badges map[string]string) (string, error) {
	r.injected = true
	badges["!chatuino"] = "[chatuino]"

	return "prepare", nil
}

func TestUserInspect_PrepareLoggedMessageInjectsContributorBadge(t *testing.T) {
	t.Parallel()

	badges := &testBadgeReplacer{}
	inspect := &userInspect{
		emoteReplacer: testEmoteReplacer{},
		badgeReplacer: badges,
	}

	event, prepare := inspect.prepareLoggedMessage(context.Background(), "channel-id", messagelog.LogEntry{
		PrivateMessage: &twitchirc.PrivateMessage{
			LoginName: "alice",
			Message:   "hi",
		},
	})

	require.True(t, badges.injected, "history must run the same contributor badge path as live chat")
	require.Equal(t, "[chatuino]", event.DisplayModifier.BadgeReplacement["!chatuino"])
	require.Equal(t, "[mod]", event.DisplayModifier.BadgeReplacement["moderator"])
	require.Equal(t, "[hi]", event.DisplayModifier.WordReplacements["hi"])
	require.Equal(t, "prepare", prepare)
}
