package httputil

import (
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"
)

// RateLimitRetryTransport is an http.RoundTripper that automatically retries
// requests that receive a 429 (Too Many Requests) response by waiting until
// the time specified in the Ratelimit-Reset header.
type RateLimitRetryTransport struct {
	// Transport is the underlying http.RoundTripper
	Transport http.RoundTripper

	// SkipEndpoints lists endpoint paths that should NOT be retried on 429
	SkipEndpoints []string
}

// RoundTrip implements http.RoundTripper
func (t *RateLimitRetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt := t.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}

	// Clone the request to preserve the body for potential retry
	reqClone := req.Clone(req.Context())
	if req.Body != nil {
		// GetBody should be set by http.NewRequest for retryable bodies
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			reqClone.Body = body
		}
	}

	resp, err := rt.RoundTrip(reqClone)
	if err != nil {
		return nil, err
	}

	// Not a 429, return immediately
	if resp.StatusCode != http.StatusTooManyRequests {
		return resp, nil
	}

	// Check if this endpoint should skip retry
	if slices.Contains(t.SkipEndpoints, req.URL.Path) {
		return resp, nil
	}

	// Check for Ratelimit-Reset header
	resetHeader := resp.Header.Get("Ratelimit-Reset")
	if resetHeader == "" {
		// No reset header, can't retry
		return resp, nil
	}

	// Parse the reset timestamp
	waitUntil, err := strconv.ParseInt(resetHeader, 10, 64)
	if err != nil {
		// Can't parse reset time, return original response
		return resp, nil
	}

	// Close the 429 response body since we're going to retry
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Calculate wait duration (add 1 second buffer)
	diff := time.Until(time.Unix(waitUntil, 0)) + time.Second

	// Create timer for the wait duration
	timer := time.NewTimer(diff)
	defer timer.Stop() // Go 1.23+ automatically drains the channel

	// Wait for either reset time or context cancellation
	select {
	case <-timer.C:
		// Reset time reached, clone request again for retry
		reqRetry := req.Clone(req.Context())
		if req.Body != nil && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			reqRetry.Body = body
		}
		return rt.RoundTrip(reqRetry)
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
}
