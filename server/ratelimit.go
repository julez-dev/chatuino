package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

const (
	// Rate limit configuration
	burstCapacity  = 100         // Max requests in burst
	windowDuration = time.Minute // Sliding window duration
	retryAfter     = 60          // Seconds to wait after rate limit hit
)

// RateLimiter implements sliding window rate limiting using Redis
type RateLimiter struct {
	client *redis.Client
	logger zerolog.Logger
}

// NewRateLimiter creates a new rate limiter with Redis client.
// If client is nil, rate limiting is disabled.
func NewRateLimiter(client *redis.Client, logger zerolog.Logger) *RateLimiter {
	return &RateLimiter{
		client: client,
		logger: logger,
	}
}

// Middleware returns a middleware that enforces rate limits per client IP
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If no Redis client, pass through (rate limiting disabled)
		if rl.client == nil {
			next.ServeHTTP(w, r)
			return
		}

		clientIP := extractClientIP(r)

		allowed, err := rl.checkLimit(r.Context(), clientIP)
		if err != nil {
			// Fail open - allow request if Redis unavailable
			rl.logger.Warn().Err(err).Str("client_ip", clientIP).Msg("rate limit check failed, allowing request")
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

// checkLimit verifies if the client is within rate limits using sliding window algorithm.
func (rl *RateLimiter) checkLimit(ctx context.Context, clientIP string) (bool, error) {
	key := fmt.Sprintf("ratelimit:%s", clientIP)
	now := time.Now()
	windowStart := now.Add(-windowDuration)

	// Pipeline batches commands for single network round-trip
	pipe := rl.client.Pipeline()

	// 1. Remove old entries outside the sliding window
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart.UnixNano(), 10))

	// 2. Count requests in current window (BEFORE adding current request)
	countCmd := pipe.ZCard(ctx, key)

	// 3. Add current request timestamp to the window
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.UnixNano()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})

	// 4. Set expiration on key (auto-cleanup for idle IPs)
	pipe.Expire(ctx, key, windowDuration*2)

	// 5. Execute all commands in one network call
	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	// 6. Get count result from command #2
	count := countCmd.Val()

	// 7. Check against burst capacity (max requests in window)
	if count >= burstCapacity {
		return false, nil
	}

	return true, nil
}

// extractClientIP extracts the client IP from the request.
//
// SECURITY NOTE: X-Forwarded-For can be spoofed by clients.
// This implementation trusts X-Forwarded-For, which is acceptable when:
// 1. Server runs behind a trusted reverse proxy (nginx, Cloudflare, etc.) (applies for chatuino's deployment)
// 2. Proxy strips/overwrites client-provided X-Forwarded-For headers
// 3. Worst case: attacker bypasses rate limit for their IP only (no privilege escalation)
func extractClientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// X-Forwarded-For format: "client, proxy1, proxy2"
		// Split on comma and take first IP (original client)
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// No X-Forwarded-For, use direct connection IP
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return ip
	}

	return r.RemoteAddr
}
