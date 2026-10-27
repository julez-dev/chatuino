package live

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWatch_BroadcasterGetsTheFullChannelSet(t *testing.T) {
	t.Parallel()

	subscriptions := Watch(RoleBroadcaster, "chan", "acc")
	require.Len(t, subscriptions, 22)

	counts := make(map[string]int, len(subscriptions))
	for _, subscription := range subscriptions {
		counts[subscription.Type()]++

		switch subscription.Type() {
		case "channel.raid":
			condition := subscription.Request().Condition
			_, hasFrom := condition["from_broadcaster_user_id"]
			_, hasTo := condition["to_broadcaster_user_id"]
			require.True(t, hasFrom || hasTo, "raid subscription must name a direction")
		case "channel.chat_settings.update":
			require.Equal(t, map[string]string{
				"broadcaster_user_id": "chan",
				"user_id":             "acc",
			}, subscription.Request().Condition)
		default:
			require.Equal(t, "chan", subscription.Request().Condition["broadcaster_user_id"], subscription.Type())
		}
	}

	for _, topic := range []string{
		"channel.poll.begin",
		"channel.prediction.end",
		"channel.channel_points_custom_reward_redemption.add",
		"channel.chat_settings.update",
		"channel.warning.send",
		"automod.message.hold",
		"channel.unban_request.create",
		"channel.ad_break.begin",
	} {
		require.Equal(t, 1, counts[topic], topic)
	}

	require.Equal(t, 2, counts["channel.raid"])
}

func TestWatch_ModeratorGetsOnlyUnbanRequests(t *testing.T) {
	t.Parallel()

	subscriptions := Watch(RoleModerator, "chan", "other")
	require.Len(t, subscriptions, 2)

	for _, subscription := range subscriptions {
		require.Contains(t, []string{
			"channel.unban_request.create",
			"channel.unban_request.resolve",
		}, subscription.Type())
		require.Equal(t, map[string]string{
			"broadcaster_user_id": "chan",
			"moderator_user_id":   "other",
		}, subscription.Request().Condition)
	}
}

func TestWatch_ViewerGetsNothing(t *testing.T) {
	t.Parallel()

	require.Empty(t, Watch(RoleViewer, "chan", "other"))
}
