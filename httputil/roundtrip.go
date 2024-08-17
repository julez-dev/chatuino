package httputil

import (
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

type RoundTripperFunc func(req *http.Request) (*http.Response, error)

func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type ChatuinoRoundTrip struct {
	rt      http.RoundTripper
	logger  zerolog.Logger
	version string
}

func NewChatuinoRoundTrip(rt http.RoundTripper, logger zerolog.Logger, userAgentVersion string) *ChatuinoRoundTrip {
	return &ChatuinoRoundTrip{
		rt:      rt,
		logger:  logger,
		version: userAgentVersion,
	}
}

func (t *ChatuinoRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) {
	rt := t.rt

	if rt == nil {
		rt = http.DefaultTransport
	}

	req = req.Clone(req.Context())

	// if strings.Contains(req.URL.Path, "/eventsub/subscriptions") {
	// 	req.URL.Scheme = "http"
	// 	req.URL.Host = "127.0.0.1:8080"
	// 	req.URL.Path = strings.TrimPrefix(req.URL.Path, "/helix")
	// }

	req.Header.Set("User-Agent", fmt.Sprintf("Chatuino/%s", t.version))

	now := time.Now()
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.logger.Error().Err(err).Msg("error while making request")
		return nil, err
	}

	dur := time.Since(now)
	t.logger.Info().
		Str("method", req.Method).
		Str("url", req.URL.String()).
		Dur("took", dur).
		Int("status", resp.StatusCode).Msg("request made")

	return resp, nil
}
