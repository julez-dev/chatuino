package badge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/twitch/command"
	"github.com/rs/zerolog/log"
)

type BadgeCache interface {
	MatchBadgeSet(broadcasterID string, ircBadge []command.Badge) map[string]twitch.BadgeVersion
}

type DisplayManager interface {
	Convert(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error)
}

type Replacer struct {
	httpClient     *http.Client
	enableGraphics bool
	cache          BadgeCache
	displayManager DisplayManager
}

func NewReplacer(httpClient *http.Client, cache BadgeCache, enableGraphics bool, theme save.Theme, displayManager DisplayManager) *Replacer {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Replacer{
		enableGraphics: enableGraphics,
		cache:          cache,
		httpClient:     httpClient,
		displayManager: displayManager,
	}
}

func (r *Replacer) Replace(broadcasterID string, badgeList []command.Badge) (string, []string, error) {
	badgeMap := r.cache.MatchBadgeSet(broadcasterID, badgeList)
	log.Logger.Info().Str("broadcasterID", broadcasterID).Any("m", badgeMap).Send()

	var (
		prepare         = strings.Builder{}
		formattedBadges = make([]string, 0, len(badgeMap))
	)

	for k, b := range badgeMap {
		u, err := r.displayManager.Convert(kittyimg.DisplayUnit{
			ID:        broadcasterID + k + b.ID,
			Directory: "badge",
			Load: func() (io.ReadCloser, string, error) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
				defer cancel()

				url := b.Image_URL_1x

				log.Logger.Info().Str("id", b.ID).Str("set", k).Str("url", url).Msg("fetching badge")

				return r.fetch(ctx, b.Image_URL_1x)
			},
		})
		if err != nil {
			return "", nil, fmt.Errorf("failed to convert (%s) badge: %w", broadcasterID+k+b.ID, err)
		}

		prepare.WriteString(u.PrepareCommand)

		formattedBadges = append(formattedBadges, u.ReplacementText)
	}

	return prepare.String(), formattedBadges, nil
}

func (r *Replacer) fetch(ctx context.Context, reqURL string) (io.ReadCloser, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status code, got: %d", resp.StatusCode)
	}

	return resp.Body, resp.Header.Get("content-type"), nil
}
