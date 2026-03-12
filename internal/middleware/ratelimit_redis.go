// Package middleware — Redis-backed distributed token bucket rate limiter.
package middleware

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/nglong14/llmgateway/internal/models"
)

// tokenBucketScript is a Lua script that implements a distributed token bucket.
// It runs atomically on the Redis server, eliminating race conditions between
// multiple gateway replicas.
//
// KEYS[1] = rate limit key (e.g., "rate:1.2.3.4")
// ARGV[1] = capacity (max burst)
// ARGV[2] = rate (tokens per second)
// ARGV[3] = now (current time in seconds as a float)
//
// Returns 1 if allowed, 0 if denied.
var tokenBucketScript = redis.NewScript(`
local key      = KEYS[1]
local capacity = tonumber(ARGV[1])
local rate     = tonumber(ARGV[2])
local now      = tonumber(ARGV[3])

-- Get current state (default: full bucket on first request).
local tokens    = tonumber(redis.call('HGET', key, 'tokens') or capacity)
local last_time = tonumber(redis.call('HGET', key, 'ts') or now)

-- Refill tokens based on elapsed time since last request.
local elapsed = math.max(0, now - last_time)
tokens = math.min(capacity, tokens + elapsed * rate)

-- Try to consume one token.
if tokens >= 1 then
    tokens = tokens - 1
    redis.call('HSET', key, 'tokens', tokens, 'ts', now)
    redis.call('EXPIRE', key, math.ceil(capacity / rate) * 2)
    return 1
end

-- Denied: save state so refill calculation stays accurate.
redis.call('HSET', key, 'tokens', tokens, 'ts', now)
redis.call('EXPIRE', key, math.ceil(capacity / rate) * 2)
return 0
`)

// RedisRateLimiter is a distributed per-IP token bucket rate limiter
// backed by Redis. All gateway replicas share the same token buckets,
// ensuring globally accurate rate limiting.
type RedisRateLimiter struct {
	rdb   *redis.Client
	rate  float64 // tokens per second (matches config RPS)
	burst int     // max bucket capacity (matches config Burst)

	// extractIP is injected so we can reuse the trusted-proxy-aware
	// extraction logic from the existing RateLimiter.
	extractIP func(r *http.Request) string
}

// NewRedisRateLimiter creates a distributed rate limiter.
// extractIP should be a function that extracts the real client IP
// from the request (handling trusted proxies, X-Forwarded-For, etc.).
func NewRedisRateLimiter(rdb *redis.Client, rps float64, burst int, extractIP func(r *http.Request) string) *RedisRateLimiter {
	return &RedisRateLimiter{
		rdb:       rdb,
		rate:      rps,
		burst:     burst,
		extractIP: extractIP,
	}
}

// Handler returns Chi-compatible middleware that enforces the rate limit.
func (rl *RedisRateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := rl.extractIP(r)

		allowed, err := rl.allow(r.Context(), ip)
		if err != nil {
			// Redis is down — log and allow the request through.
			// Availability > strictness: a brief Redis outage should not
			// cause a full gateway blackout.
			log.Printf("redis rate limiter error (allowing request): %v", err)
			next.ServeHTTP(w, r)
			return
		}

		if !allowed {
			models.WriteRateLimited(w, "rate limit exceeded, try again later")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// allow executes the token bucket Lua script against Redis.
func (rl *RedisRateLimiter) allow(ctx context.Context, ip string) (bool, error) {
	now := float64(time.Now().UnixMicro()) / 1e6 // seconds with microsecond precision

	result, err := tokenBucketScript.Run(ctx, rl.rdb,
		[]string{"rate:" + ip},
		rl.burst, // ARGV[1] = capacity
		rl.rate,  // ARGV[2] = tokens/sec
		now,      // ARGV[3] = current time
	).Int()
	if err != nil {
		return false, err
	}

	return result == 1, nil
}

// Stop is a no-op for the Redis rate limiter (no background goroutines).
// It exists to satisfy the same lifecycle contract as the in-memory RateLimiter.
func (rl *RedisRateLimiter) Stop() {}
