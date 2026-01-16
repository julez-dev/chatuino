package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

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
	reqClone, err := cloneRequest(req)
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

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Client-Id", t.clientID)
	return t.base.RoundTrip(req)
}

// cloneRequest creates a shallow copy of the request with a cloned body (if present).
func cloneRequest(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())

	if req.Body != nil {
		// Read the body
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}

		// Restore original body
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		// Set clone body
		clone.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	return clone, nil
}
