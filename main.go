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
	"time"

	"github.com/julez-dev/chatuino/bttv"
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

type transportWithLogger struct {
	rt     http.RoundTripper
	logger zerolog.Logger
}

func (t *transportWithLogger) RoundTrip(req *http.Request) (*http.Response, error) {
	rt := t.rt

	if rt == nil {
		rt = http.DefaultTransport
	}

	now := time.Now()
	resp, err := rt.RoundTrip(req)

	if err != nil {
		t.logger.Error().Err(err).Msg("error while making request")
		return nil, err
	}

	dur := time.Since(now)
	t.logger.Info().Str("method", req.Method).Str("url", req.URL.String()).Dur("took", dur).Int("status", resp.StatusCode).Msg("request made")

	return resp, nil
}

// //go:generate go run github.com/vektra/mockery/v2@latest --dir=./ui/mainui/unbanrequest --name=unbanRequestService --testonly --output=./ui/mainui/unbanrequest --outpkg=unbanrequest --structname=unbanRequestServiceMock --with-expecter=true
//
//go:generate go run github.com/vektra/mockery/v2@latest --dir=./ui/mainui --with-expecter=true --all
func main() {
	f, err := setupLogFile()
	if err != nil {
		fmt.Println("error while opening log file: %w", err)
		os.Exit(1)
	}

	defer func() {
		_ = f.Close()
	}()

	logger := zerolog.New(f).With().Timestamp().Logger()
	log.Logger = logger

	app := &cli.Command{
		Name:        "Chatuino",
		Description: "Chatuino twitch IRC Client",
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
			serverCMD(zerolog.New(os.Stdout).With().Timestamp().Logger()),
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "client-id",
				Usage: "OAuth Client-ID",
				Value: defaultClientID,
			},
			&cli.StringFlag{
				Name:  "api-host",
				Usage: "Host of the Chatuino API",
				Value: "https://chatuino-server.onrender.com",
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
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			if command.Bool("enable-profiling") {
				runProfilingServer(ctx, logger, command.String("profiling-host"))
			}

			// Override the default http client transport to log requests
			transport := http.DefaultClient.Transport

			http.DefaultClient.Transport = &transportWithLogger{
				rt:     transport,
				logger: logger,
			}

			accountProvider := save.NewAccountProvider()
			serverAPI := server.NewClient(command.String("api-host"), http.DefaultClient)
			stvAPI := seventv.NewAPI(http.DefaultClient)
			bttvAPI := bttv.NewAPI(http.DefaultClient)

			emoteStore := emote.NewStore(logger, serverAPI, stvAPI, bttvAPI)

			// If the user has provided an account we can use the users local authentication
			// Instead of using Chatuino's server to handle requests for emote fetching.
			if mainAccount, err := accountProvider.GetMainAccount(); err == nil {
				ttvAPI, err := twitch.NewAPI(command.String("client-id"), twitch.WithUserAuthentication(accountProvider, serverAPI, mainAccount.ID))
				if err == nil {
					emoteStore = emote.NewStore(logger, ttvAPI, stvAPI, bttvAPI)
				}
			}

			keys, err := save.CreateReadKeyMap()

			if err != nil {
				return fmt.Errorf("error while reading keymap: %w", err)
			}

			p := tea.NewProgram(
				mainui.NewUI(logger, accountProvider, &emoteStore, command.String("client-id"), serverAPI, keys),
				tea.WithContext(ctx),
				tea.WithAltScreen(),
			)

			final, err := p.Run()
			if err != nil {
				return fmt.Errorf("error while running TUI: %w", err)
			}

			if final, ok := final.(*mainui.Root); ok {
				if err := final.Close(); err != nil && !errors.Is(err, context.Canceled) {
					return fmt.Errorf("error while closing TUI: %w", err)
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
