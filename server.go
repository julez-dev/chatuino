package main

import (
	"context"
	"net/http"

	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"
)

func serverCMD(logger zerolog.Logger) *cli.Command {
	return &cli.Command{
		Name:  "server",
		Usage: "Start the chatuino server",
		Description: "Starts the chatuino which is responsible for proxying requests to the twitch API " +
			"which require an app access token",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "addr",
				Usage:   "The address the server should listen at",
				Value:   ":8080",
				Sources: cli.EnvVars("TUI_ADDR"),
			},
			&cli.StringFlag{
				Name:  "redirect-url",
				Usage: "The URL twitch will redirect to",
				Value: "http://localhost:8080/auth/redirect",
			},
			&cli.StringFlag{
				Name:     "client-id",
				Usage:    "OAuth Client-ID",
				Sources:  cli.EnvVars("TWITCH_CLIENT_ID"),
				Required: true,
			},
			&cli.StringFlag{
				Name:     "client-secret",
				Usage:    "OAuth Client-Secret",
				Sources:  cli.EnvVars("TWITCH_CLIENT_SECRET"),
				Required: true,
			},
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			ttvAPI, err := twitch.NewAPI(command.String("client-id"),
				twitch.WithHTTPClient(http.DefaultClient),
				twitch.WithClientSecret(command.String("client-secret")),
			)
			if err != nil {
				logger.Err(err).Msg("could not create new twitch API client")
				return err
			}

			api := server.New(
				logger,
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
}
