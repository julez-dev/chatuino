package mainui

import (
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/ui/tab"
)

// persistedDataLoadedMessage comes when state and user data is loaded
type persistedDataLoadedMessage struct {
	err                error
	ttvUsers           map[string]twitchapi.UserData
	state              save.AppState
	channelSuggestions []string
}

// versionCheckMessage carries the result of the startup version check.
type versionCheckMessage struct {
	info UpdateInfo
	err  error
}

// polledStreamInfoMessage comes when current stream info is refreshed
type polledStreamInfoMessage struct {
	streamInfos []tab.SetStreamInfoMessage
}

// appStateSaveMessage comes when current app state was saved
type appStateSaveMessage struct{}

// imageCleanupTickMessage comes when images should be cleaned up
type imageCleanupTickMessage struct {
	deletionCommand string
}
