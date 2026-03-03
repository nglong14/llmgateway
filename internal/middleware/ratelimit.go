// Package middleware provides HTTP middleware for the gateway.
package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/nglong14/llmgateway/internal/models"
)

// visitorEntry holds a per-IP rate limiter and a last-seen timestamp.
type visitorEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter is a per-IP token-bucket rate limiter.
type RateLimiter struct {
	visitors sync.Map   // map[string]*visitorEntry
	rps      rate.Limit // tokens added per second
	burst    int        // max burst size
	done     chan struct{}
}

// NewRateLimiter creates a RateLimiter and starts a background goroutine
// that removes stale entries every cleanupInterval.
func NewRateLimiter(rps float64, burst int, cleanupInterval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		rps:   rate.Limit(rps),
		burst: burst,
		done:  make(chan struct{}),
	}

	go rl.cleanupLoop(cleanupInterval) // Cleanup old entries
	return rl
}

// Handler returns Chi-compatible middleware that enforces the rate limit.
func (rl *RateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		limiter := rl.getLimiter(ip)

		if !limiter.Allow() {
			models.WriteRateLimited(w, "rate limit exceeded, try again later")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Stop signals the cleanup goroutine to exit.
func (rl *RateLimiter) Stop() {
	close(rl.done)
}

// getLimiter returns the rate.Limiter for the given IP, creating one if needed.
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	now := time.Now()

	if v, ok := rl.visitors.Load(ip); ok {
		entry := v.(*visitorEntry)
		entry.lastSeen = now
		return entry.limiter
	}

	limiter := rate.NewLimiter(rl.rps, rl.burst)
	rl.visitors.Store(ip, &visitorEntry{limiter: limiter, lastSeen: now})
	return limiter
}

// cleanupLoop removes visitors not seen for more than 2× the cleanup interval.
func (rl *RateLimiter) cleanupLoop(interval time.Duration) {
	// Create a new ticker that fires every 'interval' duration.
	ticker := time.NewTicker(interval)
	// Ensure the ticker stops when this function exits to prevent memory leaks.
	defer ticker.Stop()

	for {
		// select efficiently blocks until one of the channels receives a message.
		select {
		case <-ticker.C:
			// The ticker fired. Calculate the cutoff time for "stale" entries.
			// Using 2 * interval adds a grace period before an IP is evicted.
			cutoff := time.Now().Add(-2 * interval)

			// sync.Map provides Range for thread-safe iteration over all keys.
			rl.visitors.Range(func(key, value any) bool {
				// Type assert the empty interface 'any' back to our concrete struct pointer.
				entry := value.(*visitorEntry)

				// If the IP hasn't been seen since before the cutoff time...
				if entry.lastSeen.Before(cutoff) {
					// Safely delete it from the sync.Map.
					rl.visitors.Delete(key)
				}
				// Returning true tells Range to continue to the next item in the map.
				return true
			})
		case <-rl.done:
			// rl.Stop() was called, which closed the rl.done channel.
			// Reading from a closed channel unblocks immediately.
			// We return to break the infinite loop and gracefully stop the goroutine.
			return
		}
	}
}

// extractIP pulls the client IP from X-Forwarded-For, X-Real-IP, or RemoteAddr.
func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// First entry is the original client.
		if ip := net.ParseIP(xff); ip != nil {
			return ip.String()
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
