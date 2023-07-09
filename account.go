package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/julez-dev/chatuino/ui/accountui"
	"github.com/urfave/cli/v2"
)

var accountCMD = &cli.Command{
	Name:        "account",
	Description: "Chatuino account management",
	Usage:       "Manage accounts used by Chatuino",
	Action: func(ctx *cli.Context) error {
		p := tea.NewProgram(
			accountui.NewList(),
			tea.WithContext(ctx.Context),
			tea.WithAltScreen(),
		)

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("error while running TUI: %w", err)
		}

		return nil
	},
}
