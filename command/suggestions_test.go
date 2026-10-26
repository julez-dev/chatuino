package command

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModeratorSuggestionsIncludeNewCommands(t *testing.T) {
	t.Parallel()

	for _, suggestion := range []string{
		"/slow [duration like 30s]", "/slowoff", "/followers [duration like 10m]", "/followersoff",
		"/subscribers", "/subscribersoff", "/emoteonly", "/emoteonlyoff", "/uniquechat", "/uniquechatoff", "/clear",
		"/mod <user>", "/unmod <user>", "/mods", "/vip <user>", "/unvip <user>", "/vips",
		"/warn <user> [reason]", "/raid <channel>", "/unraid", "/shoutout <channel>",
		"/approve <message_id>", "/deny <message_id>", "/snooze", "/unbanrequests",
	} {
		require.Contains(t, ModeratorSuggestions[:], suggestion)
	}
}
