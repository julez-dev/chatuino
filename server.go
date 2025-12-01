package main

import (
	"context"
	"net/http"

	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

var serverCMD = &cli.Command{
	Name:  "server",
	Usage: "Start the chatuino server",
	Description: "Starts the chatuino which is responsible for proxying requests to the twitch API " +
		"which require an app access token",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "addr",
			Usage:   "The address the server should listen at",
			Value:   ":8080",
			Sources: cli.EnvVars("CHATUINO_ADDR"),
		},
		&cli.StringFlag{
			Name:    "redirect-url",
			Usage:   "The URL twitch will redirect to",
			Value:   "https://chatuino.net/auth/redirect",
			Sources: cli.EnvVars("CHATUINO_REDIRECT_URL"),
		},
		&cli.StringFlag{
			Name:     "client-id",
			Usage:    "OAuth Client-ID",
			Sources:  cli.EnvVars("CHATUINO_CLIENT_ID"),
			Required: true,
		},
		&cli.StringFlag{
			Name:     "client-secret",
			Usage:    "OAuth Client-Secret",
			Sources:  cli.EnvVars("CHATUINO_CLIENT_SECRET"),
			Required: true,
		},
	},
	Action: func(ctx context.Context, command *cli.Command) error {
		ttvAPI, err := twitchapi.NewAPI(command.String("client-id"),
			twitchapi.WithHTTPClient(http.DefaultClient),
			twitchapi.WithClientSecret(command.String("client-secret")),
		)
		if err != nil {
			log.Err(err).Msg("could not create new twitch API client")
			return err
		}

		api := server.New(
			log.Logger,
			server.Config{
				HostAndPort:  command.String("addr"),
				ClientID:     command.String("client-id"),
				ClientSecret: command.String("client-secret"),
				RedirectURL:  command.String("redirect-url"),
			},
			http.DefaultClient,
			ttvAPI,
		)

		if err := api.Launch(ctx); err != nil {
			return err
		}

		return nil
	},
}
