package badge

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/rs/zerolog/log"
)

type BadgeCache interface {
	MatchBadgeSet(broadcasterID string, ircBadge []twitchirc.Badge) map[string]twitchapi.BadgeVersion
}

type DisplayManager interface {
	Convert(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error)
}

type Replacer struct {
	httpClient     *http.Client
	enableGraphics bool
	cache          BadgeCache
	displayManager DisplayManager
	badeColorMap   map[string]string
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
		badeColorMap: map[string]string{
			"broadcaster": lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ChatStreamerColor)).Render("Streamer"),
			"no_audio":    "No Audio",
			"vip":         lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ChatVIPColor)).Render("VIP"),
			"subscriber":  lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ChatSubColor)).Render("Sub"),
			"admin":       "Admin",
			"staff":       "Staff",
			"Turbo":       lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ChatTurboColor)).Render("Turbo"),
			"moderator":   lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ChatModeratorColor)).Render("Mod"),
		},
	}
}

func (r *Replacer) Replace(broadcasterID string, badgeList []twitchirc.Badge) (string, map[string]string, error) {
	badgeMap := r.cache.MatchBadgeSet(broadcasterID, badgeList)
	badgesSortedKeys := slices.Sorted(maps.Keys(badgeMap))

	formattedBadges := make(map[string]string, len(badgeMap))

	if !r.enableGraphics {
		for _, k := range badgesSortedKeys {
			b := badgeMap[k]

			if colored, ok := r.badeColorMap[k]; ok {
				formattedBadges[k] = colored
				continue
			}

			formattedBadges[k] = b.Title
		}

		return "", formattedBadges, nil
	}

	prepare := strings.Builder{}

	for _, k := range badgesSortedKeys {
		b := badgeMap[k]

		u, err := r.displayManager.Convert(kittyimg.DisplayUnit{
			ID:        broadcasterID + k + b.ID,
			Directory: "badge",
			Load: func() (io.ReadCloser, string, error) {
				url := b.Image_URL_1x

				log.Logger.Info().Str("id", b.ID).Str("set", k).Str("url", url).Msg("fetching badge")

				return r.fetch(context.Background(), b.Image_URL_1x)
			},
		})
		if err != nil {
			return "", nil, fmt.Errorf("failed to convert (%s) badge: %w", broadcasterID+k+b.ID, err)
		}

		prepare.WriteString(u.PrepareCommand)

		formattedBadges[b.Title] = u.ReplacementText
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
