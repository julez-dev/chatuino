package tab

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/chat"
	"github.com/julez-dev/chatuino/ui/nav"
	"github.com/stretchr/testify/require"
)

// TestParseTimeoutDuration covers the duration syntax Twitch's own chat accepts, so a
// mod can type what they type in web chat instead of converting to seconds in their head.
func TestParseTimeoutDuration(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		input string
		want  time.Duration
		err   string
	}{
		{name: "default when omitted", input: "", want: 10 * time.Minute},
		{name: "bare number is seconds", input: "90", want: 90 * time.Second},
		{name: "existing seconds keep working", input: "600", want: 10 * time.Minute},
		{name: "seconds unit", input: "45s", want: 45 * time.Second},
		{name: "minutes", input: "10m", want: 10 * time.Minute},
		{name: "hours", input: "1h", want: time.Hour},
		{name: "days", input: "1d", want: 24 * time.Hour},
		{name: "weeks", input: "1w", want: 7 * 24 * time.Hour},
		{name: "combined units", input: "1h30m", want: 90 * time.Minute},
		{name: "combined days and hours", input: "1d2h", want: 26 * time.Hour},
		{name: "trailing bare number is seconds", input: "10m30", want: 10*time.Minute + 30*time.Second},
		{name: "maximum is two weeks", input: "2w", want: 2 * 7 * 24 * time.Hour},
		{name: "uppercase and spaces", input: " 10M ", want: 10 * time.Minute},

		{name: "above the maximum", input: "3w", err: "maximum"},
		{name: "just above the maximum", input: "2w1s", err: "maximum"},
		{name: "zero", input: "0", err: "greater than zero"},
		{name: "zero with unit", input: "0m", err: "greater than zero"},
		{name: "no digits", input: "m", err: "expected something like"},
		{name: "garbage", input: "nonsense", err: "expected something like"},
		{name: "unknown unit", input: "10x", err: "expected something like"},
		{name: "fractions", input: "1.5h", err: "expected something like"},
		{name: "negative", input: "-10m", err: "expected something like"},
		{name: "overflow beyond int64", input: "99999999999999999999", err: "expected something like"},
		// Parses as a number but overflows time.Duration once multiplied, so the cap has to
		// be checked before the multiplication, not after the sum.
		{name: "large number in seconds", input: "999999999999999999", err: "maximum"},
		{name: "large number in hours", input: "999999999999999h", err: "maximum"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseTimeoutDuration(tt.input)

			if tt.err != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestFormatTimeoutDuration pins the confirmation notice: a mod who typed "90m" must be
// able to recognise the value that was actually sent.
func TestFormatTimeoutDuration(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in   time.Duration
		want string
	}{
		{in: 0, want: "0s"},
		{in: 30 * time.Second, want: "30s"},
		{in: 10 * time.Minute, want: "10m"},
		{in: 90 * time.Minute, want: "1h30m"},
		{in: 26 * time.Hour, want: "1d2h"},
		{in: 2 * 7 * 24 * time.Hour, want: "2w"},
		{in: 8 * 24 * time.Hour, want: "1w1d"},
		{in: 10*time.Minute + 30*time.Second, want: "10m30s"},
	} {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, formatTimeoutDuration(tt.in))
		})
	}
}

// runModerationCommand executes a moderation command and returns the notice it shows.
func runModerationCommand(t *testing.T, client *fakeModerationClient, name string, args ...string) string {
	t.Helper()

	cmd := handleCommand("tab-id", name, args, "channel-id", "channel", "account-id", client)
	require.NotNil(t, cmd, "the command must produce a notice")

	msg, ok := cmd().(chat.ChatEventMessage)
	require.True(t, ok, "moderation commands answer with a chat event")

	notice, ok := msg.Message.(*twitchirc.Notice)
	require.True(t, ok)

	return notice.Message
}

// TestHandleCommand_TimeoutDuration checks the handler end to end: what reaches the API
// and what the mod is told, including that a rejected duration sends nothing at all.
func TestHandleCommand_TimeoutDuration(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name         string
		command      string
		args         []string
		wantSeconds  int
		wantReason   string
		wantNotice   string
		wantNoBanYet bool
	}{
		{
			name:        "selected variant takes the same path",
			command:     "timeout_selected",
			args:        []string{"alice", "45m", "spam"},
			wantSeconds: 2700,
			wantReason:  "spam",
			wantNotice:  "for 45m (2700 seconds)",
		},
		{
			name:        "friendly duration",
			args:        []string{"alice", "1h30m", "spam"},
			wantSeconds: 5400,
			wantReason:  "spam",
			wantNotice:  "for 1h30m (5400 seconds)",
		},
		{
			name:        "default duration when omitted",
			args:        []string{"alice"},
			wantSeconds: 600,
			wantNotice:  "for 10m (600 seconds)",
		},
		{
			name:        "bare seconds still work",
			args:        []string{"alice", "90", "spam"},
			wantSeconds: 90,
			wantReason:  "spam",
			wantNotice:  "for 1m30s (90 seconds)",
		},
		{
			name:        "multi word reason is kept whole",
			args:        []string{"alice", "10m", "spam", "and", "hate"},
			wantSeconds: 600,
			wantReason:  "spam and hate",
			wantNotice:  "because: spam and hate",
		},
		{
			name:         "above the maximum sends nothing",
			args:         []string{"alice", "3w", "spam"},
			wantNotice:   "maximum",
			wantNoBanYet: true,
		},
		{
			name:         "unparsable duration sends nothing",
			args:         []string{"alice", "nonsense", "spam"},
			wantNotice:   "expected something like",
			wantNoBanYet: true,
		},
		{
			name:        "repeated spaces still read as arguments",
			args:        []string{"alice", "", "10m", "", "spam", "and", "hate"},
			wantSeconds: 600,
			wantReason:  "spam and hate",
			wantNotice:  "because: spam and hate",
		},
		{
			name:        "timeout without a reason reads cleanly",
			args:        []string{"alice", "30m"},
			wantSeconds: 1800,
			wantNotice:  "for 30m (1800 seconds)",
		},
		{
			name:         "usage names the duration syntax",
			args:         []string{},
			wantNotice:   "Expected Usage: /timeout <username> [duration] [reason]",
			wantNoBanYet: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			command := tt.command
			if command == "" {
				command = "timeout"
			}

			client := newFakeModerationClient()
			notice := runModerationCommand(t, client, command, tt.args...)

			require.Contains(t, notice, tt.wantNotice)

			if tt.wantNoBanYet {
				require.Empty(t, client.banned, "a rejected command must not reach the API")

				return
			}

			require.Len(t, client.banned, 1)
			require.Equal(t, tt.wantSeconds, client.banned[0].DurationInSeconds)
			if tt.wantReason == "" {
				require.NotContains(t, notice, "because:", "an empty reason adds no clause")
			}

			require.Equal(t, tt.wantReason, client.banned[0].Reason)
			require.Equal(t, "alice-id", client.banned[0].UserID)
		})
	}
}

// TestHandleCommand_BanStaysPermanent pins that the shared timeout handler does not give a
// ban the timeout default: an omitted duration is what makes a ban permanent.
func TestHandleCommand_BanStaysPermanent(t *testing.T) {
	t.Parallel()

	client := newFakeModerationClient()
	notice := runModerationCommand(t, client, "ban", "alice", "spam", "and", "hate")

	require.Contains(t, notice, "received a ban by you because: spam and hate")
	require.Len(t, client.banned, 1)
	require.Zero(t, client.banned[0].DurationInSeconds, "a ban carries no duration")
	require.Equal(t, "spam and hate", client.banned[0].Reason)
}

// fakeModerationClient covers the moderation surface the commands use. The
// embedded interface keeps the fixture small: a method a test does not exercise
// would panic if the command suddenly called it.
type fakeModerationClient struct {
	ModerationAPI

	users map[string]twitchapi.UserData

	banned []twitchapi.BanUserData

	settingsUpdate twitchapi.ChatSettingsUpdateData
	settingsCalls  int
	settingsErr    error

	moderators     []twitchapi.ChatRoleUser
	vips           []twitchapi.ChatRoleUser
	addedModerator string
	removedMod     string
	addedVIP       string
	removedVIP     string

	warnedUser   string
	warnedReason string
	warnErr      error

	raidFrom string
	raidTo   string
	raidErr  error
	canceled bool

	shoutoutFrom string
	shoutoutTo   string
	shoutoutMod  string
	shoutoutErr  error

	automodAction string
	automodMsgID  string
	automodErr    error

	snoozeResp twitchapi.AdSnoozeData
	snoozeErr  error

	unbanRequests  []twitchapi.UnbanRequest
	unbanFetchErr  error
	resolvedID     string
	resolvedStatus string
	resolutionText string
	resolveErr     error
}

func (f *fakeModerationClient) GetUsers(_ context.Context, logins []string, _ []string) (twitchapi.UserResponse, error) {
	users := make([]twitchapi.UserData, 0, len(logins))
	for _, login := range logins {
		if user, ok := f.users[login]; ok {
			users = append(users, user)
		}
	}

	return twitchapi.UserResponse{Data: users}, nil
}

func (f *fakeModerationClient) BanUser(_ context.Context, _ string, _ string, data twitchapi.BanUserData) error {
	f.banned = append(f.banned, data)
	return nil
}

func (f *fakeModerationClient) UpdateChatSettings(_ context.Context, broadcasterID string, _ string, data twitchapi.ChatSettingsUpdateData) (twitchapi.GetChatSettingsResponse, error) {
	f.settingsCalls++
	f.settingsUpdate = data

	if f.settingsErr != nil {
		return twitchapi.GetChatSettingsResponse{}, f.settingsErr
	}

	return twitchapi.GetChatSettingsResponse{Data: []twitchapi.ChatSettingData{{BroadcasterID: broadcasterID}}}, nil
}

func (f *fakeModerationClient) GetModerators(context.Context, string, []string) ([]twitchapi.ChatRoleUser, error) {
	return f.moderators, nil
}

func (f *fakeModerationClient) AddChannelModerator(_ context.Context, _ string, userID string) error {
	f.addedModerator = userID
	return nil
}

func (f *fakeModerationClient) RemoveChannelModerator(_ context.Context, _ string, userID string) error {
	f.removedMod = userID
	return nil
}

func (f *fakeModerationClient) GetVIPs(context.Context, string, []string) ([]twitchapi.ChatRoleUser, error) {
	return f.vips, nil
}

func (f *fakeModerationClient) AddChannelVIP(_ context.Context, _ string, userID string) error {
	f.addedVIP = userID
	return nil
}

func (f *fakeModerationClient) RemoveChannelVIP(_ context.Context, _ string, userID string) error {
	f.removedVIP = userID
	return nil
}

func (f *fakeModerationClient) WarnChatUser(_ context.Context, _ string, _ string, userID string, reason string) error {
	f.warnedUser = userID
	f.warnedReason = reason
	return f.warnErr
}

func (f *fakeModerationClient) StartRaid(_ context.Context, from string, to string) (twitchapi.RaidData, error) {
	f.raidFrom = from
	f.raidTo = to
	return twitchapi.RaidData{}, f.raidErr
}

func (f *fakeModerationClient) CancelRaid(context.Context, string) error {
	f.canceled = true
	return f.raidErr
}

func (f *fakeModerationClient) SendShoutout(_ context.Context, from string, to string, moderator string) error {
	f.shoutoutFrom = from
	f.shoutoutTo = to
	f.shoutoutMod = moderator
	return f.shoutoutErr
}

func (f *fakeModerationClient) ManageAutoModMessage(_ context.Context, _ string, messageID string, action string) error {
	f.automodMsgID = messageID
	f.automodAction = action
	return f.automodErr
}

func (f *fakeModerationClient) SnoozeNextAd(context.Context, string) (twitchapi.AdSnoozeData, error) {
	return f.snoozeResp, f.snoozeErr
}

func (f *fakeModerationClient) FetchUnbanRequests(context.Context, string, string) ([]twitchapi.UnbanRequest, error) {
	return f.unbanRequests, f.unbanFetchErr
}

func (f *fakeModerationClient) ResolveBanRequest(_ context.Context, _ string, _ string, requestID string, status string, resolutionText string) (twitchapi.UnbanRequest, error) {
	f.resolvedID = requestID
	f.resolvedStatus = status
	f.resolutionText = resolutionText
	if f.resolveErr != nil {
		return twitchapi.UnbanRequest{}, f.resolveErr
	}

	for _, request := range f.unbanRequests {
		if request.ID == requestID {
			request.Status = status
			request.ResolutionText = resolutionText
			return request, nil
		}
	}

	return twitchapi.UnbanRequest{ID: requestID, Status: status, ResolutionText: resolutionText}, nil
}

func newFakeModerationClient() *fakeModerationClient {
	return &fakeModerationClient{
		users: map[string]twitchapi.UserData{
			"alice": {ID: "alice-id", Login: "alice", DisplayName: "Alice"},
			"bob":   {ID: "bob-id", Login: "bob", DisplayName: "Bob"},
		},
		moderators: []twitchapi.ChatRoleUser{
			{UserID: "alice-id", UserLogin: "alice", UserName: "Alice"},
			{UserID: "bob-id", UserLogin: "bob", UserName: "Bob"},
		},
		vips: []twitchapi.ChatRoleUser{
			{UserID: "alice-id", UserLogin: "alice", UserName: "Alice"},
		},
	}
}

func runCommandMessages(t *testing.T, client ModerationAPI, name string, args ...string) []tea.Msg {
	t.Helper()

	cmd := handleCommand("tab-id", name, args, "channel-id", "channel", "account-id", client)
	require.NotNil(t, cmd)

	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		msgs := make([]tea.Msg, 0, len(batch))
		for _, child := range batch {
			if child == nil {
				continue
			}

			msgs = append(msgs, child())
		}

		return msgs
	}

	return []tea.Msg{msg}
}

func noticeText(t *testing.T, msg tea.Msg) string {
	t.Helper()

	chatEvent, ok := msg.(chat.ChatEventMessage)
	require.True(t, ok)

	notice, ok := chatEvent.Message.(*twitchirc.Notice)
	require.True(t, ok)

	return notice.Message
}

func settingsUpdateFrom(t *testing.T, msgs []tea.Msg) (chatSettingsUpdatedMessage, bool) {
	t.Helper()

	for _, msg := range msgs {
		envelope, ok := msg.(nav.Envelope)
		if !ok {
			continue
		}

		if update, ok := envelope.Msg.(chatSettingsUpdatedMessage); ok {
			return update, true
		}
	}

	return chatSettingsUpdatedMessage{}, false
}

func TestHandleCommand_ChatSettings(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name         string
		command      string
		args         []string
		wantSlow     *bool
		wantSlowWait *int
		wantFollow   *bool
		wantFollowIn *int
		wantSub      *bool
		wantEmote    *bool
		wantUnique   *bool
		wantNotice   string
	}{
		{name: "slow default", command: "slow", args: []string{""}, wantSlow: boolValue(true), wantNotice: "Slow mode enabled"},
		{name: "slow with seconds", command: "slow", args: []string{"10"}, wantSlow: boolValue(true), wantSlowWait: intValue(10), wantNotice: "10s"},
		{name: "slow with combined duration", command: "slow", args: []string{"1m30s"}, wantSlow: boolValue(true), wantSlowWait: intValue(90), wantNotice: "1m30s"},
		{name: "slow off", command: "slowoff", args: []string{""}, wantSlow: boolValue(false), wantNotice: "disabled"},
		{name: "followers default", command: "followers", args: []string{""}, wantFollow: boolValue(true), wantNotice: "Followers-only mode enabled"},
		{name: "followers bare number is minutes", command: "followers", args: []string{"10"}, wantFollow: boolValue(true), wantFollowIn: intValue(10)},
		{name: "followers zero is allowed", command: "followers", args: []string{"0m"}, wantFollow: boolValue(true), wantFollowIn: intValue(0)},
		{name: "followers off", command: "followersoff", args: []string{""}, wantFollow: boolValue(false)},
		{name: "subscribers", command: "subscribers", args: []string{""}, wantSub: boolValue(true)},
		{name: "subscribers off", command: "subscribersoff", args: []string{""}, wantSub: boolValue(false)},
		{name: "emote only", command: "emoteonly", args: []string{""}, wantEmote: boolValue(true)},
		{name: "emote only off", command: "emoteonlyoff", args: []string{""}, wantEmote: boolValue(false)},
		{name: "unique chat", command: "uniquechat", args: []string{""}, wantUnique: boolValue(true)},
		{name: "unique chat off", command: "uniquechatoff", args: []string{""}, wantUnique: boolValue(false)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newFakeModerationClient()
			msgs := runCommandMessages(t, client, tt.command, tt.args...)

			require.Equal(t, 1, client.settingsCalls)
			require.Equal(t, tt.wantSlow, client.settingsUpdate.SlowMode)
			require.Equal(t, tt.wantSlowWait, client.settingsUpdate.SlowModeWaitTime)
			require.Equal(t, tt.wantFollow, client.settingsUpdate.FollowerMode)
			require.Equal(t, tt.wantFollowIn, client.settingsUpdate.FollowerModeDuration)
			require.Equal(t, tt.wantSub, client.settingsUpdate.SubscriberMode)
			require.Equal(t, tt.wantEmote, client.settingsUpdate.EmoteMode)
			require.Equal(t, tt.wantUnique, client.settingsUpdate.UniqueChatMode)

			if tt.wantNotice != "" {
				require.Contains(t, joinNotices(t, msgs), tt.wantNotice)
			}

			update, ok := settingsUpdateFrom(t, msgs)
			require.True(t, ok, "a successful write updates the tab's status bar")
			require.Equal(t, "channel-id", update.settings.BroadcasterID)
		})
	}
}

func joinNotices(t *testing.T, msgs []tea.Msg) string {
	t.Helper()

	var text string
	for _, msg := range msgs {
		if _, ok := msg.(chat.ChatEventMessage); ok {
			text += noticeText(t, msg) + "\n"
		}
	}

	return text
}

func TestHandleCommand_ChatSettingsRejectsBadDuration(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		command string
		arg     string
		want    string
	}{
		{command: "slow", arg: "2", want: "at least 3 seconds"},
		{command: "slow", arg: "3m", want: "maximum"},
		{command: "slow", arg: "nonsense", want: "expected something like"},
		{command: "followers", arg: "90s", want: "whole number of minutes"},
		{command: "followers", arg: "91d", want: "maximum"},
	} {
		t.Run(tt.command+" "+tt.arg, func(t *testing.T) {
			t.Parallel()

			client := newFakeModerationClient()
			msgs := runCommandMessages(t, client, tt.command, tt.arg)
			require.Zero(t, client.settingsCalls, "a rejected duration sends nothing")
			require.Contains(t, joinNotices(t, msgs), tt.want)
		})
	}
}

func TestHandleCommand_ChatSettingsAPIError(t *testing.T) {
	t.Parallel()

	client := newFakeModerationClient()
	client.settingsErr = twitchapi.APIError{Status: 401, Message: "missing scope"}

	msgs := runCommandMessages(t, client, "slow", "10")
	require.Contains(t, joinNotices(t, msgs), "Unauthorized")
	_, ok := settingsUpdateFrom(t, msgs)
	require.False(t, ok, "a failed write must not claim the status bar changed")
}

func TestHandleCommand_RoleCommands(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		command string
		want    string
		check   func(*testing.T, *fakeModerationClient)
	}{
		{"mod", "now a moderator", func(t *testing.T, c *fakeModerationClient) { require.Equal(t, "alice-id", c.addedModerator) }},
		{"unmod", "no longer a moderator", func(t *testing.T, c *fakeModerationClient) { require.Equal(t, "alice-id", c.removedMod) }},
		{"vip", "now a VIP", func(t *testing.T, c *fakeModerationClient) { require.Equal(t, "alice-id", c.addedVIP) }},
		{"unvip", "no longer a VIP", func(t *testing.T, c *fakeModerationClient) { require.Equal(t, "alice-id", c.removedVIP) }},
	} {
		t.Run(tt.command, func(t *testing.T) {
			t.Parallel()

			client := newFakeModerationClient()
			msgs := runCommandMessages(t, client, tt.command, "alice")
			require.Contains(t, joinNotices(t, msgs), tt.want)
			tt.check(t, client)
		})
	}

	t.Run("mods list", func(t *testing.T) {
		t.Parallel()
		client := newFakeModerationClient()
		msgs := runCommandMessages(t, client, "mods")
		require.Contains(t, joinNotices(t, msgs), "Moderators (2): Alice, Bob")
	})

	t.Run("vips list", func(t *testing.T) {
		t.Parallel()
		client := newFakeModerationClient()
		msgs := runCommandMessages(t, client, "vips")
		require.Contains(t, joinNotices(t, msgs), "VIPs (1): Alice")
	})

	t.Run("missing user", func(t *testing.T) {
		t.Parallel()
		client := newFakeModerationClient()
		msgs := runCommandMessages(t, client, "mod")
		require.Contains(t, joinNotices(t, msgs), "Expected Usage: /mod <username>")
	})

	t.Run("unknown user", func(t *testing.T) {
		t.Parallel()
		client := newFakeModerationClient()
		msgs := runCommandMessages(t, client, "warn", "nobody")
		require.Contains(t, joinNotices(t, msgs), "can not be found")
	})
}

func TestHandleCommand_Warn(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		args       []string
		wantReason string
	}{
		{name: "reason is joined", args: []string{"alice", "spam", "and", "hate"}, wantReason: "spam and hate"},
		{name: "reason is optional", args: []string{"alice"}, wantReason: "No reason provided"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newFakeModerationClient()
			msgs := runCommandMessages(t, client, "warn", tt.args...)
			require.Equal(t, "alice-id", client.warnedUser)
			require.Equal(t, tt.wantReason, client.warnedReason)
			require.Contains(t, joinNotices(t, msgs), tt.wantReason)
		})
	}

	t.Run("api error is a notice", func(t *testing.T) {
		t.Parallel()
		client := newFakeModerationClient()
		client.warnErr = twitchapi.APIError{Status: 409, Message: "warning is being updated"}
		msgs := runCommandMessages(t, client, "warn", "alice", "spam")
		require.Contains(t, joinNotices(t, msgs), "conflict")
	})
}

func TestHandleCommand_RaidAndShoutout(t *testing.T) {
	t.Parallel()

	t.Run("raid resolves the target", func(t *testing.T) {
		t.Parallel()
		client := newFakeModerationClient()
		msgs := runCommandMessages(t, client, "raid", "bob")
		require.Equal(t, "channel-id", client.raidFrom)
		require.Equal(t, "bob-id", client.raidTo)
		require.Contains(t, joinNotices(t, msgs), "Raid started towards Bob")
	})

	t.Run("raid to self is refused", func(t *testing.T) {
		t.Parallel()
		client := newFakeModerationClient()
		client.users["channel"] = twitchapi.UserData{ID: "channel-id", Login: "channel", DisplayName: "Me"}
		msgs := runCommandMessages(t, client, "raid", "channel")
		require.Empty(t, client.raidTo)
		require.Contains(t, joinNotices(t, msgs), "cannot raid itself")
	})

	t.Run("unraid cancels", func(t *testing.T) {
		t.Parallel()
		client := newFakeModerationClient()
		msgs := runCommandMessages(t, client, "unraid")
		require.True(t, client.canceled)
		require.Contains(t, joinNotices(t, msgs), "canceled")
	})

	t.Run("shoutout", func(t *testing.T) {
		t.Parallel()
		client := newFakeModerationClient()
		msgs := runCommandMessages(t, client, "shoutout", "bob")
		require.Equal(t, "channel-id", client.shoutoutFrom)
		require.Equal(t, "bob-id", client.shoutoutTo)
		require.Equal(t, "account-id", client.shoutoutMod)
		require.Contains(t, joinNotices(t, msgs), "Sent a shoutout to Bob")
	})

	t.Run("shoutout rate limit is surfaced", func(t *testing.T) {
		t.Parallel()
		client := newFakeModerationClient()
		client.shoutoutErr = twitchapi.APIError{Status: 429, Message: "too many shoutouts"}
		msgs := runCommandMessages(t, client, "shoutout", "bob")
		require.Contains(t, joinNotices(t, msgs), "Rate limited")
	})
}

func TestHandleCommand_AutoModAndSnooze(t *testing.T) {
	t.Parallel()

	t.Run("approve", func(t *testing.T) {
		t.Parallel()
		client := newFakeModerationClient()
		msgs := runCommandMessages(t, client, "approve", "msg-1")
		require.Equal(t, "msg-1", client.automodMsgID)
		require.Equal(t, "ALLOW", client.automodAction)
		require.Contains(t, joinNotices(t, msgs), "approved")
	})

	t.Run("deny", func(t *testing.T) {
		t.Parallel()
		client := newFakeModerationClient()
		msgs := runCommandMessages(t, client, "deny", "msg-1")
		require.Equal(t, "DENY", client.automodAction)
		require.Contains(t, joinNotices(t, msgs), "denied")
	})

	t.Run("missing message id", func(t *testing.T) {
		t.Parallel()
		client := newFakeModerationClient()
		msgs := runCommandMessages(t, client, "approve")
		require.Contains(t, joinNotices(t, msgs), "Expected Usage: /approve <message_id>")
	})

	t.Run("snooze", func(t *testing.T) {
		t.Parallel()
		client := newFakeModerationClient()
		client.snoozeResp = twitchapi.AdSnoozeData{NextAdAt: "2026-10-26T21:00:00Z"}
		msgs := runCommandMessages(t, client, "snooze")
		require.Contains(t, joinNotices(t, msgs), "snoozed to")
	})

	t.Run("snooze error", func(t *testing.T) {
		t.Parallel()
		client := newFakeModerationClient()
		client.snoozeErr = errors.New("no snoozes left")
		msgs := runCommandMessages(t, client, "snooze")
		require.Contains(t, joinNotices(t, msgs), "no snoozes left")
	})
}
