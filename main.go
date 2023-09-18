package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/seventv"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/ui/chatui"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v2"
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

	app := &cli.App{
		Name:        "Chatuino",
		Description: "Chatuino twitch IRC Client",
		Usage:       "A client for twitch's IRC service",
		HideVersion: true,
		Authors: []*cli.Author{
			{
				Name:  "julez-dev",
				Email: "julez-dev@pm.me",
			},
		},
		Commands: []*cli.Command{
			versionCMD,
			accountCMD,
			{
				Name: "irc",
				Action: func(ctx *cli.Context) error {
					return nil
					// chat := twitch.NewChat()
					// in := make(chan twitch.IRCer)

					// go func() {
					// 	<-ctx.Done()
					// 	fmt.Println("done")
					// }()

					// msgs, _, err := chat.Connect(ctx.Context, in, "julezdev", os.Getenv("TWITCH_OAUTH"))
					// if err != nil {
					// 	return err
					// }

					// in <- twitch.JoinMessage{Channel: "julezdev"}

					// for msg := range msgs {
					// 	_ = msg
					// }

					// return nil
				},
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

			mainAccount, _ := list.GetMainAccount()

			ttvAPI := twitch.NewAPI(nil, mainAccount.AccessToken, os.Getenv("TWITCH_CLIENT_ID"))
			stvAPI := seventv.NewAPI(nil)

			store := emote.NewStore(ttvAPI, stvAPI)

			p := tea.NewProgram(
				chatui.New(c.Context, logger, &store, ttvAPI, list),
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

	if err := app.RunContext(ctx, os.Args); err != nil {
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
