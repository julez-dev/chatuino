package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/seventv"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/ui"
	"github.com/rs/zerolog"
)

const logFileName = "log.txt"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	ttvAPI := twitch.NewAPI(nil, os.Getenv("TWITCH_OAUTH"), os.Getenv("TWITCH_CLIENT_ID"))
	stvAPI := seventv.NewAPI(nil)

	store := emote.NewStore(ttvAPI, stvAPI)

	go func() {
		if err := store.RefreshGlobal(ctx); err != nil {
			fmt.Printf("Error while refreshing: %v", err)
			os.Exit(1)
		}
	}()

	f, err := setupLogFile()
	if err != nil {
		fmt.Printf("Error while opening log file: %v", err)
		os.Exit(1)
	}
	defer f.Close()

	logger := zerolog.New(f).With().
		Timestamp().Logger()

	p := tea.NewProgram(ui.New(ctx, logger, store), tea.WithContext(ctx), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error while running application: %v", err)
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
