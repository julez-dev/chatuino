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
			Value: "https://chatuino.net",
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
			return fmt.Errorf("failed to read keymap file: %w", err)
		}

		theme, err := save.ThemeFromDisk()
		if err != nil {
			return fmt.Errorf("failed to read theme file: %w", err)
		}

		p := tea.NewProgram(
			accountui.NewList(command.String("client-id"), command.String("api-host"), save.NewAccountProvider(save.NewKeyringWrapper()), keys, theme),
			tea.WithContext(ctx),
			tea.WithAltScreen(),
		)

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("error while running TUI: %w", err)
		}

		return nil
	},
}
