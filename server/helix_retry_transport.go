package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/julez-dev/chatuino/httputil"
	"github.com/rs/zerolog/log"
)

// tokenProvider is the interface for managing app access tokens.
type tokenProvider interface {
	InvalidateToken()
	EnsureToken(ctx context.Context) (string, error)
}

// helixRetryTransport is a custom RoundTripper that injects auth headers and
// handles 401 responses by refreshing the app access token and retrying once.
type helixRetryTransport struct {
	base          http.RoundTripper
	tokenProvider tokenProvider
	clientID      string
}

// newHelixRetryTransport creates a new retry transport.
func newHelixRetryTransport(base http.RoundTripper, tokenProvider tokenProvider, clientID string) *helixRetryTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &helixRetryTransport{
		base:          base,
		tokenProvider: tokenProvider,
		clientID:      clientID,
	}
}

// RoundTrip implements http.RoundTripper.
func (t *helixRetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to allow retry
	reqClone, err := httputil.CloneRequest(req)
	if err != nil {
		return nil, err
	}

	// First attempt with current token
	resp, err := t.doAuthenticatedRequest(req)
	if err != nil {
		return nil, err
	}

	// If not 401, return response as-is
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	// Got 401 - attempt token refresh and retry
	t.tokenProvider.InvalidateToken()

	// Second attempt with new token
	respRetry, err := t.doAuthenticatedRequest(reqClone)
	if err != nil {
		// If we can't get a new token, return the original 401 response unchanged
		log.Logger.Warn().Err(err).Msg("failed to get new token, fallback to original http response")
		return resp, nil
	}

	// Close the first response body since we have retried
	resp.Body.Close()

	return respRetry, nil
}

// doAuthenticatedRequest fetches a token and sets Authorization and Client-Id headers.
func (t *helixRetryTransport) doAuthenticatedRequest(req *http.Request) (*http.Response, error) {
	token, err := t.tokenProvider.EnsureToken(req.Context())
	if err != nil {
		// If we can't get a new token, return the original 401 response unchanged
		return nil, fmt.Errorf("failed to ensure token: %w", err)
	}

	// remove all headers, expect auth headers
	req.Header = http.Header{}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Client-Id", t.clientID)
	req.Header.Set("Accept", "application/json")

	log.Logger.Info().Any("header", req.Header).Stringer("url", req.URL).Msg("send proxied req to ttv")

	return t.base.RoundTrip(req)
}
