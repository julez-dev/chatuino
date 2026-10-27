package tab

import (
	"strings"
	"testing"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/chat"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

// testNotice builds a fake chat notice for transcript tests.
func testNotice(message string) chat.ChatEventMessage {
	return chat.ChatEventMessage{
		AccountID: "acc",
		ChannelID: "chan-id",
		Message: &twitchirc.Notice{
			MsgID:   twitchirc.MsgID("notice-" + message),
			Message: message,
		},
	}
}

func draculaTheme(t *testing.T) save.Theme {
	t.Helper()

	manager, err := save.NewThemeManager(afero.NewMemMapFs(), t.TempDir(), "dracula")
	require.NoError(t, err)
	require.Empty(t, manager.Warnings())

	return manager.Active()
}

// A theme switch must repaint the chat transcript and leave everything else —
// draft, scroll position, filters — exactly as it was.
func TestBroadcast_ThemeChangedRepaintsAndKeepsState(t *testing.T) {
	t.Parallel()

	tab := newFSMTestTab(t, true)
	tab.SetSize(60, 20)
	tab.HandleResize()

	tab.chatWindow.Append(testNotice("hello"))
	tab.input.SetValue("unsent draft")

	selectedBefore, ok := tab.chatWindow.Selected()
	require.True(t, ok)

	visible, _ := tab.chatWindow.Rows(10)
	rowsBefore := append([]string(nil), visible...)

	theme := draculaTheme(t)
	_, _ = tab.Update(ThemeChangedMessage{Theme: theme})

	require.Equal(t, theme, tab.theme)
	require.Equal(t, "unsent draft", tab.input.Value(), "the composer draft survives the switch")

	selectedAfter, ok := tab.chatWindow.Selected()
	require.True(t, ok)
	require.Equal(t, selectedBefore.ID, selectedAfter.ID, "the selection survives the switch")

	reRendered, _ := tab.chatWindow.Rows(10)
	require.NotEqual(t, rowsBefore, append([]string(nil), reRendered...), "rows are re-rendered")
	require.Contains(t, strings.Join(reRendered, "\n"), "98;114;164", "rows use dracula's dimmed color (#6272a4)")
}

func TestMention_ThemeChangedRepaintsTranscript(t *testing.T) {
	t.Parallel()

	mention := NewMention("m-1", 60, 20, save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap(), nil)
	mention.chatWindow.Append(testNotice("hello"))

	visible, _ := mention.chatWindow.Rows(10)
	rowsBefore := append([]string(nil), visible...)

	theme := draculaTheme(t)
	_, _ = mention.Update(ThemeChangedMessage{Theme: theme})

	require.Equal(t, theme, mention.theme)

	reRendered, _ := mention.chatWindow.Rows(10)
	require.NotEqual(t, rowsBefore, append([]string(nil), reRendered...), "rows are re-rendered")
}

func TestLiveNotification_ThemeChangedRepaintsTranscript(t *testing.T) {
	t.Parallel()

	live := NewLiveNotification("l-1", 60, 20, save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap())
	live.chatWindow.Append(testNotice("hello"))

	visible, _ := live.chatWindow.Rows(10)
	rowsBefore := append([]string(nil), visible...)

	theme := draculaTheme(t)
	_, _ = live.Update(ThemeChangedMessage{Theme: theme})

	require.Equal(t, theme, live.theme)

	reRendered, _ := live.chatWindow.Rows(10)
	require.NotEqual(t, rowsBefore, append([]string(nil), reRendered...), "rows are re-rendered")
}

func TestComposer_SetThemeRepaintsFrame(t *testing.T) {
	t.Parallel()

	composer := newTestComposer(t, 40)
	composer.SetValue("hello")

	defaultView := composer.View()
	require.NotContains(t, defaultView, "189;147;249", "the default prompt color is not dracula's")

	composer.SetTheme(draculaTheme(t))

	require.Equal(t, "hello", composer.Value(), "the draft survives")
	require.Contains(t, composer.View(), "189;147;249", "the frame uses the new input prompt color")
}
