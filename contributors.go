package main

import (
	"context"
	"fmt"

	"github.com/julez-dev/chatuino/contributor"
	"github.com/urfave/cli/v3"
)

var contributorsCMD = &cli.Command{
	Name:        "contributors",
	Description: "List Chatuino contributors",
	Usage:       "List contributors to the Chatuino project",
	Action: func(_ context.Context, _ *cli.Command) error {
		contributors := contributor.GetAll()

		if len(contributors) == 0 {
			fmt.Println("No contributors found.")
			return nil
		}

		for _, c := range contributors {
			fmt.Printf("GitHub: %s, Email: %s", c.GitHubUser, c.Email)
			if c.TwitchLogin != "" {
				fmt.Printf(", Twitch: %s", c.TwitchLogin)
			}
			fmt.Println()
		}

		return nil
	},
}
