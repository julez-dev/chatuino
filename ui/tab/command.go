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
	"github.com/julez-dev/chatuino/chatstate"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/chat"
	"github.com/julez-dev/chatuino/ui/nav"
)

func handleCommand(tabID string, name string, args []string, channelID string, channel string, userAccountID string, ttv ModerationAPI) tea.Cmd {
	noticeCommandFunc := func(msg string) tea.Cmd {
		return func() tea.Msg {
			return chat.ChatEventMessage{
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
	case "slow", "slowoff", "followers", "followersoff", "subscribers", "subscribersoff", "emoteonly", "emoteonlyoff", "uniquechat", "uniquechatoff":
		return handleChatSetting(name, args, tabID, channelID, userAccountID, ttv, noticeCommandFunc)
	case "clear":
		return handleDeleteMessages("delete_all_messages", nil, channel, channelID, userAccountID, ttv, noticeCommandFunc)
	case "mod", "unmod", "mods", "vip", "unvip", "vips":
		return handleRoleCommand(name, args, channelID, ttv, noticeCommandFunc)
	case "warn":
		return handleWarn(args, channelID, userAccountID, ttv, noticeCommandFunc)
	case "raid", "unraid":
		return handleRaid(name, args, channelID, ttv, noticeCommandFunc)
	case "shoutout":
		return handleShoutout(args, channelID, userAccountID, ttv, noticeCommandFunc)
	case "approve", "deny":
		return handleAutoModAction(name, args, userAccountID, ttv, noticeCommandFunc)
	case "snooze":
		return handleSnooze(channelID, ttv, noticeCommandFunc)
	case "unbanrequests":
		return nav.To(tabID, openUnbanRequestsMessage{})
	}

	return nil
}

// chatSettingsUpdatedMessage carries the settings returned by a successful write
// back to the tab that made it, so its status bar and the shared cache update
// without waiting for the matching EventSub event.
type chatSettingsUpdatedMessage struct {
	settings twitchapi.ChatSettingData
}

func settingsUpdatedCmd(tabID string, settings twitchapi.ChatSettingData) tea.Cmd {
	if tabID == "" {
		return nil
	}

	return nav.To(tabID, chatSettingsUpdatedMessage{settings: settings})
}

// firstArg returns the first non-empty argument. parseSlashCommand reports a
// command without arguments as args == [""] and repeated spaces leave empty
// tokens behind, so positional handlers go through this instead of args[0].
func firstArg(args []string) string {
	for _, arg := range args {
		if trimmed := strings.TrimSpace(arg); trimmed != "" {
			return trimmed
		}
	}

	return ""
}

// argsFrom returns the whitespace-normalised arguments starting at index.
func argsFrom(args []string, index int) string {
	if index >= len(args) {
		return ""
	}

	parts := make([]string, 0, len(args)-index)
	for _, arg := range args[index:] {
		if arg != "" {
			parts = append(parts, arg)
		}
	}

	return strings.TrimSpace(strings.Join(parts, " "))
}

func boolValue(value bool) *bool { return &value }

func intValue(value int) *int { return &value }

// findUser resolves a login through Helix and returns a ready-to-show error
// message when the user does not exist.
func findUser(ctx context.Context, ttv ModerationAPI, login string) (twitchapi.UserData, string) {
	users, err := ttv.GetUsers(ctx, []string{login}, nil)
	if err != nil {
		return twitchapi.UserData{}, fmt.Sprintf("Error while fetching user ID %s: %s", login, err.Error())
	}

	if len(users.Data) < 1 {
		return twitchapi.UserData{}, fmt.Sprintf("User %s can not be found", login)
	}

	return users.Data[0], ""
}

func describeAPIError(err error, action string) string {
	var apiErr twitchapi.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case http.StatusBadRequest:
			return fmt.Sprintf("%s request invalid: %s", action, apiErr.Message)
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Sprintf("Unauthorized to %s: %s", action, apiErr.Message)
		case http.StatusNotFound:
			return fmt.Sprintf("%s target not found: %s", action, apiErr.Message)
		case http.StatusConflict:
			return fmt.Sprintf("Could not %s because of a conflict: %s", action, apiErr.Message)
		case http.StatusTooManyRequests:
			return fmt.Sprintf("Rate limited while trying to %s: %s", action, apiErr.Message)
		default:
			return fmt.Sprintf("Failed to %s: %s", action, apiErr.Message)
		}
	}

	return fmt.Sprintf("Failed to %s: %s", action, err.Error())
}

func handleChatSetting(name string, args []string, tabID string, channelID string, userAccountID string, ttv ModerationAPI, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
	data := twitchapi.ChatSettingsUpdateData{}
	success := ""

	switch name {
	case "slow":
		duration, err := parseSlowModeDuration(firstArg(args))
		if err != nil {
			return noticeFunc("Invalid duration: " + err.Error())
		}

		data.SlowMode = boolValue(true)
		if firstArg(args) != "" {
			data.SlowModeWaitTime = intValue(int(duration / time.Second))
		}
		success = "Slow mode enabled"
		if data.SlowModeWaitTime != nil {
			success = fmt.Sprintf("Slow mode enabled: %s", formatTimeoutDuration(duration))
		}
	case "slowoff":
		data.SlowMode = boolValue(false)
		success = "Slow mode disabled"
	case "followers":
		duration, err := parseFollowerModeDuration(firstArg(args))
		if err != nil {
			return noticeFunc("Invalid duration: " + err.Error())
		}

		data.FollowerMode = boolValue(true)
		if firstArg(args) != "" {
			data.FollowerModeDuration = intValue(int(duration / time.Minute))
		}
		success = "Followers-only mode enabled"
		if data.FollowerModeDuration != nil {
			success = fmt.Sprintf("Followers-only mode enabled: %s", humanizeDuration(duration))
		}
	case "followersoff":
		data.FollowerMode = boolValue(false)
		success = "Followers-only mode disabled"
	case "subscribers":
		data.SubscriberMode = boolValue(true)
		success = "Subscribers-only mode enabled"
	case "subscribersoff":
		data.SubscriberMode = boolValue(false)
		success = "Subscribers-only mode disabled"
	case "emoteonly":
		data.EmoteMode = boolValue(true)
		success = "Emote-only mode enabled"
	case "emoteonlyoff":
		data.EmoteMode = boolValue(false)
		success = "Emote-only mode disabled"
	case "uniquechat":
		data.UniqueChatMode = boolValue(true)
		success = "Unique chat enabled"
	case "uniquechatoff":
		data.UniqueChatMode = boolValue(false)
		success = "Unique chat disabled"
	default:
		return noticeFunc("Unknown chat setting command")
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := ttv.UpdateChatSettings(ctx, channelID, userAccountID, data)
		if err != nil {
			return noticeFunc(describeAPIError(err, "update chat settings"))()
		}

		settings, err := chatstate.SettingsFromResponse(resp)
		if err != nil {
			return noticeFunc(fmt.Sprintf("Chat settings were updated but could not be read back: %s", err))()
		}

		cmds := []tea.Cmd{noticeFunc(success)}
		if publish := settingsUpdatedCmd(tabID, settings); publish != nil {
			cmds = append(cmds, publish)
		}

		return tea.Batch(cmds...)()
	}
}

func handleRoleCommand(name string, args []string, channelID string, ttv ModerationAPI, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
	isList := name == "mods" || name == "vips"

	if !isList && firstArg(args) == "" {
		return noticeFunc(fmt.Sprintf("Expected Usage: /%s <username>", name))
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		switch name {
		case "mods", "vips":
			var (
				users []twitchapi.ChatRoleUser
				err   error
				label string
			)

			if name == "mods" {
				label = "Moderators"
				users, err = ttv.GetModerators(ctx, channelID, nil)
			} else {
				label = "VIPs"
				users, err = ttv.GetVIPs(ctx, channelID, nil)
			}

			if err != nil {
				return noticeFunc(describeAPIError(err, "list "+label))()
			}

			if len(users) == 0 {
				return noticeFunc(fmt.Sprintf("This channel has no %s.", strings.ToLower(label)))()
			}

			names := make([]string, 0, len(users))
			for _, user := range users {
				names = append(names, preferredDisplayName(user.UserName, user.UserLogin))
			}

			return noticeFunc(fmt.Sprintf("%s (%d): %s", label, len(names), strings.Join(names, ", ")))()
		}

		user, notice := findUser(ctx, ttv, firstArg(args))
		if notice != "" {
			return noticeFunc(notice)()
		}

		displayName := preferredDisplayName(user.DisplayName, user.Login)

		var (
			err     error
			success string
		)

		switch name {
		case "mod":
			err = ttv.AddChannelModerator(ctx, channelID, user.ID)
			success = fmt.Sprintf("User %s is now a moderator", displayName)
		case "unmod":
			err = ttv.RemoveChannelModerator(ctx, channelID, user.ID)
			success = fmt.Sprintf("User %s is no longer a moderator", displayName)
		case "vip":
			err = ttv.AddChannelVIP(ctx, channelID, user.ID)
			success = fmt.Sprintf("User %s is now a VIP", displayName)
		case "unvip":
			err = ttv.RemoveChannelVIP(ctx, channelID, user.ID)
			success = fmt.Sprintf("User %s is no longer a VIP", displayName)
		}

		if err != nil {
			return noticeFunc(describeAPIError(err, name))()
		}

		return noticeFunc(success)()
	}
}

func handleWarn(args []string, channelID string, userAccountID string, ttv ModerationAPI, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
	username := firstArg(args)
	if username == "" {
		return noticeFunc("Expected Usage: /warn <username> [reason]")
	}

	reason := argsFrom(args, 1)
	if reason == "" {
		reason = "No reason provided"
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		user, notice := findUser(ctx, ttv, username)
		if notice != "" {
			return noticeFunc(notice)()
		}

		if err := ttv.WarnChatUser(ctx, channelID, userAccountID, user.ID, reason); err != nil {
			return noticeFunc(describeAPIError(err, "warn user"))()
		}

		displayName := preferredDisplayName(user.DisplayName, user.Login)

		return noticeFunc(fmt.Sprintf("User %s has been warned because: %s", displayName, reason))()
	}
}

func handleRaid(name string, args []string, channelID string, ttv ModerationAPI, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
	if name == "unraid" {
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := ttv.CancelRaid(ctx, channelID); err != nil {
				return noticeFunc(describeAPIError(err, "cancel raid"))()
			}

			return noticeFunc("The pending raid has been canceled")()
		}
	}

	target := firstArg(args)
	if target == "" {
		return noticeFunc("Expected Usage: /raid <channel>")
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		user, notice := findUser(ctx, ttv, target)
		if notice != "" {
			return noticeFunc(notice)()
		}

		if user.ID == channelID {
			return noticeFunc("A channel cannot raid itself")()
		}

		if _, err := ttv.StartRaid(ctx, channelID, user.ID); err != nil {
			return noticeFunc(describeAPIError(err, "start raid"))()
		}

		displayName := preferredDisplayName(user.DisplayName, user.Login)

		return noticeFunc(fmt.Sprintf("Raid started towards %s; it begins once the broadcaster confirms", displayName))()
	}
}

func handleShoutout(args []string, channelID string, userAccountID string, ttv ModerationAPI, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
	target := firstArg(args)
	if target == "" {
		return noticeFunc("Expected Usage: /shoutout <channel>")
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		user, notice := findUser(ctx, ttv, target)
		if notice != "" {
			return noticeFunc(notice)()
		}

		if err := ttv.SendShoutout(ctx, channelID, user.ID, userAccountID); err != nil {
			return noticeFunc(describeAPIError(err, "send shoutout"))()
		}

		displayName := preferredDisplayName(user.DisplayName, user.Login)

		return noticeFunc(fmt.Sprintf("Sent a shoutout to %s", displayName))()
	}
}

func handleAutoModAction(name string, args []string, userAccountID string, ttv ModerationAPI, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
	messageID := firstArg(args)
	if messageID == "" {
		return noticeFunc(fmt.Sprintf("Expected Usage: /%s <message_id>", name))
	}

	action := "ALLOW"
	if name == "deny" {
		action = "DENY"
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := ttv.ManageAutoModMessage(ctx, userAccountID, messageID, action); err != nil {
			return noticeFunc(describeAPIError(err, "manage AutoMod message"))()
		}

		if action == "ALLOW" {
			return noticeFunc(fmt.Sprintf("AutoMod message %s approved", messageID))()
		}

		return noticeFunc(fmt.Sprintf("AutoMod message %s denied", messageID))()
	}
}

func handleSnooze(channelID string, ttv ModerationAPI, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := ttv.SnoozeNextAd(ctx, channelID)
		if err != nil {
			return noticeFunc(describeAPIError(err, "snooze the next ad"))()
		}

		if resp.NextAdAt == "" {
			return noticeFunc("The next ad has been snoozed")()
		}

		return noticeFunc(fmt.Sprintf("The next ad has been snoozed to %s", formatAPITime(resp.NextAdAt)))()
	}
}

func formatAPITime(value string) string {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}

	return parsed.Local().Format("02.01.2006 15:04:05")
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

	slowModeMinimum = 3 * time.Second
	slowModeMaximum = 120 * time.Second

	followerModeMaximum = 90 * 24 * time.Hour // 129600 minutes, Twitch's 3-month cap
)

// parseTimeoutDuration turns a typed duration into the value the API wants. An empty input
// yields the default, which is also what Twitch's own chat does when no duration is given.
func parseTimeoutDuration(input string) (time.Duration, error) {
	if strings.TrimSpace(input) == "" {
		return timeoutDefault, nil
	}

	return parseDuration(input, time.Second, timeoutMaximum, "2 weeks (2w)", false)
}

func parseSlowModeDuration(input string) (time.Duration, error) {
	if strings.TrimSpace(input) == "" {
		return 0, nil
	}

	// Twitch's slow-mode granularity is whole seconds: "/slow 10" means 10 seconds.
	duration, err := parseDuration(input, time.Second, slowModeMaximum, "120 seconds", false)
	if err != nil {
		return 0, err
	}

	if duration < slowModeMinimum {
		return 0, fmt.Errorf("must be at least 3 seconds")
	}

	return duration, nil
}

func parseFollowerModeDuration(input string) (time.Duration, error) {
	if strings.TrimSpace(input) == "" {
		return 0, nil
	}

	// Followers-only is configured in minutes; a bare number therefore means minutes,
	// unlike a timeout where a bare number is seconds.
	duration, err := parseDuration(input, time.Minute, followerModeMaximum, "3 months", true)
	if err != nil {
		return 0, err
	}

	if duration%time.Minute != 0 {
		return 0, fmt.Errorf("must be a whole number of minutes")
	}

	return duration, nil
}

// parseDuration reads the friendly, combinable syntax Twitch chat uses ("1h30m").
// defaultUnit decides what a segment without a unit means, which differs per
// setting, and allowZero permits "0m" for followers-only.
func parseDuration(input string, defaultUnit time.Duration, maximum time.Duration, maximumLabel string, allowZero bool) (time.Duration, error) {
	raw := strings.ToLower(strings.TrimSpace(input))

	var total time.Duration

	for i := 0; i < len(raw); {
		start := i
		for i < len(raw) && raw[i] >= '0' && raw[i] <= '9' {
			i++
		}

		if start == i {
			return 0, durationSyntaxError(input)
		}

		value, err := strconv.Atoi(raw[start:i])
		if err != nil {
			return 0, durationSyntaxError(input)
		}

		// The unit is optional on the last segment, where it uses the caller's default:
		// "/timeout x 600" keeps working for anyone who still counts in seconds.
		unit := defaultUnit

		if i < len(raw) {
			switch raw[i] {
			case 's':
				unit = time.Second
			case 'm':
				unit = time.Minute
			case 'h':
				unit = time.Hour
			case 'd':
				unit = 24 * time.Hour
			case 'w':
				unit = 7 * 24 * time.Hour
			default:
				return 0, durationSyntaxError(input)
			}

			i++
		}

		// Compare before multiplying: a large enough number overflows time.Duration, and
		// anything above the maximum is rejected anyway.
		if value > int(maximum/unit) {
			return 0, durationMaximumError(input, maximumLabel)
		}

		total += time.Duration(value) * unit
		if total > maximum {
			return 0, durationMaximumError(input, maximumLabel)
		}
	}

	if total < 0 || (!allowZero && total == 0) {
		return 0, fmt.Errorf("must be greater than zero, got %s", strings.TrimSpace(input))
	}

	return total, nil
}

// The errors stay lowercase and free of a sentence frame, so the notice decides how they
// are shown and the API layer could reuse them unchanged.
func durationSyntaxError(input string) error {
	return fmt.Errorf("could not parse %s, expected something like 90s, 10m, 1h30m, 2d or 1w", strings.TrimSpace(input))
}

func durationMaximumError(input string, maximumLabel string) error {
	return fmt.Errorf("%s is longer than the maximum of %s", strings.TrimSpace(input), maximumLabel)
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
