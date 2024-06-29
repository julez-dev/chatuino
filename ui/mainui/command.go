package mainui

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/twitch/command"
	"strconv"
)

func handleCommand(ctx context.Context, name string, args []string, channelID string, channel string, userAccountID string, ttv moderationAPIClient) tea.Cmd {
	switch name {
	case "timeout":
		return handleTimeout(ctx, args, channelID, channel, userAccountID, ttv)
	}

	return nil
}

func handleTimeout(ctx context.Context, args []string, channelID string, channel string, userAccountID string, ttv moderationAPIClient) tea.Cmd {
	respMsg := chatEventMessage{
		accountID: userAccountID,
		channel:   channel,
		message:   &command.Notice{},
	}

	if len(args) < 2 {
		return func() tea.Msg {
			respMsg.message.(*command.Notice).Message = "Expected Usage: /timeout <username> <duration> [reason]"
			return respMsg
		}
	}

	if len(args) == 2 {
		args = append(args, "")
	}

	return func() tea.Msg {
		users, err := ttv.GetUsers(ctx, []string{args[0]}, nil)

		if err != nil {
			respMsg.message.(*command.Notice).Message = fmt.Sprintf("Error while fetching user ID %s: %s", args[0], err.Error())
			return respMsg
		}

		if len(users.Data) < 1 {
			respMsg.message.(*command.Notice).Message = fmt.Sprintf("User %s can not be found", args[0])
			return respMsg
		}

		duration, err := strconv.Atoi(args[1])

		if err != nil {
			respMsg.message.(*command.Notice).Message = fmt.Sprintf("Could not convert %s to integer: %s", args[1], err.Error())
			return respMsg
		}

		err = ttv.BanUser(ctx, channelID, twitch.BanUserData{
			UserID:            users.Data[0].ID,
			DurationInSeconds: duration,
			Reason:            args[2],
		})

		if err != nil {
			respMsg.message.(*command.Notice).Message = fmt.Sprintf("Error while sending ban request: %s", err.Error())
			return respMsg
		}

		respMsg.message.(*command.Notice).Message = fmt.Sprintf("User %s received a timeout by you for %d seconds because: %s", users.Data[0].DisplayName, duration, args[2])
		return respMsg
	}
}
