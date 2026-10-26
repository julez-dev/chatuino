package tab

import (
	"fmt"
	"strings"
	"time"

	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
)

// redemptionNotice turns a channel point redemption into a chat notice. Add and
// update events share the same rendering so an update can rewrite the original
// line instead of appending a second, stale one.
func (t *Broadcast) redemptionNotice(event eventsub.Event) *twitchirc.Notice {
	text := redemptionText(event)

	return &twitchirc.Notice{
		FakeTimestamp:   time.Now(),
		ChannelUserName: t.channelLogin,
		MsgID:           twitchirc.MsgID(event.ID),
		Message:         text,
	}
}

func (t *Broadcast) trackRedemption(id string, notice *twitchirc.Notice) {
	if t.redemptions == nil {
		t.redemptions = make(map[string]*twitchirc.Notice)
	}

	t.redemptions[id] = notice
}

func redemptionText(event eventsub.Event) string {
	user := event.UserName
	if user == "" {
		user = event.UserLogin
	}

	text := fmt.Sprintf("%s redeemed %q (%d points)", user, event.Reward.Title, event.Reward.Cost)

	if input := strings.TrimSpace(event.UserInput); input != "" {
		text += ": " + input
	} else if input := strings.TrimSpace(event.Message.Text); input != "" {
		text += ": " + input
	}

	if event.Status != "" && event.Status != "unfulfilled" {
		text += fmt.Sprintf(" [%s]", event.Status)
	}

	return text
}

func redemptionStatusTerminal(status string) bool {
	return status == "fulfilled" || status == "canceled"
}
