package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	_ "net/http/pprof"
	"net/mail"
	"os"
	"os/signal"
	"syscall"

	"github.com/adrg/xdg"
	"github.com/julez-dev/chatuino/httputil"
	"github.com/julez-dev/chatuino/multiplex"
	"github.com/julez-dev/chatuino/save/messagelog"
	"github.com/julez-dev/chatuino/twitch/bttv"
	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/julez-dev/chatuino/twitch/recentmessage"
	"github.com/rs/zerolog/log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cli/browser"

	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/twitch"
	ttvCommand "github.com/julez-dev/chatuino/twitch/command"
	"github.com/julez-dev/chatuino/twitch/seventv"
	"github.com/julez-dev/chatuino/ui/mainui"
	_ "github.com/mailru/easyjson"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"
	_ "modernc.org/sqlite"
)

func init() {
	browser.Stderr = io.Discard
	browser.Stdout = io.Discard
}

const (
	defaultClientID = "jliqj1q6nmp0uh5ofangdx4iac7yd9"
)

var (
	dataDir     = xdg.DataHome + "/chatuino"
	logFileName = dataDir + "/chatuino.log"
	dbFileName  = dataDir + "/chatuino.db"
)

var maybeLogFile *os.File

//go:generate go run github.com/vektra/mockery/v2@latest --dir=./ui/mainui --dir=./emote --dir=./save/messagelog --with-expecter=true --all
//go:generate go run github.com/mailru/easyjson/easyjson@latest -snake_case -no_std_marshalers -pkg ./twitch/command
//go:generate go run github.com/mailru/easyjson/easyjson@latest -snake_case -no_std_marshalers -pkg ./emote
//go:generate go run github.com/mailru/easyjson/easyjson@latest -snake_case -pkg ./twitch
//go:generate go run github.com/mailru/easyjson/easyjson@latest -snake_case -pkg ./twitch/recentmessage
func main() {
	defer func() {
		if maybeLogFile != nil {
			_ = maybeLogFile.Sync()
			_ = maybeLogFile.Close()
		}
	}()

	app := &cli.Command{
		Name:        "Chatuino",
		Description: "Chatuino twitch IRC Client. Before using Chatuino you may want to manage your accounts using the account command.",
		Usage:       "A client for twitch's IRC service",
		Version:     Version,
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
			rebuildCacheCMD,
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

			settings, err := save.SettingsFromDisk()
			if err != nil {
				return fmt.Errorf("failed to read settings file: %w", err)
			}

			theme, err := save.ThemeFromDisk()
			if err != nil {
				return fmt.Errorf("failed to read theme file: %w", err)
			}

			keymap, err := save.CreateReadKeyMap()
			if err != nil {
				return fmt.Errorf("failed to read keymap file: %w", err)
			}

			accountProvider := save.NewAccountProvider(save.NewKeyringWrapper())
			serverAPI := server.NewClient(command.String("api-host"), http.DefaultClient)
			stvAPI := seventv.NewAPI(http.DefaultClient)
			bttvAPI := bttv.NewAPI(http.DefaultClient)
			recentMessageService := recentmessage.NewAPI(http.DefaultClient)
			chatMultiplexer := multiplex.NewChatMultiplexer(log.Logger, accountProvider)
			eventSubMultiplexer := multiplex.NewEventMultiplexer(log.Logger)
			emoteStore := emote.NewStore(log.Logger, serverAPI, stvAPI, bttvAPI)

			// message logger setup
			db, err := openDB(false)
			if err != nil {
				return fmt.Errorf("failed to open sqlite db: %w", err)
			}

			roDB, err := openDB(true)
			if err != nil {
				return fmt.Errorf("failed to open readonly sqlite db: %w", err)
			}

			defer func() {
				if err := db.Close(); err != nil {
					log.Logger.Err(err).Msg("failed to close db connection")
				}

				if err := roDB.Close(); err != nil {
					log.Logger.Err(err).Msg("failed to close db connection")
				}
			}()

			messageLogger := messagelog.NewBatchedMessageLogger(log.Logger, db, roDB, settings.Moderation.LogsChannelInclude, settings.Moderation.LogsChannelExclude)
			messageLoggerChan := make(chan *ttvCommand.PrivateMessage)
			loggerWaitSync := make(chan struct{})

			if err := messageLogger.PrepareDatabase(); err != nil {
				log.Logger.Err(err).Msg("failed to run prepare queries")
				return fmt.Errorf("failed to migrate db: %w", err)
			}

			go runChatLogger(messageLogger, messageLoggerChan, loggerWaitSync, settings.Moderation.StoreChatLogs)

			// If the user has provided an account we can use the users local authentication
			// Instead of using Chatuino's server to handle requests for emote fetching.
			if mainAccount, err := accountProvider.GetMainAccount(); err == nil {
				ttvAPI, err := twitch.NewAPI(command.String("client-id"), twitch.WithUserAuthentication(accountProvider, serverAPI, mainAccount.ID))
				if err == nil {
					emoteStore = emote.NewStore(log.Logger, ttvAPI, stvAPI, bttvAPI)
				}
			}

			var emoteReplacer *emote.Replacer

			if settings.Chat.GraphicEmotes {
				if !hasEmoteSupport() {
					return fmt.Errorf("graphical emote support enabled but not available for this platform (unix & kitty terminal only)")
				}

				cellWidth, cellHeight, err := getTermCellWidthHeight()
				if err != nil {
					return fmt.Errorf("failed to get terminal size: %w", err)
				}

				emoteReplacer = emote.NewReplacer(http.DefaultClient, emoteStore, true, cellWidth, cellHeight, theme)
			} else {
				emoteReplacer = emote.NewReplacer(http.DefaultClient, emoteStore, false, 0, 0, theme)
			}

			p := tea.NewProgram(
				mainui.NewUI(log.Logger,
					accountProvider,
					chatMultiplexer,
					emoteStore,
					command.String("client-id"),
					serverAPI,
					keymap,
					recentMessageService,
					eventSubMultiplexer,
					messageLoggerChan,
					emoteReplacer,
					messageLogger,
					mainui.UserConfiguration{Settings: settings, Theme: theme},
				),
				tea.WithContext(ctx),
				tea.WithAltScreen(),
				tea.WithFPS(120),
			)

			eventSubMultiplexer.BuildEventSub = func() multiplex.EventSub {
				eventSub := eventsub.NewConn(log.Logger, http.DefaultClient)
				eventSub.HandleMessage = func(msg eventsub.Message[eventsub.NotificationPayload]) {
					p.Send(mainui.EventSubMessage{Payload: msg})
				}
				eventSub.HandleError = func(err error) {
					var twitchApiErr twitch.APIError
					if errors.As(err, &twitchApiErr) {
						log.Logger.Info().Any("twitch-error", twitchApiErr).Send()
					}

					log.Logger.Err(err).Msg("error in eventsub connection error handler")
				}

				return eventSub
			}

			final, err := p.Run()
			if err != nil {
				return err
			}

			if final, ok := final.(*mainui.Root); ok {
				if err := final.Close(); err != nil && !errors.Is(err, context.Canceled) {
					return err
				}

				// persist open tabs on disk
				state := final.TakeStateSnapshot()

				if err := state.Save(); err != nil {
					return fmt.Errorf("error while saving state: %w", err)
				}
			}

			close(messageLoggerChan)
			<-loggerWaitSync

			return nil
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := app.Run(ctx, os.Args); err != nil {
		fmt.Printf("failed to run Chatuino: %v\n", err)
		os.Exit(1)
	}
}

func openDB(readonly bool) (*sql.DB, error) {
	var (
		db  *sql.DB
		err error
	)

	if readonly {
		db, err = sql.Open("sqlite", "file:"+dbFileName+"?mode=ro&_time_format=sqlite")
	} else {
		db, err = sql.Open("sqlite", "file:"+dbFileName+"?_time_format=sqlite")
	}

	if err != nil {
		log.Logger.Err(err).Msg("failed to create sqlite connection")
		return nil, err
	}

	db.SetMaxOpenConns(1)

	return db, nil
}

func runChatLogger(messageLogger *messagelog.BatchedMessageLogger, messageLoggerChan chan *ttvCommand.PrivateMessage, loggerWaitSync chan struct{}, enabled bool) {
	defer func() {
		for range messageLoggerChan {
		}
		close(loggerWaitSync)
	}()

	if !enabled {
		log.Logger.Debug().Msg("storing chat logs disabled")
		return
	}

	if err := messageLogger.LogMessages(messageLoggerChan); err != nil {
		log.Logger.Err(err).Send()
		return
	}
}

func beforeAction(ctx context.Context, command *cli.Command) (context.Context, error) {
	// Setup logging
	//  - If logging not enabled, skip
	//  - If log-to-file is enabled, log to file, else stderr
	//  - If human-readable is enabled, log in human readable format (disable colors if log-to-file is enabled)
	// This action runs before any command is executed, including sub commands, but will run for all sub commands
	// Override the default http client transport to log requests
	// at the end of this function, set roundtripper logger to whatever logger was setup

	defer func() {
		transport := http.DefaultClient.Transport
		http.DefaultClient.Transport = httputil.NewChatuinoRoundTrip(transport, log.Logger, Version)
	}()

	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return ctx, fmt.Errorf("failed to create data directory: %w", err)
	}

	if !command.Bool("log") {
		log.Logger = zerolog.Nop()
		return ctx, nil
	}

	shouldLogToFile := command.Bool("log-to-file")

	var logFile *os.File
	if shouldLogToFile {
		f, err := setupLogFile()
		if err != nil {
			return ctx, fmt.Errorf("error while opening log file: %w", err)
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

	return ctx, nil
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
