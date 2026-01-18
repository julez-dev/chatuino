package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestHelixProxyHandler(t *testing.T) {
	t.Parallel()

	t.Run("only allows GET requests", func(t *testing.T) {
		t.Parallel()

		api := createTestAPI(t)
		handler := api.handleHelixProxy()

		// Test POST request
		req := httptest.NewRequest(http.MethodPost, "/ttv/chat/emotes/global", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusMethodNotAllowed, rec.Code, "POST should be rejected")

		// Test PUT request
		req = httptest.NewRequest(http.MethodPut, "/ttv/users", nil)
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusMethodNotAllowed, rec.Code, "PUT should be rejected")

		// Test DELETE request
		req = httptest.NewRequest(http.MethodDelete, "/ttv/streams", nil)
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusMethodNotAllowed, rec.Code, "DELETE should be rejected")
	})

	t.Run("path rewriting /ttv/* -> /helix/*", func(t *testing.T) {
		t.Parallel()

		var capturedPath string
		mockTwitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":[]}`))
		}))
		defer mockTwitch.Close()

		api := createTestAPI(t)
		target, err := url.Parse(mockTwitch.URL)
		require.NoError(t, err, "failed to parse mock server URL")
		handler := api.helixProxyHandlerWithTarget(target)

		req := httptest.NewRequest(http.MethodGet, "/ttv/chat/emotes/global", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "/helix/chat/emotes/global", capturedPath)
	})

	t.Run("query params preserved", func(t *testing.T) {
		t.Parallel()

		var capturedQuery string
		mockTwitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedQuery = r.URL.RawQuery
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":[]}`))
		}))
		defer mockTwitch.Close()

		api := createTestAPI(t)
		target, err := url.Parse(mockTwitch.URL)
		require.NoError(t, err, "failed to parse mock server URL")
		handler := api.helixProxyHandlerWithTarget(target)

		req := httptest.NewRequest(http.MethodGet, "/ttv/chat/emotes?broadcaster_id=12345", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "broadcaster_id=12345", capturedQuery)
	})

	t.Run("auth headers injected", func(t *testing.T) {
		t.Parallel()

		var capturedAuth, capturedClientID string
		mockTwitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedAuth = r.Header.Get("Authorization")
			capturedClientID = r.Header.Get("Client-Id")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":[]}`))
		}))
		defer mockTwitch.Close()

		api := createTestAPI(t)
		target, err := url.Parse(mockTwitch.URL)
		require.NoError(t, err, "failed to parse mock server URL")
		handler := api.helixProxyHandlerWithTarget(target)

		req := httptest.NewRequest(http.MethodGet, "/ttv/users", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "Bearer test-token-123", capturedAuth)
		require.Equal(t, "test-client-id", capturedClientID)
	})

	t.Run("response passed through unchanged", func(t *testing.T) {
		t.Parallel()

		expectedBody := `{"data":[]}`
		mockTwitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Custom-Header", "custom-value")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(expectedBody))
		}))
		defer mockTwitch.Close()

		api := createTestAPI(t)
		target, err := url.Parse(mockTwitch.URL)
		require.NoError(t, err, "failed to parse mock server URL")
		handler := api.helixProxyHandlerWithTarget(target)

		req := httptest.NewRequest(http.MethodGet, "/ttv/streams", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "application/json", rec.Header().Get("Content-Type"), "Content-Type should be copied")
		require.Empty(t, rec.Header().Get("X-Custom-Header"), "custom headers should not be copied")

		body, err := io.ReadAll(rec.Body)
		require.NoError(t, err, "failed to read response body")
		require.Equal(t, expectedBody, string(body))
	})

	t.Run("error status codes passed through", func(t *testing.T) {
		t.Parallel()

		mockTwitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"Not Found"}`))
		}))
		defer mockTwitch.Close()

		api := createTestAPI(t)
		target, err := url.Parse(mockTwitch.URL)
		require.NoError(t, err, "failed to parse mock server URL")
		handler := api.helixProxyHandlerWithTarget(target)

		req := httptest.NewRequest(http.MethodGet, "/ttv/users", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("401 unauthorized passed through", func(t *testing.T) {
		t.Parallel()

		mockTwitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"Unauthorized","status":401,"message":"Invalid OAuth token"}`))
		}))
		defer mockTwitch.Close()

		api := createTestAPI(t)
		target, err := url.Parse(mockTwitch.URL)
		require.NoError(t, err, "failed to parse mock server URL")
		handler := api.helixProxyHandlerWithTarget(target)

		req := httptest.NewRequest(http.MethodGet, "/ttv/users", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code)
		body, err := io.ReadAll(rec.Body)
		require.NoError(t, err, "failed to read 401 response body")
		require.Contains(t, string(body), "Invalid OAuth token")
	})

	t.Run("returns 502 when upstream unreachable", func(t *testing.T) {
		t.Parallel()

		api := createTestAPI(t)
		target, err := url.Parse("http://localhost:99999")
		require.NoError(t, err, "failed to parse hardcoded test URL")
		handler := api.helixProxyHandlerWithTarget(target)

		req := httptest.NewRequest(http.MethodGet, "/ttv/users", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusBadGateway, rec.Code)
	})

	t.Run("strips rate limit headers on 200 response", func(t *testing.T) {
		t.Parallel()

		mockTwitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate Twitch returning rate limit headers
			w.Header().Set("Ratelimit-Limit", "800")
			w.Header().Set("Ratelimit-Remaining", "799")
			w.Header().Set("Ratelimit-Reset", "1640000000")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":[]}`))
		}))
		defer mockTwitch.Close()

		api := createTestAPI(t)
		target, err := url.Parse(mockTwitch.URL)
		require.NoError(t, err, "failed to parse mock server URL")
		handler := api.helixProxyHandlerWithTarget(target)

		req := httptest.NewRequest(http.MethodGet, "/ttv/users", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Empty(t, rec.Header().Get("Ratelimit-Limit"), "Ratelimit-Limit should be stripped on 200")
		require.Empty(t, rec.Header().Get("Ratelimit-Remaining"), "Ratelimit-Remaining should be stripped on 200")
		require.Empty(t, rec.Header().Get("Ratelimit-Reset"), "Ratelimit-Reset should be stripped on 200")
		require.Equal(t, "application/json", rec.Header().Get("Content-Type"), "other headers should be preserved")
	})

	t.Run("preserves only Ratelimit-Reset on 429 response", func(t *testing.T) {
		t.Parallel()

		mockTwitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Ratelimit-Limit", "800")
			w.Header().Set("Ratelimit-Remaining", "0")
			w.Header().Set("Ratelimit-Reset", "1640000000")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"Too Many Requests","status":429,"message":"rate limit exceeded"}`))
		}))
		defer mockTwitch.Close()

		api := createTestAPI(t)
		target, err := url.Parse(mockTwitch.URL)
		require.NoError(t, err, "failed to parse mock server URL")
		handler := api.helixProxyHandlerWithTarget(target)

		req := httptest.NewRequest(http.MethodGet, "/ttv/users", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusTooManyRequests, rec.Code)
		require.Empty(t, rec.Header().Get("Ratelimit-Limit"), "Ratelimit-Limit should be stripped even on 429")
		require.Empty(t, rec.Header().Get("Ratelimit-Remaining"), "Ratelimit-Remaining should be stripped even on 429")
		require.Equal(t, "1640000000", rec.Header().Get("Ratelimit-Reset"), "Ratelimit-Reset should be preserved on 429")
		require.Equal(t, "application/json", rec.Header().Get("Content-Type"), "other headers should be preserved")
	})

	t.Run("strips rate limit headers on 401 response", func(t *testing.T) {
		t.Parallel()

		mockTwitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Ratelimit-Limit", "800")
			w.Header().Set("Ratelimit-Remaining", "799")
			w.Header().Set("Ratelimit-Reset", "1640000000")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"Unauthorized"}`))
		}))
		defer mockTwitch.Close()

		api := createTestAPI(t)
		target, err := url.Parse(mockTwitch.URL)
		require.NoError(t, err, "failed to parse mock server URL")
		handler := api.helixProxyHandlerWithTarget(target)

		req := httptest.NewRequest(http.MethodGet, "/ttv/users", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.Empty(t, rec.Header().Get("Ratelimit-Limit"), "Ratelimit-Limit should be stripped on 401")
		require.Empty(t, rec.Header().Get("Ratelimit-Remaining"), "Ratelimit-Remaining should be stripped on 401")
		require.Empty(t, rec.Header().Get("Ratelimit-Reset"), "Ratelimit-Reset should be stripped on 401")
	})
}

// createTestAPI creates a test API instance with a pre-populated token.
func createTestAPI(t *testing.T) *API {
	t.Helper()

	api := &API{
		logger: zerolog.Nop(),
		conf: Config{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		},
		client:             http.DefaultClient,
		helixTokenProvider: NewHelixTokenProvider(http.DefaultClient, "test-client-id", "test-client-secret"),
	}

	// Pre-populate token to avoid hitting real Twitch
	api.helixTokenProvider.token = "test-token-123"

	return api
}
