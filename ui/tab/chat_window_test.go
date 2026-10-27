package tab

import (
	"fmt"
	"strings"
	"testing"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/chat"
	"github.com/stretchr/testify/require"
)

func renderedText(c *chatWindow) string {
	rows, _ := c.Rows(c.height)
	return strings.Join(rows, "\n")
}

func TestChatWindow_SearchShowsHintUntilThreeCharacters(t *testing.T) {
	t.Parallel()

	c := newChatWindow(80, 10, save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap())
	for _, content := range []string{"alpha", "beta target", "gamma"} {
		c.Append(chat.ChatEventMessage{Message: &twitchirc.PrivateMessage{
			DisplayName: "Alice",
			LoginName:   "alice",
			Message:     content,
		}})
	}

	c.handleStartSearchMode()

	c.searchInput.SetValue("a")
	c.applySearch()

	require.Equal(t, "type at least 3 characters", c.Buffer.FilterStatus().Hint)
	require.False(t, c.Buffer.IsFiltering(), "a short query must leave the transcript unfiltered")
	require.Contains(t, c.View(), "type at least 3 characters")
	require.Contains(t, renderedText(c), "alpha")

	c.searchInput.SetValue("beta")
	c.applySearch()

	require.True(t, c.Buffer.IsFiltering())
	require.Equal(t, 1, c.Buffer.FilterStatus().Matches)
	require.Contains(t, c.View(), "1 matches")
	require.NotContains(t, renderedText(c), "gamma")

	c.handleStopSearchMode()

	require.False(t, c.Buffer.IsFiltering())
	require.Contains(t, renderedText(c), "gamma")
}

func TestChatWindow_SearchStartKeepsSelection(t *testing.T) {
	t.Parallel()

	c := newChatWindow(80, 10, save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap())
	for i := range 10 {
		c.Append(chat.ChatEventMessage{Message: &twitchirc.PrivateMessage{
			DisplayName: "Alice",
			LoginName:   "alice",
			Message:     fmt.Sprintf("message %d", i),
		}})
	}

	c.PageUp()
	before, ok := c.Selected()
	require.True(t, ok)

	c.handleStartSearchMode()

	after, ok := c.Selected()
	require.True(t, ok)
	require.Equal(t, before.ID, after.ID, "opening search must not jump to the newest message")
}
