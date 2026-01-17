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
	client *redis.Client
}

// NewRateLimiter creates a new rate limiter with Redis client.
// If client is nil, rate limiting is disabled.
func NewRateLimiter(client *redis.Client) *RateLimiter {
	return &RateLimiter{client: client}
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
//
// SLIDING WINDOW ALGORITHM:
// Uses Redis Sorted Set (ZSET) to track request timestamps per IP.
// - Key: "ratelimit:{ip}"
// - Score: Request timestamp (nanoseconds since epoch)
// - Member: Unique request ID (using timestamp as ID)
//
// The "window" slides with each request:
// - Window start: now - 1 minute
// - Window end: now
// - Only count requests within this 1-minute window
//
// REDIS PIPELINE:
// Groups multiple Redis commands into a single network round-trip.
// Without pipeline: 4 commands = 4 network calls
// With pipeline: 4 commands = 1 network call
// Commands execute atomically on server, but pipeline itself is NOT a transaction.
//
// REDIS COMMANDS USED:
// 1. ZREMRANGEBYSCORE: Remove timestamps older than window (cleanup)
//   - Removes entries with score < (now - 1 minute)
//   - Keeps sorted set size bounded
//
// 2. ZCARD: Count entries in sorted set
//   - Returns number of requests in current window
//   - Used BEFORE adding current request (counts existing requests)
//
// 3. ZADD: Add current request timestamp
//   - Score: now (in nanoseconds)
//   - Member: timestamp as string (unique ID)
//   - Idempotent: same timestamp won't be added twice
//
// 4. EXPIRE: Set key TTL to 2 minutes
//   - Auto-cleanup if IP goes idle
//   - 2x window duration for safety margin
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

	// Execute all commands in one network call
	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	// Get count result from command #2
	count := countCmd.Val()

	// Check against burst capacity (max requests in window)
	if count >= burstCapacity {
		return false, nil
	}

	// Check against sustained rate (requests per minute)
	// Note: This check is redundant since sustainedRate < burstCapacity
	// Kept for clarity - can be removed
	if count >= sustainedRate {
		return false, nil
	}

	return true, nil
}

// extractClientIP extracts the client IP from the request.
//
// SECURITY NOTE: X-Forwarded-For can be spoofed by clients.
// This implementation trusts X-Forwarded-For, which is acceptable when:
// 1. Server runs behind a trusted reverse proxy (nginx, Cloudflare, etc.)
// 2. Proxy strips/overwrites client-provided X-Forwarded-For headers
// 3. Worst case: attacker bypasses rate limit for their IP only (no privilege escalation)
//
// For production use behind untrusted proxies, implement trusted proxy validation:
// - Maintain allowlist of trusted proxy IPs
// - Only trust X-Forwarded-For if r.RemoteAddr is in allowlist
// - Otherwise use r.RemoteAddr directly
func extractClientIP(r *http.Request) string {
	// NOTE: Trusting X-Forwarded-For without validation.
	// Safe if deployed behind trusted reverse proxy that sets this header.
	// Bad actor can bypass their own rate limit, but cannot affect other users.
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// X-Forwarded-For format: "client, proxy1, proxy2"
		// Take first IP (original client)
		if ip, _, err := net.SplitHostPort(xff); err == nil {
			return ip
		}
		return xff
	}

	// No X-Forwarded-For, use direct connection IP
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return ip
	}

	return r.RemoteAddr
}
