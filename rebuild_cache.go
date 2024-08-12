package main

import (
	"context"
	"fmt"
	"os"

	"net/http"

	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/twitch/bttv"
	"github.com/julez-dev/chatuino/twitch/seventv"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
)

var rebuildCacheCMD = &cli.Command{
	Name:        "rebuild-cache",
	Usage:       "Rebuild Chatuino emote cache",
	Description: "Purges and refreshes the emote cache for the provided channels + global channels used by Chatuino",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:  "channels",
			Usage: "channel names that should be pre fetched",
		},
		&cli.StringFlag{
			Name:    "api-host",
			Usage:   "Host of the Chatuino API",
			Value:   "https://chatuino.net",
			Sources: cli.EnvVars("CHATUINO_API_HOST"),
		},
	},
	Action: func(ctx context.Context, command *cli.Command) error {
		termSize, err := getTermSize()
		if err != nil {
			return nil
		}

		fmt.Println("pruning cache")

		dir, err := emote.EnsureEmoteDirExists()
		if err != nil {
			return err
		}

		if err := os.RemoveAll(dir); err != nil {
			return err
		}

		cellWidth := float32(termSize.Xpixel) / float32(termSize.Col)
		cellHeight := float32(termSize.Ypixel) / float32(termSize.Row)
		ij := emote.NewInjector(http.DefaultClient, nil, cellWidth, cellHeight)

		sttvAPI := seventv.NewAPI(http.DefaultClient)
		bttvAPI := bttv.NewAPI(http.DefaultClient)
		ttvAPI := server.NewClient(command.String("api-host"), http.DefaultClient)

		store := emote.NewStore(log.Logger, ttvAPI, sttvAPI, bttvAPI)
		if err := store.RefreshGlobal(ctx); err != nil {
			return err
		}

		channels := command.StringSlice("channels")
		if len(channels) > 0 {
			userLoginsID := map[string]string{}

			users, err := ttvAPI.GetUsers(ctx, command.StringSlice("channels"), nil)
			if err != nil {
				return err
			}

			for _, u := range users.Data {
				userLoginsID[u.Login] = u.ID
			}

			for _, channel := range channels {
				fmt.Println("loading emotes for", channel)
				if err := store.RefreshLocal(ctx, userLoginsID[channel]); err != nil {
					return err
				}
			}
		}

		errgroup, ctx := errgroup.WithContext(ctx)
		errgroup.SetLimit(5)

		for _, e := range store.GetAll() {
			errgroup.Go(func() error {
				fmt.Println("caching", e.Text, e.URL)
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.URL, nil)
				if err != nil {
					return err
				}

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return err
				}

				defer resp.Body.Close()

				decoded, err := ij.ConvertEmote(e, resp.Body)
				if err != nil {
					fmt.Printf("failed converting: %s (%s) (%s): %s", e.ID, e.URL, e.Format, err.Error())
					return nil
				}
				fmt.Println("converted", e.Text)

				if err := emote.CacheEmote(e, decoded); err != nil {
					return err
				}

				fmt.Println("cached", e.Text)

				return nil
			})
		}

		if err := errgroup.Wait(); err != nil {
			return err
		}

		fmt.Println(len(store.GetAll()), "emotes cached")

		return nil
	},
}
