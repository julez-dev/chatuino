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
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/twitch/command"
)

func handleCommand(name string, args []string, channelID string, channel string, userAccountID string, ttv moderationAPIClient) tea.Cmd {
	switch name {
	case "timeout", "timeout_selected":
		return handleTimeout(name, args, channelID, channel, userAccountID, ttv)
	case "ban", "ban_selected":
		return handleTimeout(name, args, channelID, channel, userAccountID, ttv)
	case "unban", "unban_selected":
		return handleUnban(args, channel, channelID, userAccountID, ttv)
	case "delete_all_messages", "delete_selected_message":
		return handleDeleteMessages(name, args, channel, channelID, userAccountID, ttv)
	case "announcement":
		return handleAnnouncement(args, channel, channelID, userAccountID, ttv)
	case "marker":
		return handleMarker(args, channelID, channel, userAccountID, ttv)
	}

	return nil
}

func handleMarker(args []string, channelID string, channel string, userAccountID string, ttv moderationAPIClient) tea.Cmd {
	notice := &command.Notice{
		FakeTimestamp: time.Now(),
	}
	respMsg := chatEventMessage{
		accountID:   userAccountID,
		channel:     channel,
		message:     notice,
		isFakeEvent: true,
	}

	var description string
	if len(args) > 0 {
		description = strings.Join(args, " ")
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := ttv.CreateStreamMarker(ctx, twitch.CreateStreamMarkerRequest{
			UserID:      channelID,
			Description: description,
		})

		notice.FakeTimestamp = time.Now()

		if err != nil {
			var apiErr twitch.APIError
			if errors.As(err, &apiErr) {
				switch apiErr.Status {
				case http.StatusBadRequest:
					notice.Message = fmt.Sprintf("Marker request invalid: %s", apiErr.Message)
				case http.StatusUnauthorized, http.StatusForbidden:
					notice.Message = fmt.Sprintf("Unauthorized to create markers: %s", apiErr.Message)
				case http.StatusNotFound:
					notice.Message = fmt.Sprintf("%s not live or has VODs disabled: %s", channel, apiErr.Message)
				}
				return respMsg
			}

			notice.Message = fmt.Sprintf("Failed to create marker: %s", err.Error())
			return respMsg
		}

		dur := time.Duration(int(time.Second) * resp.PositionSeconds)
		notice.Message = fmt.Sprintf("Marker (%s) created at %s; stream time: %s", resp.ID, resp.CreatedAt.Local().Format("02.01.2006 15:04:05"), dur)

		return respMsg
	}
}

func handleAnnouncement(args []string, channel string, channelID string, userAccountID string, ttv moderationAPIClient) tea.Cmd {
	notice := &command.Notice{
		FakeTimestamp: time.Now(),
	}
	respMsg := chatEventMessage{
		accountID:   userAccountID,
		channel:     channel,
		message:     notice,
		isFakeEvent: true,
	}

	if len(args) < 2 {
		return func() tea.Msg {
			notice.Message = "Expected Usage: /announcement <blue|green|orange|purple|primary> <message>"
			return respMsg
		}
	}

	color := args[0]

	allowed := []string{
		string(twitch.ChatAnnouncementColorBlue),
		string(twitch.ChatAnnouncementColorGreen),
		string(twitch.ChatAnnouncementColorOrange),
		string(twitch.ChatAnnouncementColorPurple),
		string(twitch.ChatAnnouncementColorPrimary),
	}

	if !slices.Contains(allowed, color) {
		return func() tea.Msg {
			notice.Message = fmt.Sprintf("Invalid color %s, expected one of %s", color, allowed)
			return respMsg
		}
	}

	annoucementMessage := strings.Join(args[1:], " ")

	if annoucementMessage == "" {
		return func() tea.Msg {
			notice.Message = "Message can not be empty"
			return respMsg
		}
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := ttv.SendChatAnnouncement(ctx, channelID, userAccountID, twitch.CreateChatAnnouncementRequest{
			Color:   twitch.ChatAnnouncementColor(color),
			Message: annoucementMessage,
		})

		notice.FakeTimestamp = time.Now()

		if err != nil {
			var apiErr twitch.APIError
			if errors.As(err, &apiErr) {
				switch apiErr.Status {
				case http.StatusBadRequest:
					notice.Message = fmt.Sprintf("Announcement request invalid: %s", apiErr.Message)
				case http.StatusUnauthorized:
					notice.Message = fmt.Sprintf("Unauthorized to send announcement: %s", apiErr.Message)
				case http.StatusTooManyRequests:
					notice.Message = fmt.Sprintf("Exceeded the number of announcements for %s: %s", channel, apiErr.Message)
				}
				return respMsg
			}

			notice.Message = fmt.Sprintf("Error while sending announcement: %s", err.Error())
			return respMsg
		}

		return nil
	}
}

func handleUnban(args []string, channel string, channelID string, userAccountID string, ttv moderationAPIClient) tea.Cmd {
	respMsg := chatEventMessage{
		isFakeEvent: true,
		accountID:   userAccountID,
		channel:     channel,
		message: &command.Notice{
			FakeTimestamp: time.Now(),
		},
	}

	if len(args) < 1 {
		return func() tea.Msg {
			respMsg.message.(*command.Notice).Message = "Expected Usage: /unban <username>"
			return respMsg
		}
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		users, err := ttv.GetUsers(ctx, []string{args[0]}, nil)
		if err != nil {
			respMsg.message.(*command.Notice).Message = fmt.Sprintf("Error while fetching user ID %s: %s", args[0], err.Error())
			return respMsg
		}

		if len(users.Data) < 1 {
			respMsg.message.(*command.Notice).Message = fmt.Sprintf("User %s can not be found", args[0])
			return respMsg
		}

		err = ttv.UnbanUser(ctx, channelID, userAccountID, users.Data[0].ID)
		if err != nil {
			respMsg.message.(*command.Notice).Message = fmt.Sprintf("Error while sending unban request: %s", err.Error())
			return respMsg
		}

		respMsg.message.(*command.Notice).Message = fmt.Sprintf("User %s received an unban by you", users.Data[0].DisplayName)
		return respMsg
	}
}

func handleTimeout(name string, args []string, channelID string, channel string, userAccountID string, ttv moderationAPIClient) tea.Cmd {
	respMsg := chatEventMessage{
		accountID: userAccountID,
		channel:   channel,
		message:   &command.Notice{},
	}

	if len(args) < 1 && (name == "timeout" || name == "timeout_selected") {
		return func() tea.Msg {
			respMsg.message.(*command.Notice).Message = "Expected Usage: /timeout <username> [duration] [reason]"
			return respMsg
		}
	}

	if len(args) < 1 && (name == "ban" || name == "ban_selected") {
		return func() tea.Msg {
			respMsg.message.(*command.Notice).Message = "Expected Usage: /ban <username> [reason]"
			return respMsg
		}
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
			respMsg.message.(*command.Notice).Message = fmt.Sprintf("Error while fetching user ID %s: %s", args[0], err.Error())
			return respMsg
		}

		if len(users.Data) < 1 {
			respMsg.message.(*command.Notice).Message = fmt.Sprintf("User %s can not be found", args[0])
			return respMsg
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
					respMsg.message.(*command.Notice).Message = fmt.Sprintf("Could not convert %s to integer: %s", args[1], err.Error())
					return respMsg
				}

				if duration < 1 {
					duration = 1
				}
			}
		}

		err = ttv.BanUser(ctx, channelID, userAccountID, twitch.BanUserData{
			UserID:            users.Data[0].ID,
			DurationInSeconds: duration,
			Reason:            args[2],
		})
		if err != nil {
			respMsg.message.(*command.Notice).Message = fmt.Sprintf("Error while sending ban request: %s", err.Error())
			return respMsg
		}

		if name == "ban" || name == "ban_selected" {
			respMsg.message.(*command.Notice).Message = fmt.Sprintf("User %s received a ban by you because: %s", users.Data[0].DisplayName, args[2])
			return respMsg
		}

		respMsg.message.(*command.Notice).Message = fmt.Sprintf("User %s received a timeout by you for %d seconds because: %s", users.Data[0].DisplayName, duration, args[2])
		return respMsg
	}
}

func handleDeleteMessages(name string, args []string, channel string, channelID string, userAccountID string, ttv moderationAPIClient) tea.Cmd {
	notice := &command.Notice{
		FakeTimestamp: time.Now(),
	}
	respMsg := chatEventMessage{
		accountID:   userAccountID,
		channel:     channel,
		message:     notice,
		isFakeEvent: true,
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var messageID string
		if name == "delete_selected_message" {
			if len(args) < 1 {
				notice.Message = "Expected Usage: /delete_selected_message <message_id>"
				return respMsg
			}
			messageID = args[0]
		}

		err := ttv.DeleteMessage(ctx, channelID, userAccountID, messageID)
		if err != nil {
			var apiErr twitch.APIError
			if errors.As(err, &apiErr) {
				switch apiErr.Status {
				case http.StatusBadRequest:
					notice.Message = fmt.Sprintf("Delete message request invalid: %s", apiErr.Message)
				case http.StatusUnauthorized, http.StatusForbidden:
					notice.Message = fmt.Sprintf("Unauthorized to delete message(s): %s", apiErr.Message)
				case http.StatusNotFound:
					notice.Message = fmt.Sprintf("Message not found: %s", apiErr.Message)
				default:
					notice.Message = fmt.Sprintf("Failed to delete message(s): %s", apiErr.Message)
				}
				return respMsg
			}
			notice.Message = fmt.Sprintf("Failed to delete message(s): %s", err.Error())
			return respMsg
		}

		if name == "delete_selected_message" {
			notice.Message = fmt.Sprintf("Message %s deleted.", messageID)
		} else {
			notice.Message = "All messages deleted."
		}
		return respMsg
	}
}
