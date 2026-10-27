package tab

import (
	"testing"

	"github.com/julez-dev/chatuino/save"
	"github.com/stretchr/testify/require"
)

func TestLiveNotification_AppendKeepsSmoothScrollCommand(t *testing.T) {
	t.Parallel()

	settings := save.BuildDefaultSettings()
	settings.Chat.SmoothScroll = true

	tab := NewLiveNotification("live", 80, 22, settings, save.BuildDefaultTheme(), save.BuildDefaultKeyMap())

	_, _ = tab.Update(SetStreamInfoMessage{Target: "chan", Username: "Alice", IsLive: true})

	_, cmd := tab.Update(SetStreamInfoMessage{Target: "chan", Username: "Alice", IsLive: false})
	require.NotNil(t, cmd, "the transcript's smooth-scroll command must reach the shell")
}
