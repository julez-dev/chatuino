package httputil

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetryOn429(t *testing.T) {
	t.Run("returns immediately on non-429 response", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		retryFunc := func() (*http.Response, error) {
			callCount++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("success")),
			}, nil
		}

		ctx := context.Background()
		resp, err := RetryOn429(ctx, retryFunc)

		require.NoError(t, err, "should not error on successful response")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, 1, callCount, "should only call once for non-429")
	})

	t.Run("returns 429 response when no Ratelimit-Reset header", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		retryFunc := func() (*http.Response, error) {
			callCount++
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("rate limited")),
			}, nil
		}

		ctx := context.Background()
		resp, err := RetryOn429(ctx, retryFunc)

		require.NoError(t, err, "should not error when no reset header")
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		require.Equal(t, 1, callCount, "should not retry when no reset header")
	})

	t.Run("retries on 429 with valid Ratelimit-Reset header", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		// Set reset time to 1 second in the future
		resetTime := time.Now().Add(1 * time.Second).Unix()

		retryFunc := func() (*http.Response, error) {
			callCount++
			if callCount == 1 {
				// First call returns 429
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Header: http.Header{
						"Ratelimit-Reset": []string{strconv.FormatInt(resetTime, 10)},
					},
					Body: io.NopCloser(strings.NewReader("rate limited")),
				}, nil
			}
			// Second call succeeds
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("success after retry")),
			}, nil
		}

		ctx := context.Background()
		start := time.Now()
		resp, err := RetryOn429(ctx, retryFunc)
		elapsed := time.Since(start)

		require.NoError(t, err, "should not error after successful retry")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, 2, callCount, "should retry once after 429")
		require.Greater(t, elapsed, 1*time.Second, "should wait at least 1 second")
	})

	t.Run("respects context cancellation during wait", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		// Set reset time to 10 seconds in the future
		resetTime := time.Now().Add(10 * time.Second).Unix()

		retryFunc := func() (*http.Response, error) {
			callCount++
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header: http.Header{
					"Ratelimit-Reset": []string{strconv.FormatInt(resetTime, 10)},
				},
				Body: io.NopCloser(strings.NewReader("rate limited")),
			}, nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		resp, err := RetryOn429(ctx, retryFunc)

		require.Error(t, err, "should error on context cancellation")
		require.ErrorIs(t, err, context.DeadlineExceeded, "error should be deadline exceeded")
		require.Nil(t, resp, "response should be nil on context cancellation")
		require.Equal(t, 1, callCount, "should only call once before context cancelled")
	})

	t.Run("handles invalid Ratelimit-Reset value", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		retryFunc := func() (*http.Response, error) {
			callCount++
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header: http.Header{
					"Ratelimit-Reset": []string{"invalid"},
				},
				Body: io.NopCloser(strings.NewReader("rate limited")),
			}, nil
		}

		ctx := context.Background()
		resp, err := RetryOn429(ctx, retryFunc)

		require.NoError(t, err, "should not error on invalid reset header")
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		require.Equal(t, 1, callCount, "should not retry with invalid reset header")
	})
}

func TestShouldSkipRetry(t *testing.T) {
	t.Run("returns true for endpoint in skip list", func(t *testing.T) {
		skipList := []string{"/eventsub/subscriptions", "/other/endpoint"}
		require.True(t, ShouldSkipRetry("/eventsub/subscriptions", skipList))
	})

	t.Run("returns false for endpoint not in skip list", func(t *testing.T) {
		skipList := []string{"/eventsub/subscriptions"}
		require.False(t, ShouldSkipRetry("/users", skipList))
	})

	t.Run("returns false for empty skip list", func(t *testing.T) {
		require.False(t, ShouldSkipRetry("/users", []string{}))
	})
}
