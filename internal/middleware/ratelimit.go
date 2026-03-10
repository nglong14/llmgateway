// Package middleware provides HTTP middleware for the gateway.
package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/nglong14/llmgateway/internal/models"
)

// visitorEntry holds a per-IP rate limiter and a last-seen timestamp.
type visitorEntry struct {
	limiter  *rate.Limiter
	lastSeen atomic.Int64 // Unix nano timestamp for lock-free reads/writes
}

// RateLimiter is a per-IP token-bucket rate limiter.
type RateLimiter struct {
	visitors       sync.Map   // map[string]*visitorEntry
	rps            rate.Limit // tokens added per second
	burst          int        // max burst size
	trustedProxies []net.IPNet
	done           chan struct{}
}

// NewRateLimiter creates a RateLimiter and starts a background goroutine
// that removes stale entries every cleanupInterval.
// trustedCIDRs is a list of CIDR strings (e.g., "10.0.0.0/8") representing
// proxies whose X-Forwarded-For headers should be trusted.
func NewRateLimiter(rps float64, burst int, cleanupInterval time.Duration, trustedCIDRs []string) *RateLimiter {
	var proxies []net.IPNet
	for _, cidr := range trustedCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			// Skip invalid CIDRs (logged at startup by caller).
			continue
		}
		proxies = append(proxies, *network)
	}

	rl := &RateLimiter{
		rps:            rate.Limit(rps),
		burst:          burst,
		trustedProxies: proxies,
		done:           make(chan struct{}),
	}

	go rl.cleanupLoop(cleanupInterval) // Cleanup old entries
	return rl
}

// Handler returns Chi-compatible middleware that enforces the rate limit.
func (rl *RateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := rl.extractIP(r)
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
// Uses LoadOrStore to avoid the race where two goroutines both miss Load
// and create duplicate limiters.
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	now := time.Now().UnixNano()

	// Fast path: entry already exists.
	if v, ok := rl.visitors.Load(ip); ok {
		entry := v.(*visitorEntry)
		entry.lastSeen.Store(now)
		return entry.limiter
	}

	// Slow path: create a new entry and atomically try to store it.
	newEntry := &visitorEntry{limiter: rate.NewLimiter(rl.rps, rl.burst)}
	newEntry.lastSeen.Store(now)

	v, _ := rl.visitors.LoadOrStore(ip, newEntry)
	entry := v.(*visitorEntry)
	entry.lastSeen.Store(now)
	return entry.limiter
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
			cutoff := time.Now().Add(-2 * interval).UnixNano()

			// sync.Map provides Range for thread-safe iteration over all keys.
			rl.visitors.Range(func(key, value any) bool {
				// Type assert the empty interface 'any' back to our concrete struct pointer.
				entry := value.(*visitorEntry)

				// If the IP hasn't been seen since before the cutoff time...
				if entry.lastSeen.Load() < cutoff {
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

// extractIP pulls the client IP from the request. It only trusts
// X-Forwarded-For / X-Real-IP when RemoteAddr belongs to a trusted proxy.
func (rl *RateLimiter) extractIP(r *http.Request) string {
	// Get the direct connection IP (cannot be spoofed).
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	// Only trust proxy headers if the direct connection is from a known proxy.
	if rl.isTrustedProxy(remoteIP) {
		// X-Forwarded-For: client, proxy1, proxy2
		// Walk right-to-left to find the first untrusted (real client) IP.
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ips := strings.Split(xff, ",")
			for i := len(ips) - 1; i >= 0; i-- {
				ip := strings.TrimSpace(ips[i])
				if parsed := net.ParseIP(ip); parsed != nil && !rl.isTrustedProxy(ip) {
					return parsed.String()
				}
			}
		}

		// Fallback: X-Real-IP (set by Nginx).
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			if parsed := net.ParseIP(xri); parsed != nil {
				return parsed.String()
			}
		}
	}

	return remoteIP
}

// isTrustedProxy checks if the given IP belongs to any trusted proxy CIDR.
func (rl *RateLimiter) isTrustedProxy(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, cidr := range rl.trustedProxies {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}
