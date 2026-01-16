package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// mockTokenProvider implements tokenProvider for testing.
type mockTokenProvider struct {
	tokens           []string
	tokenIndex       int
	invalidateCalled atomic.Int32
}

func (m *mockTokenProvider) InvalidateToken() {
	m.invalidateCalled.Add(1)
	m.tokenIndex++
}

func (m *mockTokenProvider) EnsureToken(ctx context.Context) (string, error) {
	if m.tokenIndex >= len(m.tokens) {
		return "", nil
	}
	return m.tokens[m.tokenIndex], nil
}

// mockRoundTripper allows us to control responses for testing.
type mockRoundTripper struct {
	responses []*http.Response
	callCount int
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.callCount >= len(m.responses) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("unexpected call")),
		}, nil
	}

	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

func TestHelixRetryTransport(t *testing.T) {
	t.Parallel()

	t.Run("non-401 responses", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name       string
			statusCode int
		}{
			{"200 OK", http.StatusOK},
			{"400 Bad Request", http.StatusBadRequest},
			{"403 Forbidden", http.StatusForbidden},
			{"404 Not Found", http.StatusNotFound},
			{"429 Too Many Requests", http.StatusTooManyRequests},
			{"500 Internal Server Error", http.StatusInternalServerError},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				tokenProvider := &mockTokenProvider{
					tokens: []string{"token1"},
				}

				baseTransport := &mockRoundTripper{
					responses: []*http.Response{
						{
							StatusCode: tc.statusCode,
							Body:       io.NopCloser(strings.NewReader("response")),
						},
					},
				}

				transport := newHelixRetryTransport(baseTransport, tokenProvider, "client123")

				req := httptest.NewRequest(http.MethodGet, "https://api.twitch.tv/helix/users", nil)
				resp, err := transport.RoundTrip(req)

				require.NoError(t, err, "RoundTrip should not return error")
				require.Equal(t, tc.statusCode, resp.StatusCode, "status should match")
				require.Equal(t, 1, baseTransport.callCount, "should make exactly one call (no retry)")
				require.Equal(t, int32(0), tokenProvider.invalidateCalled.Load(), "should not invalidate token")
			})
		}
	})

	t.Run("401 retry behavior", func(t *testing.T) {
		t.Parallel()

		t.Run("success after retry", func(t *testing.T) {
			t.Parallel()

			tokenProvider := &mockTokenProvider{
				tokens: []string{"old_token", "new_token"},
			}

			baseTransport := &mockRoundTripper{
				responses: []*http.Response{
					{
						StatusCode: http.StatusUnauthorized,
						Body:       io.NopCloser(strings.NewReader("invalid token")),
					},
					{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader("success")),
					},
				},
			}

			transport := newHelixRetryTransport(baseTransport, tokenProvider, "client123")

			req := httptest.NewRequest(http.MethodGet, "https://api.twitch.tv/helix/users", nil)

			resp, err := transport.RoundTrip(req)

			require.NoError(t, err, "RoundTrip should not return error")
			require.Equal(t, http.StatusOK, resp.StatusCode, "status should be 200 after retry")
			require.Equal(t, 2, baseTransport.callCount, "should make exactly two calls")
			require.Equal(t, int32(1), tokenProvider.invalidateCalled.Load(), "should invalidate token once")
		})

		t.Run("retry also fails", func(t *testing.T) {
			t.Parallel()

			tokenProvider := &mockTokenProvider{
				tokens: []string{"old_token", "new_token"},
			}

			baseTransport := &mockRoundTripper{
				responses: []*http.Response{
					{
						StatusCode: http.StatusUnauthorized,
						Body:       io.NopCloser(strings.NewReader("invalid token")),
					},
					{
						StatusCode: http.StatusUnauthorized,
						Body:       io.NopCloser(strings.NewReader("still invalid")),
					},
				},
			}

			transport := newHelixRetryTransport(baseTransport, tokenProvider, "client123")

			req := httptest.NewRequest(http.MethodGet, "https://api.twitch.tv/helix/users", nil)

			resp, err := transport.RoundTrip(req)

			require.NoError(t, err, "RoundTrip should not return error")
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "status should be 401 when retry fails")
			require.Equal(t, 2, baseTransport.callCount, "should make exactly two calls")
			require.Equal(t, int32(1), tokenProvider.invalidateCalled.Load(), "should invalidate token once")
		})
	})

	t.Run("auth headers injection", func(t *testing.T) {
		t.Parallel()

		t.Run("injects headers on first request", func(t *testing.T) {
			t.Parallel()

			tokenProvider := &mockTokenProvider{
				tokens: []string{"initial_token"},
			}

			var capturedHeaders http.Header

			baseTransport := &captureHeadersTransport{
				base: &mockRoundTripper{
					responses: []*http.Response{
						{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(strings.NewReader("success")),
						},
					},
				},
				captureFunc: func(req *http.Request) { capturedHeaders = req.Header.Clone() },
				captureOn:   0,
			}

			transport := newHelixRetryTransport(baseTransport, tokenProvider, "client123")

			req := httptest.NewRequest(http.MethodGet, "https://api.twitch.tv/helix/users", nil)
			_, err := transport.RoundTrip(req)

			require.NoError(t, err, "RoundTrip should not return error")
			require.Equal(t, "Bearer initial_token", capturedHeaders.Get("Authorization"), "should inject token")
			require.Equal(t, "client123", capturedHeaders.Get("Client-Id"), "should inject client ID")
		})

		t.Run("updates headers on retry", func(t *testing.T) {
			t.Parallel()

			tokenProvider := &mockTokenProvider{
				tokens: []string{"old_token", "new_token"},
			}

			var secondRequestHeaders http.Header

			baseTransport := &captureHeadersTransport{
				base: &mockRoundTripper{
					responses: []*http.Response{
						{
							StatusCode: http.StatusUnauthorized,
							Body:       io.NopCloser(strings.NewReader("invalid token")),
						},
						{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(strings.NewReader("success")),
						},
					},
				},
				captureFunc: func(req *http.Request) { secondRequestHeaders = req.Header.Clone() },
				captureOn:   1,
			}

			transport := newHelixRetryTransport(baseTransport, tokenProvider, "client123")

			req := httptest.NewRequest(http.MethodGet, "https://api.twitch.tv/helix/users", nil)
			resp, err := transport.RoundTrip(req)

			require.NoError(t, err, "RoundTrip should not return error")
			require.Equal(t, http.StatusOK, resp.StatusCode, "status should be 200 after retry")
			require.Equal(t, "Bearer new_token", secondRequestHeaders.Get("Authorization"), "should use new token")
			require.Equal(t, "client123", secondRequestHeaders.Get("Client-Id"), "should update client ID")
		})
	})
}

// captureHeadersTransport wraps a RoundTripper to capture headers on a specific call.
type captureHeadersTransport struct {
	base        http.RoundTripper
	captureFunc func(*http.Request)
	captureOn   int
	callCount   int
}

func (c *captureHeadersTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if c.callCount == c.captureOn {
		c.captureFunc(req)
	}
	c.callCount++
	return c.base.RoundTrip(req)
}
