package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
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
			Value: "http://localhost:8080",
		},
		&cli.StringFlag{
			Name:     "client-id",
			Usage:    "OAuth Client-ID",
			Sources:  cli.EnvVars("TWITCH_CLIENT_ID"),
			Required: true,
		},
	},
	Action: func(ctx *cli.Context) error {
		p := tea.NewProgram(
			accountui.NewList(ctx.String("client-id"), ctx.String("api-host")),
			tea.WithContext(ctx.Context),
			tea.WithAltScreen(),
		)

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("error while running TUI: %w", err)
		}

		return nil
	},
}
