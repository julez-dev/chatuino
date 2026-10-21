package emote

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/julez-dev/chatuino/httputil"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/save"
	"github.com/stretchr/testify/require"
)

func TestReplacer_DeadlineCancelsImageDownload(t *testing.T) {
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
	store := &mockEmoteStore{emotes: map[string]Emote{
		"Kappa": {ID: "deadline", Text: "Kappa", URL: "https://example.com/kappa.png", Platform: Twitch},
	}}
	display := &mockDisplayManager{convertFunc: func(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
		body, _, err := unit.Load()
		if body != nil {
			body.Close()
		}
		return kittyimg.KittyDisplayUnit{}, err
	}}
	replacer := NewReplacer(client, store, true, save.BuildDefaultTheme(), display)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()

	_, replacements, err := replacer.Replace(ctx, "channel", "Kappa", nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, replacements)
}
