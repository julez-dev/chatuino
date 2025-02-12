package mainui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/twitch/command"
)

func handleCommand(name string, args []string, channelID string, channel string, userAccountID string, ttv moderationAPIClient) tea.Cmd {
	switch name {
	case "timeout":
		return handleTimeout(name, args, channelID, channel, userAccountID, ttv)
	case "ban":
		return handleTimeout(name, args, channelID, channel, userAccountID, ttv)
	case "unban":
		return handleUnban(args, channel, channelID, userAccountID, ttv)
	case "announcement":
		return handleAnnouncement(args, channel, channelID, userAccountID, ttv)
	}

	return nil
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

	if args[1] == "" {
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
			Message: args[1],
		})

		notice.FakeTimestamp = time.Now()

		if err != nil {
			var apiErr *twitch.APIError
			if !errors.As(err, &apiErr) {
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

	if len(args) < 1 && name == "timeout" {
		return func() tea.Msg {
			respMsg.message.(*command.Notice).Message = "Expected Usage: /timeout <username> [duration] [reason]"
			return respMsg
		}
	}

	if len(args) < 1 && name == "ban" {
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
		if name == "timeout" {
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

		if name == "ban" {
			respMsg.message.(*command.Notice).Message = fmt.Sprintf("User %s received a ban by you because: %s", users.Data[0].DisplayName, args[2])
			return respMsg
		}

		respMsg.message.(*command.Notice).Message = fmt.Sprintf("User %s received a timeout by you for %d seconds because: %s", users.Data[0].DisplayName, duration, args[2])
		return respMsg
	}
}
