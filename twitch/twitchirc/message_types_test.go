package twitchirc

import (
	"encoding/json/v2"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// privateMessageMembers is the JSON object shape of a persisted PrivateMessage.
// The message log stores this payload, and older binaries read those rows with
// the former easyjson-generated snake_case mapping, so the member names are a
// storage format and must not change.
func privateMessageMembers() []string {
	return []string{
		"badge_info", "badges", "bits", "channel_user_name", "color",
		"display_name", "emotes", "first_msg", "id", "login_name", "message",
		"mod", "paid_amount", "paid_currency", "paid_exponent", "paid_is_system_message",
		"paid_level", "parent_display_name", "parent_msg_body", "parent_msg_id",
		"parent_user_id", "parent_user_login", "room_id", "source_badges",
		"source_id", "source_room_id", "subscriber", "thread_parent_msg_id",
		"thread_parent_user_login", "tmi_sent_ts", "turbo", "user_id",
		"user_type", "vip",
	}
}

func TestPrivateMessage_JSONStorageFormat(t *testing.T) {
	t.Parallel()

	msg := PrivateMessage{
		BadgeInfo:    []Badge{{Name: "subscriber", Version: "3"}},
		Badges:       []Badge{{Name: "moderator", Version: "1"}},
		Emotes:       []Emote{{ID: "25", Positions: []EmotePosition{{Start: 0, End: 4}}}},
		SourceBadges: []Badge{{Name: "glhf", Version: "1"}},
		TMISentTS:    time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC),
	}

	encoded, err := json.Marshal(msg)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(encoded, &raw))
	require.ElementsMatch(t, privateMessageMembers(), slices.Collect(maps.Keys(raw)))

	var decoded struct {
		BadgeInfo []map[string]any `json:"badge_info"`
		Badges    []map[string]any `json:"badges"`
		Emotes    []map[string]any `json:"emotes"`
		Source    []map[string]any `json:"source_badges"`
	}
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Len(t, decoded.BadgeInfo, 1)
	require.ElementsMatch(t, []string{"name", "version"}, slices.Collect(maps.Keys(decoded.BadgeInfo[0])))
	require.ElementsMatch(t, []string{"name", "version"}, slices.Collect(maps.Keys(decoded.Badges[0])))
	require.ElementsMatch(t, []string{"name", "version"}, slices.Collect(maps.Keys(decoded.Source[0])))
	require.ElementsMatch(t, []string{"id", "positions"}, slices.Collect(maps.Keys(decoded.Emotes[0])))

	var nested struct {
		Emotes []struct {
			Positions []map[string]any `json:"positions"`
		} `json:"emotes"`
	}
	require.NoError(t, json.Unmarshal(encoded, &nested))
	require.Len(t, nested.Emotes, 1)
	require.Len(t, nested.Emotes[0].Positions, 1)
	require.ElementsMatch(t, []string{"start", "end"}, slices.Collect(maps.Keys(nested.Emotes[0].Positions[0])))
}

func TestPrivateMessage_UnmarshalLegacyPayload(t *testing.T) {
	t.Parallel()

	// Exactly what the removed easyjson marshaller wrote.
	const legacy = `{"badge_info":[{"name":"subscriber","version":"3"}],"badges":null,"bits":100,` +
		`"color":"#ff0000","display_name":"Alice","emotes":[{"id":"25","positions":[{"start":0,"end":4}]}],` +
		`"id":"abc-123","mod":true,"first_msg":true,"paid_amount":500,"paid_currency":"EUR",` +
		`"paid_exponent":2,"paid_level":"ONE","paid_is_system_message":true,"parent_msg_id":"parent",` +
		`"parent_user_id":"u1","parent_user_login":"bob","parent_display_name":"Bob",` +
		`"parent_msg_body":"earlier","thread_parent_msg_id":"","thread_parent_user_login":"",` +
		`"room_id":"42","channel_user_name":"chan","login_name":"alice","subscriber":true,` +
		`"tmi_sent_ts":"2024-05-06T07:08:09Z","turbo":false,"user_id":"u2","user_type":"mod",` +
		`"vip":true,"source_id":"s1","source_room_id":"s2","source_badges":[],"message":"hello there"}`

	var msg PrivateMessage
	require.NoError(t, json.Unmarshal([]byte(legacy), &msg))

	require.Equal(t, []Badge{{Name: "subscriber", Version: "3"}}, msg.BadgeInfo)
	require.Nil(t, msg.Badges)
	require.Equal(t, 100, msg.Bits)
	require.Equal(t, "#ff0000", msg.Color)
	require.Equal(t, "Alice", msg.DisplayName)
	require.Equal(t, []Emote{{ID: "25", Positions: []EmotePosition{{Start: 0, End: 4}}}}, msg.Emotes)
	require.Equal(t, "abc-123", msg.ID)
	require.True(t, msg.Mod)
	require.True(t, msg.FirstMsg)
	require.Equal(t, 500, msg.PaidAmount)
	require.Equal(t, "EUR", msg.PaidCurrency)
	require.Equal(t, 2, msg.PaidExponent)
	require.Equal(t, "ONE", msg.PaidLevel)
	require.True(t, msg.PaidIsSystemMessage)
	require.Equal(t, "parent", msg.ParentMsgID)
	require.Equal(t, "Bob", msg.ParentDisplayName)
	require.Equal(t, "earlier", msg.ParentMsgBody)
	require.Equal(t, "42", msg.RoomID)
	require.Equal(t, "chan", msg.ChannelUserName)
	require.Equal(t, "alice", msg.LoginName)
	require.True(t, msg.Subscriber)
	require.Equal(t, time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC), msg.TMISentTS)
	require.Equal(t, "u2", msg.UserID)
	require.Equal(t, UserType("mod"), msg.UserType)
	require.True(t, msg.VIP)
	require.Equal(t, "s1", msg.SourceID)
	require.Equal(t, "hello there", msg.Message)

	// Re-encoding a payload read from the log is stable. Nil slices are the one
	// value that changes shape (v2 writes them as []), which v1 readers accept.
	again, err := json.Marshal(msg)
	require.NoError(t, err)

	var roundTripped PrivateMessage
	require.NoError(t, json.Unmarshal(again, &roundTripped))

	againTwice, err := json.Marshal(roundTripped)
	require.NoError(t, err)
	require.JSONEq(t, string(again), string(againTwice))
}
