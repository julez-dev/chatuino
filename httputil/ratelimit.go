package httputil

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

// RetryOn429 wraps an HTTP request function and retries on 429 (Too Many Requests)
// by waiting until the Ratelimit-Reset time specified in the response header.
// The retryFunc should execute the HTTP request and return the response.
// If skipEndpoints is provided, 429 responses for those endpoints will not be retried.
func RetryOn429(ctx context.Context, retryFunc func() (*http.Response, error), skipEndpoints ...string) (*http.Response, error) {
	resp, err := retryFunc()
	if err != nil {
		return nil, err
	}

	// Not a 429, return immediately
	if resp.StatusCode != http.StatusTooManyRequests {
		return resp, nil
	}

	// Check if this endpoint should skip retry
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
	resp.Body.Close()

	// Calculate wait duration (add 1 second buffer)
	diff := time.Until(time.Unix(waitUntil, 0)) + time.Second

	// Create timer for the wait duration
	timer := time.NewTimer(diff)
	defer func() {
		timer.Stop()
		select {
		case <-timer.C:
		default:
		}
	}()

	// Wait for either reset time or context cancellation
	select {
	case <-timer.C:
		// Reset time reached, retry the request
		return retryFunc()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ShouldSkipRetry checks if the endpoint is in the skip list
func ShouldSkipRetry(endpoint string, skipList []string) bool {
	for _, skip := range skipList {
		if endpoint == skip {
			return true
		}
	}
	return false
}
