package tab

import (
	"strings"
	"testing"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/chat"
	"github.com/stretchr/testify/require"
)

func TestChatWindow_SearchShowsHintUntilThreeCharacters(t *testing.T) {
	t.Parallel()

	c := newChatWindow(80, 10, save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap())
	for _, content := range []string{"alpha", "beta target", "gamma"} {
		c.handleMessage(chat.ChatEventMessage{Message: &twitchirc.PrivateMessage{
			DisplayName: "Alice",
			LoginName:   "alice",
			Message:     content,
		}})
	}

	c.handleStartSearchMode()

	c.searchInput.SetValue("a")
	c.applySearch()

	require.Equal(t, "type at least 3 characters", c.buffer.FilterStatus().Hint)
	require.False(t, c.buffer.IsFiltering(), "a short query must leave the transcript unfiltered")
	require.Contains(t, c.View(), "type at least 3 characters")
	require.Contains(t, strings.Join(c.RenderedRows(), "\n"), "alpha")

	c.searchInput.SetValue("beta")
	c.applySearch()

	require.True(t, c.buffer.IsFiltering())
	require.Equal(t, 1, c.buffer.FilterStatus().Matches)
	require.Contains(t, c.View(), "1 matches")
	require.NotContains(t, strings.Join(c.RenderedRows(), "\n"), "gamma")

	c.handleStopSearchMode()

	require.False(t, c.buffer.IsFiltering())
	require.Contains(t, strings.Join(c.RenderedRows(), "\n"), "gamma")
}
