package tab

import (
	"context"
	"testing"
	"time"

	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
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

// fakeModerationClient records the ban request and answers GetUsers, so the command
// handlers can be exercised without the network.
type fakeModerationClient struct {
	ModerationAPI

	banned []twitchapi.BanUserData
}

func (f *fakeModerationClient) GetUsers(ctx context.Context, logins []string, ids []string) (twitchapi.UserResponse, error) {
	return twitchapi.UserResponse{Data: []twitchapi.UserData{{
		ID:          "user-id",
		Login:       "alice",
		DisplayName: "Alice",
	}}}, nil
}

func (f *fakeModerationClient) BanUser(ctx context.Context, broadcasterID string, moderatorID string, data twitchapi.BanUserData) error {
	f.banned = append(f.banned, data)

	return nil
}

// runModerationCommand executes a moderation command and returns the notice it shows.
func runModerationCommand(t *testing.T, client *fakeModerationClient, name string, args ...string) string {
	t.Helper()

	cmd := handleCommand(name, args, "channel-id", "channel", "account-id", client)
	require.NotNil(t, cmd, "the command must produce a notice")

	msg, ok := cmd().(ChatEventMessage)
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

			client := &fakeModerationClient{}
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
			require.Equal(t, "user-id", client.banned[0].UserID)
		})
	}
}

// TestHandleCommand_BanStaysPermanent pins that the shared timeout handler does not give a
// ban the timeout default: an omitted duration is what makes a ban permanent.
func TestHandleCommand_BanStaysPermanent(t *testing.T) {
	t.Parallel()

	client := &fakeModerationClient{}
	notice := runModerationCommand(t, client, "ban", "alice", "spam", "and", "hate")

	require.Contains(t, notice, "received a ban by you because: spam and hate")
	require.Len(t, client.banned, 1)
	require.Zero(t, client.banned[0].DurationInSeconds, "a ban carries no duration")
	require.Equal(t, "spam and hate", client.banned[0].Reason)
}
