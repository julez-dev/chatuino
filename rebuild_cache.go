package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"

	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/twitch/bttv"
	"github.com/julez-dev/chatuino/twitch/seventv"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
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
		if !hasEmoteSupport() {
			return fmt.Errorf("graphical emote support enabled but not available for this platform (unix & kitty terminal only)")
		}

		cellWidth, cellHeight, err := getTermCellWidthHeight()
		if err != nil {
			return fmt.Errorf("failed to get terminal size: %w", err)
		}

		fmt.Println("removing cached emotes")

		emoteDir, err := emote.EmoteCacheDir()
		if err != nil {
			return err
		}

		if err := os.RemoveAll(emoteDir); err != nil {
			return err
		}

		fmt.Println("removed cached emotes ✅")

		ij := emote.NewReplacer(http.DefaultClient, nil, true, cellWidth, cellHeight, save.Theme{})

		sttvAPI := seventv.NewAPI(http.DefaultClient)
		bttvAPI := bttv.NewAPI(http.DefaultClient)
		ttvAPI := server.NewClient(command.String("api-host"), http.DefaultClient)

		store := emote.NewCache(log.Logger, ttvAPI, sttvAPI, bttvAPI)
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

			for channel := range slices.Values(channels) {
				fmt.Println("loading emotes for", channel)
				if err := store.RefreshLocal(ctx, userLoginsID[channel]); err != nil {
					return err
				}
			}
		}

		errgroup, ctx := errgroup.WithContext(ctx)
		errgroup.SetLimit(6)
		emotes := store.GetAll()

		p := mpb.NewWithContext(ctx)

		bar := p.New(int64(len(emotes)),
			mpb.BarStyle().Lbound("[").Filler("-").Tip("C").Padding("·").Rbound("]"),
			mpb.PrependDecorators(
				decor.Name("Emotes:", decor.WC{C: decor.DindentRight | decor.DextraSpace}),
				decor.CountersNoUnit("%d / %d", decor.WCSyncSpaceR),
				decor.OnComplete(decor.AverageETA(decor.ET_STYLE_GO), "✅"),
			),
			mpb.AppendDecorators(decor.Percentage()),
		)

		for _, e := range emotes {
			errgroup.Go(func() error {
				defer bar.Increment()
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
					//	fmt.Printf("failed converting: %s (%s) (%s): %s", e.ID, e.URL, e.Format, err.Error())
					return nil
				}

				if err := emote.SaveCache(e, decoded); err != nil {
					return err
				}

				return nil
			})
		}

		errgroup.Go(func() error {
			p.Wait()
			return nil
		})

		if err := errgroup.Wait(); err != nil {
			return err
		}

		return nil
	},
}
