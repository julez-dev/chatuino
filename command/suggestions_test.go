package command

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModeratorSuggestionsIncludeNewCommands(t *testing.T) {
	t.Parallel()

	joined := strings.Join(ModeratorSuggestions[:], "\n")
	for _, command := range []string{
		"/slow", "/slowoff", "/followers", "/followersoff", "/subscribers", "/subscribersoff",
		"/emoteonly", "/emoteonlyoff", "/uniquechat", "/uniquechatoff", "/clear",
		"/mod ", "/unmod ", "/mods", "/vip ", "/unvip ", "/vips",
		"/warn ", "/raid ", "/unraid", "/shoutout ", "/approve ", "/deny ", "/snooze",
	} {
		require.Contains(t, joined, command)
	}
}
