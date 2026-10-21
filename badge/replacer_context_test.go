package badge

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/julez-dev/chatuino/httputil"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/stretchr/testify/require"
)

func TestReplacer_DeadlineCancelsBadgeDownload(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: httputil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if _, ok := req.Context().Deadline(); !ok {
			return nil, fmt.Errorf("download has no deadline")
		}
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(time.Second):
			return nil, fmt.Errorf("download did not receive its caller's deadline")
		}
	})}
	cache := &mockBadgeCache{matchBadgeSetFunc: func(string, []twitchirc.Badge) map[string]twitchapi.BadgeVersion {
		return map[string]twitchapi.BadgeVersion{"subscriber": {ID: "1", Image_URL_1x: "https://example.com/badge.png"}}
	}}
	display := &mockDisplayManager{convertFunc: func(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
		body, _, err := unit.Load()
		if body != nil {
			body.Close()
		}
		return kittyimg.KittyDisplayUnit{}, err
	}}
	replacer := NewReplacer(client, cache, true, save.BuildDefaultTheme(), display)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()

	_, replacements, err := replacer.Replace(ctx, "channel", []twitchirc.Badge{{Name: "subscriber", Version: "1"}})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, replacements)
}

func TestReplacer_CancelledContributorPreparationDoesNotMutateBadges(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	replacer := NewReplacer(nil, nil, false, save.BuildDefaultTheme(), nil)
	badges := map[string]string{}
	_, err := replacer.InjectContributorBadge(ctx, "julez", badges)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, badges)
}
