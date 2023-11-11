package main

import (
	"context"
	"fmt"
	"net/http"
	"net/mail"
	"os"
	"os/signal"
	"syscall"

	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/ui/mainui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/seventv"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"
)

const logFileName = "log.txt"

func main() {
	f, err := setupLogFile()
	if err != nil {
		fmt.Println("error while opening log file: %w", err)
		os.Exit(1)
	}

	defer f.Close()

	logger := zerolog.New(f).With().Timestamp().Logger()

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
				Value: "jliqj1q6nmp0uh5ofangdx4iac7yd9",
			},
			&cli.StringFlag{
				Name:  "api-host",
				Usage: "Host of the Chatuino API",
				Value: "http://localhost:8080",
			},
		},
		Action: func(c *cli.Context) error {
			list, err := save.AccountListFromDisk()
			if err != nil {
				return fmt.Errorf("error while fetching accounts from disk: %w", err)
			}

			defer func() {
				err = list.Save()
				if err != nil {
					fmt.Printf("error while saving save file: %v", err)
					os.Exit(1)
				}
			}()

			serverAPI := server.NewClient(c.String("api-host"), http.DefaultClient)
			stvAPI := seventv.NewAPI(http.DefaultClient)

			store := emote.NewStore(serverAPI, stvAPI)

			p := tea.NewProgram(
				mainui.NewUI(logger, list, &store, c.String("client-id"), serverAPI),
				tea.WithContext(c.Context),
				tea.WithAltScreen(),
			)

			// Refresh global emotes in the background to reduce start up time, quit tea event loop if error occurred
			go func() {
				if err := store.RefreshGlobal(c.Context); err != nil {
					p.Quit()
					p.Wait()
					fmt.Printf("error while fetching global emotes: %v", err)
					os.Exit(1)
				}
			}()

			if _, err := p.Run(); err != nil {
				return fmt.Errorf("error while running TUI: %w", err)
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

func setupLogFile() (*os.File, error) {
	f, err := os.OpenFile(logFileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}

	return f, nil
}
