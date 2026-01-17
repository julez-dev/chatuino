package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// Rate limit configuration
	burstCapacity  = 100         // Max requests in burst
	sustainedRate  = 25          // Requests per minute sustained
	windowDuration = time.Minute // Sliding window duration
	retryAfter     = 60          // Seconds to wait after rate limit hit
)

// RateLimiter implements sliding window rate limiting using Redis
type RateLimiter struct {
	store RatelimitStore
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(store RatelimitStore) *RateLimiter {
	return &RateLimiter{store: store}
}

// Middleware returns a middleware that enforces rate limits per client IP
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := extractClientIP(r)

		allowed, err := rl.checkLimit(r.Context(), clientIP)
		if err != nil {
			// Fail open - allow request if Redis unavailable
			next.ServeHTTP(w, r)
			return
		}

		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// checkLimit verifies if the client is within rate limits using sliding window
func (rl *RateLimiter) checkLimit(ctx context.Context, clientIP string) (bool, error) {
	// Check if using no-op client (rate limiting disabled)
	if _, ok := rl.store.(*NopRedisClient); ok {
		return true, nil
	}

	redisClient, ok := rl.store.(*RedisClient)
	if !ok {
		// Unknown store type, fail open
		return true, nil
	}

	key := fmt.Sprintf("ratelimit:%s", clientIP)
	now := time.Now()
	windowStart := now.Add(-windowDuration)

	pipe := redisClient.client.Pipeline()

	// Remove old entries outside the sliding window
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart.UnixNano(), 10))

	// Count requests in current window
	countCmd := pipe.ZCard(ctx, key)

	// Add current request timestamp
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.UnixNano()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})

	// Set expiration on key (cleanup)
	pipe.Expire(ctx, key, windowDuration*2)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	count := countCmd.Val()

	// Check against burst capacity
	if count >= burstCapacity {
		return false, nil
	}

	// Check against sustained rate (requests per minute)
	if count >= sustainedRate {
		return false, nil
	}

	return true, nil
}

// extractClientIP extracts the client IP from the request
// Respects X-Forwarded-For header from trusted proxies
func extractClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// Take the first IP in the chain (original client)
		if ip, _, err := net.SplitHostPort(xff); err == nil {
			return ip
		}
		// If no port, return as-is
		return xff
	}

	// Fallback to RemoteAddr
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return ip
	}

	return r.RemoteAddr
}
