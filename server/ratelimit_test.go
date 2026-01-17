package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRateLimiter_Middleware(t *testing.T) {
	t.Run("requests under limit succeed", func(t *testing.T) {
		t.Parallel()

		// Setup Redis client for testing
		client := redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
		})
		defer client.Close()

		// Skip test if Redis unavailable
		ctx := context.Background()
		if err := client.Ping(ctx).Err(); err != nil {
			t.Skip("Redis not available:", err)
		}

		// Clear any existing test data
		testKey := "ratelimit:192.0.2.1"
		client.Del(ctx, testKey)
		defer client.Del(ctx, testKey)

		store := &RedisClient{client: client}
		limiter := NewRateLimiter(store)

		handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		// Make 10 requests (well under limit)
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = "192.0.2.1:12345"
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code, "request %d should succeed", i)
		}
	})

	t.Run("requests over burst limit return 429", func(t *testing.T) {
		t.Parallel()

		client := redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
		})
		defer client.Close()

		ctx := context.Background()
		if err := client.Ping(ctx).Err(); err != nil {
			t.Skip("Redis not available:", err)
		}

		testKey := "ratelimit:192.0.2.2"
		client.Del(ctx, testKey)
		defer client.Del(ctx, testKey)

		store := &RedisClient{client: client}
		limiter := NewRateLimiter(store)

		handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		// Make burstCapacity + 1 requests
		successCount := 0
		rateLimitedCount := 0

		for i := 0; i < burstCapacity+10; i++ {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = "192.0.2.2:12345"
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code == http.StatusOK {
				successCount++
			} else if w.Code == http.StatusTooManyRequests {
				rateLimitedCount++
			}
		}

		require.Greater(t, successCount, 0, "some requests should succeed")
		require.Greater(t, rateLimitedCount, 0, "some requests should be rate limited")
		require.LessOrEqual(t, successCount, burstCapacity, "should not exceed burst capacity")
	})

	t.Run("Retry-After header present in 429 response", func(t *testing.T) {
		t.Parallel()

		client := redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
		})
		defer client.Close()

		ctx := context.Background()
		if err := client.Ping(ctx).Err(); err != nil {
			t.Skip("Redis not available:", err)
		}

		testKey := "ratelimit:192.0.2.3"
		client.Del(ctx, testKey)
		defer client.Del(ctx, testKey)

		store := &RedisClient{client: client}
		limiter := NewRateLimiter(store)

		handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		// Exhaust rate limit
		for i := 0; i < burstCapacity+5; i++ {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = "192.0.2.3:12345"
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code == http.StatusTooManyRequests {
				retryAfterHeader := w.Header().Get("Retry-After")
				require.NotEmpty(t, retryAfterHeader, "Retry-After header should be present")

				seconds, err := strconv.Atoi(retryAfterHeader)
				require.NoError(t, err, "Retry-After should be valid integer")
				require.Equal(t, 60, seconds, "Retry-After should be 60 seconds")
				return
			}
		}

		t.Fatal("should have received at least one 429 response")
	})

	t.Run("different IPs have independent limits", func(t *testing.T) {
		t.Parallel()

		client := redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
		})
		defer client.Close()

		ctx := context.Background()
		if err := client.Ping(ctx).Err(); err != nil {
			t.Skip("Redis not available:", err)
		}

		testKey1 := "ratelimit:192.0.2.4"
		testKey2 := "ratelimit:192.0.2.5"
		client.Del(ctx, testKey1, testKey2)
		defer client.Del(ctx, testKey1, testKey2)

		store := &RedisClient{client: client}
		limiter := NewRateLimiter(store)

		handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		// IP 1: Make 10 requests
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = "192.0.2.4:12345"
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, "IP1 request %d should succeed", i)
		}

		// IP 2: Make 10 requests (should also succeed, independent limit)
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = "192.0.2.5:12345"
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, "IP2 request %d should succeed", i)
		}
	})

	t.Run("middleware fails open when Redis unavailable", func(t *testing.T) {
		t.Parallel()

		// Use NopRedisClient (simulates disabled rate limiting)
		store := NewNopRedisClient()
		limiter := NewRateLimiter(store)

		handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		// Make many requests (should all succeed with no-op client)
		for i := 0; i < burstCapacity*2; i++ {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = "192.0.2.6:12345"
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, "request %d should succeed (fail-open)", i)
		}
	})
}

func TestExtractClientIP(t *testing.T) {
	t.Run("extracts IP from X-Forwarded-For", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.1")
		req.RemoteAddr = "192.0.2.1:12345"

		ip := extractClientIP(req)
		require.Equal(t, "203.0.113.1", ip, "should use X-Forwarded-For")
	})

	t.Run("extracts IP from RemoteAddr when no X-Forwarded-For", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.0.2.1:12345"

		ip := extractClientIP(req)
		require.Equal(t, "192.0.2.1", ip, "should extract IP from RemoteAddr")
	})

	t.Run("handles RemoteAddr without port", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.0.2.1"

		ip := extractClientIP(req)
		require.Equal(t, "192.0.2.1", ip, "should handle addr without port")
	})
}
