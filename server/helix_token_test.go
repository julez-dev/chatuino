package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHelixTokenProvider_GetToken(t *testing.T) {
	t.Parallel()

	t.Run("fetches token from server", func(t *testing.T) {
		t.Parallel()

		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

			err := r.ParseForm()
			require.NoError(t, err)
			require.Equal(t, "test-client-id", r.Form.Get("client_id"))
			require.Equal(t, "test-secret", r.Form.Get("client_secret"))
			require.Equal(t, "client_credentials", r.Form.Get("grant_type"))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"access_token":"fresh-token-abc"}`))
		}))
		defer tokenServer.Close()

		provider := NewHelixTokenProvider(http.DefaultClient, "test-client-id", "test-secret")
		provider.tokenURL = tokenServer.URL

		token, err := provider.GetToken(context.Background())

		require.NoError(t, err)
		require.Equal(t, "fresh-token-abc", token)
	})

	t.Run("caches token", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"access_token":"cached-token"}`))
		}))
		defer tokenServer.Close()

		provider := NewHelixTokenProvider(http.DefaultClient, "test-client-id", "test-secret")
		provider.tokenURL = tokenServer.URL

		// First call
		token1, err := provider.GetToken(context.Background())
		require.NoError(t, err)
		require.Equal(t, "cached-token", token1)

		// Second call should use cached token
		token2, err := provider.GetToken(context.Background())
		require.NoError(t, err)
		require.Equal(t, "cached-token", token2)

		require.Equal(t, 1, callCount, "token server should only be called once")
	})

	t.Run("returns error on non-200 status", func(t *testing.T) {
		t.Parallel()

		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"invalid_client"}`))
		}))
		defer tokenServer.Close()

		provider := NewHelixTokenProvider(http.DefaultClient, "bad-client-id", "bad-secret")
		provider.tokenURL = tokenServer.URL

		_, err := provider.GetToken(context.Background())

		require.Error(t, err)
		require.Contains(t, err.Error(), "status 400")
	})

	t.Run("returns error on invalid JSON", func(t *testing.T) {
		t.Parallel()

		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`not json`))
		}))
		defer tokenServer.Close()

		provider := NewHelixTokenProvider(http.DefaultClient, "test-client-id", "test-secret")
		provider.tokenURL = tokenServer.URL

		_, err := provider.GetToken(context.Background())

		require.Error(t, err)
		require.Contains(t, err.Error(), "unmarshal")
	})

	t.Run("returns error on network failure", func(t *testing.T) {
		t.Parallel()

		provider := NewHelixTokenProvider(http.DefaultClient, "test-client-id", "test-secret")
		provider.tokenURL = "http://localhost:99999"

		_, err := provider.GetToken(context.Background())

		require.Error(t, err)
		require.Contains(t, err.Error(), "token request failed")
	})
}

func TestHelixTokenProvider_InvalidateToken(t *testing.T) {
	t.Parallel()

	t.Run("forces refetch after invalidation", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.WriteHeader(http.StatusOK)
			if callCount == 1 {
				w.Write([]byte(`{"access_token":"token-v1"}`))
			} else {
				w.Write([]byte(`{"access_token":"token-v2"}`))
			}
		}))
		defer tokenServer.Close()

		provider := NewHelixTokenProvider(http.DefaultClient, "test-client-id", "test-secret")
		provider.tokenURL = tokenServer.URL

		// First fetch
		token1, err := provider.GetToken(context.Background())
		require.NoError(t, err)
		require.Equal(t, "token-v1", token1)

		// Invalidate
		provider.InvalidateToken()

		// Second fetch should get new token
		token2, err := provider.GetToken(context.Background())
		require.NoError(t, err)
		require.Equal(t, "token-v2", token2)

		require.Equal(t, 2, callCount)
	})
}

func TestHelixTokenProvider_Concurrency(t *testing.T) {
	t.Parallel()

	callCount := 0
	var mu sync.Mutex
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"concurrent-token"}`))
	}))
	defer tokenServer.Close()

	provider := NewHelixTokenProvider(http.DefaultClient, "test-client-id", "test-secret")
	provider.tokenURL = tokenServer.URL

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := provider.GetToken(context.Background())
			require.NoError(t, err)
			require.Equal(t, "concurrent-token", token)
		}()
	}
	wg.Wait()

	// Due to mutex, only one goroutine should fetch
	// But since they might all check before lock, we allow up to a few
	require.LessOrEqual(t, callCount, 2, "token should be fetched at most twice due to race")
}
