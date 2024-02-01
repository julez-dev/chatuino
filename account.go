package main

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/accountui"
	"github.com/urfave/cli/v3"
)

var accountCMD = &cli.Command{
	Name:        "account",
	Description: "Chatuino account management",
	Usage:       "Manage accounts used by Chatuino",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "api-host",
			Usage: "Host of the Chatuino API",
			Value: "https://chatuino-server.onrender.com",
		},
		&cli.StringFlag{
			Name:  "client-id",
			Usage: "OAuth Client-ID",
			Value: defaultClientID,
		},
	},
	Action: func(ctx context.Context, command *cli.Command) error {
		keys, err := save.CreateReadKeyMap()

		if err != nil {
			return fmt.Errorf("error while reading keymap: %w", err)
		}

		p := tea.NewProgram(
			accountui.NewList(command.String("client-id"), command.String("api-host"), save.NewAccountProvider(), keys),
			tea.WithContext(ctx),
			tea.WithAltScreen(),
		)

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("error while running TUI: %w", err)
		}

		return nil
	},
}
