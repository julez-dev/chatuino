package mainui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
)

func handleCommand(name string, args []string, channelID string, channel string, userAccountID string, ttv moderationAPIClient) tea.Cmd {
	noticeCommandFunc := func(msg string) tea.Cmd {
		return func() tea.Msg {
			return chatEventMessage{
				accountID: userAccountID,
				channel:   channel,
				channelID: channelID,
				message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					MsgID:         twitchirc.MsgID(uuid.New().String()),
					Message:       msg,
				},
				isFakeEvent: true,
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

func handleMarker(args []string, channelID string, channel string, userAccountID string, ttv moderationAPIClient, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
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

func handleAnnouncement(args []string, channel string, channelID string, userAccountID string, ttv moderationAPIClient, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
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

func handleUnban(args []string, channel string, channelID string, userAccountID string, ttv moderationAPIClient, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
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

func handleTimeout(name string, args []string, channelID string, channel string, userAccountID string, ttv moderationAPIClient, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
	if len(args) < 1 && (name == "timeout" || name == "timeout_selected") {
		return noticeFunc("Expected Usage: /timeout <username> [duration] [reason]")
	}

	if len(args) < 1 && (name == "ban" || name == "ban_selected") {
		return noticeFunc("Expected Usage: /ban <username> [reason]")
	}

	// fill up possibly missing arguments
	if len(args) < 3 {
		fillArgs := make([]string, 3-len(args))
		args = append(args, fillArgs...)
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

		var duration int

		// parse duration for timeouts
		// if timeout is not set, default to 1 second
		if name == "timeout" || name == "timeout_selected" {
			if args[1] == "" {
				duration = 1
			} else {
				var err error
				duration, err = strconv.Atoi(args[1])
				if err != nil {
					return noticeFunc(fmt.Sprintf("Could not convert %s to integer: %s", args[1], err.Error()))()
				}

				if duration < 1 {
					duration = 1
				}
			}
		}

		err = ttv.BanUser(ctx, channelID, userAccountID, twitchapi.BanUserData{
			UserID:            users.Data[0].ID,
			DurationInSeconds: duration,
			Reason:            args[2],
		})
		if err != nil {
			return noticeFunc(fmt.Sprintf("Error while sending ban request: %s", err.Error()))()
		}

		if name == "ban" || name == "ban_selected" {
			return noticeFunc(fmt.Sprintf("User %s received a ban by you because: %s", users.Data[0].DisplayName, args[2]))()
		}

		return noticeFunc(fmt.Sprintf("User %s received a timeout by you for %d seconds because: %s", users.Data[0].DisplayName, duration, args[2]))()
	}
}

func handleDeleteMessages(name string, args []string, channel string, channelID string, userAccountID string, ttv moderationAPIClient, noticeFunc func(msg string) tea.Cmd) tea.Cmd {
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
