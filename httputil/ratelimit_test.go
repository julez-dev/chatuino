package httputil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRateLimitRetryTransport(t *testing.T) {
	t.Run("returns immediately on non-429 response", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		mockTransport := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			callCount++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("success")),
			}, nil
		})

		transport := &RateLimitRetryTransport{
			Transport: mockTransport,
		}

		req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
		resp, err := transport.RoundTrip(req)

		require.NoError(t, err, "should not error on successful response")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, 1, callCount, "should only call once for non-429")
	})

	t.Run("returns 429 response when no Ratelimit-Reset header", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		mockTransport := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			callCount++
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("rate limited")),
			}, nil
		})

		transport := &RateLimitRetryTransport{
			Transport: mockTransport,
		}

		req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
		resp, err := transport.RoundTrip(req)

		require.NoError(t, err, "should not error when no reset header")
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		require.Equal(t, 1, callCount, "should not retry when no reset header")
	})

	t.Run("retries on 429 with valid Ratelimit-Reset header", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		resetTime := time.Now().Add(1 * time.Second).Unix()
		expectedBody := `{"test":"data"}`

		mockTransport := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			callCount++

			// Verify request body is present and correct on both calls
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err, "should read request body")
			require.Equal(t, expectedBody, string(body), "request body should match on call %d", callCount)

			if callCount == 1 {
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Header: http.Header{
						"Ratelimit-Reset": []string{strconv.FormatInt(resetTime, 10)},
					},
					Body: io.NopCloser(strings.NewReader("rate limited")),
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("success after retry")),
			}, nil
		})

		transport := &RateLimitRetryTransport{
			Transport: mockTransport,
		}

		// Use http.NewRequest (not httptest.NewRequest) to ensure GetBody is set
		req, err := http.NewRequest(http.MethodPost, "http://example.com/test", strings.NewReader(expectedBody))
		require.NoError(t, err, "should create request")

		start := time.Now()
		resp, err := transport.RoundTrip(req)
		elapsed := time.Since(start)

		require.NoError(t, err, "should not error after successful retry")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, 2, callCount, "should retry once after 429")
		require.Greater(t, elapsed, 1*time.Second, "should wait at least 1 second")
	})

	t.Run("respects context cancellation during wait", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		resetTime := time.Now().Add(10 * time.Second).Unix()

		mockTransport := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			callCount++
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header: http.Header{
					"Ratelimit-Reset": []string{strconv.FormatInt(resetTime, 10)},
				},
				Body: io.NopCloser(strings.NewReader("rate limited")),
			}, nil
		})

		transport := &RateLimitRetryTransport{
			Transport: mockTransport,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
		req = req.WithContext(ctx)

		resp, err := transport.RoundTrip(req)

		require.Error(t, err, "should error on context cancellation")
		require.ErrorIs(t, err, context.DeadlineExceeded, "error should be deadline exceeded")
		require.Nil(t, resp, "response should be nil on context cancellation")
		require.Equal(t, 1, callCount, "should only call once before context cancelled")
	})

	t.Run("handles invalid Ratelimit-Reset value", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		mockTransport := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			callCount++
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header: http.Header{
					"Ratelimit-Reset": []string{"invalid"},
				},
				Body: io.NopCloser(strings.NewReader("rate limited")),
			}, nil
		})

		transport := &RateLimitRetryTransport{
			Transport: mockTransport,
		}

		req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
		resp, err := transport.RoundTrip(req)

		require.NoError(t, err, "should not error on invalid reset header")
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		require.Equal(t, 1, callCount, "should not retry with invalid reset header")
	})

	t.Run("skips retry for endpoints in skip list", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		resetTime := time.Now().Add(1 * time.Second).Unix()

		mockTransport := RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			callCount++
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header: http.Header{
					"Ratelimit-Reset": []string{strconv.FormatInt(resetTime, 10)},
				},
				Body: io.NopCloser(strings.NewReader("rate limited")),
			}, nil
		})

		transport := &RateLimitRetryTransport{
			Transport:     mockTransport,
			SkipEndpoints: []string{"/eventsub/subscriptions"},
		}

		req := httptest.NewRequest(http.MethodGet, "http://example.com/eventsub/subscriptions", nil)
		resp, err := transport.RoundTrip(req)

		require.NoError(t, err, "should not error")
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		require.Equal(t, 1, callCount, "should not retry skipped endpoint")
	})

	t.Run("uses default transport when Transport is nil", func(t *testing.T) {
		t.Parallel()

		transport := &RateLimitRetryTransport{}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "success")
		}))
		defer server.Close()

		req := httptest.NewRequest(http.MethodGet, server.URL, nil)
		resp, err := transport.RoundTrip(req)

		require.NoError(t, err, "should work with default transport")
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
