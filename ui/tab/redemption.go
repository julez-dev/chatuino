package tab

import (
	"fmt"
	"strings"
	"time"

	"github.com/julez-dev/chatuino/twitch/live"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
)

// redemptionNotice turns a channel point redemption into a chat notice. Add and
// update events share the same rendering so an update can rewrite the original
// line instead of appending a second, stale one.
func (t *Broadcast) redemptionNotice(redemption live.Redemption) *twitchirc.Notice {
	return &twitchirc.Notice{
		FakeTimestamp:   time.Now(),
		ChannelUserName: t.channelLogin,
		MsgID:           twitchirc.MsgID(redemption.ID),
		Message:         redemptionText(redemption),
	}
}

func (t *Broadcast) trackRedemption(id string, notice *twitchirc.Notice) {
	if t.redemptions == nil {
		t.redemptions = make(map[string]*twitchirc.Notice)
	}

	t.redemptions[id] = notice
}

func redemptionText(redemption live.Redemption) string {
	text := fmt.Sprintf("%s redeemed %q (%d points)", redemption.User, redemption.Reward, redemption.Cost)

	if input := strings.TrimSpace(redemption.UserInput); input != "" {
		text += ": " + input
	} else if input := strings.TrimSpace(redemption.MessageText); input != "" {
		text += ": " + input
	}

	if redemption.Status != "" && redemption.Status != "unfulfilled" {
		text += fmt.Sprintf(" [%s]", redemption.Status)
	}

	return text
}

func redemptionStatusTerminal(status string) bool {
	return status == "fulfilled" || status == "canceled"
}
