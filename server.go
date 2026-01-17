package main

import (
	"context"
	"net/http"

	"github.com/julez-dev/chatuino/server"
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
		&cli.StringFlag{
			Name:    "redis-addr",
			Usage:   "Redis server address (host:port). Leave empty to disable Redis.",
			Sources: cli.EnvVars("CHATUINO_REDIS_ADDR"),
		},
		&cli.StringFlag{
			Name:    "redis-password",
			Usage:   "Redis password (optional)",
			Sources: cli.EnvVars("CHATUINO_REDIS_PASSWORD"),
		},
		&cli.IntFlag{
			Name:    "redis-db",
			Usage:   "Redis database number",
			Value:   0,
			Sources: cli.EnvVars("CHATUINO_REDIS_DB"),
		},
		&cli.BoolFlag{
			Name:    "enable-ratelimit",
			Usage:   "If proxy ratelimiting should be set",
			Sources: cli.EnvVars("CHATUINO_PROXY_RATELIMIT"),
		},
	},
	Action: func(ctx context.Context, command *cli.Command) error {
		redisDB := command.Int("redis-db")
		if redisDB < 0 || redisDB > 15 {
			log.Warn().Int("db", redisDB).Msg("Invalid Redis DB number, using 0")
			redisDB = 0
		}

		api := server.New(
			log.Logger,
			server.Config{
				HostAndPort:          command.String("addr"),
				ClientID:             command.String("client-id"),
				ClientSecret:         command.String("client-secret"),
				RedirectURL:          command.String("redirect-url"),
				EnableProxyRateLimit: command.Bool("enable-ratelimit"),
				Redis: server.RedisConfig{
					Addr:     command.String("redis-addr"),
					Password: command.String("redis-password"),
					DB:       redisDB,
				},
			},
			http.DefaultClient,
		)

		if err := api.Launch(ctx); err != nil {
			return err
		}

		return nil
	},
}
