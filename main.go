package main

import (
	"context"
	"fmt"
	"net/http"
	"net/mail"
	"os"
	"os/signal"
	"syscall"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/ui/mainui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/seventv"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"
)

const logFileName = "log.txt"

// type (
// 	errMsg error
// )

// type model struct {
// 	textInput *component.SuggestionTextInput
// 	err       error
// }

// func initialModel() model {
// 	ti := component.NewSuggestionTextInput()
// 	ti.Focus()
// 	ti.SetWidth(20)

// 	return model{
// 		textInput: &ti,
// 		err:       nil,
// 	}
// }

// func (m model) Init() tea.Cmd {
// 	return textinput.Blink
// }

// func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
// 	var cmd tea.Cmd

// 	switch msg := msg.(type) {
// 	case tea.KeyMsg:
// 		switch msg.Type {
// 		case tea.KeyEnter, tea.KeyCtrlC, tea.KeyEsc:
// 			return m, tea.Quit
// 		}

// 		switch msg.String() {
// 		case "f1":
// 			m.textInput.SetSuggestions([]string{"KEKW"})
// 		}

// 	// We handle errors just like any other message
// 	case errMsg:
// 		m.err = msg
// 		return m, nil
// 	}

// 	m.textInput, cmd = m.textInput.Update(msg)
// 	return m, cmd
// }

// func (m model) View() string {
// 	return fmt.Sprintf(
// 		"What’s your favorite Pokémon?\n\n%s\n\n%s",
// 		m.textInput.View(),
// 		"(esc to quit)",
// 	) + "\n"
// }

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
				Value: "https://chatuino-server.onrender.com",
			},
		},
		Action: func(c *cli.Context) error {
			accountProvider := save.NewAccountProvider()
			serverAPI := server.NewClient(c.String("api-host"), http.DefaultClient)
			stvAPI := seventv.NewAPI(http.DefaultClient)

			store := emote.NewStore(serverAPI, stvAPI)

			p := tea.NewProgram(
				mainui.NewUI(logger, accountProvider, &store, c.String("client-id"), serverAPI),
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
