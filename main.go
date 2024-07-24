package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof"
	"net/mail"
	"os"
	"os/signal"
	"syscall"

	"github.com/julez-dev/chatuino/bttv"
	"github.com/julez-dev/chatuino/httputil"
	"github.com/julez-dev/chatuino/multiplex"
	"github.com/rs/zerolog/log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cli/browser"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/seventv"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/ui/mainui"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"
)

func init() {
	browser.Stderr = io.Discard
	browser.Stdout = io.Discard
}

const (
	defaultClientID = "jliqj1q6nmp0uh5ofangdx4iac7yd9"
	logFileName     = "log.txt"
)

var maybeLogFile *os.File

//go:generate go run github.com/vektra/mockery/v2@latest --dir=./ui/mainui --with-expecter=true --all
func main() {
	defer func() {
		if maybeLogFile != nil {
			maybeLogFile.Close()
		}
	}()

	app := &cli.Command{
		Name:        "Chatuino",
		Description: "Chatuino twitch IRC Client. Before using Chatuino you may want to manage your accounts using the account command.",
		Usage:       "A client for twitch's IRC service",
		// HideVersion: true,
		Authors: []any{
			&mail.Address{
				Name:    "julez-dev",
				Address: "julez-dev@pm.me",
			},
		},
		Commands: []*cli.Command{
			versionCMD,
			accountCMD,
			serverCMD,
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "client-id",
				Usage:   "OAuth Client-ID",
				Value:   defaultClientID,
				Sources: cli.EnvVars("CHATUINO_CLIENT_ID"),
			},
			&cli.StringFlag{
				Name:    "api-host",
				Usage:   "Host of the Chatuino API",
				Value:   "https://chatuino.net",
				Sources: cli.EnvVars("CHATUINO_API_HOST"),
			},
			&cli.BoolFlag{
				Name:  "enable-profiling",
				Usage: "If profiling should be enabled",
				Value: false,
			},
			&cli.StringFlag{
				Name:  "profiling-host",
				Usage: "Host of the profiling http server",
				Value: "0.0.0.0:6060",
			},
			&cli.BoolFlag{
				Name:  "log",
				Usage: "If the application should log",
			},
			&cli.BoolFlag{
				Name:  "log-to-file",
				Usage: "If the application should log to log file instead of stderr",
			},
			&cli.BoolFlag{
				Name:  "human-readable",
				Usage: "If the log should be human readable",
			},
		},
		Before: beforeAction,
		Action: func(ctx context.Context, command *cli.Command) error {
			if command.Bool("enable-profiling") {
				runProfilingServer(ctx, log.Logger, command.String("profiling-host"))
			}

			// Override the default http client transport to log requests
			transport := http.DefaultClient.Transport
			http.DefaultClient.Transport = httputil.NewChatuinoRoundTrip(transport, log.Logger, Version)

			accountProvider := save.NewAccountProvider(save.KeyringWrapper{})
			serverAPI := server.NewClient(command.String("api-host"), http.DefaultClient)
			stvAPI := seventv.NewAPI(http.DefaultClient)
			bttvAPI := bttv.NewAPI(http.DefaultClient)
			multiplexer := multiplex.NewMultiplexer(log.Logger, accountProvider)

			emoteStore := emote.NewStore(log.Logger, serverAPI, stvAPI, bttvAPI)

			// If the user has provided an account we can use the users local authentication
			// Instead of using Chatuino's server to handle requests for emote fetching.
			if mainAccount, err := accountProvider.GetMainAccount(); err == nil {
				ttvAPI, err := twitch.NewAPI(command.String("client-id"), twitch.WithUserAuthentication(accountProvider, serverAPI, mainAccount.ID))
				if err == nil {
					emoteStore = emote.NewStore(log.Logger, ttvAPI, stvAPI, bttvAPI)
				}
			}

			keys, err := save.CreateReadKeyMap()

			if err != nil {
				return fmt.Errorf("error while reading keymap: %w", err)
			}

			p := tea.NewProgram(
				mainui.NewUI(log.Logger, accountProvider, multiplexer, emoteStore, command.String("client-id"), serverAPI, keys),
				tea.WithContext(ctx),
				tea.WithAltScreen(),
				tea.WithFPS(120),
			)

			final, err := p.Run()
			if err != nil {
				return err
			}

			if final, ok := final.(*mainui.Root); ok {
				if err := final.Close(); err != nil && !errors.Is(err, context.Canceled) {
					return err
				}

				state := final.TakeStateSnapshot()

				if err := state.Save(); err != nil {
					return fmt.Errorf("error while saving state: %w", err)
				}
			}

			return nil
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := app.Run(ctx, os.Args); err != nil {
		fmt.Printf("error while running Chatuino: %v", err)
		os.Exit(1)
	}
}

func beforeAction(ctx context.Context, command *cli.Command) error {
	// Setup logging
	//  - If logging not enabled, skip
	//  - If log-to-file is enabled, log to file, else stderr
	//  - If human-readable is enabled, log in human readable format (disable colors if log-to-file is enabled)
	// This action runs before any command is executed, including sub commands, but will run for all sub commands

	if !command.Bool("log") {
		log.Logger = zerolog.Nop()
		return nil
	}

	shouldLogToFile := command.Bool("log-to-file")

	var logFile *os.File
	if shouldLogToFile {
		f, err := setupLogFile()
		if err != nil {
			return fmt.Errorf("error while opening log file: %w", err)
		}

		maybeLogFile = f
		logFile = f
	} else {
		logFile = os.Stderr
	}

	if command.Bool("human-readable") {
		log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: logFile, NoColor: shouldLogToFile}).With().Timestamp().Logger()
	} else {
		log.Logger = zerolog.New(logFile).With().Timestamp().Logger()
	}

	return nil
}

func runProfilingServer(ctx context.Context, logger zerolog.Logger, host string) {
	srv := &http.Server{
		Addr: host,
	}

	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10)
		defer cancel()

		logger.Info().Msg("shutting down profiling server")
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error().Err(err).Msg("error while shutting down profiling server")
			os.Exit(1)
		}
	}()

	go func() {
		logger.Info().Str("host", host).Msg("running profiling server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("error while running profiling server: %v", err)
			logger.Error().Err(err).Msg("error while running profiling server")
			os.Exit(1)
		}
	}()
}

func setupLogFile() (*os.File, error) {
	f, err := os.OpenFile(logFileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}

	return f, nil
}
