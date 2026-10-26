package tab

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/google/uuid"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
)

func handleCommand(name string, args []string, channelID string, channel string, userAccountID string, ttv ModerationAPI) tea.Cmd {
	noticeCommandFunc := func(msg string) tea.Cmd {
		return func() tea.Msg {
			return ChatEventMessage{
				AccountID: userAccountID,
				Channel:   channel,
				ChannelID: channelID,
				Message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					MsgID:         twitchirc.MsgID(uuid.New().String()),
					Message:       msg,
				},
				IsFakeEvent: true,
			}
		}
	}

	switch name {
	case "timeout", "timeout_selected":
		return handleTimeout(name, args, channelID, channel, userAccountID, ttv, noticeCommandFunc)
	case "ban", "ban_selected":
		return handleTimeout(name, args, channelID, channel, userAccountID, ttv, noticeCommandFunc)
	case "unban", "unban_selected":
		return handleUnban(args, channel, channelID, userAccountID, ttv, noticeCommandFunc)
	case "delete_all_messages", "delete_selected_message":
		return handleDeleteMessages(name, args, channel, channelID, userAccountID, ttv, noticeCommandFunc)
	case "announcement":
		return handleAnnouncement(args, channel, channelID, userAccountID, ttv, noticeCommandFunc)
	case "marker":
		return handleMarker(args, channelID, channel, userAccountID, ttv, noticeCommandFunc)
	}

	return nil
}

// argAt reads an optional argument, so callers do not have to pad their args slice.
func argAt(args []string, index int) string {
	if index >= len(args) {
		return ""
	}

	return args[index]
}

func handleMarker(args []string, channelID string, channel string, userAccountID string, ttv ModerationAPI, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
	var description string
	if len(args) > 0 {
		description = strings.Join(args, " ")
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := ttv.CreateStreamMarker(ctx, twitchapi.CreateStreamMarkerRequest{
			UserID:      channelID,
			Description: description,
		})

		if err != nil {
			var apiErr twitchapi.APIError
			if errors.As(err, &apiErr) {
				switch apiErr.Status {
				case http.StatusBadRequest:
					return noticeFunc(fmt.Sprintf("Marker request invalid: %s", apiErr.Message))()
				case http.StatusUnauthorized, http.StatusForbidden:
					return noticeFunc(fmt.Sprintf("Unauthorized to create markers: %s", apiErr.Message))()
				case http.StatusNotFound:
					return noticeFunc(fmt.Sprintf("%s not live or has VODs disabled: %s", channel, apiErr.Message))()
				}
			}

			return noticeFunc(fmt.Sprintf("Failed to create marker: %s", err.Error()))()
		}

		dur := time.Duration(int(time.Second) * resp.PositionSeconds)
		return noticeFunc(fmt.Sprintf("Marker (%s) created at %s; stream time: %s", resp.ID, resp.CreatedAt.Local().Format("02.01.2006 15:04:05"), dur))()
	}
}

func handleAnnouncement(args []string, channel string, channelID string, userAccountID string, ttv ModerationAPI, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
	if len(args) < 2 {
		return noticeFunc("Expected Usage: /announcement <blue|green|orange|purple|primary> <message>")
	}

	color := args[0]

	allowed := []string{
		string(twitchapi.ChatAnnouncementColorBlue),
		string(twitchapi.ChatAnnouncementColorGreen),
		string(twitchapi.ChatAnnouncementColorOrange),
		string(twitchapi.ChatAnnouncementColorPurple),
		string(twitchapi.ChatAnnouncementColorPrimary),
	}

	if !slices.Contains(allowed, color) {
		return noticeFunc(fmt.Sprintf("Invalid color %s, expected one of %s", color, allowed))
	}

	annoucementMessage := strings.Join(args[1:], " ")

	if annoucementMessage == "" {
		return noticeFunc("Message can not be empty")
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := ttv.SendChatAnnouncement(ctx, channelID, userAccountID, twitchapi.CreateChatAnnouncementRequest{
			Color:   twitchapi.ChatAnnouncementColor(color),
			Message: annoucementMessage,
		})

		if err != nil {
			var apiErr twitchapi.APIError
			if errors.As(err, &apiErr) {
				switch apiErr.Status {
				case http.StatusBadRequest:
					return noticeFunc(fmt.Sprintf("Announcement request invalid: %s", apiErr.Message))()
				case http.StatusUnauthorized:
					return noticeFunc(fmt.Sprintf("Unauthorized to send announcement: %s", apiErr.Message))()
				case http.StatusTooManyRequests:
					return noticeFunc(fmt.Sprintf("Exceeded the number of announcements for %s: %s", channel, apiErr.Message))()
				}
			}

			return noticeFunc(fmt.Sprintf("Error while sending announcement: %s", err.Error()))()
		}

		return nil
	}
}

func handleUnban(args []string, channel string, channelID string, userAccountID string, ttv ModerationAPI, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
	if len(args) < 1 {
		return noticeFunc("Expected Usage: /unban <username>")
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		users, err := ttv.GetUsers(ctx, []string{args[0]}, nil)
		if err != nil {
			return noticeFunc(fmt.Sprintf("Error while fetching user ID %s: %s", args[0], err.Error()))()
		}

		if len(users.Data) < 1 {
			return noticeFunc(fmt.Sprintf("User %s can not be found", args[0]))()
		}

		err = ttv.UnbanUser(ctx, channelID, userAccountID, users.Data[0].ID)
		if err != nil {
			return noticeFunc(fmt.Sprintf("Error while sending unban request: %s", err.Error()))()
		}

		return noticeFunc(fmt.Sprintf("User %s received an unban by you", users.Data[0].DisplayName))()
	}
}

// Twitch takes a timeout as a whole number of seconds and treats an omitted duration as a
// permanent ban, so the friendly syntax is converted here instead of in the mod's head.
// The grammar is the one Twitch's own chat accepts - units s, m, h, d and w, combinable as
// in "1h30m", a bare number meaning seconds - so muscle memory from web chat carries over.
// https://dev.twitch.tv/docs/api/reference/#ban-user
const (
	timeoutDefault = 10 * time.Minute
	timeoutMaximum = 2 * 7 * 24 * time.Hour
)

// parseTimeoutDuration turns a typed duration into the value the API wants. An empty input
// yields the default, which is also what Twitch's own chat does when no duration is given.
func parseTimeoutDuration(input string) (time.Duration, error) {
	raw := strings.ToLower(strings.TrimSpace(input))
	if raw == "" {
		return timeoutDefault, nil
	}

	var total time.Duration

	for i := 0; i < len(raw); {
		start := i
		for i < len(raw) && raw[i] >= '0' && raw[i] <= '9' {
			i++
		}

		if start == i {
			return 0, timeoutSyntaxError(input)
		}

		value, err := strconv.Atoi(raw[start:i])
		if err != nil {
			return 0, timeoutSyntaxError(input)
		}

		// The unit is optional on the last segment, where it means seconds: "/timeout x 600"
		// keeps working for anyone who still counts in seconds.
		unit := time.Second

		if i < len(raw) {
			switch raw[i] {
			case 's':
			case 'm':
				unit = time.Minute
			case 'h':
				unit = time.Hour
			case 'd':
				unit = 24 * time.Hour
			case 'w':
				unit = 7 * 24 * time.Hour
			default:
				return 0, timeoutSyntaxError(input)
			}

			i++
		}

		// Compare before multiplying: a large enough number overflows time.Duration, and
		// anything above the maximum is rejected anyway.
		if value > int(timeoutMaximum/unit) {
			return 0, timeoutMaximumError(input)
		}

		total += time.Duration(value) * unit
		if total > timeoutMaximum {
			return 0, timeoutMaximumError(input)
		}
	}

	if total <= 0 {
		return 0, fmt.Errorf("must be greater than zero, got %s", strings.TrimSpace(input))
	}

	return total, nil
}

// The errors stay lowercase and free of a sentence frame, so the notice decides how they
// are shown and the API layer could reuse them unchanged.
func timeoutSyntaxError(input string) error {
	return fmt.Errorf("could not parse %s, expected something like 90s, 10m, 1h30m, 2d or 1w", strings.TrimSpace(input))
}

func timeoutMaximumError(input string) error {
	return fmt.Errorf("%s is longer than the maximum of 2 weeks (2w)", strings.TrimSpace(input))
}

// formatTimeoutDuration renders a duration the way a mod types it, so the confirmation
// echoes the value they meant rather than an opaque number of seconds.
func formatTimeoutDuration(d time.Duration) string {
	units := []struct {
		size   time.Duration
		suffix string
	}{
		{7 * 24 * time.Hour, "w"},
		{24 * time.Hour, "d"},
		{time.Hour, "h"},
		{time.Minute, "m"},
		{time.Second, "s"},
	}

	var b strings.Builder

	for _, unit := range units {
		if count := d / unit.size; count > 0 {
			fmt.Fprintf(&b, "%d%s", count, unit.suffix)
			d -= count * unit.size
		}
	}

	if b.Len() == 0 {
		return "0s"
	}

	return b.String()
}

func handleTimeout(name string, args []string, channelID string, channel string, userAccountID string, ttv ModerationAPI, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
	isTimeout := name == "timeout" || name == "timeout_selected"

	// Dropping empty tokens keeps the positional arguments meaningful when the input
	// contains repeated spaces: "/timeout alice  3w" must still be read as a duration and
	// rejected, not as a default timeout with "3w" as its reason.
	args = slices.DeleteFunc(slices.Clone(args), func(arg string) bool { return arg == "" })

	// parseSlashCommand reports a command without arguments as args == [""], which the
	// filter above turns into no arguments at all.
	if len(args) < 1 {
		if isTimeout {
			return noticeFunc("Expected Usage: /timeout <username> [duration] [reason] - duration like 90s, 10m, 1h30m or 2d")
		}

		return noticeFunc("Expected Usage: /ban <username> [reason]")
	}

	username := args[0]
	var duration time.Duration

	reasonIndex := 1
	if isTimeout {
		// The duration is positional, so a typo is reported instead of being read as the
		// start of the reason and silently shortening the timeout to the default. A reason
		// without a duration passes the default explicitly: "/timeout alice 10m spam".
		var err error

		duration, err = parseTimeoutDuration(argAt(args, 1))
		if err != nil {
			return noticeFunc("Invalid duration: " + err.Error())
		}

		reasonIndex = 2
	}

	reason := ""
	if len(args) > reasonIndex {
		reason = strings.TrimSpace(strings.Join(args[reasonIndex:], " "))
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		users, err := ttv.GetUsers(ctx, []string{username}, nil)
		if err != nil {
			return noticeFunc(fmt.Sprintf("Error while fetching user ID %s: %s", username, err.Error()))()
		}

		if len(users.Data) < 1 {
			return noticeFunc(fmt.Sprintf("User %s can not be found", username))()
		}

		displayName := users.Data[0].DisplayName
		// Zero for a ban: the API makes a ban permanent by the duration being absent, and
		// duration only ever has a value on the timeout path.
		seconds := int(duration / time.Second)

		err = ttv.BanUser(ctx, channelID, userAccountID, twitchapi.BanUserData{
			UserID:            users.Data[0].ID,
			DurationInSeconds: seconds,
			Reason:            reason,
		})
		if err != nil {
			return noticeFunc(fmt.Sprintf("Error while sending ban request: %s", err.Error()))()
		}

		if !isTimeout {
			if reason == "" {
				return noticeFunc(fmt.Sprintf("User %s received a ban by you", displayName))()
			}

			return noticeFunc(fmt.Sprintf("User %s received a ban by you because: %s", displayName, reason))()
		}

		confirmation := fmt.Sprintf("User %s received a timeout by you for %s (%d seconds)", displayName, formatTimeoutDuration(duration), seconds)
		if reason != "" {
			confirmation += " because: " + reason
		}

		return noticeFunc(confirmation)()
	}
}

func handleDeleteMessages(name string, args []string, channel string, channelID string, userAccountID string, ttv ModerationAPI, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var messageID string
		if name == "delete_selected_message" {
			if len(args) < 1 {
				return noticeFunc("Expected Usage: /delete_selected_message <message_id>")()
			}
			messageID = args[0]
		}

		err := ttv.DeleteMessage(ctx, channelID, userAccountID, messageID)
		if err != nil {
			var apiErr twitchapi.APIError
			if errors.As(err, &apiErr) {
				switch apiErr.Status {
				case http.StatusBadRequest:
					return noticeFunc(fmt.Sprintf("Delete message request invalid: %s", apiErr.Message))()
				case http.StatusUnauthorized, http.StatusForbidden:
					return noticeFunc(fmt.Sprintf("Unauthorized to delete message(s): %s", apiErr.Message))()
				case http.StatusNotFound:
					return noticeFunc(fmt.Sprintf("Message not found: %s", apiErr.Message))()
				default:
					return noticeFunc(fmt.Sprintf("Failed to delete message(s): %s", apiErr.Message))()
				}
			}
			return noticeFunc(fmt.Sprintf("Failed to delete message(s): %s", err.Error()))()
		}

		if name == "delete_selected_message" {
			return noticeFunc(fmt.Sprintf("Message %s deleted.", messageID))()
		}
		return noticeFunc("All messages deleted.")()
	}
}
